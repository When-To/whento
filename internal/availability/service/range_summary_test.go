// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
)

var errStore = errors.New("store unavailable")

// The mocks below are hand-written and deliberately narrow: only the methods the
// summary paths reach carry behaviour, the rest satisfy the interface and would fail
// loudly if a future change started calling them.

type mockAvailabilityRepo struct {
	inRange    []*models.Availability
	inRangeErr error
	onDate     []*models.Availability
	onDateErr  error
}

var _ AvailabilityRepository = (*mockAvailabilityRepo)(nil)

func (m *mockAvailabilityRepo) Create(context.Context, *models.Availability) error { return nil }

func (m *mockAvailabilityRepo) GetByParticipantID(context.Context, uuid.UUID) ([]*models.Availability, error) {
	return nil, nil
}

func (m *mockAvailabilityRepo) GetByParticipantIDWithDateRange(
	context.Context, uuid.UUID, *time.Time, *time.Time,
) ([]*models.Availability, error) {
	return nil, nil
}

func (m *mockAvailabilityRepo) GetByParticipantAndDate(
	context.Context, uuid.UUID, time.Time,
) (*models.Availability, error) {
	return nil, nil
}

func (m *mockAvailabilityRepo) GetByDate(context.Context, uuid.UUID, time.Time) ([]*models.Availability, error) {
	return m.onDate, m.onDateErr
}

func (m *mockAvailabilityRepo) GetByCalendarDateRange(
	context.Context, uuid.UUID, time.Time, time.Time,
) ([]*models.Availability, error) {
	return m.inRange, m.inRangeErr
}

func (m *mockAvailabilityRepo) GetParticipantCountForDate(context.Context, uuid.UUID, time.Time) (int, error) {
	return 0, nil
}

func (m *mockAvailabilityRepo) Update(context.Context, *models.Availability) error { return nil }

func (m *mockAvailabilityRepo) Delete(context.Context, uuid.UUID, time.Time) error { return nil }

type mockCalendarInfoRepo struct {
	calendar *repository.Calendar
	err      error
}

var _ CalendarRepository = (*mockCalendarInfoRepo)(nil)

func (m *mockCalendarInfoRepo) GetByPublicToken(context.Context, string) (uuid.UUID, error) {
	if m.err != nil {
		return uuid.Nil, m.err
	}

	return m.calendar.ID, nil
}

func (m *mockCalendarInfoRepo) GetCalendarInfoByPublicToken(context.Context, string) (*repository.Calendar, error) {
	return m.calendar, m.err
}

type mockParticipantsRepo struct {
	participants []*repository.Participant
	err          error
}

var _ ParticipantRepository = (*mockParticipantsRepo)(nil)

func (m *mockParticipantsRepo) GetByID(_ context.Context, id uuid.UUID) (*repository.Participant, error) {
	for _, p := range m.participants {
		if p.ID == id {
			return p, nil
		}
	}

	return nil, repository.ErrParticipantNotFound
}

func (m *mockParticipantsRepo) GetByCalendarID(context.Context, uuid.UUID) ([]*repository.Participant, error) {
	return m.participants, m.err
}

type mockRecurrenceRepo struct {
	byCalendar    []models.Recurrence
	byCalendarErr error
	exceptions    map[uuid.UUID][]models.RecurrenceException
	exceptionsErr error
}

var _ RecurrenceRepository = (*mockRecurrenceRepo)(nil)

func (m *mockRecurrenceRepo) CreateRecurrence(context.Context, *models.Recurrence) error { return nil }

func (m *mockRecurrenceRepo) GetRecurrencesByParticipant(context.Context, uuid.UUID) ([]models.Recurrence, error) {
	return nil, nil
}

func (m *mockRecurrenceRepo) GetRecurrencesByCalendar(context.Context, uuid.UUID) ([]models.Recurrence, error) {
	return m.byCalendar, m.byCalendarErr
}

