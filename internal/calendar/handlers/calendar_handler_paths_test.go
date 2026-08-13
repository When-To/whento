// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	authModels "github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/calendar/handlers"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/service"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/testutil"
)

// The handlers used to hold *service.CalendarService, *authRepo.UserRepository and a
// *pgxpool.Pool, so reaching a 404 meant standing up two repositories and a cache,
// and reaching the quota lock meant a database. They now take interfaces, and these
// stubs are what stands behind them.

// stubCalendarService satisfies both handlers.CalendarService and
// handlers.ParticipantService: one stub, because one service implements both.
type stubCalendarService struct {
	calendar  *models.CalendarResponse
	calendars []*models.CalendarResponse
	public    *models.PublicCalendarResponse
	partic    *models.Participant
	err       error

	createCalls int
	lastUserID  string
	lastRole    string
	lastID      string
	lastToken   string
}

func (s *stubCalendarService) CreateCalendar(_ context.Context, userID string, _ *models.CreateCalendarRequest) (*models.CalendarResponse, error) {
	s.createCalls++
	s.lastUserID = userID
	if s.err != nil {
		return nil, s.err
	}
	return s.calendar, nil
}

func (s *stubCalendarService) GetCalendar(_ context.Context, userID, userRole, calendarID string) (*models.CalendarResponse, error) {
	s.lastUserID, s.lastRole, s.lastID = userID, userRole, calendarID
	if s.err != nil {
		return nil, s.err
	}
	return s.calendar, nil
}

func (s *stubCalendarService) ListMyCalendars(_ context.Context, userID string) ([]*models.CalendarResponse, error) {
	s.lastUserID = userID
	if s.err != nil {
		return nil, s.err
	}
	return s.calendars, nil
}

func (s *stubCalendarService) UpdateCalendar(_ context.Context, userID, userRole, calendarID string, _ *models.UpdateCalendarRequest) (*models.CalendarResponse, error) {
	s.lastUserID, s.lastRole, s.lastID = userID, userRole, calendarID
	if s.err != nil {
		return nil, s.err
	}
	return s.calendar, nil
}

func (s *stubCalendarService) DeleteCalendar(_ context.Context, userID, userRole, calendarID string) error {
	s.lastUserID, s.lastRole, s.lastID = userID, userRole, calendarID
	return s.err
}

func (s *stubCalendarService) RegenerateToken(_ context.Context, userID, userRole, calendarID, tokenType string) (*models.CalendarResponse, error) {
	s.lastUserID, s.lastRole, s.lastID, s.lastToken = userID, userRole, calendarID, tokenType
	if s.err != nil {
		return nil, s.err
	}
	return s.calendar, nil
}

func (s *stubCalendarService) GetPublicCalendar(_ context.Context, token, participantID string) (*models.PublicCalendarResponse, error) {
	s.lastToken, s.lastID = token, participantID
	if s.err != nil {
		return nil, s.err
	}
	return s.public, nil
}

func (s *stubCalendarService) ListUserCalendars(_ context.Context, targetUserID string) ([]*models.CalendarResponse, error) {
	s.lastUserID = targetUserID
	if s.err != nil {
		return nil, s.err
	}
	return s.calendars, nil
}

func (s *stubCalendarService) AddParticipant(_ context.Context, userID, userRole, calendarID string, _ *models.AddParticipantRequest) (*models.Participant, error) {
	s.lastUserID, s.lastRole, s.lastID = userID, userRole, calendarID
	if s.err != nil {
		return nil, s.err
	}
	return s.partic, nil
}

func (s *stubCalendarService) AddAnonymousParticipant(_ context.Context, publicToken string, _ *models.AddParticipantRequest) (*models.Participant, error) {
	s.lastToken = publicToken
	if s.err != nil {
		return nil, s.err
	}
	return s.partic, nil
}

func (s *stubCalendarService) UpdateParticipant(_ context.Context, userID, userRole, calendarID, participantID string, _ *models.UpdateParticipantRequest) (*models.Participant, error) {
	s.lastUserID, s.lastRole, s.lastID = userID, userRole, participantID
	if s.err != nil {
		return nil, s.err
	}
	return s.partic, nil
}

