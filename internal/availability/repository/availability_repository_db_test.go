// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authModels "github.com/whento/whento/internal/auth/models"
	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// The service tests cover the expansion and summary rules with fakes. What only a real
// database shows is the UNIQUE(participant_id, date) constraint, the cascades, and the
// date and time columns surviving a round trip — a DATE read back as a timestamp in the
// wrong zone shifts every availability by a day.

type fixture struct {
	calendar    *calendarModels.Calendar
	participant *calendarModels.Participant
	other       *calendarModels.Participant
}

// seed builds an owner, a calendar and two participants. Everything cascades from the
// owner, so one cleanup covers the lot.
func seed(t *testing.T, pool *pgxpool.Pool) *fixture {
	t.Helper()

	ctx := dbtest.Context(t)
	id := uuid.New()

	owner := &authModels.User{
		Email:        fmt.Sprintf("avail-%s@example.test", id),
		PasswordHash: "$2a$04$abcdefghijklmnopqrstuv",
		DisplayName:  "Owner",
		Role:         authModels.RoleUser,
		Locale:       authModels.LocaleEN,
		Timezone:     "Europe/Paris",
	}
	owner.ID = id
	if err := authRepo.NewUserRepository(pool).Create(ctx, owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	dbtest.Cleanup(t, pool, `DELETE FROM users WHERE id = $1`, owner.ID)

	calendar := &calendarModels.Calendar{
		OwnerID:         owner.ID,
		Name:            "Availability Test",
		Threshold:       2,
		AllowedWeekdays: []int{0, 1, 2, 3, 4, 5, 6},
		Timezone:        "Europe/Paris",
		HolidaysPolicy:  "ignore",
		PublicToken:     fmt.Sprintf("pub-%s", id),
		ICSToken:        fmt.Sprintf("ics-%s", id),
	}
	calendar.ID = uuid.New()

	created, err := calendarRepo.NewCalendarRepository(pool).CreateWithParticipants(
		ctx, calendar, []calendarRepo.ParticipantInput{
			{Name: "Ada", Locale: "en"},
			{Name: "Grace", Locale: "en"},
		})
	if err != nil {
		t.Fatalf("create calendar: %v", err)
	}

	return &fixture{calendar: calendar, participant: &created[0], other: &created[1]}
}

func day(offset int) time.Time {
	// A fixed base rather than time.Now: these rows are compared by date, and a test
	// running at midnight should not straddle two days.
	base := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)

	return base.AddDate(0, 0, offset)
}

func availability(participantID uuid.UUID, date time.Time, start, end *string) *models.Availability {
	entry := &models.Availability{
		ParticipantID: participantID,
		Date:          date,
		StartTime:     start,
		EndTime:       end,
		// Create inserts source explicitly, so the column DEFAULT never applies and
		// a zero value trips availabilities_source_check. Production always sets it.
		Source: "manual",
	}
	entry.ID = uuid.New()

	return entry
}

func ptr(s string) *string { return &s }

func TestAvailabilityRoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	entry := availability(f.participant.ID, day(0), ptr("09:00"), ptr("17:00"))
	entry.Note = "a note"

	if err := repo.Create(ctx, entry); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByParticipantAndDate(ctx, f.participant.ID, day(0))
	if err != nil {
		t.Fatalf("GetByParticipantAndDate: %v", err)
	}

	// The date column is a DATE. Reading it back as a timestamp in the wrong zone would
	// shift every availability by a day, which no in-memory test can catch.
	if got.Date.Format("2006-01-02") != day(0).Format("2006-01-02") {
		t.Errorf("Date = %v, want %v", got.Date, day(0))
	}
	if got.StartTime == nil || *got.StartTime != "09:00" {
		t.Errorf("StartTime = %v, want 09:00", got.StartTime)
	}
	if got.EndTime == nil || *got.EndTime != "17:00" {
		t.Errorf("EndTime = %v, want 17:00", got.EndTime)
	}
	if got.Note != "a note" {
		t.Errorf("Note = %q, want %q", got.Note, "a note")
	}
}

