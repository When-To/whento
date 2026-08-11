// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/ics/repository"
	"github.com/whento/whento/internal/ics/service"
)

// What a handler test adds over the service test is the HTTP contract: which service
// error becomes which status code, and the shape of the {success, data, error} envelope
// the frontend unwraps. Getting the mapping wrong turns a 403 into a 500, and the client
// retries something it should have reported.

type stubFeedRepo struct {
	feed        *repository.UnifiedFeed
	calendarIDs []uuid.UUID
	getErr      error
	created     *repository.UnifiedFeed
	createErr   error
	owned       bool
	updateErr   error
	newToken    string
	tokenErr    error
}

func (s *stubFeedRepo) GetByUserID(context.Context, uuid.UUID) (*repository.UnifiedFeed, []uuid.UUID, error) {
	if s.getErr != nil {
		return nil, nil, s.getErr
	}

	return s.feed, s.calendarIDs, nil
}

func (s *stubFeedRepo) Create(context.Context, uuid.UUID) (*repository.UnifiedFeed, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}

	return s.created, nil
}

func (s *stubFeedRepo) UpdateCalendars(context.Context, uuid.UUID, []uuid.UUID) error {
	return s.updateErr
}

func (s *stubFeedRepo) RegenerateToken(context.Context, uuid.UUID) (string, error) {
	if s.tokenErr != nil {
		return "", s.tokenErr
	}

	return s.newToken, nil
}

func (s *stubFeedRepo) ValidateCalendarOwnership(context.Context, uuid.UUID, []uuid.UUID) (bool, error) {
	return s.owned, nil
}

func newConfigHandler(repo *stubFeedRepo) *UnifiedFeedConfigHandler {
	return NewUnifiedFeedConfigHandler(service.NewUnifiedFeedConfigService(repo))
}

// authenticated returns a request carrying a user id in the context, the way the Auth
// middleware leaves it.
func authenticated(method, path, body string, userID uuid.UUID) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}

	return req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID.String()))
}

// envelope is the response shape every endpoint returns.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()

	var body envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response is not the standard envelope: %v (%q)", err, rec.Body.String())
	}

	return body
}

// TestEveryEndpointRequiresAuthentication walks all four. The router puts these behind
// the Auth middleware, but a handler that trusted an absent user id would fall back to
// the zero UUID and read somebody else's feed.
func TestEveryEndpointRequiresAuthentication(t *testing.T) {
	handler := newConfigHandler(&stubFeedRepo{})

	tests := []struct {
		name   string
		method string
		path   string
		serve  func(http.ResponseWriter, *http.Request)
	}{
		{"GetConfig", http.MethodGet, "/api/v1/ics/unified-feed", handler.GetConfig},
		{"Create", http.MethodPost, "/api/v1/ics/unified-feed", handler.Create},
		{"UpdateCalendars", http.MethodPatch, "/api/v1/ics/unified-feed/calendars", handler.UpdateCalendars},
		{"RegenerateToken", http.MethodPost, "/api/v1/ics/unified-feed/regenerate-token", handler.RegenerateToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.serve(rec, httptest.NewRequest(tt.method, tt.path, strings.NewReader("{}")))

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			if body := decode(t, rec); body.Success || body.Error == nil {
				t.Errorf("the error envelope is malformed: %+v", body)
			}
		})
	}

	// A user id that is present but not a UUID is equally unauthenticated rather than
	// a 500: it can only come from a forged or corrupted token.
	t.Run("a user id that is not a UUID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ics/unified-feed", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, "not-a-uuid"))

		rec := httptest.NewRecorder()
		handler.GetConfig(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}

func TestGetConfigEndpoint(t *testing.T) {
	calendarID := uuid.New()
	handler := newConfigHandler(&stubFeedRepo{
		feed:        &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "abc123"},
		calendarIDs: []uuid.UUID{calendarID},
	})

	rec := httptest.NewRecorder()
	handler.GetConfig(rec, authenticated(http.MethodGet, "/api/v1/ics/unified-feed", "", uuid.New()))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// The token is a bearer credential; caching it in a shared proxy would hand the feed
	// to whoever fetched the page next.
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store on a response carrying the feed token", cc)
	}

	body := decode(t, rec)
	if !body.Success {
		t.Errorf("success = false on a 200: %+v", body)
	}

	var config service.UnifiedFeedConfig
	if err := json.Unmarshal(body.Data, &config); err != nil {
		t.Fatalf("the payload is not a UnifiedFeedConfig: %v", err)
	}
	if !config.Configured || config.ICSToken != "abc123" {
		t.Errorf("got %+v", config)
	}
	if len(config.IncludedCalendarIDs) != 1 || config.IncludedCalendarIDs[0] != calendarID.String() {
		t.Errorf("IncludedCalendarIDs = %v, want %v", config.IncludedCalendarIDs, calendarID)
	}
}

