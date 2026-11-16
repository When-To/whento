// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/email"
	"github.com/whento/pkg/logger"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
)

type fakeMailer struct {
	configured bool
	sendErr    error
	sent       []email.Email
	// ctxErr is the send context's error at send time: the confirmation must
	// survive a request whose client has already gone.
	ctxErr error
}

func (m *fakeMailer) IsConfigured() bool { return m.configured }

func (m *fakeMailer) SendContext(ctx context.Context, msg email.Email) error {
	m.ctxErr = ctx.Err()
	m.sent = append(m.sent, msg)
	return m.sendErr
}

const deletionPassword = "Str0ng!Passw0rd"

func TestDeleteOwnAccount(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		role      string
		deleteErr error
		mailer    fakeMailer
		wantErr   error
		wantGone  bool
		wantMails int
	}{
		{name: "wrong password deletes nothing", password: "nope", role: models.RoleUser, mailer: fakeMailer{configured: true}, wantErr: ErrPasswordMismatch},
		{name: "last admin is refused", password: deletionPassword, role: models.RoleAdmin, deleteErr: repository.ErrLastAdmin, mailer: fakeMailer{configured: true}, wantErr: ErrLastAdmin},
		{name: "success sends the confirmation", password: deletionPassword, role: models.RoleUser, mailer: fakeMailer{configured: true}, wantGone: true, wantMails: 1},
		{name: "failed send keeps the deletion", password: deletionPassword, role: models.RoleUser, mailer: fakeMailer{configured: true, sendErr: errors.New("smtp down")}, wantGone: true, wantMails: 1},
		{name: "no SMTP sends nothing", password: deletionPassword, role: models.RoleUser, mailer: fakeMailer{}, wantGone: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, nil)
			*f.mailer = tt.mailer
			user := f.withUser(t, "owner@example.com", deletionPassword, tt.role)
			f.users.deleteErr = tt.deleteErr
			f.users.ownedTokens = []string{"tok-a", "tok-b"}
			f.cache.values[cache.CalendarByPublicTokenKey("tok-a")] = 1

			err := f.service.DeleteOwnAccount(context.Background(), user.ID.String(), tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}

			if gone := f.users.deleted == user.ID; gone != tt.wantGone {
				t.Errorf("deleted = %v, want %v", gone, tt.wantGone)
			}
			if len(f.mailer.sent) != tt.wantMails {
				t.Errorf("sent %d emails, want %d", len(f.mailer.sent), tt.wantMails)
			}

			_, revoked := f.cache.values[cache.UserPasswordChangedKey(user.ID.String())]
			if revoked != tt.wantGone {
				t.Errorf("access tokens revoked = %v, want %v", revoked, tt.wantGone)
			}
			if tt.wantGone {
				for _, token := range []string{"tok-a", "tok-b"} {
					if !slices.Contains(f.cache.deletes, cache.CalendarByPublicTokenKey(token)) {
						t.Errorf("public calendar %q not purged from the cache", token)
					}
				}
			} else if len(f.cache.deletes) != 0 {
				t.Errorf("a refused deletion purged %v", f.cache.deletes)
			}
		})
	}
}

