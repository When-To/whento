// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/middleware"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/notify/models"
)

// This endpoint is the only place a user can hand the server a URL and have it make an
// outbound request later. That makes the webhook validation an SSRF boundary rather than
// a formatting nicety: a webhook pointed at 169.254.169.254 would have the server fetch
// cloud instance credentials and post them wherever the notification goes.

type stubCalendarStore struct {
	calendar *calendarModels.Calendar
	getErr   error

	savedConfig  string
	savedEnabled bool
	saveErr      error
	saveCalls    int
}

func (s *stubCalendarStore) GetByID(context.Context, uuid.UUID) (*calendarModels.Calendar, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}

	return s.calendar, nil
}

func (s *stubCalendarStore) UpdateNotifyConfig(_ context.Context, _ uuid.UUID, notifyConfig string, enabled bool) error {
	s.saveCalls++
	s.savedConfig = notifyConfig
	s.savedEnabled = enabled

	return s.saveErr
}

func newConfigHandler(store *stubCalendarStore) *NotifyConfigHandler {
	return NewNotifyConfigHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// request builds a request carrying the chi URL parameter and the authenticated user,
// the way the router and the Auth middleware leave them.
func request(method, body, calendarID string, userID uuid.UUID) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, "/api/v1/calendars/"+calendarID+"/notify-config", nil)
	} else {
		req = httptest.NewRequest(method, "/api/v1/calendars/"+calendarID+"/notify-config", strings.NewReader(body))
	}

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", calendarID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	if userID != uuid.Nil {
		ctx = context.WithValue(ctx, middleware.UserIDKey, userID.String())
	}

	return req.WithContext(ctx)
}

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

func ownedCalendar(ownerID uuid.UUID, notifyConfig *string) *calendarModels.Calendar {
	calendar := &calendarModels.Calendar{OwnerID: ownerID, Name: "Team", NotifyConfig: notifyConfig}
	calendar.ID = uuid.New()

	return calendar
}

func TestGetConfigDefaultsWhenNothingIsStored(t *testing.T) {
	owner := uuid.New()
	store := &stubCalendarStore{calendar: ownedCalendar(owner, nil)}

	rec := httptest.NewRecorder()
	newConfigHandler(store).GetConfig(rec, request(http.MethodGet, "", store.calendar.ID.String(), owner))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	var payload models.NotifyConfigResponse
	if err := json.Unmarshal(decode(t, rec).Data, &payload); err != nil {
		t.Fatalf("the payload is not a NotifyConfigResponse: %v", err)
	}

	// A calendar that has never been configured must come back with notifications off.
	// Defaulting the other way would start mailing participants who never opted in.
	if payload.Config.Enabled {
		t.Error("notifications default to enabled")
	}
	if payload.Config.NotifyParticipants {
		t.Error("participants are notified by default")
	}
	// The owner is the one exception: they asked for the calendar, and email is the only
	// channel that needs no further configuration.
	if !payload.Config.NotifyOwner || !payload.Config.Channels.Email.Enabled {
		t.Errorf("the owner-email default was lost: %+v", payload.Config)
	}
	if payload.Config.Reminders.HoursBefore != 24 {
		t.Errorf("Reminders.HoursBefore = %d, want 24", payload.Config.Reminders.HoursBefore)
	}
}