func (m *mockRecurrenceRepo) GetRecurrenceByID(context.Context, uuid.UUID) (*models.Recurrence, error) {
	return nil, nil
}

func (m *mockRecurrenceRepo) UpdateRecurrence(context.Context, *models.Recurrence) error { return nil }

func (m *mockRecurrenceRepo) DeleteRecurrence(context.Context, uuid.UUID) error { return nil }

func (m *mockRecurrenceRepo) CreateException(context.Context, *models.RecurrenceException) error {
	return nil
}

func (m *mockRecurrenceRepo) GetExceptionsByRecurrence(
	_ context.Context, recurrenceID uuid.UUID,
) ([]models.RecurrenceException, error) {
	return m.exceptions[recurrenceID], m.exceptionsErr
}

func (m *mockRecurrenceRepo) DeleteException(context.Context, uuid.UUID, string) error { return nil }

type mockNotifyService struct{}

var _ NotifyService = (*mockNotifyService)(nil)

func (m *mockNotifyService) CheckThresholdAndNotify(context.Context, uuid.UUID, time.Time, int) error {
	return nil
}

// summaryFixture assembles a calendar, two participants and a service around them.
type summaryFixture struct {
	service     *AvailabilityService
	calendarID  uuid.UUID
	alice       *repository.Participant
	bob         *repository.Participant
	availRepo   *mockAvailabilityRepo
	recurrences *mockRecurrenceRepo
}

func newSummaryFixture(t *testing.T, configure func(*repository.Calendar)) *summaryFixture {
	t.Helper()

	calendarID := uuid.New()

	alice := &repository.Participant{ID: uuid.New(), CalendarID: calendarID, Name: "Alice"}
	bob := &repository.Participant{ID: uuid.New(), CalendarID: calendarID, Name: "Bob"}

	calendar := &repository.Calendar{
		ID:              calendarID,
		Threshold:       2,
		AllowedWeekdays: []int{0, 1, 2, 3, 4, 5, 6},
		Timezone:        "Europe/Paris",
		HolidaysPolicy:  "ignore",
	}
	if configure != nil {
		configure(calendar)
	}

	availRepo := &mockAvailabilityRepo{}
	recurrenceRepo := &mockRecurrenceRepo{exceptions: map[uuid.UUID][]models.RecurrenceException{}}

	return &summaryFixture{
		service: NewAvailabilityService(
			availRepo,
			&mockCalendarInfoRepo{calendar: calendar},
			&mockParticipantsRepo{participants: []*repository.Participant{alice, bob}},
			recurrenceRepo,
			&mockNotifyService{},
			cache.NewRedisCache(nil),
		),
		calendarID:  calendarID,
		alice:       alice,
		bob:         bob,
		availRepo:   availRepo,
		recurrences: recurrenceRepo,
	}
}

func availabilityOn(t *testing.T, participantID uuid.UUID, iso string, start, end *string) *models.Availability {
	t.Helper()

	availability := &models.Availability{
		ParticipantID: participantID,
		Date:          mustDate(t, iso),
		StartTime:     start,
		EndTime:       end,
	}
	availability.ID = uuid.New()

	return availability
}

func recurrenceOn(participantID uuid.UUID, dayOfWeek int, startDate string, endDate *string) models.Recurrence {
	recurrence := models.Recurrence{
		ParticipantID: participantID,
		DayOfWeek:     dayOfWeek,
		StartDate:     startDate,
		EndDate:       endDate,
	}
	recurrence.ID = uuid.New()

	return recurrence
}

// byDate makes the assertions independent of map iteration order, which GetRangeSummary
// does not control.
func byDate(summaries []models.PublicDateAvailabilitySummary) map[string]models.PublicDateAvailabilitySummary {
	out := make(map[string]models.PublicDateAvailabilitySummary, len(summaries))
	for _, s := range summaries {
		out[s.Date] = s
	}

	return out
}