func TestDeleteOwnAccountEmail(t *testing.T) {
	tests := []struct {
		locale      string
		wantSubject string
	}{
		{models.LocaleFR, "Votre compte WhenTo a été supprimé"},
		{models.LocaleEN, "Your WhenTo account has been deleted"},
	}

	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			f := newFixture(t, nil)
			user := f.withUser(t, "owner@example.com", deletionPassword, models.RoleUser)
			user.Locale = tt.locale
			user.DisplayName = "<b>Owner</b>"
			f.users.ownedTokens = []string{"tok-a", "tok-b"}

			// A request whose client already hung up: the notice must still go.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := f.service.DeleteOwnAccount(ctx, user.ID.String(), deletionPassword); err != nil {
				t.Fatalf("DeleteOwnAccount: %v", err)
			}

			if len(f.mailer.sent) != 1 {
				t.Fatalf("sent %d emails, want 1", len(f.mailer.sent))
			}
			msg := f.mailer.sent[0]
			if f.mailer.ctxErr != nil {
				t.Errorf("send context was cancelled with the request: %v", f.mailer.ctxErr)
			}
			if msg.Subject != tt.wantSubject {
				t.Errorf("subject = %q, want %q", msg.Subject, tt.wantSubject)
			}
			if len(msg.To) != 1 || msg.To[0] != user.Email {
				t.Errorf("to = %v, want [%s]", msg.To, user.Email)
			}
			if !strings.Contains(msg.Body, ": 2.") {
				t.Errorf("body does not state the 2 deleted calendars:\n%s", msg.Body)
			}
			if strings.Contains(msg.Body, "<b>Owner</b>") {
				t.Error("display name was not escaped")
			}
			if strings.Contains(msg.Body, "href") {
				t.Error("the confirmation must not carry a link")
			}
		})
	}
}

// TestDeleteUserSharesTheDeletionPath pins that the admin path revokes and purges
// like the self-service one, and sends no confirmation.
func TestDeleteUserSharesTheDeletionPath(t *testing.T) {
	f := newFixture(t, nil)
	admin := f.withUser(t, "admin@example.com", deletionPassword, models.RoleAdmin)
	target := f.withUser(t, "target@example.com", deletionPassword, models.RoleUser)
	f.users.ownedTokens = []string{"tok-a"}

	if err := f.service.DeleteUser(context.Background(), admin.ID.String(), target.ID.String()); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	if _, ok := f.cache.values[cache.UserPasswordChangedKey(target.ID.String())]; !ok {
		t.Error("the deleted user's access tokens were not revoked")
	}
	if !slices.Contains(f.cache.deletes, cache.CalendarByPublicTokenKey("tok-a")) {
		t.Error("the deleted user's public calendar was not purged")
	}
	if len(f.mailer.sent) != 0 {
		t.Error("an admin deletion sent a confirmation email")
	}

	if err := f.service.DeleteUser(context.Background(), admin.ID.String(), uuid.NewString()); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown target gave %v, want ErrUserNotFound", err)
	}
}

// TestDeleteOwnAccountTransientLookupIsNotUnauthorized pins that a database
// hiccup is a 500, not the 401 that would sign the user out.
func TestDeleteOwnAccountTransientLookupIsNotUnauthorized(t *testing.T) {
	f := newFixture(t, nil)
	user := f.withUser(t, "owner@example.com", deletionPassword, models.RoleUser)
	f.users.getByIDErr = errStore

	err := f.service.DeleteOwnAccount(context.Background(), user.ID.String(), deletionPassword)
	if err == nil || errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, want a non-ErrUserNotFound failure", err)
	}
	if f.users.deleted != uuid.Nil {
		t.Error("a failed lookup still deleted the account")
	}
}

// TestDeleteOwnAccountEmailFailureLogsNoAddress: SMTP servers echo the
// recipient in their rejection, and the address must not reach the logs.
func TestDeleteOwnAccountEmailFailureLogsNoAddress(t *testing.T) {
	original := logger.Default()
	t.Cleanup(func() { logger.SetDefault(original) })
	var buf bytes.Buffer
	logger.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	f := newFixture(t, nil)
	user := f.withUser(t, "owner@example.com", deletionPassword, models.RoleUser)
	f.mailer.sendErr = fmt.Errorf("rcpt: 550 5.1.1 <%s>: Recipient address rejected", user.Email)

	if err := f.service.DeleteOwnAccount(context.Background(), user.ID.String(), deletionPassword); err != nil {
		t.Fatalf("DeleteOwnAccount: %v", err)
	}
	if strings.Contains(buf.String(), user.Email) {
		t.Errorf("the log carries the address:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "Recipient address rejected") {
		t.Errorf("the SMTP reason was lost:\n%s", buf.String())
	}
}