func TestGetConfigReturnsWhatWasStored(t *testing.T) {
	owner := uuid.New()
	stored := `{"enabled":true,"notify_owner":false,"notify_participants":true,` +
		`"channels":{"email":{"enabled":false},"discord":{"enabled":true,"webhook_url":"https://discord.com/api/webhooks/1/abc"},` +
		`"slack":{"enabled":false},"telegram":{"enabled":false}},` +
		`"reminders":{"enabled":true,"hours_before":48}}`
	store := &stubCalendarStore{calendar: ownedCalendar(owner, &stored)}

	rec := httptest.NewRecorder()
	newConfigHandler(store).GetConfig(rec, request(http.MethodGet, "", store.calendar.ID.String(), owner))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	var payload models.NotifyConfigResponse
	if err := json.Unmarshal(decode(t, rec).Data, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !payload.Config.Enabled || !payload.Config.NotifyParticipants || payload.Config.NotifyOwner {
		t.Errorf("the stored flags did not survive: %+v", payload.Config)
	}
	if payload.Config.Reminders.HoursBefore != 48 {
		t.Errorf("HoursBefore = %d, want 48", payload.Config.Reminders.HoursBefore)
	}
}

func TestConfigAccessControl(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	calendar := ownedCalendar(owner, nil)
	body := `{"config":{"enabled":false,"notify_owner":true,"notify_participants":false,` +
		`"channels":{"email":{"enabled":true},"discord":{"enabled":false},"slack":{"enabled":false},"telegram":{"enabled":false}},` +
		`"reminders":{"enabled":false,"hours_before":24}}}`

	tests := []struct {
		name       string
		store      *stubCalendarStore
		calendarID string
		userID     uuid.UUID
		wantStatus int
	}{
		{
			name:       "the owner is allowed",
			store:      &stubCalendarStore{calendar: calendar},
			calendarID: calendar.ID.String(), userID: owner, wantStatus: http.StatusOK,
		},
		{
			// The notification config holds webhook URLs and bot tokens, so reading it is
			// as sensitive as writing it.
			name:       "somebody else is refused",
			store:      &stubCalendarStore{calendar: calendar},
			calendarID: calendar.ID.String(), userID: stranger, wantStatus: http.StatusForbidden,
		},
		{
			// No user in the context parses to the nil UUID, which cannot own anything,
			// so this comes out as 403 rather than 401. The router puts the endpoint
			// behind Auth, so it is unreachable in practice; pinned as the contract.
			name:       "an unauthenticated request",
			store:      &stubCalendarStore{calendar: calendar},
			calendarID: calendar.ID.String(), userID: uuid.Nil, wantStatus: http.StatusForbidden,
		},
		{
			name:       "a calendar id that is not a UUID",
			store:      &stubCalendarStore{calendar: calendar},
			calendarID: "not-a-uuid", userID: owner, wantStatus: http.StatusBadRequest,
		},
		{
			name:       "a calendar that does not exist",
			store:      &stubCalendarStore{getErr: errors.New("no rows")},
			calendarID: uuid.New().String(), userID: owner, wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" (read)", func(t *testing.T) {
			rec := httptest.NewRecorder()
			newConfigHandler(tt.store).GetConfig(rec, request(http.MethodGet, "", tt.calendarID, tt.userID))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})

		t.Run(tt.name+" (write)", func(t *testing.T) {
			store := *tt.store
			rec := httptest.NewRecorder()
			newConfigHandler(&store).UpdateConfig(rec, request(http.MethodPatch, body, tt.calendarID, tt.userID))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			// And nothing is written unless the request was allowed.
			if tt.wantStatus != http.StatusOK && store.saveCalls != 0 {
				t.Error("the configuration was written despite the request being refused")
			}
		})
	}
}

// TestWebhookURLsAreValidated is the SSRF boundary. Every URL here would otherwise
// become an outbound request from inside the deployment's network.
func TestWebhookURLsAreValidated(t *testing.T) {
	owner := uuid.New()
	calendar := ownedCalendar(owner, nil)

	withDiscord := func(url string) string {
		return `{"config":{"enabled":true,"notify_owner":true,"notify_participants":false,` +
			`"channels":{"email":{"enabled":true},"discord":{"enabled":true,"webhook_url":"` + url + `"},` +
			`"slack":{"enabled":false},"telegram":{"enabled":false}},` +
			`"reminders":{"enabled":false,"hours_before":24}}}`
	}

	tests := []struct {
		name    string
		url     string
		wantOK  bool
		comment string
	}{
		{name: "a genuine Discord webhook", url: "https://discord.com/api/webhooks/123/abcdef", wantOK: true},
		{name: "the link-local metadata endpoint", url: "http://169.254.169.254/latest/meta-data/"},
		{name: "loopback", url: "http://127.0.0.1:8080/internal"},
		{name: "a private range", url: "http://10.0.0.1/admin"},
		{name: "another host entirely", url: "https://evil.example/webhook"},
		{name: "a host that merely ends in the right name", url: "https://discord.com.evil.example/api/webhooks/1/a"},
		{name: "the file scheme", url: "file:///etc/passwd"},
		{name: "not a URL at all", url: "not-a-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubCalendarStore{calendar: calendar}
			rec := httptest.NewRecorder()
			newConfigHandler(store).UpdateConfig(rec, request(http.MethodPatch, withDiscord(tt.url), calendar.ID.String(), owner))

			if tt.wantOK {
				if rec.Code != http.StatusOK {
					t.Errorf("a legitimate webhook was refused: %d (%q)", rec.Code, rec.Body.String())
				}

				return
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 for %q", rec.Code, tt.url)
			}
			// Refused before the write: a stored URL would be used by the notification
			// worker later, long after the request that set it.
			if store.saveCalls != 0 {
				t.Errorf("the webhook %q was stored despite failing validation", tt.url)
			}
		})
	}
}