func (s *stubCalendarService) RemoveParticipant(_ context.Context, userID, userRole, calendarID, participantID string) error {
	s.lastUserID, s.lastRole, s.lastID = userID, userRole, participantID
	return s.err
}

// stubUserLookup answers the one question the create path asks of the user store.
type stubUserLookup struct {
	user *authModels.User
	err  error
}

func (s *stubUserLookup) GetByID(_ context.Context, _ uuid.UUID) (*authModels.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.user, nil
}

// stubQuotaLock stands in for the advisory lock, and records whether the body ran
// inside it — which is the whole point of the lock existing.
type stubQuotaLock struct {
	err     error
	calls   int
	lastKey int64
	ranBody bool
}

func (s *stubQuotaLock) WithQuotaLock(ctx context.Context, key int64, fn func(context.Context) error) error {
	s.calls++
	s.lastKey = key
	if s.err != nil {
		// The contract: a lock that could not be taken never runs the body.
		return s.err
	}
	s.ranBody = true
	return fn(ctx)
}

var (
	_ handlers.CalendarService    = (*stubCalendarService)(nil)
	_ handlers.ParticipantService = (*stubCalendarService)(nil)
	_ handlers.UserLookup         = (*stubUserLookup)(nil)
	_ handlers.QuotaLock          = (*stubQuotaLock)(nil)
)

func noVerificationConfig() *config.Config {
	return &config.Config{Email: config.EmailConfig{VerificationEnabled: false}}
}

func calendarResponse() *models.CalendarResponse {
	return &models.CalendarResponse{ID: uuid.New(), Name: "Board game night"}
}