func TestCreateEndpoint(t *testing.T) {
	t.Run("201 with the new feed", func(t *testing.T) {
		handler := newConfigHandler(&stubFeedRepo{
			getErr:  errors.New("no rows"),
			created: &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "fresh"},
		})

		rec := httptest.NewRecorder()
		handler.Create(rec, authenticated(http.MethodPost, "/api/v1/ics/unified-feed", "", uuid.New()))

		// 201 rather than 200: the client uses the status to decide whether to show the
		// URL as newly minted.
		if rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want 201", rec.Code)
		}
	})

	t.Run("409 when a feed already exists", func(t *testing.T) {
		handler := newConfigHandler(&stubFeedRepo{
			feed: &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "existing"},
		})

		rec := httptest.NewRecorder()
		handler.Create(rec, authenticated(http.MethodPost, "/api/v1/ics/unified-feed", "", uuid.New()))

		if rec.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409", rec.Code)
		}
		if body := decode(t, rec); body.Error == nil || body.Error.Code != "feed_exists" {
			t.Errorf("the error code is not feed_exists: %+v", body.Error)
		}
	})

	t.Run("500 when the insert fails", func(t *testing.T) {
		handler := newConfigHandler(&stubFeedRepo{
			getErr:    errors.New("no rows"),
			createErr: errors.New("connection refused"),
		})

		rec := httptest.NewRecorder()
		handler.Create(rec, authenticated(http.MethodPost, "/api/v1/ics/unified-feed", "", uuid.New()))

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		// The driver's message must not reach the client: it names hosts and sometimes
		// the query.
		if body := decode(t, rec); body.Error != nil && strings.Contains(body.Error.Message, "connection refused") {
			t.Errorf("the internal error leaked to the client: %q", body.Error.Message)
		}
	})
}

func TestUpdateCalendarsEndpoint(t *testing.T) {
	feed := &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "token"}
	calendarID := uuid.New()

	tests := []struct {
		name       string
		repo       *stubFeedRepo
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "204 on success",
			repo:       &stubFeedRepo{feed: feed, owned: true},
			body:       `{"calendar_ids":["` + calendarID.String() + `"]}`,
			wantStatus: http.StatusNoContent,
		},
		{
			// An empty list clears the feed, which is a legitimate operation and not a
			// missing parameter.
			name:       "204 when clearing the selection",
			repo:       &stubFeedRepo{feed: feed, owned: true},
			body:       `{"calendar_ids":[]}`,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "400 on a malformed body",
			repo:       &stubFeedRepo{feed: feed, owned: true},
			body:       `{"calendar_ids":`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			name:       "404 when the user has no feed",
			repo:       &stubFeedRepo{getErr: errors.New("no rows")},
			body:       `{"calendar_ids":[]}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			// The one that matters: adding somebody else's calendar to your own feed
			// would expose it through a URL you control, for ever.
			name:       "403 for a calendar the user does not own",
			repo:       &stubFeedRepo{feed: feed, owned: false},
			body:       `{"calendar_ids":["` + calendarID.String() + `"]}`,
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
		{
			// A malformed UUID reaches the client as 500 rather than 400. The service
			// returns a bare fmt.Errorf for it instead of a sentinel, so the handler has
			// no case to match and falls through. Pinned as the current contract.
			name:       "500 for a malformed calendar id",
			repo:       &stubFeedRepo{feed: feed, owned: true},
			body:       `{"calendar_ids":["not-a-uuid"]}`,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
		{
			name:       "500 when the write fails",
			repo:       &stubFeedRepo{feed: feed, owned: true, updateErr: errors.New("connection refused")},
			body:       `{"calendar_ids":["` + calendarID.String() + `"]}`,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newConfigHandler(tt.repo).UpdateCalendars(
				rec, authenticated(http.MethodPatch, "/api/v1/ics/unified-feed/calendars", tt.body, uuid.New()))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode == "" {
				// 204 carries no body at all.
				if rec.Body.Len() != 0 {
					t.Errorf("a %d response carried a body: %q", rec.Code, rec.Body.String())
				}

				return
			}
			if body := decode(t, rec); body.Error == nil || body.Error.Code != tt.wantCode {
				t.Errorf("error code = %+v, want %q", body.Error, tt.wantCode)
			}
		})
	}
}

func TestRegenerateTokenEndpoint(t *testing.T) {
	t.Run("200 with the new token", func(t *testing.T) {
		handler := newConfigHandler(&stubFeedRepo{
			feed:     &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "old"},
			newToken: "new-token",
		})

		rec := httptest.NewRecorder()
		handler.RegenerateToken(rec, authenticated(http.MethodPost, "/api/v1/ics/unified-feed/regenerate-token", "", uuid.New()))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var payload map[string]string
		if err := json.Unmarshal(decode(t, rec).Data, &payload); err != nil {
			t.Fatalf("the payload is not a string map: %v", err)
		}
		// The frontend reads this key to rebuild the subscription URL; renaming it
		// breaks the settings page with no error anywhere.
		if payload["ics_token"] != "new-token" {
			t.Errorf("payload = %v, want ics_token=new-token", payload)
		}
	})

	t.Run("404 when the user has no feed", func(t *testing.T) {
		handler := newConfigHandler(&stubFeedRepo{getErr: errors.New("no rows")})

		rec := httptest.NewRecorder()
		handler.RegenerateToken(rec, authenticated(http.MethodPost, "/api/v1/ics/unified-feed/regenerate-token", "", uuid.New()))

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("500 when the rotation fails", func(t *testing.T) {
		handler := newConfigHandler(&stubFeedRepo{
			feed:     &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "old"},
			tokenErr: errors.New("connection refused"),
		})

		rec := httptest.NewRecorder()
		handler.RegenerateToken(rec, authenticated(http.MethodPost, "/api/v1/ics/unified-feed/regenerate-token", "", uuid.New()))

		// A failed rotation must report failure: telling the user the URL was revoked
		// when it was not is the worst possible answer here.
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
	})
}
