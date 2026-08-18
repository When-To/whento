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

// Four tests were dropped from this file when the expansion moved into SQL, because the
// service no longer decides any of it. Each is carried by a repository database test:
//
//   - expanding a recurrence into the dates it falls on — TestOccurrencesExpandRecurrencesAndCarryTimes
//   - a manual answer suppressing that day's occurrence — TestAManualAnswerSuppressesTheRecurrence
//   - an exception removing a single date from a range — TestOccurrencesForRangeCoverEveryDay
//   - recurrence start and end bounds — TestAvailableParticipantsHonourRecurrenceBounds
//
// A fifth, TestGetRangeSummaryUnknownParticipant, is answered by the occurrence query's
// JOIN on participants scoped to the calendar rather than by any loop here.

// The mocks below are hand-written and deliberately narrow: only the methods the
// summary paths reach carry behaviour, the rest satisfy the interface and would fail
// loudly if a future change started calling them.

type mockAvailabilityRepo struct {
	createErr error

	// The expansion the summary endpoints now read. Recurrence semantics — which
	// occurrences a manual answer suppresses, which an exception removes — moved to SQL
	// with it, so they are covered by the repository's database tests rather than mocked
	// here. What is left for these tests is what the service still decides: masking,
	// minimum duration, overlap, and the shape of an empty answer.
	occurrences    []models.Occurrence
	occurrencesErr error
}

var _ AvailabilityRepository = (*mockAvailabilityRepo)(nil)

func (m *mockAvailabilityRepo) Create(context.Context, *models.Availability) error {
	return m.createErr
}

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

func (m *mockAvailabilityRepo) GetParticipantCountForDate(context.Context, uuid.UUID, time.Time) (int, error) {
	return 0, nil
}

func (m *mockAvailabilityRepo) GetOccurrencesForDate(
	context.Context, uuid.UUID, time.Time,
) ([]models.Occurrence, error) {
	return m.occurrences, m.occurrencesErr
}

func (m *mockAvailabilityRepo) GetOccurrencesForRange(
	context.Context, uuid.UUID, time.Time, time.Time,
) ([]models.Occurrence, error) {
	return m.occurrences, m.occurrencesErr
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
	byParticipant []models.Recurrence
	exceptions    map[uuid.UUID][]models.RecurrenceException
	exceptionsErr error
}

var _ RecurrenceRepository = (*mockRecurrenceRepo)(nil)

func (m *mockRecurrenceRepo) CreateRecurrence(context.Context, *models.Recurrence) error { return nil }

func (m *mockRecurrenceRepo) GetRecurrencesByParticipant(context.Context, uuid.UUID) ([]models.Recurrence, error) {
	return m.byParticipant, nil
}

func (m *mockRecurrenceRepo) GetRecurrencesByCalendar(context.Context, uuid.UUID) ([]models.Recurrence, error) {
	return nil, nil
}

func (m *mockRecurrenceRepo) GetRecurrenceByID(context.Context, uuid.UUID) (*models.Recurrence, error) {
	return nil, nil
}

func (m *mockRecurrenceRepo) UpdateRecurrence(context.Context, *models.Recurrence) error { return nil }

func (m *mockRecurrenceRepo) DeleteRecurrence(context.Context, uuid.UUID) error { return nil }

func (m *mockRecurrenceRepo) CreateException(context.Context, *models.RecurrenceException) error {
	return nil
}

func (m *mockRecurrenceRepo) GetExceptionsByRecurrenceIDs(
	_ context.Context, recurrenceIDs []uuid.UUID,
) (map[uuid.UUID][]models.RecurrenceException, error) {
	if m.exceptionsErr != nil {
		return nil, m.exceptionsErr
	}

	// Mirror the repository: only recurrences that actually have exceptions appear.
	out := make(map[uuid.UUID][]models.RecurrenceException, len(recurrenceIDs))
	for _, id := range recurrenceIDs {
		if exceptions := m.exceptions[id]; len(exceptions) > 0 {
			out[id] = exceptions
		}
	}

	return out, nil
}

