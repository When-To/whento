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
