// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/config"
)

// fakeParticipantEmailStore records what the service asked the database to do.
//
// It is mutex-guarded for the same reason fakeMailer is: AddEmail and
// ResendVerification hand delivery to a goroutine that reads the participant.
type fakeParticipantEmailStore struct {
	mu sync.Mutex

	participant *calendarModels.Participant
	byToken     *calendarModels.Participant

	setTokenErr    error
	byTokenErr     error
	byIDErr        error
	verifyErr      error
	setTokenCalls  []setTokenCall
	verifiedIDs    []uuid.UUID
	lastTokenValue string
}

type setTokenCall struct {
	participantID uuid.UUID
	email         string
	expiresAt     time.Time
}

func (f *fakeParticipantEmailStore) SetEmailVerificationToken(
	_ context.Context, participantID uuid.UUID, emailAddress, token string, expiresAt time.Time,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setTokenErr != nil {
		return f.setTokenErr
	}
	f.setTokenCalls = append(f.setTokenCalls, setTokenCall{participantID, emailAddress, expiresAt})
	f.lastTokenValue = token
	return nil
}

func (f *fakeParticipantEmailStore) GetByVerificationToken(_ context.Context, _ string) (*calendarModels.Participant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byTokenErr != nil {
		return nil, f.byTokenErr
	}
	return f.byToken, nil
}

func (f *fakeParticipantEmailStore) GetByID(_ context.Context, _ uuid.UUID) (*calendarModels.Participant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byIDErr != nil {
		return nil, f.byIDErr
	}
	return f.participant, nil
}

func (f *fakeParticipantEmailStore) VerifyEmail(_ context.Context, participantID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.verifyErr != nil {
		return f.verifyErr
	}
	f.verifiedIDs = append(f.verifiedIDs, participantID)
	return nil
}

func (f *fakeParticipantEmailStore) tokenCalls() []setTokenCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]setTokenCall(nil), f.setTokenCalls...)
}

func (f *fakeParticipantEmailStore) issuedToken() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastTokenValue
}

var _ ParticipantEmailStore = (*fakeParticipantEmailStore)(nil)

func participantEmailConfig() *config.Config {
	return &config.Config{
		AppURL: "https://whento.test",
		Email:  config.EmailConfig{VerificationExpiry: 24 * time.Hour},
	}
}

func newParticipantEmailService(
	store ParticipantEmailStore, mailer Mailer,
) *ParticipantEmailService {
	return NewParticipantEmailService(store, mailer, participantEmailConfig(), quietLogger())
}

