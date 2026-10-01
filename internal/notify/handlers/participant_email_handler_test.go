// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/notify/handlers"
	"github.com/whento/whento/internal/testutil"
)

// The three endpoints below were unreachable from a test while the handler held two
// concrete repositories and a service that parses templates in its constructor.
// These are the fakes the interfaces made possible.

type addEmailCall struct {
	participantID uuid.UUID
	address       string
	name          string
	locale        string
}

type stubEmailService struct {
	addErr     error
	verifyErr  error
	resendErr  error
	configured bool

	added     []addEmailCall
	verified  []string
	resent    []uuid.UUID
	lastToken string
}

func (s *stubEmailService) IsConfigured() bool { return s.configured }

func (s *stubEmailService) AddEmail(_ context.Context, participantID uuid.UUID, address, name, locale string) error {
	if s.addErr != nil {
		return s.addErr
	}
	s.added = append(s.added, addEmailCall{participantID, address, name, locale})
	return nil
}

func (s *stubEmailService) VerifyEmail(_ context.Context, token string) error {
	s.lastToken = token
	if s.verifyErr != nil {
		return s.verifyErr
	}
	s.verified = append(s.verified, token)
	return nil
}

func (s *stubEmailService) ResendVerification(_ context.Context, participantID uuid.UUID, _ string) error {
	if s.resendErr != nil {
		return s.resendErr
	}
	s.resent = append(s.resent, participantID)
	return nil
}

type stubCalendarByToken struct {
	calendar *calendarModels.Calendar
	err      error
}

func (s *stubCalendarByToken) GetByPublicToken(_ context.Context, _ string) (*calendarModels.Calendar, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.calendar, nil
}

type stubParticipantByID struct {
	participant *calendarModels.Participant
	err         error
}

func (s *stubParticipantByID) GetByID(_ context.Context, _ uuid.UUID) (*calendarModels.Participant, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.participant, nil
}