func dates(summaries []models.PublicDateAvailabilitySummary) []string {
	out := make([]string, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, s.Date)
	}
	sort.Strings(out)

	return out
}

// TestGetRangeSummaryExpandsRecurrences is the behaviour the frontend is built around:
// the range summary hands back recurrence *occurrences* alongside explicit answers,
// with no marker distinguishing the two. That is why ParticipantView fetches its own
// availabilities separately.
func TestGetRangeSummaryExpandsRecurrences(t *testing.T) {
	fixture := newSummaryFixture(t, nil)

	// Every Thursday from 1 March. 2026-03-05, 03-12, 03-19 and 03-26 are Thursdays.
	fixture.recurrences.byCalendar = []models.Recurrence{
		recurrenceOn(fixture.alice.ID, 4, "2026-03-01", nil),
	}

	got, err := fixture.service.GetRangeSummary(
		context.Background(), "token", "2026-03-01", "2026-03-31", "",
	)
	if err != nil {
		t.Fatalf("GetRangeSummary: %v", err)
	}

	want := []string{"2026-03-05", "2026-03-12", "2026-03-19", "2026-03-26"}
	if gotDates := dates(got); len(gotDates) != len(want) {
		t.Fatalf("got dates %v, want %v", gotDates, want)
	}
	for i, date := range dates(got) {
		if date != want[i] {
			t.Errorf("date %d = %s, want %s", i, date, want[i])
		}
	}

	for _, summary := range got {
		if len(summary.Participants) != 1 || summary.Participants[0].ParticipantName != "Alice" {
			t.Errorf("%s: participants = %+v, want Alice alone", summary.Date, summary.Participants)
		}
	}
}

// TestGetRangeSummaryDeduplicates covers the guard that stops one participant being
// counted twice on a date they both answered explicitly and are covered for by a
// recurrence. Without it, a threshold of two could be "met" by one person.
func TestGetRangeSummaryDeduplicates(t *testing.T) {
	fixture := newSummaryFixture(t, nil)

	// Alice recurs every Thursday, and has also answered explicitly on 2026-03-05.
	fixture.recurrences.byCalendar = []models.Recurrence{
		recurrenceOn(fixture.alice.ID, 4, "2026-03-01", nil),
	}
	fixture.availRepo.inRange = []*models.Availability{
		availabilityOn(t, fixture.alice.ID, "2026-03-05", ptr("14:00"), ptr("18:00")),
	}

	got, err := fixture.service.GetRangeSummary(
		context.Background(), "token", "2026-03-05", "2026-03-05", "",
	)
	if err != nil {
		t.Fatalf("GetRangeSummary: %v", err)
	}

	summary, ok := byDate(got)["2026-03-05"]
	if !ok {
		t.Fatal("2026-03-05 is missing from the summary")
	}

	if len(summary.Participants) != 1 {
		t.Fatalf("Alice appears %d times, want once", len(summary.Participants))
	}

	// The explicit answer wins: its times are the ones reported, not the
	// recurrence's.
	if deref(summary.Participants[0].StartTime) != "14:00" {
		t.Errorf("start = %s, want the explicit 14:00", deref(summary.Participants[0].StartTime))
	}
	if summary.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", summary.TotalCount)
	}
}