// TestCreateCalendarUnderTheQuotaLock is what the refactoring was for: the quota
// check and the creation happen inside the lock, and a lock that cannot be taken
// creates nothing.
func TestCreateCalendarUnderTheQuotaLock(t *testing.T) {
	body := map[string]any{"name": "Board game night"}

	t.Run("the check and the creation run inside the lock", func(t *testing.T) {
		svc := &stubCalendarService{calendar: calendarResponse()}
		lock := &stubQuotaLock{}
		quota := &mockQuotaService{canCreate: true}
		handler := handlers.NewCalendarHandler(svc, quota, nil, noVerificationConfig(), lock)

		req := testutil.WithAuth(testutil.MakeJSONRequest(http.MethodPost, "/api/v1/calendars", body), uuid.New().String(), "user")
		w := httptest.NewRecorder()

		handler.CreateCalendar(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		if lock.calls != 1 {
			t.Errorf("the lock was taken %d times, want once", lock.calls)
		}
		if !lock.ranBody {
			t.Error("the creation ran outside the lock")
		}
		if svc.createCalls != 1 {
			t.Errorf("CreateCalendar called %d times", svc.createCalls)
		}
	})

	t.Run("a lock that cannot be taken refuses the request and creates nothing", func(t *testing.T) {
		svc := &stubCalendarService{calendar: calendarResponse()}
		lock := &stubQuotaLock{err: errors.New("connection refused")}
		handler := handlers.NewCalendarHandler(svc, &mockQuotaService{canCreate: true}, nil, noVerificationConfig(), lock)

		req := testutil.WithAuth(testutil.MakeJSONRequest(http.MethodPost, "/api/v1/calendars", body), uuid.New().String(), "user")
		w := httptest.NewRecorder()

		handler.CreateCalendar(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
		if svc.createCalls != 0 {
			t.Error("a calendar was created without the quota lock")
		}
		// The failure is the database's, not the caller's: it must not read as
		// a quota refusal.
		if strings.Contains(w.Body.String(), "quota_exceeded") {
			t.Errorf("a lock failure was reported as a quota refusal: %s", w.Body.String())
		}
	})

	t.Run("without a lock the creation still happens", func(t *testing.T) {
		svc := &stubCalendarService{calendar: calendarResponse()}
		handler := handlers.NewCalendarHandler(svc, &mockQuotaService{canCreate: true}, nil, noVerificationConfig(), nil)

		req := testutil.WithAuth(testutil.MakeJSONRequest(http.MethodPost, "/api/v1/calendars", body), uuid.New().String(), "user")
		w := httptest.NewRecorder()

		handler.CreateCalendar(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
	})
}

// TestCreateCalendarRejections covers every refusal the create path can produce.
func TestCreateCalendarRejections(t *testing.T) {
	tests := []struct {
		name       string
		body       any
		rawBody    string
		userID     string
		cfg        *config.Config
		users      handlers.UserLookup
		quota      *mockQuotaService
		svcErr     error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "no authenticated user",
			body:       map[string]any{"name": "Quiz night"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed JSON",
			rawBody:    "{",
			userID:     uuid.New().String(),
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid request body",
		},
		{
			name:       "missing name fails validation",
			body:       map[string]any{"description": "no name"},
			userID:     uuid.New().String(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "duplicate participant names",
			body:       map[string]any{"name": "Quiz night", "participants": []string{"Ada", "Ada"}},
			userID:     uuid.New().String(),
			wantStatus: http.StatusBadRequest,
			wantBody:   "Duplicate participant name",
		},
		{
			name:       "threshold above the participant count",
			body:       map[string]any{"name": "Quiz night", "participants": []string{"Ada"}, "threshold": 3},
			userID:     uuid.New().String(),
			wantStatus: http.StatusBadRequest,
			wantBody:   "Threshold cannot exceed",
		},
		{
			name: "threshold above the count is allowed when anyone may join",
			body: map[string]any{
				"name": "Quiz night", "participants": []string{"Ada"}, "threshold": 3,
				"allow_anonymous_participants": true,
			},
			userID:     uuid.New().String(),
			wantStatus: http.StatusCreated,
		},
		{
			name:       "the user id in the token is not a UUID",
			body:       map[string]any{"name": "Quiz night"},
			userID:     "not-a-uuid",
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid user ID",
		},
		{
			name:       "email verification required and the address is unverified",
			body:       map[string]any{"name": "Quiz night"},
			userID:     uuid.New().String(),
			cfg:        &config.Config{Email: config.EmailConfig{VerificationEnabled: true}},
			users:      &stubUserLookup{user: &authModels.User{EmailVerified: false}},
			wantStatus: http.StatusForbidden,
			wantBody:   "verify your email",
		},
		{
			name:       "email verification required and the user cannot be read",
			body:       map[string]any{"name": "Quiz night"},
			userID:     uuid.New().String(),
			cfg:        &config.Config{Email: config.EmailConfig{VerificationEnabled: true}},
			users:      &stubUserLookup{err: errors.New("database down")},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "email verification required and the address is verified",
			body:       map[string]any{"name": "Quiz night"},
			userID:     uuid.New().String(),
			cfg:        &config.Config{Email: config.EmailConfig{VerificationEnabled: true}},
			users:      &stubUserLookup{user: &authModels.User{EmailVerified: true}},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "the quota check itself fails",
			body:       map[string]any{"name": "Quiz night"},
			userID:     uuid.New().String(),
			quota:      &mockQuotaService{err: errors.New("count failed")},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "the calendar cannot be created",
			body:       map[string]any{"name": "Quiz night"},
			userID:     uuid.New().String(),
			svcErr:     errors.New("insert failed"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to create calendar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			if cfg == nil {
				cfg = noVerificationConfig()
			}
			quota := tt.quota
			if quota == nil {
				quota = &mockQuotaService{canCreate: true}
			}
			svc := &stubCalendarService{calendar: calendarResponse(), err: tt.svcErr}
			handler := handlers.NewCalendarHandler(svc, quota, tt.users, cfg, &stubQuotaLock{})

			var req *http.Request
			if tt.rawBody != "" {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/calendars", strings.NewReader(tt.rawBody))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = testutil.MakeJSONRequest(http.MethodPost, "/api/v1/calendars", tt.body)
			}
			if tt.userID != "" {
				req = testutil.WithAuth(req, tt.userID, "user")
			}
			w := httptest.NewRecorder()

			handler.CreateCalendar(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Errorf("body %q does not contain %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

// calendarHandler builds a handler over a stub service, for the read and write
// endpoints that have no quota involvement.
func calendarHandler(svc *stubCalendarService) *handlers.CalendarHandler {
	return handlers.NewCalendarHandler(svc, &mockQuotaService{canCreate: true}, nil, noVerificationConfig(), nil)
}

// TestCalendarHandlerErrorTranslation is the contract between the service's
// sentinel errors and the status codes clients branch on.
func TestCalendarHandlerErrorTranslation(t *testing.T) {
	tests := []struct {
		name       string
		invoke     func(*handlers.CalendarHandler, http.ResponseWriter, *http.Request)
		method     string
		body       any
		authed     bool
		err        error
		wantStatus int
	}{
		{
			name:       "GetCalendar without a session",
			invoke:     (*handlers.CalendarHandler).GetCalendar,
			method:     http.MethodGet,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "GetCalendar not found",
			invoke:     (*handlers.CalendarHandler).GetCalendar,
			method:     http.MethodGet,
			authed:     true,
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "GetCalendar forbidden",
			invoke:     (*handlers.CalendarHandler).GetCalendar,
			method:     http.MethodGet,
			authed:     true,
			err:        service.ErrUnauthorized,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "GetCalendar unknown failure",
			invoke:     (*handlers.CalendarHandler).GetCalendar,
			method:     http.MethodGet,
			authed:     true,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "GetCalendar success",
			invoke:     (*handlers.CalendarHandler).GetCalendar,
			method:     http.MethodGet,
			authed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "ListMyCalendars without a session",
			invoke:     (*handlers.CalendarHandler).ListMyCalendars,
			method:     http.MethodGet,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "ListMyCalendars failure",
			invoke:     (*handlers.CalendarHandler).ListMyCalendars,
			method:     http.MethodGet,
			authed:     true,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "ListMyCalendars success",
			invoke:     (*handlers.CalendarHandler).ListMyCalendars,
			method:     http.MethodGet,
			authed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "UpdateCalendar without a session",
			invoke:     (*handlers.CalendarHandler).UpdateCalendar,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "renamed"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "UpdateCalendar not found",
			invoke:     (*handlers.CalendarHandler).UpdateCalendar,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "renamed"},
			authed:     true,
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "UpdateCalendar forbidden",
			invoke:     (*handlers.CalendarHandler).UpdateCalendar,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "renamed"},
			authed:     true,
			err:        service.ErrUnauthorized,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "UpdateCalendar unknown failure",
			invoke:     (*handlers.CalendarHandler).UpdateCalendar,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "renamed"},
			authed:     true,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "UpdateCalendar success",
			invoke:     (*handlers.CalendarHandler).UpdateCalendar,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "renamed"},
			authed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "DeleteCalendar without a session",
			invoke:     (*handlers.CalendarHandler).DeleteCalendar,
			method:     http.MethodDelete,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "DeleteCalendar not found",
			invoke:     (*handlers.CalendarHandler).DeleteCalendar,
			method:     http.MethodDelete,
			authed:     true,
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "DeleteCalendar forbidden",
			invoke:     (*handlers.CalendarHandler).DeleteCalendar,
			method:     http.MethodDelete,
			authed:     true,
			err:        service.ErrUnauthorized,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "DeleteCalendar unknown failure",
			invoke:     (*handlers.CalendarHandler).DeleteCalendar,
			method:     http.MethodDelete,
			authed:     true,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "DeleteCalendar success",
			invoke:     (*handlers.CalendarHandler).DeleteCalendar,
			method:     http.MethodDelete,
			authed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "RegenerateToken without a session",
			invoke:     (*handlers.CalendarHandler).RegenerateToken,
			method:     http.MethodPost,
			body:       map[string]any{"token_type": "public"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "RegenerateToken not found",
			invoke:     (*handlers.CalendarHandler).RegenerateToken,
			method:     http.MethodPost,
			body:       map[string]any{"token_type": "public"},
			authed:     true,
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "RegenerateToken forbidden",
			invoke:     (*handlers.CalendarHandler).RegenerateToken,
			method:     http.MethodPost,
			body:       map[string]any{"token_type": "public"},
			authed:     true,
			err:        service.ErrUnauthorized,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "RegenerateToken rejects an unknown token type",
			invoke:     (*handlers.CalendarHandler).RegenerateToken,
			method:     http.MethodPost,
			body:       map[string]any{"token_type": "public"},
			authed:     true,
			err:        service.ErrInvalidTokenType,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "RegenerateToken unknown failure",
			invoke:     (*handlers.CalendarHandler).RegenerateToken,
			method:     http.MethodPost,
			body:       map[string]any{"token_type": "ics"},
			authed:     true,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "RegenerateToken success",
			invoke:     (*handlers.CalendarHandler).RegenerateToken,
			method:     http.MethodPost,
			body:       map[string]any{"token_type": "ics"},
			authed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "GetPublicCalendar not found",
			invoke:     (*handlers.CalendarHandler).GetPublicCalendar,
			method:     http.MethodGet,
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "GetPublicCalendar unknown failure",
			invoke:     (*handlers.CalendarHandler).GetPublicCalendar,
			method:     http.MethodGet,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "GetPublicCalendar success",
			invoke:     (*handlers.CalendarHandler).GetPublicCalendar,
			method:     http.MethodGet,
			wantStatus: http.StatusOK,
		},
		{
			name:       "ListUserCalendars failure",
			invoke:     (*handlers.CalendarHandler).ListUserCalendars,
			method:     http.MethodGet,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "ListUserCalendars success",
			invoke:     (*handlers.CalendarHandler).ListUserCalendars,
			method:     http.MethodGet,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubCalendarService{
				calendar: calendarResponse(),
				public:   &models.PublicCalendarResponse{Name: "Board game night"},
				err:      tt.err,
			}
			handler := calendarHandler(svc)

			var req *http.Request
			if tt.body != nil {
				req = testutil.MakeJSONRequest(tt.method, "/api/v1/calendars/x", tt.body)
			} else {
				req = testutil.MakeRequest(tt.method, "/api/v1/calendars/x")
			}
			req = testutil.WithURLParams(req, map[string]string{"id": uuid.New().String(), "token": "public-token"})
			if tt.authed {
				req = testutil.WithAuth(req, uuid.New().String(), "user")
			}
			w := httptest.NewRecorder()

			tt.invoke(handler, w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

// TestCalendarHandlerRejectsMalformedBodies covers the decode and validation
// branches of the two endpoints that take one.
func TestCalendarHandlerRejectsMalformedBodies(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*handlers.CalendarHandler, http.ResponseWriter, *http.Request)
		body   string
	}{
		{name: "UpdateCalendar, malformed JSON", invoke: (*handlers.CalendarHandler).UpdateCalendar, body: "{"},
		{name: "UpdateCalendar, invalid field", invoke: (*handlers.CalendarHandler).UpdateCalendar, body: `{"threshold":-4}`},
		{name: "RegenerateToken, malformed JSON", invoke: (*handlers.CalendarHandler).RegenerateToken, body: "{"},
		{name: "RegenerateToken, missing token type", invoke: (*handlers.CalendarHandler).RegenerateToken, body: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := calendarHandler(&stubCalendarService{calendar: calendarResponse()})

			req := httptest.NewRequest(http.MethodPatch, "/api/v1/calendars/x", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = testutil.WithAuth(req, uuid.New().String(), "user")
			w := httptest.NewRecorder()

			tt.invoke(handler, w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestGetPublicCalendarPassesTheParticipantFilter records that the participant_id
// query parameter reaches the service: it is what decides whose identity is left
// unmasked on a locked calendar.
func TestGetPublicCalendarPassesTheParticipantFilter(t *testing.T) {
	svc := &stubCalendarService{public: &models.PublicCalendarResponse{Name: "Board game night"}}
	handler := calendarHandler(svc)

	participantID := uuid.New().String()
	req := testutil.MakeRequest(http.MethodGet, "/api/v1/calendars/public/tok?participant_id="+participantID)
	req = testutil.WithURLParams(req, map[string]string{"token": "tok"})
	w := httptest.NewRecorder()

	handler.GetPublicCalendar(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if svc.lastToken != "tok" || svc.lastID != participantID {
		t.Errorf("service received token=%q participant=%q", svc.lastToken, svc.lastID)
	}

	var envelope struct {
		Data models.PublicCalendarResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("response is not the standard envelope: %v", err)
	}
	if envelope.Data.Name != "Board game night" {
		t.Errorf("payload = %+v", envelope.Data)
	}
}