var (
	_ handlers.ParticipantEmailService = (*stubEmailService)(nil)
	_ handlers.CalendarByToken         = (*stubCalendarByToken)(nil)
	_ handlers.ParticipantByID         = (*stubParticipantByID)(nil)
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// emailFixture is a calendar with one participant on it.
type emailFixture struct {
	calendar    *calendarModels.Calendar
	participant *calendarModels.Participant
	service     *stubEmailService
	calendars   *stubCalendarByToken
	people      *stubParticipantByID
}

func newEmailFixture() *emailFixture {
	configJSON := `{"enabled":true,"notify_owner":true,"notify_participants":true,"channels":{"email":{"enabled":true}},"reminders":{"enabled":false,"hours_before":24}}`
	calendar := &calendarModels.Calendar{
		Name:              "Board game night",
		PublicToken:       "public-token",
		NotifyConfig:      &configJSON,
		NotifyOnThreshold: true,
	}
	calendar.ID = uuid.New()

	participant := &calendarModels.Participant{CalendarID: calendar.ID, Name: "Ada", Locale: "en"}
	participant.ID = uuid.New()

	return &emailFixture{
		calendar:    calendar,
		participant: participant,
		service:     &stubEmailService{configured: true},
		calendars:   &stubCalendarByToken{calendar: calendar},
		people:      &stubParticipantByID{participant: participant},
	}
}

func (f *emailFixture) handler() *handlers.ParticipantEmailHandler {
	return handlers.NewParticipantEmailHandler(f.service, f.people, f.calendars, silentLogger())
}

// TestAddEmail covers the capability check that guards the endpoint: the caller
// must present a token that names a calendar, and a participant that belongs to it.
func TestAddEmail(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		pid        string
		arrange    func(*emailFixture)
		wantStatus int
		wantAdded  bool
	}{
		{
			name:       "accepted",
			body:       `{"email":"ada@example.test"}`,
			wantStatus: http.StatusOK,
			wantAdded:  true,
		},
		{
			name:       "the participant id is not a UUID",
			body:       `{"email":"ada@example.test"}`,
			pid:        "not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed body",
			body:       "{",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "the address does not validate",
			body:       `{"email":"not-an-address"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "the token names no calendar",
			body:       `{"email":"ada@example.test"}`,
			arrange:    func(f *emailFixture) { f.calendars.err = errors.New("no rows") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "the participant does not exist",
			body:       `{"email":"ada@example.test"}`,
			arrange:    func(f *emailFixture) { f.people.err = errors.New("no rows") },
			wantStatus: http.StatusNotFound,
		},
		{
			name: "the participant belongs to another calendar",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				f.participant.CalendarID = uuid.New()
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "the email channel is disabled in the notify config",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				closed := `{"enabled":true,"notify_owner":true,"notify_participants":true,"channels":{"email":{"enabled":false}}}`
				f.calendar.NotifyConfig = &closed
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "the instance has no SMTP configured",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				f.service.configured = false
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "the participant email is disabled in the notify config",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				closed := `{"enabled":true,"notify_owner":true,"notify_participants":false}`
				f.calendar.NotifyConfig = &closed
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "notifications are disabled entirely",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				closed := `{"enabled":false,"notify_participants":true}`
				f.calendar.NotifyConfig = &closed
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "a calendar with no notify config accepts no participant email",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				f.calendar.NotifyConfig = nil
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "the email channel is disabled in the notify config",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				closed := `{"enabled":true,"notify_participants":true,"channels":{"email":{"enabled":false}}}`
				f.calendar.NotifyConfig = &closed
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "the SMTP capability is absent",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				f.service.configured = false
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "a malformed stored config accepts no participant email",
			body: `{"email":"ada@example.test"}`,
			arrange: func(f *emailFixture) {
				bad := `{not json`
				f.calendar.NotifyConfig = &bad
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "the service refuses",
			body:       `{"email":"ada@example.test"}`,
			arrange:    func(f *emailFixture) { f.service.addErr = errors.New("address already in use") },
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEmailFixture()
			if tt.arrange != nil {
				tt.arrange(f)
			}

			pid := tt.pid
			if pid == "" {
				pid = f.participant.ID.String()
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/calendars/tok/participants/pid/email", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = testutil.WithURLParams(req, map[string]string{"token": "public-token", "pid": pid})
			w := httptest.NewRecorder()

			f.handler().AddEmail(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if got := len(f.service.added) > 0; got != tt.wantAdded {
				t.Fatalf("AddEmail called = %v, want %v", got, tt.wantAdded)
			}
			if tt.wantAdded {
				call := f.service.added[0]
				if call.participantID != f.participant.ID || call.address != "ada@example.test" || call.name != "Ada" {
					t.Errorf("service received %+v", call)
				}
				if call.locale != "en" {
					t.Errorf("service locale = %q, want the participant's own locale", call.locale)
				}
				// The response is what the frontend types against.
				body := w.Body.String()
				for _, want := range []string{f.participant.ID.String(), "ada@example.test", `"verified":false`, "Verification email sent"} {
					if !strings.Contains(body, want) {
						t.Errorf("response %q does not contain %q", body, want)
					}
				}
			}
		})
	}
}

// TestVerifyEmail covers the link a participant clicks.
func TestVerifyEmail(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		verifyErr  error
		wantStatus int
	}{
		{name: "accepted", token: "a-token", wantStatus: http.StatusOK},
		{name: "no token in the path", wantStatus: http.StatusBadRequest},
		{
			name:       "the token is unknown or expired",
			token:      "a-token",
			verifyErr:  errors.New("invalid or expired verification token"),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEmailFixture()
			f.service.verifyErr = tt.verifyErr

			req := testutil.MakeRequest(http.MethodGet, "/api/v1/calendars/participants/verify-email/x")
			req = testutil.WithURLParams(req, map[string]string{"token": tt.token})
			w := httptest.NewRecorder()

			f.handler().VerifyEmail(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantStatus == http.StatusOK && !strings.Contains(w.Body.String(), "Email verified successfully") {
				t.Errorf("response does not carry the confirmation: %s", w.Body.String())
			}
		})
	}
}

// TestResendVerification covers the same capability check as AddEmail, on the
// endpoint that has no body to validate.
func TestResendVerification(t *testing.T) {
	tests := []struct {
		name       string
		pid        string
		arrange    func(*emailFixture)
		wantStatus int
		wantResent bool
	}{
		{name: "accepted", wantStatus: http.StatusOK, wantResent: true},
		{name: "the participant id is not a UUID", pid: "not-a-uuid", wantStatus: http.StatusBadRequest},
		{
			name:       "the token names no calendar",
			arrange:    func(f *emailFixture) { f.calendars.err = errors.New("no rows") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "the participant does not exist",
			arrange:    func(f *emailFixture) { f.people.err = errors.New("no rows") },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "the participant belongs to another calendar",
			arrange:    func(f *emailFixture) { f.participant.CalendarID = uuid.New() },
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "the service refuses",
			arrange:    func(f *emailFixture) { f.service.resendErr = errors.New("email already verified") },
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "the participant email switch was turned off since the link was sent",
			arrange: func(f *emailFixture) {
				closed := `{"enabled":true,"notify_owner":true,"notify_participants":false}`
				f.calendar.NotifyConfig = &closed
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "the email channel was turned off since the link was sent",
			arrange: func(f *emailFixture) {
				closed := `{"enabled":true,"notify_participants":true,"channels":{"email":{"enabled":false}}}`
				f.calendar.NotifyConfig = &closed
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "SMTP disappeared since the link was sent",
			arrange: func(f *emailFixture) {
				f.service.configured = false
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEmailFixture()
			if tt.arrange != nil {
				tt.arrange(f)
			}

			pid := tt.pid
			if pid == "" {
				pid = f.participant.ID.String()
			}

			req := testutil.MakeRequest(http.MethodPost, "/api/v1/calendars/tok/participants/pid/resend-verification")
			req = testutil.WithURLParams(req, map[string]string{"token": "public-token", "pid": pid})
			w := httptest.NewRecorder()

			f.handler().ResendVerification(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if got := len(f.service.resent) > 0; got != tt.wantResent {
				t.Fatalf("ResendVerification called = %v, want %v", got, tt.wantResent)
			}
			if tt.wantResent && !strings.Contains(w.Body.String(), "Verification email resent") {
				t.Errorf("response does not carry the confirmation: %s", w.Body.String())
			}
		})
	}
}
