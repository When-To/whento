// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whento/whento/internal/auth/handlers"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/config"
)

// The auth-side mail bodies had no test at all. Every value reaching them was escaped by
// hand on the way in, which was correct but rested on nobody ever adding a field and
// forgetting. These pin the two properties that matter — the layout renders, and a
// display name carrying markup arrives as text — so the templating engine underneath can
// be changed without doing it blind.
//
// SendVerificationEmail is the seam: it is the one auth path that sends synchronously.
// Registration and magic links hand the same builders to a detached goroutine.

func verificationRig(t *testing.T, user *models.User) (*handlers.EmailVerificationHandler, *fakeEmailSender) {
	t.Helper()

	store := &fakeUserStore{user: user}
	mail := &fakeEmailSender{configured: true}
	cfg := &config.Config{
		AppURL: "https://whento.example",
		Email: config.EmailConfig{
			VerificationEnabled: true,
			VerificationExpiry:  24 * time.Hour,
		},
	}
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	return handlers.NewEmailVerificationHandler(nil, store, mail, cfg, discard), mail
}

func resend(t *testing.T, user *models.User) string {
	t.Helper()

	handler, mail := verificationRig(t, user)

	rec := httptest.NewRecorder()
	req := asUser(post("/api/v1/auth/send-verification", `{}`), user.ID)
	handler.SendVerificationEmail(rec, req)

	if len(mail.sent) != 1 {
		t.Fatalf("sent %d mails, want 1 (status %d: %q)", len(mail.sent), rec.Code, rec.Body.String())
	}

	return mail.sent[0].Body
}

func TestVerificationMailRenders(t *testing.T) {
	user := &models.User{
		Email:       "ada@example.test",
		DisplayName: "Ada",
		Locale:      "en",
	}

	body := resend(t, user)

	for _, want := range []string{
		"Hello Ada,",
		"Verify Email Address",
		"https://whento.example/",
		// The signature is the one locale string carrying deliberate markup. It has to
		// stay a line break rather than become visible text.
		"Best regards,<br>The WhenTo Team",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the mail body is missing %q:\n%s", want, body)
		}
	}
}

func TestVerificationMailRendersInFrench(t *testing.T) {
	user := &models.User{
		Email:       "ada@example.test",
		DisplayName: "Ada",
		Locale:      "fr",
	}

	body := resend(t, user)

	if !strings.Contains(body, "Bonjour Ada,") {
		t.Errorf("the French greeting is missing:\n%s", body)
	}
	if !strings.Contains(body, "Cordialement,<br>L'équipe WhenTo") {
		t.Errorf("the French signature is missing or escaped:\n%s", body)
	}
}

// TestVerificationMailEscapesTheDisplayName is the one that has to keep passing whatever
// renders the body. A display name is free text the account holder chooses, and it lands
// in a mail a person reads.
func TestVerificationMailEscapesTheDisplayName(t *testing.T) {
	user := &models.User{
		Email:       "ada@example.test",
		DisplayName: `<script>alert(1)</script>`,
		Locale:      "en",
	}

	body := resend(t, user)

	if strings.Contains(body, "<script>") {
		t.Errorf("the display name reached the body as markup:\n%s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("the display name is not escaped exactly once:\n%s", body)
	}
	// Escaping it twice would render the entity text itself, which is a bug the other
	// direction: the reader sees "&lt;script&gt;" instead of "<script>".
	if strings.Contains(body, "&amp;lt;") {
		t.Errorf("the display name is double-escaped:\n%s", body)
	}
}
