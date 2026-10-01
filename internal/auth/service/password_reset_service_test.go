// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/email"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/config"
)

// The fakes below let the password-reset flow run without a database or an SMTP
// server. They exist because the service used to hold concrete repositories,
// which left the configured expiry completely untested — the exact defect this
// test guards.

type resetUserFixture struct {
	user            *models.User
	getByEmailErr   error
	storedExpiresAt time.Time
	storedToken     string
	updateErr       error
	clearErr        error
}

func (f *resetUserFixture) GetByEmail(_ context.Context, _ string) (*models.User, error) {
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	return f.user, nil
}

func (f *resetUserFixture) GetByPasswordResetToken(_ context.Context, _ string) (*models.User, error) {
	return f.user, nil
}

func (f *resetUserFixture) SetPasswordResetToken(_ context.Context, _ uuid.UUID, token string, expiresAt time.Time) error {
	f.storedToken = token
	f.storedExpiresAt = expiresAt
	return nil
}

func (f *resetUserFixture) ClearPasswordResetToken(context.Context, uuid.UUID) error {
	return f.clearErr
}

func (f *resetUserFixture) UpdatePassword(context.Context, uuid.UUID, string) error {
	return f.updateErr
}

type resetTokenFixture struct {
	created *models.RefreshToken
}

func (f *resetTokenFixture) Create(_ context.Context, token *models.RefreshToken, _ int64) error {
	f.created = token
	return nil
}

func (f *resetTokenFixture) DeleteByUserID(context.Context, uuid.UUID) (int64, error) { return 1, nil }

type resetMailerFixture struct {
	configured bool
	sent       []email.Email
}

func (f *resetMailerFixture) IsConfigured() bool { return f.configured }

func (f *resetMailerFixture) Send(msg email.Email) error {
	f.sent = append(f.sent, msg)
	return nil
}

type resetTokenIssuerFixture struct {
	refreshExpiresAt time.Time
}

func (f *resetTokenIssuerFixture) GenerateAccessToken(_, _, _ string) (string, error) {
	return "access-token", nil
}

func (f *resetTokenIssuerFixture) GenerateRefreshToken(string) (string, time.Time, error) {
	return "refresh-token", f.refreshExpiresAt, nil
}

func (f *resetTokenIssuerFixture) IssueRefreshToken(string, string) (string, time.Time, string, error) {
	return "refresh-token", f.refreshExpiresAt, "family-reset", nil
}

func newResetServiceFixture(t *testing.T, resetExpiry time.Duration) (*PasswordResetService, *resetUserFixture, *resetMailerFixture) {
	t.Helper()

	user := &models.User{
		Email:         "ada@example.test",
		DisplayName:   "Ada",
		Locale:        models.LocaleEN,
		Timezone:      "Europe/Paris",
		EmailVerified: true,
	}
	user.ID = uuid.New()

	users := &resetUserFixture{user: user}
	mailer := &resetMailerFixture{configured: true}
	issuer := &resetTokenIssuerFixture{refreshExpiresAt: time.Now().Add(48 * time.Hour)}

	svc := NewPasswordResetService(
		users, &resetTokenFixture{}, mailer, issuer,
		&config.Config{Email: config.EmailConfig{PasswordResetExpiry: resetExpiry}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		4,
	)

	return svc, users, mailer
}

// TestPasswordResetHonoursTheConfiguredExpiry pins the defect this finding was
// about: the stored token expiry and the email copy both used a hard-coded one
// hour, and PASSWORD_RESET_EXPIRY changed nothing.
func TestPasswordResetHonoursTheConfiguredExpiry(t *testing.T) {
	const expiry = 17 * time.Minute

	svc, users, mailer := newResetServiceFixture(t, expiry)
	before := time.Now()

	svc.processPasswordReset("ada@example.test")

	// The stored token must carry the configured lifetime, not a hard-coded hour.
	if users.storedExpiresAt.Before(before.Add(expiry-2*time.Second)) ||
		users.storedExpiresAt.After(before.Add(expiry+2*time.Second)) {
		t.Errorf("stored reset token expires at %v, want ~%v from now", users.storedExpiresAt, expiry)
	}

	// The email the user reads must say the same thing.
	if len(mailer.sent) != 1 {
		t.Fatalf("sent %d reset emails, want 1", len(mailer.sent))
	}
	if !strings.Contains(mailer.sent[0].Body, expiry.String()) {
		t.Errorf("reset email does not mention the configured expiry %q:\n%s", expiry, mailer.sent[0].Body)
	}
	if strings.Contains(mailer.sent[0].Body, time.Hour.String()) {
		t.Errorf("reset email still names the old hard-coded one-hour expiry:\n%s", mailer.sent[0].Body)
	}
}
