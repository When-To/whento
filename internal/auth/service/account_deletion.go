// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/email"
	"github.com/whento/pkg/logger"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
)

//go:embed templates/account_deleted.html
var accountDeletedTemplateHTML string

//go:embed templates/locales/account_deleted.json
var accountDeletedTranslationsJSON string

// Both are embedded constants: a broken one is a build defect, so it fails at
// start-up (and in every test of this package) rather than on someone's last
// request.
var (
	accountDeletedTemplate     = template.Must(template.New("account_deleted").Parse(accountDeletedTemplateHTML))
	accountDeletedTranslations = func() map[string]map[string]string {
		var t map[string]map[string]string
		if err := json.Unmarshal([]byte(accountDeletedTranslationsJSON), &t); err != nil {
			panic(fmt.Sprintf("account_deleted.json: %v", err))
		}
		return t
	}()
)

// accountDeletedEmailTimeout bounds the confirmation send. It runs detached
// from the request (a client that hangs up must not cancel it), so this is
// the only thing that stops it.
const accountDeletedEmailTimeout = 10 * time.Second

// AccountMailer is the slice of pkg/email the account-deletion confirmation needs.
type AccountMailer interface {
	IsConfigured() bool
	SendContext(ctx context.Context, msg email.Email) error
}

// DeleteOwnAccount deletes the caller's own account after re-checking their
// password, then sends a best-effort confirmation email. An administrator can
// only do this while another administrator exists (ErrLastAdmin).
func (s *AuthService) DeleteOwnAccount(ctx context.Context, userID, password string) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return ErrUserNotFound
	}

	// Loaded before the delete: the confirmation needs the address, name and
	// locale, and they are gone afterwards.
	user, err := s.userRepo.GetByID(ctx, uid)
	if err != nil {
		// Only a genuine miss is a 401: a transient database error answered as
		// one would make the client refresh, fail again, and sign the user out.
		if errors.Is(err, repository.ErrUserNotFound) {
			return ErrUserNotFound
		}
		return fmt.Errorf("failed to load user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return ErrPasswordMismatch
	}

	calendars, err := s.deleteAccount(ctx, uid)
	if err != nil {
		return err
	}

	s.sendAccountDeletedEmail(ctx, user, calendars, time.Now())
	return nil
}

// deleteAccount is the one deletion path, shared by the admin and the
// self-service endpoints so both revoke and purge the same way. It returns how
// many calendars went with the account.
func (s *AuthService) deleteAccount(ctx context.Context, uid uuid.UUID) (int, error) {
	publicTokens, err := s.userRepo.Delete(ctx, uid)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrLastAdmin):
			return 0, ErrLastAdmin
		case errors.Is(err, repository.ErrUserNotFound):
			return 0, ErrUserNotFound
		}
		return 0, err
	}

	// Refresh tokens went with the row (cascade). Access tokens are
	// self-contained, so they are revoked through the same marker a password
	// change uses. One second ahead: the middleware compares whole seconds and
	// refuses only tokens issued strictly before the marker, and unlike a
	// password change no fresh token has to survive this one.
	//
	// A cache failure does not undo the deletion (the database is the source of
	// truth), but a missed revocation leaves earlier access tokens, admin claim
	// included, valid until they expire, so it is logged for the operator.
	if s.cache != nil && s.cache.IsEnabled() {
		if err := s.cache.Set(ctx, cache.UserPasswordChangedKey(uid.String()), time.Now().Unix()+1, s.jwtManager.AccessExpiry()); err != nil {
			logger.FromContext(ctx).Error("failed to revoke access tokens of a deleted account", "error", err)
		}
		for _, token := range publicTokens {
			_ = s.cache.Delete(ctx, cache.CalendarByPublicTokenKey(token))
		}
	}

	logger.FromContext(ctx).Info("account deleted", "calendars_deleted", len(publicTokens))
	return len(publicTokens), nil
}

// sendAccountDeletedEmail tells the holder their account is gone, so a deletion
// made from a hijacked session does not go unnoticed. Best effort: the account
// is already deleted, so a failure is logged and nothing else.
func (s *AuthService) sendAccountDeletedEmail(ctx context.Context, user *models.User, calendars int, deletedAt time.Time) {
	if s.mailer == nil || !s.mailer.IsConfigured() {
		return
	}

	trans, ok := accountDeletedTranslations[user.Locale]
	if !ok {
		trans = accountDeletedTranslations["en"]
	}

	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		loc = time.UTC
	}

	var body bytes.Buffer
	if err := accountDeletedTemplate.Execute(&body, map[string]any{
		"Subject":   trans["subject"],
		"Greeting":  email.ReplaceVar(trans["greeting"], "DisplayName", user.DisplayName),
		"DeletedAt": email.ReplaceVar(trans["deleted_at"], "DeletedAt", deletedAt.In(loc).Format("2006-01-02 15:04 MST")),
		"Calendars": email.ReplaceVar(trans["calendars_deleted"], "Count", strconv.Itoa(calendars)),
		"Security":  trans["security_notice"],
		// Deliberate markup: the <br> between the sign-off and the team name.
		"Signature": template.HTML(trans["signature"]),
	}); err != nil {
		logger.FromContext(ctx).Error("failed to render account deletion email", "error", err)
		return
	}

	// Detached from the request: the response is about to clear the session,
	// and a client that disconnects must not cancel the notice.
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), accountDeletedEmailTimeout)
	defer cancel()

	if err := s.mailer.SendContext(sendCtx, email.Email{
		To:      []string{user.Email},
		Subject: trans["subject"],
		Body:    body.String(),
		HTML:    true,
	}); err != nil {
		// SMTP rejections echo the recipient ("550 <alice@...>: rejected"), and
		// the address must not reach the logs (ADR-0005).
		logger.FromContext(ctx).Error("failed to send account deletion email",
			"error", strings.ReplaceAll(err.Error(), user.Email, "<recipient>"))
	}
}