func TestAvailabilityAllDayKeepsNullTimes(t *testing.T) {
	// An untimed availability is a different state from 00:00-23:59: the month cell
	// renders "All day" only for the former.
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	if err := repo.Create(ctx, availability(f.participant.ID, day(1), nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByParticipantAndDate(ctx, f.participant.ID, day(1))
	if err != nil {
		t.Fatalf("GetByParticipantAndDate: %v", err)
	}
	if got.StartTime != nil || got.EndTime != nil {
		t.Errorf("times = %v/%v, want both nil", got.StartTime, got.EndTime)
	}
}

func TestAvailabilityNotFoundIsASentinel(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	if _, err := repo.GetByParticipantAndDate(ctx, f.participant.ID, day(99)); !errors.Is(err, repository.ErrAvailabilityNotFound) {
		t.Errorf("error = %v, want ErrAvailabilityNotFound", err)
	}
}

// TestOneAvailabilityPerParticipantPerDay covers UNIQUE(participant_id, date), which is
// what stops one participant being counted twice toward a threshold.
func TestOneAvailabilityPerParticipantPerDay(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	if err := repo.Create(ctx, availability(f.participant.ID, day(0), ptr("09:00"), ptr("10:00"))); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Create(ctx, availability(f.participant.ID, day(0), ptr("14:00"), ptr("15:00"))); err == nil {
		t.Error("a second availability was accepted for the same participant and day")
	}

	// The constraint is per participant: somebody else answering the same day is fine.
	if err := repo.Create(ctx, availability(f.other.ID, day(0), ptr("09:00"), ptr("10:00"))); err != nil {
		t.Errorf("another participant was refused the same day: %v", err)
	}
}

func TestAvailabilityUpdateAndDelete(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	entry := availability(f.participant.ID, day(0), ptr("09:00"), ptr("17:00"))
	if err := repo.Create(ctx, entry); err != nil {
		t.Fatalf("Create: %v", err)
	}

	entry.StartTime = ptr("10:00")
	entry.EndTime = ptr("12:00")
	entry.Note = "moved"
	if err := repo.Update(ctx, entry); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByParticipantAndDate(ctx, f.participant.ID, day(0))
	if err != nil {
		t.Fatalf("GetByParticipantAndDate: %v", err)
	}
	if *got.StartTime != "10:00" || *got.EndTime != "12:00" || got.Note != "moved" {
		t.Errorf("the update did not persist: %+v", got)
	}

	if err := repo.Delete(ctx, f.participant.ID, day(0)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByParticipantAndDate(ctx, f.participant.ID, day(0)); !errors.Is(err, repository.ErrAvailabilityNotFound) {
		t.Errorf("the availability survived deletion: %v", err)
	}
}

func TestDeleteOnAnUnknownDay(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	if err := repo.Delete(ctx, f.participant.ID, day(50)); !errors.Is(err, repository.ErrAvailabilityNotFound) {
		t.Errorf("deleting nothing gave %v, want ErrAvailabilityNotFound", err)
	}
}

// TestDateRangesAreInclusive pins the boundary. An exclusive end would silently drop the
// last day of every month the calendar renders.
func TestDateRangesAreInclusive(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	for _, offset := range []int{0, 1, 2, 3} {
		if err := repo.Create(ctx, availability(f.participant.ID, day(offset), ptr("09:00"), ptr("10:00"))); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	inRange, err := repo.GetByCalendarDateRange(ctx, f.calendar.ID, day(1), day(2))
	if err != nil {
		t.Fatalf("GetByCalendarDateRange: %v", err)
	}
	if len(inRange) != 2 {
		t.Errorf("got %d rows for a two-day inclusive range, want 2", len(inRange))
	}

	// A single-day range must return that day, not nothing.
	single, err := repo.GetByCalendarDateRange(ctx, f.calendar.ID, day(0), day(0))
	if err != nil {
		t.Fatalf("GetByCalendarDateRange: %v", err)
	}
	if len(single) != 1 {
		t.Errorf("got %d rows for a single-day range, want 1", len(single))
	}
}

func TestGetByCalendarDateRangeSpansParticipants(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	if err := repo.Create(ctx, availability(f.participant.ID, day(0), ptr("09:00"), ptr("10:00"))); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, availability(f.other.ID, day(0), ptr("11:00"), ptr("12:00"))); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The range summary is built from this: it has to see the whole calendar, not one
	// participant.
	rows, err := repo.GetByCalendarDateRange(ctx, f.calendar.ID, day(0), day(0))
	if err != nil {
		t.Fatalf("GetByCalendarDateRange: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("got %d rows, want both participants", len(rows))
	}

	// Another calendar's rows must not appear.
	other := seed(t, pool)
	if err := repo.Create(ctx, availability(other.participant.ID, day(0), ptr("09:00"), ptr("10:00"))); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stillTwo, err := repo.GetByCalendarDateRange(ctx, f.calendar.ID, day(0), day(0))
	if err != nil {
		t.Fatalf("GetByCalendarDateRange: %v", err)
	}
	if len(stillTwo) != 2 {
		t.Errorf("the query leaked across calendars: %d rows", len(stillTwo))
	}
}

func TestGetByParticipantIDWithDateRange(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	for _, offset := range []int{0, 5, 10} {
		if err := repo.Create(ctx, availability(f.participant.ID, day(offset), nil, nil)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	bounded, err := repo.GetByParticipantIDWithDateRange(ctx, f.participant.ID, timePtr(day(0)), timePtr(day(5)))
	if err != nil {
		t.Fatalf("GetByParticipantIDWithDateRange: %v", err)
	}
	if len(bounded) != 2 {
		t.Errorf("got %d rows in range, want 2", len(bounded))
	}

	// Nil bounds mean "no restriction", which is how ParticipantView asks for
	// everything it has.
	all, err := repo.GetByParticipantIDWithDateRange(ctx, f.participant.ID, nil, nil)
	if err != nil {
		t.Fatalf("GetByParticipantIDWithDateRange(nil, nil): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("got %d rows without bounds, want all 3", len(all))
	}
}

func TestGetByDateIsScopedToTheCalendar(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	other := seed(t, pool)

	if err := repo.Create(ctx, availability(f.participant.ID, day(0), nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, availability(other.participant.ID, day(0), nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rows, err := repo.GetByDate(ctx, f.calendar.ID, day(0))
	if err != nil {
		t.Fatalf("GetByDate: %v", err)
	}
	if len(rows) != 1 || rows[0].ParticipantID != f.participant.ID {
		t.Errorf("GetByDate returned %d rows from the wrong calendar", len(rows))
	}
}

// TestAvailableParticipantsIncludeRecurrences is the defect this method exists for.
// Recurrences are expanded when read and never stored, so asking the availabilities
// table who is available answers "who has a row" — and a participant available every
// Friday has none. The threshold count expanded them and saw three of three; the
// notification read the table and saw two. The participant was counted towards the
// event, left out of the list in the email, and never told about it.
func TestAvailableParticipantsIncludeRecurrences(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	recurrences := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	// day(0) is 2027-03-01, a Monday.
	monday := day(0)
	if got := monday.Weekday(); got != time.Monday {
		t.Fatalf("the fixture date is a %s; this test assumes Monday", got)
	}

	// One participant answers directly, the other stands on a weekly recurrence.
	if err := repo.Create(ctx, availability(f.participant.ID, monday, nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	weekly := recurrence(f.other.ID, int(time.Monday), nil, nil)
	if err := recurrences.CreateRecurrence(ctx, weekly); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}

	available, err := repo.GetAvailableParticipantsForDate(ctx, f.calendar.ID, monday)
	if err != nil {
		t.Fatalf("GetAvailableParticipantsForDate: %v", err)
	}

	got := map[uuid.UUID]string{}
	for _, p := range available {
		got[p.ID] = p.Name
	}
	if len(got) != 2 {
		t.Fatalf("got %d available participants, want 2: %v", len(got), got)
	}
	if _, ok := got[f.other.ID]; !ok {
		t.Error("the participant available through a recurrence is missing")
	}
	// The name is what the email prints, so an id alone would not have caught the
	// half of this defect that was visible.
	if got[f.other.ID] == "" {
		t.Error("the recurrence-only participant came back without a name")
	}

	// The count is derived from this list, so the two cannot disagree — which is the
	// property that failed in production.
	count, err := repo.GetParticipantCountForDate(ctx, f.calendar.ID, monday)
	if err != nil {
		t.Fatalf("GetParticipantCountForDate: %v", err)
	}
	if count != len(available) {
		t.Errorf("count = %d but the list holds %d: they have diverged again", count, len(available))
	}
}

// TestAvailableParticipantsHonourRecurrenceBounds covers the three ways a recurrence
// does not apply to a date it otherwise matches.
func TestAvailableParticipantsHonourRecurrenceBounds(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	recurrences := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	monday := day(0)

	weekly := recurrence(f.other.ID, int(time.Monday), nil, nil)
	if err := recurrences.CreateRecurrence(ctx, weekly); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}

	available := func(t *testing.T, date time.Time) bool {
		t.Helper()

		people, err := repo.GetAvailableParticipantsForDate(ctx, f.calendar.ID, date)
		if err != nil {
			t.Fatalf("GetAvailableParticipantsForDate: %v", err)
		}
		for _, p := range people {
			if p.ID == f.other.ID {
				return true
			}
		}

		return false
	}

	if !available(t, monday) {
		t.Fatal("the recurrence does not apply on its own weekday")
	}
	// A Tuesday: the same week, the wrong day.
	if available(t, day(1)) {
		t.Error("the recurrence applied on a day it does not fall on")
	}
	// Before the recurrence starts. StartDate is 2027-03-01, so day(-7) precedes it.
	if available(t, day(-7)) {
		t.Error("the recurrence applied before its start date")
	}

	// An exception on the matching Monday removes it for that date only.
	exception := &models.RecurrenceException{RecurrenceID: weekly.ID, ExcludedDate: "2027-03-08"}
	exception.ID = uuid.New()
	if err := recurrences.CreateException(ctx, exception); err != nil {
		t.Fatalf("CreateException: %v", err)
	}
	if available(t, day(7)) {
		t.Error("an excluded date still counted the participant as available")
	}
	if !available(t, monday) {
		t.Error("the exception removed a date it was not for")
	}
}

func TestGetParticipantCountForDate(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	// This count is what the threshold notification compares against, so it must be
	// zero before anyone answers rather than an error.
	count, err := repo.GetParticipantCountForDate(ctx, f.calendar.ID, day(0))
	if err != nil {
		t.Fatalf("GetParticipantCountForDate: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d before anyone answered", count)
	}

	if err := repo.Create(ctx, availability(f.participant.ID, day(0), nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, availability(f.other.ID, day(0), nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	count, err = repo.GetParticipantCountForDate(ctx, f.calendar.ID, day(0))
	if err != nil {
		t.Fatalf("GetParticipantCountForDate: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

// TestAvailabilitiesGoWithTheirParticipant covers the cascade: removing a participant
// must take their answers with them, or the counts keep including somebody who left.
func TestAvailabilitiesGoWithTheirParticipant(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	if err := repo.Create(ctx, availability(f.participant.ID, day(0), nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := calendarRepo.NewParticipantRepository(pool).Delete(ctx, f.participant.ID); err != nil {
		t.Fatalf("delete participant: %v", err)
	}

	if _, err := repo.GetByParticipantAndDate(ctx, f.participant.ID, day(0)); !errors.Is(err, repository.ErrAvailabilityNotFound) {
		t.Errorf("an availability outlived its participant: %v", err)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// The occurrence expansion is the one answer to "who is available, when". These tests
// carry the semantics that used to live in the availability service's Go loops — a manual
// answer suppressing a recurrence, exceptions, bounds — because that is where the logic
// went, not because they were invented here.

func TestOccurrencesExpandRecurrencesAndCarryTimes(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	recurrences := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	monday := day(0)

	start, end := ptr("18:00"), ptr("22:00")
	weekly := recurrence(f.other.ID, int(time.Monday), start, end)
	if err := recurrences.CreateRecurrence(ctx, weekly); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}
	if err := repo.Create(ctx, availability(f.participant.ID, monday, nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetOccurrencesForDate(ctx, f.calendar.ID, monday)
	if err != nil {
		t.Fatalf("GetOccurrencesForDate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d occurrences, want 2: %+v", len(got), got)
	}

	byParticipant := map[uuid.UUID]models.Occurrence{}
	for _, occurrence := range got {
		byParticipant[occurrence.ParticipantID] = occurrence
	}

	// The recurrence's own times come through — a summary that lost them would show an
	// evening slot as an all-day one, and the overlap count would be wrong with it.
	fromRecurrence, ok := byParticipant[f.other.ID]
	if !ok {
		t.Fatal("the recurrence produced no occurrence")
	}
	if fromRecurrence.StartTime == nil || *fromRecurrence.StartTime != "18:00" {
		t.Errorf("start = %v, want 18:00", fromRecurrence.StartTime)
	}
	if fromRecurrence.EndTime == nil || *fromRecurrence.EndTime != "22:00" {
		t.Errorf("end = %v, want 22:00", fromRecurrence.EndTime)
	}
	if fromRecurrence.ParticipantName == "" {
		t.Error("the occurrence carries no participant name")
	}

	// An all-day answer stays all-day rather than being filled in with a whole-day span.
	manual, ok := byParticipant[f.participant.ID]
	if !ok {
		t.Fatal("the manual answer produced no occurrence")
	}
	if manual.StartTime != nil || manual.EndTime != nil {
		t.Errorf("the all-day answer gained times: %v..%v", manual.StartTime, manual.EndTime)
	}
}

// TestAManualAnswerSuppressesTheRecurrence is the precedence rule, and the reason a
// participant is never counted twice for one date. It held in three separate places
// before; it holds in one now.
func TestAManualAnswerSuppressesTheRecurrence(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	recurrences := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	monday := day(0)

	weekly := recurrence(f.participant.ID, int(time.Monday), ptr("18:00"), ptr("22:00"))
	if err := recurrences.CreateRecurrence(ctx, weekly); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}
	// The same participant then answers that date by hand, all day.
	if err := repo.Create(ctx, availability(f.participant.ID, monday, nil, nil)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetOccurrencesForDate(ctx, f.calendar.ID, monday)
	if err != nil {
		t.Fatalf("GetOccurrencesForDate: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d occurrences for one participant, want 1: %+v", len(got), got)
	}
	// Wholesale, not merged: the typed answer wins even though it is the vaguer one.
	if got[0].StartTime != nil || got[0].EndTime != nil {
		t.Errorf("the recurrence's times survived the manual answer: %v..%v", got[0].StartTime, got[0].EndTime)
	}
}

func TestOccurrencesForRangeCoverEveryDay(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	recurrences := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	weekly := recurrence(f.other.ID, int(time.Monday), nil, nil)
	if err := recurrences.CreateRecurrence(ctx, weekly); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}
	// An exception on the second Monday, so the range has to skip exactly one of them.
	exception := &models.RecurrenceException{RecurrenceID: weekly.ID, ExcludedDate: "2027-03-08"}
	exception.ID = uuid.New()
	if err := recurrences.CreateException(ctx, exception); err != nil {
		t.Fatalf("CreateException: %v", err)
	}

	// Three weeks: three Mondays, minus the excluded one.
	got, err := repo.GetOccurrencesForRange(ctx, f.calendar.ID, day(0), day(20))
	if err != nil {
		t.Fatalf("GetOccurrencesForRange: %v", err)
	}

	dates := map[string]bool{}
	for _, occurrence := range got {
		dates[occurrence.Date.Format("2006-01-02")] = true
	}
	for _, want := range []string{"2027-03-01", "2027-03-15"} {
		if !dates[want] {
			t.Errorf("the range is missing the Monday of %s: %v", want, dates)
		}
	}
	if dates["2027-03-08"] {
		t.Errorf("the excluded Monday is in the range: %v", dates)
	}
	// Ordered by date, so the caller never has to sort what SQL already knows.
	for i := 1; i < len(got); i++ {
		if got[i].Date.Before(got[i-1].Date) {
			t.Errorf("occurrence %d is out of order: %v before %v", i, got[i].Date, got[i-1].Date)
		}
	}
}

// TestCountMeasuresOverlapNotAnswers is the behaviour change. Three people answering for
// the same day at hours that never meet is not three people who can meet.
func TestCountMeasuresOverlapNotAnswers(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	monday := day(0)

	if err := repo.Create(ctx, availability(f.participant.ID, monday, ptr("08:00"), ptr("10:00"))); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, availability(f.other.ID, monday, ptr("18:00"), ptr("20:00"))); err != nil {
		t.Fatalf("Create: %v", err)
	}

	count, err := repo.GetParticipantCountForDate(ctx, f.calendar.ID, monday)
	if err != nil {
		t.Fatalf("GetParticipantCountForDate: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1: two people free the same day at hours that never meet", count)
	}

	// Both are still available that day — the list is a different question from the count.
	available, err := repo.GetAvailableParticipantsForDate(ctx, f.calendar.ID, monday)
	if err != nil {
		t.Fatalf("GetAvailableParticipantsForDate: %v", err)
	}
	if len(available) != 2 {
		t.Errorf("got %d available participants, want 2", len(available))
	}
}

// TestCountHonoursTheCalendarMinimum keeps the number the notification threshold reads
// and the number the page shows as one number. They were briefly two: the summary
// endpoints started counting only overlaps long enough to hold the event while this still
// counted any overlap, so a notification could announce a gathering the interface was
// already saying could not happen.
func TestCountHonoursTheCalendarMinimum(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	monday := day(0)

	// Half an hour together, on a calendar that wants two.
	if err := repo.Create(ctx, availability(f.participant.ID, monday, ptr("10:00"), ptr("13:30"))); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Create(ctx, availability(f.other.ID, monday, ptr("13:00"), ptr("18:00"))); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE calendars SET min_duration_hours = 2 WHERE id = $1`, f.calendar.ID); err != nil {
		t.Fatalf("set the minimum: %v", err)
	}

	count, err := repo.GetParticipantCountForDate(ctx, f.calendar.ID, monday)
	if err != nil {
		t.Fatalf("GetParticipantCountForDate: %v", err)
	}
	// Each is free long enough alone; the pair is not.
	if count != 1 {
		t.Errorf("count = %d, want 1: half an hour together cannot hold a two-hour event", count)
	}
}

// TestCountIsUnchangedForWholeDayAnswers bounds the change above: a calendar answered in
// whole days — the common case — counts exactly as it always did.
func TestCountIsUnchangedForWholeDayAnswers(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewAvailabilityRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	monday := day(0)

	for _, id := range []uuid.UUID{f.participant.ID, f.other.ID} {
		if err := repo.Create(ctx, availability(id, monday, nil, nil)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	count, err := repo.GetParticipantCountForDate(ctx, f.calendar.ID, monday)
	if err != nil {
		t.Fatalf("GetParticipantCountForDate: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2: all-day answers span the whole day", count)
	}
}
