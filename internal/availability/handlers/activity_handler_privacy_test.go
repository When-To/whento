// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/availability/handlers"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
	"github.com/whento/whento/internal/availability/service"
	calendarModels "github.com/whento/whento/internal/calendar/models"
)

// The activity journal answers a question about everybody on a calendar — who joined,
// who is available, who walked away — and it answers it by name. The only thing between
// that and anyone holding a public link is that this route is authenticated and
// owner-checked, so those two properties are what the first two tests pin.
//
// The third pins a decision rather than a defect: lock_participants does NOT hide names
// here. It hides participants from each other in the public view; the owner reads every
// one of those names on the settings page this endpoint feeds.

const activityDate = "2026-03-05"

// activityCalendarStore is the one row the ownership check reads.
type activityCalendarStore struct {
	calendar *calendarModels.Calendar
}

func (s *activityCalendarStore) GetByID(context.Context, uuid.UUID) (*calendarModels.Calendar, error) {
	return s.calendar, nil
}

// activityFixture is a calendar owned by owner, with Alice available on activityDate and
// a journal saying Alice joined and Bob withdrew.
type activityFixture struct {
	handler    *handlers.ActivityHandler
	calendarID uuid.UUID
}

func newActivityFixture(t *testing.T, owner uuid.UUID, locked bool) activityFixture {
	t.Helper()

	calendarID := uuid.New()
	alice := &repository.Participant{ID: uuid.New(), CalendarID: calendarID, Name: "Alice"}
	bob := &repository.Participant{ID: uuid.New(), CalendarID: calendarID, Name: "Bob"}

	date, err := time.Parse("2006-01-02", activityDate)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	at := date.Add(-24 * time.Hour)

	availabilityService := service.NewAvailabilityService(
		&stubAvailabilityRepo{
			dateStats: map[string]models.DateStats{
				activityDate: {
					Available: []models.AvailableParticipant{{ID: alice.ID, Name: alice.Name}},
					Count:     1,
				},
			},
		},
		&stubCalendarRepo{calendar: &repository.Calendar{ID: calendarID, Threshold: 2}},
		&stubParticipantRepo{participants: []*repository.Participant{alice, bob}},
		&stubRecurrenceRepo{},
		&stubActivityLog{
			journal: map[string]*models.DateActivity{
				activityDate: {
					Date:          date,
					LastJoined:    &models.ParticipantRef{ID: alice.ID, Name: alice.Name, At: at},
					LastWithdrawn: &models.ParticipantRef{ID: bob.ID, Name: bob.Name, At: at},
				},
			},
		},
		&stubNotifyService{},
		cache.NewRedisCache(nil),
	)

	calendar := &calendarModels.Calendar{
		OwnerID:          owner,
		Threshold:        2,
		LockParticipants: locked,
	}
	calendar.ID = calendarID

	return activityFixture{
		handler:    handlers.NewActivityHandler(&activityCalendarStore{calendar: calendar}, availabilityService),
		calendarID: calendarID,
	}
}

func activityURL(calendarID uuid.UUID) string {
	return "/api/v1/calendars/" + calendarID.String() + "/activity?start=2026-03-01&end=2026-03-31"
}

// as issues the request the way the router and the Auth middleware would leave it for an
// authenticated user: the chi URL parameter bound, and the user id in the context.
func (f activityFixture) as(userID uuid.UUID) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, activityURL(f.calendarID), nil)

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", f.calendarID.String())
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID.String())

	recorder := httptest.NewRecorder()
	f.handler.GetActivity(recorder, req.WithContext(ctx))

	return recorder
}

func TestGetActivityRequiresAuthentication(t *testing.T) {
	f := newActivityFixture(t, uuid.New(), false)

	// Mounted behind the same middleware as the real route. A nil manager and nil cache
	// are safe: Auth answers a request with no Authorization header before it touches
	// either, and that is the only path this test drives.
	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(middleware.Auth(nil, nil))
		r.Get("/api/v1/calendars/{id}/activity", f.handler.GetActivity)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, activityURL(f.calendarID), nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if strings.Contains(recorder.Body.String(), "Alice") {
		t.Error("the unauthenticated response names a participant")
	}
}

func TestGetActivityRefusesANonOwner(t *testing.T) {
	f := newActivityFixture(t, uuid.New(), false)

	recorder := f.as(uuid.New())

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	// Authenticated is not authorised: a logged-in user with any account must not read
	// another owner's calendar by guessing its id.
	body := recorder.Body.String()
	for _, name := range []string{"Alice", "Bob"} {
		if strings.Contains(body, name) {
			t.Errorf("the 403 response names %q", name)
		}
	}
}

func TestGetActivityShowsNamesToTheOwnerDespiteLockParticipants(t *testing.T) {
	owner := uuid.New()
	f := newActivityFixture(t, owner, true)

	recorder := f.as(owner)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	// Asserting on the raw body, because that is what actually leaves the handler —
	// the same reason date_summary_privacy_test.go does.
	body := recorder.Body.String()
	for _, want := range []string{"Alice", "Bob", "last_joined", "last_withdrawn", activityDate} {
		if !strings.Contains(body, want) {
			t.Errorf("the owner's response is missing %q: %s", want, body)
		}
	}
}