// TestGetRangeSummaryHonoursExceptions covers the subtraction: an excepted date must
// disappear from the expansion entirely, not merely be marked.
func TestGetRangeSummaryHonoursExceptions(t *testing.T) {
	fixture := newSummaryFixture(t, nil)

	recurrence := recurrenceOn(fixture.alice.ID, 4, "2026-03-01", nil)
	fixture.recurrences.byCalendar = []models.Recurrence{recurrence}
	fixture.recurrences.exceptions[recurrence.ID] = []models.RecurrenceException{
		{RecurrenceID: recurrence.ID, ExcludedDate: "2026-03-12"},
	}

	got, err := fixture.service.GetRangeSummary(
		context.Background(), "token", "2026-03-01", "2026-03-31", "",
	)
	if err != nil {
		t.Fatalf("GetRangeSummary: %v", err)
	}

	if _, excepted := byDate(got)["2026-03-12"]; excepted {
		t.Error("the excepted date is still present in the summary")
	}
	if _, kept := byDate(got)["2026-03-05"]; !kept {
		t.Error("an unexcepted occurrence was dropped along with the exception")
	}
	if len(got) != 3 {
		t.Errorf("got %d dates, want 3 (four Thursdays minus one exception)", len(got))
	}
}

// TestGetRangeSummaryRecurrenceBounds pins the window arithmetic. The comparison is
// done on the ISO strings rather than on parsed dates, deliberately, to sidestep
// timezone drift — so the bounds are inclusive at both ends.
func TestGetRangeSummaryRecurrenceBounds(t *testing.T) {
	tests := []struct {
		name      string
		start     string
		end       *string
		queryFrom string
		queryTo   string
		wantDates []string
	}{
		{
			name:      "starts mid-range",
			start:     "2026-03-10",
			queryFrom: "2026-03-01", queryTo: "2026-03-31",
			wantDates: []string{"2026-03-12", "2026-03-19", "2026-03-26"},
		},
		{
			name:  "ends mid-range, inclusively",
			start: "2026-03-01", end: ptr("2026-03-12"),
			queryFrom: "2026-03-01", queryTo: "2026-03-31",
			wantDates: []string{"2026-03-05", "2026-03-12"},
		},
		{
			name:      "starts exactly on an occurrence",
			start:     "2026-03-05",
			queryFrom: "2026-03-01", queryTo: "2026-03-10",
			wantDates: []string{"2026-03-05"},
		},
		{
			name:  "a single-day window",
			start: "2026-03-01", end: ptr("2026-03-05"),
			queryFrom: "2026-03-05", queryTo: "2026-03-05",
			wantDates: []string{"2026-03-05"},
		},
		{
			name:  "the recurrence ends before the query starts",
			start: "2026-01-01", end: ptr("2026-02-01"),
			queryFrom: "2026-03-01", queryTo: "2026-03-31",
			wantDates: nil,
		},
		{
			name:      "the recurrence starts after the query ends",
			start:     "2026-06-01",
			queryFrom: "2026-03-01", queryTo: "2026-03-31",
			wantDates: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSummaryFixture(t, nil)
			fixture.recurrences.byCalendar = []models.Recurrence{
				recurrenceOn(fixture.alice.ID, 4, tt.start, tt.end),
			}

			got, err := fixture.service.GetRangeSummary(
				context.Background(), "token", tt.queryFrom, tt.queryTo, "",
			)
			if err != nil {
				t.Fatalf("GetRangeSummary: %v", err)
			}

			gotDates := dates(got)
			if len(gotDates) != len(tt.wantDates) {
				t.Fatalf("got %v, want %v", gotDates, tt.wantDates)
			}
			for i, date := range gotDates {
				if date != tt.wantDates[i] {
					t.Errorf("date %d = %s, want %s", i, date, tt.wantDates[i])
				}
			}
		})
	}
}

