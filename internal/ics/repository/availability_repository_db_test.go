// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/ics/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// GetEventsAboveThreshold is one 80-line query that expands recurrences, subtracts their
// exceptions, lets manual entries override them and only then counts against the
// threshold. None of that logic exists in Go, so nothing but a real database can test it —
// and the result is what subscribers see in Google Calendar, where a wrong day is worse
// than no feed at all.

// march returns a date in March 2027, a month fully inside the recurrence bounds these
// tests set. A fixed month keeps the generated date series deterministic.
func march(dayOfMonth int) time.Time {
	return time.Date(2027, 3, dayOfMonth, 0, 0, 0, 0, time.UTC)
}

func TestEventsAboveThresholdCountsManualAvailabilities(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seedCalendar(t, pool, "Ada", "Grace", "Katherine")

	// Two on the 10th reaches the threshold of two; one on the 11th does not.
	addAvailability(t, pool, f.participants[0].ID, march(10), "09:00", "12:00")
	addAvailability(t, pool, f.participants[1].ID, march(10), "10:00", "14:00")
	addAvailability(t, pool, f.participants[2].ID, march(11), "09:00", "12:00")

	events, err := repo.GetEventsAboveThreshold(ctx, f.calendar.ID, 2)
	if err != nil {
		t.Fatalf("GetEventsAboveThreshold: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("got %d dates, want only the 10th: %v", len(events), events)
	}
	day, ok := events[march(10)]
	if !ok {
		t.Fatalf("the 10th is missing; got %v", events)
	}
	if len(day) != 2 {
		t.Fatalf("the 10th has %d entries, want 2", len(day))
	}

	// Ordered by participant name, which is the order they appear in the event
	// description.
	if day[0].ParticipantName != "Ada" || day[1].ParticipantName != "Grace" {
		t.Errorf("participants = %q then %q, want Ada then Grace", day[0].ParticipantName, day[1].ParticipantName)
	}
	// The counts drive the "2/3 available" line in the summary.
	if day[0].AvailableCount != 2 || day[0].TotalParticipants != 3 {
		t.Errorf("counts = %d/%d, want 2/3", day[0].AvailableCount, day[0].TotalParticipants)
	}
	// TIME columns are reformatted to HH:MM in Go; the raw value carries seconds, which
	// would end up in the exported event title.
	if day[0].StartTime == nil || *day[0].StartTime != "09:00" {
		t.Errorf("StartTime = %v, want 09:00", day[0].StartTime)
	}
	if day[1].EndTime == nil || *day[1].EndTime != "14:00" {
		t.Errorf("EndTime = %v, want 14:00", day[1].EndTime)
	}
}

func TestEventsAboveThresholdKeepsAllDayEntriesUntimed(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seedCalendar(t, pool, "Ada", "Grace")
	addAvailability(t, pool, f.participants[0].ID, march(10), "", "")
	addAvailability(t, pool, f.participants[1].ID, march(10), "", "")

	events, err := repo.GetEventsAboveThreshold(ctx, f.calendar.ID, 2)
	if err != nil {
		t.Fatalf("GetEventsAboveThreshold: %v", err)
	}

	day := events[march(10)]
	if len(day) != 2 {
		t.Fatalf("the 10th has %d entries, want 2", len(day))
	}
	// An untimed availability must stay untimed: the ICS writer turns nil into an all-day
	// VEVENT, and a zero time would become a midnight-to-midnight appointment instead.
	if day[0].StartTime != nil || day[0].EndTime != nil {
		t.Errorf("times = %v/%v, want both nil", day[0].StartTime, day[0].EndTime)
	}
}

func TestEventsAboveThresholdRespectsTheThreshold(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seedCalendar(t, pool, "Ada", "Grace", "Katherine")
	for _, p := range f.participants {
		addAvailability(t, pool, p.ID, march(10), "09:00", "12:00")
	}

	// Three answered, so a threshold of three still matches and one of four does not.
	for _, tt := range []struct {
		threshold int
		wantDates int
	}{{1, 1}, {2, 1}, {3, 1}, {4, 0}} {
		events, err := repo.GetEventsAboveThreshold(ctx, f.calendar.ID, tt.threshold)
		if err != nil {
			t.Fatalf("threshold %d: %v", tt.threshold, err)
		}
		if len(events) != tt.wantDates {
			t.Errorf("threshold %d produced %d dates, want %d", tt.threshold, len(events), tt.wantDates)
		}
	}
}

func TestEventsAboveThresholdIsScopedToTheCalendar(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	mine := seedCalendar(t, pool, "Ada", "Grace")
	theirs := seedCalendar(t, pool, "Alan", "Edsger")

	for _, p := range theirs.participants {
		addAvailability(t, pool, p.ID, march(10), "09:00", "12:00")
	}

	// The feed is public to anyone holding the token, so leaking another calendar's
	// availability here would be a genuine disclosure rather than a display bug.
	events, err := repo.GetEventsAboveThreshold(ctx, mine.calendar.ID, 2)
	if err != nil {
		t.Fatalf("GetEventsAboveThreshold: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("another calendar's availabilities leaked into the feed: %v", events)
	}
}

// TestRecurrencesAreExpandedIntoEvents is the core of the query: a weekly commitment is
// stored once but must appear on every matching day of the feed.
func TestRecurrencesAreExpandedIntoEvents(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seedCalendar(t, pool, "Ada", "Grace")

	// Both recur on the same weekday, bounded to March so the generated series is
	// deterministic rather than anchored on today's date.
	from, to := march(1), march(31)
	addRecurrence(t, pool, f.participants[0].ID, from, to)
	addRecurrence(t, pool, f.participants[1].ID, from, to)

	events, err := repo.GetEventsAboveThreshold(ctx, f.calendar.ID, 2)
	if err != nil {
		t.Fatalf("GetEventsAboveThreshold: %v", err)
	}

	// 1, 8, 15, 22 and 29 March all fall on the same weekday as the 1st.
	want := []int{1, 8, 15, 22, 29}
	if len(events) != len(want) {
		t.Fatalf("got %d dates, want %d: %v", len(events), len(want), events)
	}
	for _, dayOfMonth := range want {
		day, ok := events[march(dayOfMonth)]
		if !ok {
			t.Errorf("%d March is missing from the expansion", dayOfMonth)
			continue
		}
		if len(day) != 2 {
			t.Errorf("%d March has %d entries, want both participants", dayOfMonth, len(day))
		}
		// The recurrence times have to survive expansion, or every recurring event
		// exports as all-day.
		if day[0].StartTime == nil || *day[0].StartTime != "09:00" {
			t.Errorf("%d March StartTime = %v, want 09:00", dayOfMonth, day[0].StartTime)
		}
	}
}

// TestRecurrenceExceptionsAreSubtracted covers the opt-out. A skipped week that still
// exported would put a subscriber somewhere they said they could not be.
func TestRecurrenceExceptionsAreSubtracted(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seedCalendar(t, pool, "Ada", "Grace")
	from, to := march(1), march(31)
	recurrence := addRecurrence(t, pool, f.participants[0].ID, from, to)
	addRecurrence(t, pool, f.participants[1].ID, from, to)

	if _, err := pool.Exec(ctx, `
		INSERT INTO recurrence_exceptions (id, recurrence_id, excluded_date) VALUES ($1, $2, $3)`,
		uuid.New(), recurrence, march(15)); err != nil {
		t.Fatalf("insert exception: %v", err)
	}

	events, err := repo.GetEventsAboveThreshold(ctx, f.calendar.ID, 2)
	if err != nil {
		t.Fatalf("GetEventsAboveThreshold: %v", err)
	}

	// Only one of the two still recurs on the 15th, so the date drops below the
	// threshold and leaves the feed entirely.
	if _, ok := events[march(15)]; ok {
		t.Errorf("15 March survived the exception: %v", events[march(15)])
	}
	if _, ok := events[march(22)]; !ok {
		t.Error("the exception removed more than the one excluded date")
	}
}

// TestAManualEntryOverridesTheRecurrence covers the last NOT EXISTS clause. Without it a
// participant who edited one occurrence would be counted twice that day and could reach
// the threshold on their own.
func TestAManualEntryOverridesTheRecurrence(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seedCalendar(t, pool, "Ada", "Grace")
	from, to := march(1), march(31)
	addRecurrence(t, pool, f.participants[0].ID, from, to)
	addRecurrence(t, pool, f.participants[1].ID, from, to)

	// Ada moves the 15th to the afternoon.
	addAvailability(t, pool, f.participants[0].ID, march(15), "14:00", "18:00")

	events, err := repo.GetEventsAboveThreshold(ctx, f.calendar.ID, 2)
	if err != nil {
		t.Fatalf("GetEventsAboveThreshold: %v", err)
	}

	day := events[march(15)]
	if len(day) != 2 {
		t.Fatalf("15 March has %d entries, want one per participant: %+v", len(day), day)
	}
	if day[0].ParticipantName != "Ada" {
		t.Fatalf("entries are not in name order: %+v", day)
	}
	// The manual times win; the 09:00 from the recurrence must not also be there.
	if day[0].StartTime == nil || *day[0].StartTime != "14:00" {
		t.Errorf("Ada's StartTime = %v, want the manual 14:00", day[0].StartTime)
	}
	if day[0].AvailableCount != 2 {
		t.Errorf("AvailableCount = %d, want 2 — the override was counted twice", day[0].AvailableCount)
	}
}

func TestEventsForACalendarWithNothingInIt(t *testing.T) {
	// A brand new calendar produces an empty feed rather than an error: the ICS endpoint
	// still has to return a valid, empty VCALENDAR.
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)

	f := seedCalendar(t, pool, "Ada")

	events, err := repo.GetEventsAboveThreshold(dbtest.Context(t), f.calendar.ID, 1)
	if err != nil {
		t.Fatalf("GetEventsAboveThreshold: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("an empty calendar produced %d dates", len(events))
	}
}

// TestEventsAboveThresholdForCalendarsMatchesOneAtATime is the guard on the unified
// feed's N+1 fix. The batched read pipelines the very same SQL, so the only way it can
// go wrong is by pairing a result set with the wrong calendar — which would publish one
// calendar's events under another's name, to a subscriber who cannot tell.
func TestEventsAboveThresholdForCalendarsMatchesOneAtATime(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	// Two independent calendars, deliberately different in shape: different
	// participants, different dates, and different thresholds, so a swapped or reused
	// result set cannot pass.
	first := seedCalendar(t, pool, "Ada", "Grace")
	addAvailability(t, pool, first.participants[0].ID, march(10), "09:00", "12:00")
	addAvailability(t, pool, first.participants[1].ID, march(10), "10:00", "14:00")

	second := seedCalendar(t, pool, "Katherine", "Dorothy", "Mary")
	for _, p := range second.participants {
		addAvailability(t, pool, p.ID, march(20), "08:00", "18:00")
	}
	addAvailability(t, pool, second.participants[0].ID, march(21), "08:00", "18:00")

	empty := seedCalendar(t, pool, "Nobody")

	wanted := []repository.CalendarThreshold{
		{CalendarID: first.calendar.ID, Threshold: 2},
		{CalendarID: second.calendar.ID, Threshold: 3},
		{CalendarID: empty.calendar.ID, Threshold: 1},
	}

	batched, err := repo.GetEventsAboveThresholdForCalendars(ctx, wanted)
	if err != nil {
		t.Fatalf("GetEventsAboveThresholdForCalendars: %v", err)
	}
	if len(batched) != len(wanted) {
		t.Fatalf("got %d calendars back, want %d", len(batched), len(wanted))
	}

	for _, cal := range wanted {
		one, err := repo.GetEventsAboveThreshold(ctx, cal.CalendarID, cal.Threshold)
		if err != nil {
			t.Fatalf("GetEventsAboveThreshold(%s): %v", cal.CalendarID, err)
		}

		got := batched[cal.CalendarID]
		if len(got) != len(one) {
			t.Errorf("calendar %s: batched has %d dates, single has %d", cal.CalendarID, len(got), len(one))

			continue
		}

		for date, want := range one {
			have := got[date]
			if len(have) != len(want) {
				t.Errorf("calendar %s on %s: batched has %d rows, single has %d",
					cal.CalendarID, date.Format("2006-01-02"), len(have), len(want))

				continue
			}
			for i := range want {
				if have[i].ParticipantName != want[i].ParticipantName ||
					have[i].AvailableCount != want[i].AvailableCount ||
					have[i].TotalParticipants != want[i].TotalParticipants ||
					have[i].Note != want[i].Note {
					t.Errorf("calendar %s on %s row %d: batched = %+v, single = %+v",
						cal.CalendarID, date.Format("2006-01-02"), i, have[i], want[i])
				}
			}
		}
	}

	// The threshold is applied per calendar: the second one needs all three, so its
	// lone availability on the 21st must not appear.
	if _, found := batched[second.calendar.ID][march(21)]; found {
		t.Error("a date below the calendar's own threshold was published")
	}

	t.Run("no calendars means no batch", func(t *testing.T) {
		got, err := repo.GetEventsAboveThresholdForCalendars(ctx, nil)
		if err != nil {
			t.Fatalf("GetEventsAboveThresholdForCalendars(nil): %v", err)
		}
		if len(got) != 0 {
			t.Errorf("an empty request returned %d calendars", len(got))
		}
	})
}