func (m *mockRecurrenceRepo) DeleteException(context.Context, uuid.UUID, string) error { return nil }

type mockNotifyService struct{}

var _ NotifyService = (*mockNotifyService)(nil)

func (m *mockNotifyService) CheckThresholdAndNotify(context.Context, uuid.UUID, time.Time, int) error {
	return nil
}

// available seeds one occurrence, which is what the repository now hands the service.
// Whether it came from a typed answer or a recurrence is settled before this point — see
// the repository's database tests — so these tests no longer have to say.
func available(p *repository.Participant, date time.Time, start, end *string) models.Occurrence {
	return models.Occurrence{
		Date:            date,
		ParticipantID:   p.ID,
		ParticipantName: p.Name,
		StartTime:       start,
		EndTime:         end,
	}
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

// TestGetRangeSummaryGroupsOccurrencesByDate covers the bucketing, which is all the range
// endpoint still does with the expansion: one summary per date that has an occurrence,
// several participants on a date collapsing into that one summary, and no marker telling
// an occurrence a recurrence produced from one somebody typed. That last part is the
// behaviour the frontend is built around, and why ParticipantView fetches its own
// availabilities separately.
func TestGetRangeSummaryGroupsOccurrencesByDate(t *testing.T) {
	fixture := newSummaryFixture(t, nil)

	// Alice on every Thursday of March, as the repository expands a weekly rule, with Bob
	// answering one of them.
	for _, iso := range []string{"2026-03-05", "2026-03-12", "2026-03-19", "2026-03-26"} {
		fixture.availRepo.occurrences = append(
			fixture.availRepo.occurrences,
			available(fixture.alice, mustDate(t, iso), nil, nil),
		)
	}
	fixture.availRepo.occurrences = append(
		fixture.availRepo.occurrences,
		available(fixture.bob, mustDate(t, "2026-03-12"), ptr("09:00"), ptr("17:00")),
	)

	got, err := fixture.service.GetRangeSummary(
		context.Background(), "token", "2026-03-01", "2026-03-31", "",
	)
	if err != nil {
		t.Fatalf("GetRangeSummary: %v", err)
	}

	want := []string{"2026-03-05", "2026-03-12", "2026-03-19", "2026-03-26"}
	gotDates := dates(got)
	if len(gotDates) != len(want) {
		t.Fatalf("got dates %v, want %v", gotDates, want)
	}
	for i, date := range gotDates {
		if date != want[i] {
			t.Errorf("date %d = %s, want %s", i, date, want[i])
		}
	}

	for _, summary := range got {
		wantParticipants := 1
		if summary.Date == "2026-03-12" {
			wantParticipants = 2
		}
		if len(summary.Participants) != wantParticipants {
			t.Errorf(
				"%s: participants = %+v, want %d",
				summary.Date, summary.Participants, wantParticipants,
			)
		}
		if summary.Participants[0].ParticipantName != "Alice" {
			t.Errorf("%s: first participant = %s, want Alice", summary.Date, summary.Participants[0].ParticipantName)
		}
	}
}

// TestGetRangeSummaryMinDuration covers the filter that hides dates too short to be
// worth meeting on. It runs on the *overlap*, not on any one participant's span.
// TestGetRangeSummaryMinDuration covers what the minimum now decides: how many people
// count as able to meet, not whether the date appears at all. A date used to be dropped
// from the range entirely when the overlap fell short, so the grid showed nothing for a
// day several people had answered for.
func TestGetRangeSummaryMinDuration(t *testing.T) {
	tests := []struct {
		name        string
		minDuration int
		aliceStart  string
		aliceEnd    string
		bobStart    string
		bobEnd      string
		wantCount   int
	}{
		{
			name: "the overlap clears the minimum", minDuration: 2,
			aliceStart: "09:00", aliceEnd: "17:00",
			bobStart: "10:00", bobEnd: "14:00",
			wantCount: 2,
		},
		{
			// Alice is free for eight hours, so she can hold the event alone; Bob's two
			// hours with her are not enough for the pair.
			name: "the overlap is too short", minDuration: 4,
			aliceStart: "09:00", aliceEnd: "17:00",
			bobStart: "10:00", bobEnd: "12:00",
			wantCount: 1,
		},
		{
			name: "no minimum counts any overlap", minDuration: 0,
			aliceStart: "09:00", aliceEnd: "17:00",
			bobStart: "10:00", bobEnd: "10:30",
			wantCount: 2,
		},
		{
			name: "an exact match counts", minDuration: 2,
			aliceStart: "09:00", aliceEnd: "17:00",
			bobStart: "10:00", bobEnd: "12:00",
			wantCount: 2,
		},
		{
			// The reported defect, at the range grain.
			name: "nobody overlaps at all", minDuration: 2,
			aliceStart: "10:00", aliceEnd: "13:00",
			bobStart: "14:00", bobEnd: "18:00",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSummaryFixture(t, func(c *repository.Calendar) {
				c.MinDurationHours = tt.minDuration
			})

			fixture.availRepo.occurrences = []models.Occurrence{
				available(fixture.alice, mustDate(t, "2026-03-05"), ptr(tt.aliceStart), ptr(tt.aliceEnd)),
				available(fixture.bob, mustDate(t, "2026-03-05"), ptr(tt.bobStart), ptr(tt.bobEnd)),
			}

			got, err := fixture.service.GetRangeSummary(
				context.Background(), "token", "2026-03-05", "2026-03-05", "",
			)
			if err != nil {
				t.Fatalf("GetRangeSummary: %v", err)
			}

			// The date is always described, whatever the count comes to.
			if len(got) != 1 {
				t.Fatalf("got %d dates, want 1: the date is dropped from the grid", len(got))
			}
			if got[0].TotalCount != tt.wantCount {
				t.Errorf("TotalCount = %d, want %d", got[0].TotalCount, tt.wantCount)
			}
			if len(got[0].Participants) != 2 {
				t.Errorf("listed %d participants, want 2", len(got[0].Participants))
			}
		})
	}
}