// TestGetRangeSummaryMinDuration covers the filter that hides dates too short to be
// worth meeting on. It runs on the *overlap*, not on any one participant's span.
func TestGetRangeSummaryMinDuration(t *testing.T) {
	tests := []struct {
		name        string
		minDuration int
		aliceStart  string
		aliceEnd    string
		bobStart    string
		bobEnd      string
		wantKept    bool
	}{
		{
			name: "the overlap clears the minimum", minDuration: 2,
			aliceStart: "09:00", aliceEnd: "17:00",
			bobStart: "10:00", bobEnd: "14:00",
			wantKept: true,
		},
		{
			name: "the overlap is too short", minDuration: 4,
			aliceStart: "09:00", aliceEnd: "17:00",
			bobStart: "10:00", bobEnd: "12:00",
			wantKept: false,
		},
		{
			name: "no minimum keeps everything", minDuration: 0,
			aliceStart: "09:00", aliceEnd: "17:00",
			bobStart: "10:00", bobEnd: "10:30",
			wantKept: true,
		},
		{
			name: "an exact match is kept", minDuration: 2,
			aliceStart: "09:00", aliceEnd: "17:00",
			bobStart: "10:00", bobEnd: "12:00",
			wantKept: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSummaryFixture(t, func(c *repository.Calendar) {
				c.MinDurationHours = tt.minDuration
			})

			fixture.availRepo.inRange = []*models.Availability{
				availabilityOn(t, fixture.alice.ID, "2026-03-05", ptr(tt.aliceStart), ptr(tt.aliceEnd)),
				availabilityOn(t, fixture.bob.ID, "2026-03-05", ptr(tt.bobStart), ptr(tt.bobEnd)),
			}

			got, err := fixture.service.GetRangeSummary(
				context.Background(), "token", "2026-03-05", "2026-03-05", "",
			)
			if err != nil {
				t.Fatalf("GetRangeSummary: %v", err)
			}

			_, kept := byDate(got)["2026-03-05"]
			if kept != tt.wantKept {
				t.Errorf("date kept = %v, want %v", kept, tt.wantKept)
			}
		})
	}
}

// TestGetRangeSummaryCountsSimultaneous guards the difference between "how many people
// answered" and "how many can actually meet", which is what the threshold gauge shows.
func TestGetRangeSummaryCountsSimultaneous(t *testing.T) {
	fixture := newSummaryFixture(t, nil)

	// Two people on the same date who never overlap: two answers, but never two at
	// once, so the count must be one.
	fixture.availRepo.inRange = []*models.Availability{
		availabilityOn(t, fixture.alice.ID, "2026-03-05", ptr("09:00"), ptr("10:00")),
		availabilityOn(t, fixture.bob.ID, "2026-03-05", ptr("14:00"), ptr("16:00")),
	}

	got, err := fixture.service.GetRangeSummary(
		context.Background(), "token", "2026-03-05", "2026-03-05", "",
	)
	if err != nil {
		t.Fatalf("GetRangeSummary: %v", err)
	}

	summary := byDate(got)["2026-03-05"]
	if len(summary.Participants) != 2 {
		t.Errorf("got %d participants, want 2", len(summary.Participants))
	}
	if summary.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1: they never overlap", summary.TotalCount)
	}
}

func TestGetRangeSummaryPrivacy(t *testing.T) {
	fixture := newSummaryFixture(t, func(c *repository.Calendar) { c.LockParticipants = true })

	fixture.availRepo.inRange = []*models.Availability{
		availabilityOn(t, fixture.alice.ID, "2026-03-05", ptr("09:00"), ptr("17:00")),
		availabilityOn(t, fixture.bob.ID, "2026-03-05", ptr("09:00"), ptr("17:00")),
	}

	got, err := fixture.service.GetRangeSummary(
		context.Background(), "token", "2026-03-05", "2026-03-05", fixture.alice.ID.String(),
	)
	if err != nil {
		t.Fatalf("GetRangeSummary: %v", err)
	}

	summary := byDate(got)["2026-03-05"]

	identified := 0
	for _, p := range summary.Participants {
		if p.ParticipantID != nil {
			identified++
			if *p.ParticipantID != fixture.alice.ID {
				t.Errorf("the identified participant is %v, want Alice", *p.ParticipantID)
			}
		}
	}
	if identified != 1 {
		t.Errorf("%d participants were identified, want 1 under lock_participants", identified)
	}
}

