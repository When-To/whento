// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/whento/internal/availability/handlers"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
	"github.com/whento/whento/internal/availability/service"
)

// This file replaces a placeholder that did nothing but t.Skip("Service-level tests
// provide coverage") — while the service it deferred to sat at 6.7%. The handler layer
// owns the HTTP contract: which query parameters are required, and which service error
// becomes which status code. None of that is visible from a service test.

// The mocks below satisfy the exported repository interfaces, so a real service can be
// assembled and driven through the real handler.

type stubAvailabilityRepo struct {
	inRange []*models.Availability
}

var _ service.AvailabilityRepository = (*stubAvailabilityRepo)(nil)

func (s *stubAvailabilityRepo) Create(context.Context, *models.Availability) error { return nil }

func (s *stubAvailabilityRepo) GetByParticipantID(context.Context, uuid.UUID) ([]*models.Availability, error) {
	return nil, nil
}

func (s *stubAvailabilityRepo) GetByParticipantIDWithDateRange(
	context.Context, uuid.UUID, *time.Time, *time.Time,
) ([]*models.Availability, error) {
	return nil, nil
}

func (s *stubAvailabilityRepo) GetByParticipantAndDate(
	context.Context, uuid.UUID, time.Time,
) (*models.Availability, error) {
	return nil, nil
}

func (s *stubAvailabilityRepo) GetByDate(context.Context, uuid.UUID, time.Time) ([]*models.Availability, error) {
	return nil, nil
}

func (s *stubAvailabilityRepo) GetByCalendarDateRange(
	context.Context, uuid.UUID, time.Time, time.Time,
) ([]*models.Availability, error) {
	return s.inRange, nil
}

func (s *stubAvailabilityRepo) GetParticipantCountForDate(context.Context, uuid.UUID, time.Time) (int, error) {
	return 0, nil
}

func (s *stubAvailabilityRepo) Update(context.Context, *models.Availability) error { return nil }

func (s *stubAvailabilityRepo) Delete(context.Context, uuid.UUID, time.Time) error { return nil }

type stubCalendarRepo struct {
	calendar *repository.Calendar
	err      error
}

var _ service.CalendarRepository = (*stubCalendarRepo)(nil)

func (s *stubCalendarRepo) GetByPublicToken(context.Context, string) (uuid.UUID, error) {
	if s.err != nil {
		return uuid.Nil, s.err
	}

	return s.calendar.ID, nil
}

func (s *stubCalendarRepo) GetCalendarInfoByPublicToken(context.Context, string) (*repository.Calendar, error) {
	return s.calendar, s.err
}

type stubParticipantRepo struct {
	participants []*repository.Participant
}

var _ service.ParticipantRepository = (*stubParticipantRepo)(nil)

func (s *stubParticipantRepo) GetByID(_ context.Context, id uuid.UUID) (*repository.Participant, error) {
	for _, p := range s.participants {
		if p.ID == id {
			return p, nil
		}
	}

	return nil, repository.ErrParticipantNotFound
}

func (s *stubParticipantRepo) GetByCalendarID(context.Context, uuid.UUID) ([]*repository.Participant, error) {
	return s.participants, nil
}

type stubRecurrenceRepo struct{}

var _ service.RecurrenceRepository = (*stubRecurrenceRepo)(nil)

func (s *stubRecurrenceRepo) CreateRecurrence(context.Context, *models.Recurrence) error { return nil }

func (s *stubRecurrenceRepo) GetRecurrencesByParticipant(context.Context, uuid.UUID) ([]models.Recurrence, error) {
	return nil, nil
}

func (s *stubRecurrenceRepo) GetRecurrencesByCalendar(context.Context, uuid.UUID) ([]models.Recurrence, error) {
	return nil, nil
}

func (s *stubRecurrenceRepo) GetRecurrenceByID(context.Context, uuid.UUID) (*models.Recurrence, error) {
	return nil, nil
}

func (s *stubRecurrenceRepo) UpdateRecurrence(context.Context, *models.Recurrence) error { return nil }

func (s *stubRecurrenceRepo) DeleteRecurrence(context.Context, uuid.UUID) error { return nil }

func (s *stubRecurrenceRepo) CreateException(context.Context, *models.RecurrenceException) error {
	return nil
}

func (s *stubRecurrenceRepo) GetExceptionsByRecurrence(
	context.Context, uuid.UUID,
) ([]models.RecurrenceException, error) {
	return nil, nil
}

func (s *stubRecurrenceRepo) DeleteException(context.Context, uuid.UUID, string) error { return nil }

type stubNotifyService struct{}

var _ service.NotifyService = (*stubNotifyService)(nil)

func (s *stubNotifyService) CheckThresholdAndNotify(context.Context, uuid.UUID, time.Time, int) error {
	return nil
}

// newHandler assembles the real service and handler over the stubs above, and returns a
// chi router so that chi.URLParam("token") resolves the way it does in production.
func newHandler(t *testing.T, calendar *repository.Calendar, calendarErr error, availabilities []*models.Availability, participants []*repository.Participant) http.Handler {
	t.Helper()

	availabilityService := service.NewAvailabilityService(
		&stubAvailabilityRepo{inRange: availabilities},
		&stubCalendarRepo{calendar: calendar, err: calendarErr},
		&stubParticipantRepo{participants: participants},
		&stubRecurrenceRepo{},
		&stubNotifyService{},
		cache.NewRedisCache(nil),
	)

	handler := handlers.NewAvailabilityHandler(availabilityService)

	router := chi.NewRouter()
	router.Get("/api/v1/public/calendars/{token}/availabilities/range", handler.GetRangeSummary)

	return router
}