// TestParticipantEmailAddEmail covers the token issue path, including the one
// failure that must stop it.
func TestParticipantEmailAddEmail(t *testing.T) {
	t.Run("issues a token, stores it and sends the verification mail", func(t *testing.T) {
		store := &fakeParticipantEmailStore{}
		mailer := &fakeMailer{configured: true}
		svc := newParticipantEmailService(store, mailer)

		pid := uuid.New()
		before := time.Now()
		if err := svc.AddEmail(context.Background(), pid, "ada@example.test", "Ada", "en"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		calls := store.tokenCalls()
		if len(calls) != 1 {
			t.Fatalf("expected one token to be stored, got %d", len(calls))
		}
		if calls[0].participantID != pid || calls[0].email != "ada@example.test" {
			t.Errorf("stored the wrong participant or address: %+v", calls[0])
		}
		if !calls[0].expiresAt.After(before.Add(23 * time.Hour)) {
			t.Errorf("expiry %v does not honour the configured 24h window", calls[0].expiresAt)
		}
		if len(store.issuedToken()) != 64 {
			t.Errorf("verification token is %d characters, want 64", len(store.issuedToken()))
		}

		msgs := mailer.awaitMessages(t, 1)
		if msgs[0].To[0] != "ada@example.test" {
			t.Errorf("mail went to %v", msgs[0].To)
		}
		if !msgs[0].HTML {
			t.Error("the verification mail should be HTML")
		}
		if !strings.Contains(msgs[0].Body, "/c/verify-email/"+store.issuedToken()) {
			t.Error("the mail does not carry the verification link")
		}
		if !strings.Contains(msgs[0].Body, "Ada") {
			t.Error("the mail does not greet the participant by name")
		}
	})

	t.Run("a database failure stops the send", func(t *testing.T) {
		store := &fakeParticipantEmailStore{setTokenErr: errors.New("write failed")}
		mailer := &fakeMailer{configured: true}
		svc := newParticipantEmailService(store, mailer)

		err := svc.AddEmail(context.Background(), uuid.New(), "ada@example.test", "Ada", "en")
		if err == nil {
			t.Fatal("expected the write failure to surface")
		}
		if len(mailer.messages()) != 0 {
			t.Error("mail was sent even though the token was never stored")
		}
	})

	t.Run("an unconfigured mailer is not an error", func(t *testing.T) {
		store := &fakeParticipantEmailStore{}
		mailer := &fakeMailer{configured: false}
		svc := newParticipantEmailService(store, mailer)

		if err := svc.AddEmail(context.Background(), uuid.New(), "ada@example.test", "Ada", "en"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(store.tokenCalls()) != 1 {
			t.Error("the token should still be stored so the address can be verified later")
		}
	})

	t.Run("an unknown locale falls back to english", func(t *testing.T) {
		store := &fakeParticipantEmailStore{}
		mailer := &fakeMailer{configured: true}
		svc := newParticipantEmailService(store, mailer)

		if err := svc.AddEmail(context.Background(), uuid.New(), "ada@example.test", "Ada", "kl"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		msgs := mailer.awaitMessages(t, 1)
		english := svc.translations["en"]["subject"]
		if msgs[0].Subject != english {
			t.Errorf("subject = %q, want the english %q", msgs[0].Subject, english)
		}
	})

	t.Run("a participant name carrying markup is escaped into the mail", func(t *testing.T) {
		store := &fakeParticipantEmailStore{}
		mailer := &fakeMailer{configured: true}
		svc := newParticipantEmailService(store, mailer)

		if err := svc.AddEmail(context.Background(), uuid.New(), "ada@example.test", `<script>x</script>`, "en"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		msgs := mailer.awaitMessages(t, 1)
		if strings.Contains(msgs[0].Body, "<script>") {
			t.Error("an unescaped script tag reached the verification mail")
		}
	})
}

// TestParticipantEmailVerifyEmail covers the three answers the verification link
// can get.
func TestParticipantEmailVerifyEmail(t *testing.T) {
	verified := &calendarModels.Participant{Name: "Ada", Email: strptr("ada@example.test"), EmailVerified: true}
	verified.ID = uuid.New()
	pending := &calendarModels.Participant{Name: "Ada", Email: strptr("ada@example.test")}
	pending.ID = uuid.New()

	tests := []struct {
		name       string
		store      *fakeParticipantEmailStore
		wantErr    bool
		wantVerify bool
	}{
		{
			name:       "a valid token verifies the address",
			store:      &fakeParticipantEmailStore{byToken: pending},
			wantVerify: true,
		},
		{
			name:  "an already verified address is accepted without a second write",
			store: &fakeParticipantEmailStore{byToken: verified},
		},
		{
			name:    "an unknown or expired token is refused",
			store:   &fakeParticipantEmailStore{byTokenErr: errors.New("no rows")},
			wantErr: true,
		},
		{
			name:    "the write fails",
			store:   &fakeParticipantEmailStore{byToken: pending, verifyErr: errors.New("write failed")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newParticipantEmailService(tt.store, &fakeMailer{configured: true})

			err := svc.VerifyEmail(context.Background(), "a-token")

			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(tt.store.verifiedIDs) > 0; got != tt.wantVerify {
				t.Errorf("verified = %v, want %v", got, tt.wantVerify)
			}
		})
	}
}

// TestParticipantEmailResendVerification covers the refusals, which are the whole
// of the endpoint's logic: only a pending address with a known participant is resent.
func TestParticipantEmailResendVerification(t *testing.T) {
	pending := func() *calendarModels.Participant {
		p := &calendarModels.Participant{Name: "Ada", Email: strptr("ada@example.test")}
		p.ID = uuid.New()
		return p
	}

	t.Run("resends and rotates the token", func(t *testing.T) {
		store := &fakeParticipantEmailStore{participant: pending()}
		mailer := &fakeMailer{configured: true}
		svc := newParticipantEmailService(store, mailer)

		if err := svc.ResendVerification(context.Background(), store.participant.ID, "fr"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		calls := store.tokenCalls()
		if len(calls) != 1 {
			t.Fatalf("expected the token to be rewritten once, got %d", len(calls))
		}
		if calls[0].email != "ada@example.test" {
			t.Errorf("resent to the wrong address: %q", calls[0].email)
		}

		msgs := mailer.awaitMessages(t, 1)
		french := svc.translations["fr"]["subject"]
		if msgs[0].Subject != french {
			t.Errorf("subject = %q, want the french %q", msgs[0].Subject, french)
		}
	})

	tests := []struct {
		name  string
		store *fakeParticipantEmailStore
	}{
		{
			name:  "unknown participant",
			store: &fakeParticipantEmailStore{byIDErr: errors.New("no rows")},
		},
		{
			name: "already verified",
			store: func() *fakeParticipantEmailStore {
				p := pending()
				p.EmailVerified = true
				return &fakeParticipantEmailStore{participant: p}
			}(),
		},
		{
			name: "no address on file",
			store: func() *fakeParticipantEmailStore {
				p := pending()
				p.Email = nil
				return &fakeParticipantEmailStore{participant: p}
			}(),
		},
		{
			name: "empty address on file",
			store: func() *fakeParticipantEmailStore {
				p := pending()
				p.Email = strptr("")
				return &fakeParticipantEmailStore{participant: p}
			}(),
		},
		{
			name: "the token cannot be rewritten",
			store: &fakeParticipantEmailStore{
				participant: pending(), setTokenErr: errors.New("write failed"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" is refused", func(t *testing.T) {
			mailer := &fakeMailer{configured: true}
			svc := newParticipantEmailService(tt.store, mailer)

			if err := svc.ResendVerification(context.Background(), uuid.New(), "en"); err == nil {
				t.Fatal("expected an error, got none")
			}
			if len(mailer.messages()) != 0 {
				t.Error("mail was sent on a refused resend")
			}
		})
	}
}

// TestReplaceVar covers the tiny substitution the templates rely on, including the
// escaping that keeps a participant's name from becoming markup.
func TestReplaceVar(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		vari  string
		value string
		want  string
	}{
		{name: "substitutes", in: "Hi {{.ParticipantName}}", vari: "ParticipantName", value: "Ada", want: "Hi Ada"},
		{name: "escapes", in: "Hi {{.ParticipantName}}", vari: "ParticipantName", value: "<b>", want: "Hi &lt;b&gt;"},
		{name: "leaves other placeholders alone", in: "Hi {{.Other}}", vari: "ParticipantName", value: "Ada", want: "Hi {{.Other}}"},
		{name: "substitutes every occurrence", in: "{{.X}}/{{.X}}", vari: "X", value: "a", want: "a/a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replaceVar(tt.in, tt.vari, tt.value); got != tt.want {
				t.Errorf("replaceVar() = %q, want %q", got, tt.want)
			}
		})
	}
}