// TestGetRangeSummaryUnknownParticipant covers a consistency gap: an availability or
// recurrence whose participant is no longer on the calendar is dropped rather than
// reported under an empty name.
func TestGetRangeSummaryUnknownParticipant(t *testing.T) {
	fixture := newSummaryFixture(t, nil)

	ghost := uuid.New()
	fixture.availRepo.inRange = []*models.Availability{
		availabilityOn(t, ghost, "2026-03-05", ptr("09:00"), ptr("17:00")),
	}
	fixture.recurrences.byCalendar = []models.Recurrence{recurrenceOn(ghost, 4, "2026-03-01", nil)}

	got, err := fixture.service.GetRangeSummary(
		context.Background(), "token", "2026-03-05", "2026-03-05", "",
	)
	if err != nil {
		t.Fatalf("GetRangeSummary: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("got %d dates, want none: the participant is unknown", len(got))
	}
}

func TestGetRangeSummaryErrors(t *testing.T) {
	t.Run("an unknown token", func(t *testing.T) {
		service := NewAvailabilityService(
			&mockAvailabilityRepo{},
			&mockCalendarInfoRepo{err: repository.ErrCalendarNotFound},
			&mockParticipantsRepo{},
			&mockRecurrenceRepo{},
			&mockNotifyService{},
			cache.NewRedisCache(nil),
		)

		_, err := service.GetRangeSummary(context.Background(), "nope", "2026-03-01", "2026-03-31", "")
		if !errors.Is(err, ErrCalendarNotFound) {
			t.Errorf("error = %v, want ErrCalendarNotFound", err)
		}
	})

	dateTests := []struct {
		name       string
		start, end string
	}{
		{name: "an unparseable start", start: "01/03/2026", end: "2026-03-31"},
		{name: "an unparseable end", start: "2026-03-01", end: "the end of time"},
		{name: "an empty start", start: "", end: "2026-03-31"},
	}

	for _, tt := range dateTests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSummaryFixture(t, nil)

			_, err := fixture.service.GetRangeSummary(context.Background(), "token", tt.start, tt.end, "")
			if !errors.Is(err, ErrInvalidDate) {
				t.Errorf("error = %v, want ErrInvalidDate", err)
			}
		})
	}

	t.Run("an inverted range", func(t *testing.T) {
		fixture := newSummaryFixture(t, nil)

		_, err := fixture.service.GetRangeSummary(context.Background(), "token", "2026-03-31", "2026-03-01", "")
		if err == nil {
			t.Error("an end before the start was accepted")
		}
	})

	t.Run("a store failure is propagated", func(t *testing.T) {
		fixture := newSummaryFixture(t, nil)
		fixture.availRepo.inRangeErr = errStore

		_, err := fixture.service.GetRangeSummary(context.Background(), "token", "2026-03-01", "2026-03-31", "")
		if !errors.Is(err, errStore) {
			t.Errorf("error = %v, want the store failure", err)
		}
	})

	t.Run("an exception lookup failure is propagated", func(t *testing.T) {
		fixture := newSummaryFixture(t, nil)
		fixture.recurrences.byCalendar = []models.Recurrence{recurrenceOn(fixture.alice.ID, 4, "2026-03-01", nil)}
		fixture.recurrences.exceptionsErr = errStore

		_, err := fixture.service.GetRangeSummary(context.Background(), "token", "2026-03-01", "2026-03-31", "")
		if !errors.Is(err, errStore) {
			t.Errorf("error = %v, want the store failure", err)
		}
	})
}

// TestGetRangeSummaryEmptyRange confirms a quiet calendar produces no rows rather than
// a row per day with nobody in it.
func TestGetRangeSummaryEmptyRange(t *testing.T) {
	fixture := newSummaryFixture(t, nil)

	got, err := fixture.service.GetRangeSummary(
		context.Background(), "token", "2026-03-01", "2026-03-31", "",
	)
	if err != nil {
		t.Fatalf("GetRangeSummary: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d dates, want none", len(got))
	}
}