func TestGetRangeSummaryPrivacy(t *testing.T) {
	fixture := newSummaryFixture(t, func(c *repository.Calendar) { c.LockParticipants = true })

	fixture.availRepo.occurrences = []models.Occurrence{
		available(fixture.alice, mustDate(t, "2026-03-05"), ptr("09:00"), ptr("17:00")),
		available(fixture.bob, mustDate(t, "2026-03-05"), ptr("09:00"), ptr("17:00")),
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
		fixture.availRepo.occurrencesErr = errStore

		_, err := fixture.service.GetRangeSummary(context.Background(), "token", "2026-03-01", "2026-03-31", "")
		if !errors.Is(err, errStore) {
			t.Errorf("error = %v, want the store failure", err)
		}
	})
}

// TestExceptionLookupFailureIsPropagated used to be a subtest of TestGetRangeSummaryErrors,
// back when the range summary loaded exceptions to subtract them itself. GetParticipantRecurrences
// is the last caller of exceptionsFor, and the assertion is the one that mattered either
// way: a failed lookup is reported, not swallowed into an answer missing its exceptions.
func TestExceptionLookupFailureIsPropagated(t *testing.T) {
	fixture := newSummaryFixture(t, nil)

	recurrence := models.Recurrence{ParticipantID: fixture.alice.ID, DayOfWeek: 4, StartDate: "2026-03-01"}
	recurrence.ID = uuid.New()
	fixture.recurrences.byParticipant = []models.Recurrence{recurrence}
	fixture.recurrences.exceptionsErr = errStore

	_, err := fixture.service.GetParticipantRecurrences(
		context.Background(), "token", fixture.alice.ID.String(),
	)
	if !errors.Is(err, errStore) {
		t.Errorf("error = %v, want the store failure", err)
	}
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