// TestDisabledChannelsAreStillValidated matters because the UI lets a user paste a URL
// and leave the channel off. Skipping validation there would let the value sit in the
// database until somebody flips the switch, with no check at that point.
func TestDisabledChannelsAreStillValidated(t *testing.T) {
	owner := uuid.New()
	calendar := ownedCalendar(owner, nil)
	store := &stubCalendarStore{calendar: calendar}

	body := `{"config":{"enabled":false,"notify_owner":true,"notify_participants":false,` +
		`"channels":{"email":{"enabled":true},` +
		`"discord":{"enabled":false,"webhook_url":"http://169.254.169.254/"},` +
		`"slack":{"enabled":false},"telegram":{"enabled":false}},` +
		`"reminders":{"enabled":false,"hours_before":24}}}`

	rec := httptest.NewRecorder()
	newConfigHandler(store).UpdateConfig(rec, request(http.MethodPatch, body, calendar.ID.String(), owner))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — a disabled channel's URL is validated too", rec.Code)
	}
	if store.saveCalls != 0 {
		t.Error("the URL was stored because the channel was disabled")
	}
}

func TestUpdateConfigRejectsMalformedBodies(t *testing.T) {
	owner := uuid.New()
	calendar := ownedCalendar(owner, nil)

	for _, body := range []string{`{"config":`, `not json at all`, ``} {
		store := &stubCalendarStore{calendar: calendar}
		rec := httptest.NewRecorder()
		newConfigHandler(store).UpdateConfig(rec, request(http.MethodPatch, body, calendar.ID.String(), owner))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q gave status %d, want 400", body, rec.Code)
		}
	}
}

func TestUpdateConfigPersistsAndMirrorsTheFlag(t *testing.T) {
	owner := uuid.New()
	calendar := ownedCalendar(owner, nil)
	store := &stubCalendarStore{calendar: calendar}

	body := `{"config":{"enabled":true,"notify_owner":true,"notify_participants":true,` +
		`"channels":{"email":{"enabled":true},"discord":{"enabled":false},"slack":{"enabled":false},"telegram":{"enabled":false}},` +
		`"reminders":{"enabled":true,"hours_before":12}}}`

	rec := httptest.NewRecorder()
	newConfigHandler(store).UpdateConfig(rec, request(http.MethodPatch, body, calendar.ID.String(), owner))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	// notify_on_threshold is a separate column the notification worker reads to decide
	// whether to look at this calendar at all. If it fell out of step with the JSON, a
	// calendar could be configured for notifications that never fire.
	if !store.savedEnabled {
		t.Error("notify_on_threshold was not set alongside an enabled configuration")
	}

	var saved models.NotifyConfig
	if err := json.Unmarshal([]byte(store.savedConfig), &saved); err != nil {
		t.Fatalf("what was stored is not valid JSON: %v (%q)", err, store.savedConfig)
	}
	if !saved.Enabled || saved.Reminders.HoursBefore != 12 {
		t.Errorf("the stored configuration is %+v", saved)
	}

	// The response echoes what was saved, so the settings page does not need a re-fetch.
	var payload models.NotifyConfigResponse
	if err := json.Unmarshal(decode(t, rec).Data, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Config.Reminders.HoursBefore != 12 {
		t.Errorf("the response does not echo the saved configuration: %+v", payload.Config)
	}
}

func TestUpdateConfigReportsAFailedWrite(t *testing.T) {
	owner := uuid.New()
	calendar := ownedCalendar(owner, nil)
	store := &stubCalendarStore{calendar: calendar, saveErr: errors.New("connection refused")}

	body := `{"config":{"enabled":true,"notify_owner":true,"notify_participants":false,` +
		`"channels":{"email":{"enabled":true},"discord":{"enabled":false},"slack":{"enabled":false},"telegram":{"enabled":false}},` +
		`"reminders":{"enabled":false,"hours_before":24}}}`

	rec := httptest.NewRecorder()
	newConfigHandler(store).UpdateConfig(rec, request(http.MethodPatch, body, calendar.ID.String(), owner))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	// Reporting success on a failed write would leave the user believing notifications
	// are on when nothing was saved.
	if body := decode(t, rec); body.Success {
		t.Error("a failed write was reported as success")
	}
}
