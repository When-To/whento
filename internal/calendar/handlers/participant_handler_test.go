// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/calendar/handlers"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/service"
	"github.com/whento/whento/internal/testutil"
)

func participantFixture() *models.Participant {
	p := &models.Participant{CalendarID: uuid.New(), Name: "Ada", Locale: "en"}
	p.ID = uuid.New()
	return p
}

// TestParticipantHandlerErrorTranslation pins the mapping from the service's
// sentinel errors to status codes for all four endpoints. Every one of these was
// unreachable while the handler held the concrete service.
func TestParticipantHandlerErrorTranslation(t *testing.T) {
	tests := []struct {
		name       string
		invoke     func(*handlers.ParticipantHandler, http.ResponseWriter, *http.Request)
		method     string
		body       any
		authed     bool
		err        error
		wantStatus int
	}{
		{
			name:       "AddParticipant without a session",
			invoke:     (*handlers.ParticipantHandler).AddParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "AddParticipant calendar not found",
			invoke:     (*handlers.ParticipantHandler).AddParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "AddParticipant forbidden",
			invoke:     (*handlers.ParticipantHandler).AddParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        service.ErrUnauthorized,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "AddParticipant duplicate name",
			invoke:     (*handlers.ParticipantHandler).AddParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        service.ErrParticipantExists,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "AddParticipant unknown failure",
			invoke:     (*handlers.ParticipantHandler).AddParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "AddParticipant success",
			invoke:     (*handlers.ParticipantHandler).AddParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "AddAnonymousParticipant calendar not found",
			invoke:     (*handlers.ParticipantHandler).AddAnonymousParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "AddAnonymousParticipant not allowed on this calendar",
			invoke:     (*handlers.ParticipantHandler).AddAnonymousParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			err:        service.ErrUnauthorized,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "AddAnonymousParticipant duplicate name",
			invoke:     (*handlers.ParticipantHandler).AddAnonymousParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			err:        service.ErrParticipantExists,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "AddAnonymousParticipant unknown failure",
			invoke:     (*handlers.ParticipantHandler).AddAnonymousParticipant,
			method:     http.MethodPost,
			body:       map[string]any{"name": "Ada"},
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "UpdateParticipant without a session",
			invoke:     (*handlers.ParticipantHandler).UpdateParticipant,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "Ada"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "UpdateParticipant calendar not found",
			invoke:     (*handlers.ParticipantHandler).UpdateParticipant,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "UpdateParticipant participant not found",
			invoke:     (*handlers.ParticipantHandler).UpdateParticipant,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        service.ErrParticipantNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "UpdateParticipant forbidden",
			invoke:     (*handlers.ParticipantHandler).UpdateParticipant,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        service.ErrUnauthorized,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "UpdateParticipant duplicate name",
			invoke:     (*handlers.ParticipantHandler).UpdateParticipant,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        service.ErrParticipantExists,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "UpdateParticipant unknown failure",
			invoke:     (*handlers.ParticipantHandler).UpdateParticipant,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "UpdateParticipant success",
			invoke:     (*handlers.ParticipantHandler).UpdateParticipant,
			method:     http.MethodPatch,
			body:       map[string]any{"name": "Ada"},
			authed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "RemoveParticipant without a session",
			invoke:     (*handlers.ParticipantHandler).RemoveParticipant,
			method:     http.MethodDelete,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "RemoveParticipant calendar not found",
			invoke:     (*handlers.ParticipantHandler).RemoveParticipant,
			method:     http.MethodDelete,
			authed:     true,
			err:        service.ErrCalendarNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "RemoveParticipant participant not found",
			invoke:     (*handlers.ParticipantHandler).RemoveParticipant,
			method:     http.MethodDelete,
			authed:     true,
			err:        service.ErrParticipantNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "RemoveParticipant forbidden",
			invoke:     (*handlers.ParticipantHandler).RemoveParticipant,
			method:     http.MethodDelete,
			authed:     true,
			err:        service.ErrUnauthorized,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "RemoveParticipant unknown failure",
			invoke:     (*handlers.ParticipantHandler).RemoveParticipant,
			method:     http.MethodDelete,
			authed:     true,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "RemoveParticipant success",
			invoke:     (*handlers.ParticipantHandler).RemoveParticipant,
			method:     http.MethodDelete,
			authed:     true,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubCalendarService{partic: participantFixture(), err: tt.err}
			handler := handlers.NewParticipantHandler(svc)

			var req *http.Request
			if tt.body != nil {
				req = testutil.MakeJSONRequest(tt.method, "/api/v1/calendars/x/participants", tt.body)
			} else {
				req = testutil.MakeRequest(tt.method, "/api/v1/calendars/x/participants")
			}
			req = testutil.WithURLParams(req, map[string]string{
				"id": uuid.New().String(), "pid": uuid.New().String(), "token": "public-token",
			})
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

// TestParticipantHandlerRejectsMalformedBodies covers decode and validation.
func TestParticipantHandlerRejectsMalformedBodies(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*handlers.ParticipantHandler, http.ResponseWriter, *http.Request)
		body   string
	}{
		{name: "AddParticipant, malformed JSON", invoke: (*handlers.ParticipantHandler).AddParticipant, body: "{"},
		{name: "AddParticipant, empty name", invoke: (*handlers.ParticipantHandler).AddParticipant, body: `{"name":""}`},
		{name: "AddAnonymousParticipant, malformed JSON", invoke: (*handlers.ParticipantHandler).AddAnonymousParticipant, body: "{"},
		{name: "AddAnonymousParticipant, empty name", invoke: (*handlers.ParticipantHandler).AddAnonymousParticipant, body: `{"name":""}`},
		{name: "UpdateParticipant, malformed JSON", invoke: (*handlers.ParticipantHandler).UpdateParticipant, body: "{"},
		{name: "UpdateParticipant, empty name", invoke: (*handlers.ParticipantHandler).UpdateParticipant, body: `{"name":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handlers.NewParticipantHandler(&stubCalendarService{partic: participantFixture()})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/calendars/x/participants", strings.NewReader(tt.body))
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

// TestAddAnonymousParticipantResponseShape guards the one endpoint that builds its
// payload by hand instead of marshalling the model: an anonymous joiner needs their
// own participant id back, since it is half of what will authorise them afterwards.
func TestAddAnonymousParticipantResponseShape(t *testing.T) {
	participant := participantFixture()
	svc := &stubCalendarService{partic: participant}
	handler := handlers.NewParticipantHandler(svc)

	req := testutil.MakeJSONRequest(http.MethodPost, "/api/v1/calendars/public/tok/participants", map[string]any{"name": "Ada"})
	req = testutil.WithURLParams(req, map[string]string{"token": "public-token"})
	w := httptest.NewRecorder()

	handler.AddAnonymousParticipant(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if svc.lastToken != "public-token" {
		t.Errorf("the public token did not reach the service: %q", svc.lastToken)
	}

	var envelope struct {
		Data struct {
			ID            uuid.UUID `json:"id"`
			CalendarID    uuid.UUID `json:"calendar_id"`
			Name          string    `json:"name"`
			EmailVerified bool      `json:"email_verified"`
			Locale        string    `json:"locale"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("response is not the standard envelope: %v", err)
	}
	if envelope.Data.ID != participant.ID {
		t.Errorf("participant id = %v, want %v", envelope.Data.ID, participant.ID)
	}
	if envelope.Data.CalendarID != participant.CalendarID || envelope.Data.Name != "Ada" || envelope.Data.Locale != "en" {
		t.Errorf("unexpected payload: %+v", envelope.Data)
	}
}