func TestGetRangeSummaryRequiresBothDates(t *testing.T) {
	calendarID := uuid.New()
	calendar := &repository.Calendar{ID: calendarID, AllowedWeekdays: []int{0, 1, 2, 3, 4, 5, 6}, Timezone: "Europe/Paris"}

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "both dates present", query: "?start=2026-03-01&end=2026-03-31", want: http.StatusOK},
		{name: "no dates at all", query: "", want: http.StatusBadRequest},
		{name: "only a start", query: "?start=2026-03-01", want: http.StatusBadRequest},
		{name: "only an end", query: "?end=2026-03-31", want: http.StatusBadRequest},
		{name: "an empty start", query: "?start=&end=2026-03-31", want: http.StatusBadRequest},
		// A malformed date reaches the service and comes back as a 400 too, but by
		// a different route — worth distinguishing from the missing-parameter case.
		{name: "a malformed start", query: "?start=01/03/2026&end=2026-03-31", want: http.StatusBadRequest},
		{name: "an inverted range", query: "?start=2026-03-31&end=2026-03-01", want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newHandler(t, calendar, nil, nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/public/calendars/tok/availabilities/range"+tt.query, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestGetRangeSummaryUnknownCalendarIsNotFound(t *testing.T) {
	router := newHandler(t, nil, repository.ErrCalendarNotFound, nil, nil)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/public/calendars/nope/availabilities/range?start=2026-03-01&end=2026-03-31",
		nil,
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (body %s)", rec.Code, http.StatusNotFound, rec.Body)
	}
}

// TestGetRangeSummaryResponseShape pins the envelope the frontend unwraps. apiClient
// reads response.data.data, so a summary that arrived at the top level would break
// every calendar view without any status code changing.
func TestGetRangeSummaryResponseShape(t *testing.T) {
	calendarID := uuid.New()
	alice := &repository.Participant{ID: uuid.New(), CalendarID: calendarID, Name: "Alice"}

	calendar := &repository.Calendar{
		ID:              calendarID,
		Threshold:       1,
		AllowedWeekdays: []int{0, 1, 2, 3, 4, 5, 6},
		Timezone:        "Europe/Paris",
	}

	date, err := time.Parse("2006-01-02", "2026-03-05")
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}

	start, end := "09:00", "17:00"
	availability := &models.Availability{
		ParticipantID: alice.ID,
		Date:          date,
		StartTime:     &start,
		EndTime:       &end,
	}
	availability.ID = uuid.New()

	router := newHandler(t, calendar, nil, []*models.Availability{availability}, []*repository.Participant{alice})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/public/calendars/tok/availabilities/range?start=2026-03-01&end=2026-03-31",
		nil,
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}

	var body struct {
		Data []struct {
			Date         string `json:"date"`
			TotalCount   int    `json:"total_count"`
			Participants []struct {
				ParticipantName string  `json:"participant_name"`
				StartTime       *string `json:"start_time"`
			} `json:"participants"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, rec.Body)
	}

	if len(body.Data) != 1 {
		t.Fatalf("got %d dates, want 1 (body %s)", len(body.Data), rec.Body)
	}
	if body.Data[0].Date != "2026-03-05" {
		t.Errorf("date = %q, want %q", body.Data[0].Date, "2026-03-05")
	}
	if body.Data[0].TotalCount != 1 {
		t.Errorf("total_count = %d, want 1", body.Data[0].TotalCount)
	}
	if len(body.Data[0].Participants) != 1 || body.Data[0].Participants[0].ParticipantName != "Alice" {
		t.Errorf("participants = %+v, want Alice alone", body.Data[0].Participants)
	}
	if body.Data[0].Participants[0].StartTime == nil || *body.Data[0].Participants[0].StartTime != "09:00" {
		t.Error("the participant's start time did not survive the response")
	}
}

// TestGetRangeSummaryEmptySerialisesAsNull pins a wart rather than a virtue.
//
// GetRangeSummary accumulates into a nil slice, so a calendar with nothing in range
// serialises as `null`, not `[]`. ParticipantView already compensates —
// `Array.isArray(summaries) ? summaries : []`, commented "handle null/undefined
// responses" — so nothing is broken today, but the guard exists because of this.
//
// Left as-is deliberately: changing it alters an API response shape, which is a
// decision to take on its own terms rather than as a side effect of adding tests. If it
// is ever changed, this test fails and points at the frontend guard that can then go.
func TestGetRangeSummaryEmptySerialisesAsNull(t *testing.T) {
	calendar := &repository.Calendar{
		ID:              uuid.New(),
		AllowedWeekdays: []int{0, 1, 2, 3, 4, 5, 6},
		Timezone:        "Europe/Paris",
	}

	router := newHandler(t, calendar, nil, nil, nil)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/public/calendars/tok/availabilities/range?start=2026-03-01&end=2026-03-31",
		nil,
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if string(body.Data) != "null" {
		t.Errorf(
			"data = %s, want null — if this now serialises as [], the ParticipantView "+
				"Array.isArray guard is no longer needed and this test should be updated",
			body.Data,
		)
	}
}
