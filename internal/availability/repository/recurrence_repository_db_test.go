// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// Recurrences are stored as DATE and TIME columns but read back as formatted strings via
// TO_CHAR. That conversion is the whole point of these tests: a recurrence whose dates
// come back in the wrong format silently stops expanding, and the participant's weekly
// availability disappears from the calendar without an error anywhere.

func recurrence(participantID uuid.UUID, dayOfWeek int, start, end *string) *models.Recurrence {
	rec := &models.Recurrence{
		ParticipantID: participantID,
		DayOfWeek:     dayOfWeek,
		StartTime:     start,
		EndTime:       end,
		StartDate:     "2027-03-01",
		// CreatedAt is inserted explicitly rather than defaulted, so a zero value would
		// store a year-1 timestamp.
		CreatedAt: time.Date(2027, 2, 1, 12, 0, 0, 0, time.UTC),
	}
	rec.ID = uuid.New()

	return rec
}

func TestRecurrenceRoundTrip(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	rec := recurrence(f.participant.ID, 3, ptr("09:00"), ptr("17:30"))
	rec.Note = "every Wednesday"
	rec.EndDate = ptr("2027-06-30")

	if err := repo.CreateRecurrence(ctx, rec); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}

	got, err := repo.GetRecurrenceByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetRecurrenceByID: %v", err)
	}

	// TO_CHAR is what turns the TIME column back into "HH:MM". Postgres would otherwise
	// hand back "09:00:00", which the frontend time inputs reject.
	if got.StartTime == nil || *got.StartTime != "09:00" {
		t.Errorf("StartTime = %v, want 09:00", got.StartTime)
	}
	if got.EndTime == nil || *got.EndTime != "17:30" {
		t.Errorf("EndTime = %v, want 17:30", got.EndTime)
	}
	if got.StartDate != "2027-03-01" {
		t.Errorf("StartDate = %q, want 2027-03-01", got.StartDate)
	}
	if got.EndDate == nil || *got.EndDate != "2027-06-30" {
		t.Errorf("EndDate = %v, want 2027-06-30", got.EndDate)
	}
	if got.DayOfWeek != 3 {
		t.Errorf("DayOfWeek = %d, want 3", got.DayOfWeek)
	}
	if got.Note != "every Wednesday" {
		t.Errorf("Note = %q", got.Note)
	}
}

func TestRecurrenceOpenEndedKeepsANullEndDate(t *testing.T) {
	// A recurrence with no end date runs for ever. Reading it back as a zero date rather
	// than nil would make the expansion stop at year 1.
	pool := dbtest.Pool(t)
	repo := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	rec := recurrence(f.participant.ID, 1, nil, nil)

	if err := repo.CreateRecurrence(ctx, rec); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}

	got, err := repo.GetRecurrenceByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetRecurrenceByID: %v", err)
	}
	if got.EndDate != nil {
		t.Errorf("EndDate = %v, want nil", *got.EndDate)
	}
	if got.StartTime != nil || got.EndTime != nil {
		t.Errorf("times = %v/%v, want both nil for an all-day recurrence", got.StartTime, got.EndTime)
	}
}

func TestGetRecurrenceByIDRejectsAnUnknownID(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewRecurrenceRepository(pool)

	if _, err := repo.GetRecurrenceByID(dbtest.Context(t), uuid.New()); err == nil {
		t.Error("an unknown recurrence id returned no error")
	}
}

// TestDayOfWeekIsBounded covers the CHECK constraint. Day 7 has no meaning and would
// expand to nothing, so it must be refused at write time rather than read time.
func TestDayOfWeekIsBounded(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	for _, day := range []int{-1, 7} {
		if err := repo.CreateRecurrence(ctx, recurrence(f.participant.ID, day, nil, nil)); err == nil {
			t.Errorf("day_of_week %d was accepted", day)
		}
	}
	// The whole valid range must go through.
	for day := range 7 {
		if err := repo.CreateRecurrence(ctx, recurrence(f.participant.ID, day, nil, nil)); err != nil {
			t.Errorf("day_of_week %d was refused: %v", day, err)
		}
	}
}

func TestGetRecurrencesByParticipantAndByCalendar(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	// Two for one participant, one for the other. Nothing in the schema stops a
	// participant having several recurrences on the same weekday.
	for _, day := range []int{5, 2} {
		if err := repo.CreateRecurrence(ctx, recurrence(f.participant.ID, day, ptr("08:00"), ptr("09:00"))); err != nil {
			t.Fatalf("CreateRecurrence: %v", err)
		}
	}
	if err := repo.CreateRecurrence(ctx, recurrence(f.other.ID, 4, nil, nil)); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}

	mine, err := repo.GetRecurrencesByParticipant(ctx, f.participant.ID)
	if err != nil {
		t.Fatalf("GetRecurrencesByParticipant: %v", err)
	}
	if len(mine) != 2 {
		t.Fatalf("GetRecurrencesByParticipant returned %d, want 2", len(mine))
	}
	// Ordered by day of week, so the Tuesday comes before the Friday however they were
	// written. The calendar draws them in that order.
	if mine[0].DayOfWeek != 2 || mine[1].DayOfWeek != 5 {
		t.Errorf("days = %d then %d, want 2 then 5", mine[0].DayOfWeek, mine[1].DayOfWeek)
	}

	// The calendar query joins through participants; it must pick up everyone's.
	all, err := repo.GetRecurrencesByCalendar(ctx, f.calendar.ID)
	if err != nil {
		t.Fatalf("GetRecurrencesByCalendar: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("GetRecurrencesByCalendar returned %d, want 3", len(all))
	}

	// A calendar nobody belongs to gets nothing rather than everything.
	empty, err := repo.GetRecurrencesByCalendar(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetRecurrencesByCalendar for an unknown calendar: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("an unknown calendar returned %d recurrences", len(empty))
	}
}

func TestUpdateAndDeleteRecurrence(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	rec := recurrence(f.participant.ID, 1, ptr("09:00"), ptr("10:00"))
	if err := repo.CreateRecurrence(ctx, rec); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}

	rec.DayOfWeek = 6
	rec.StartTime = ptr("14:00")
	rec.EndTime = nil
	rec.Note = "moved to Saturday"
	rec.EndDate = ptr("2027-12-31")
	if err := repo.UpdateRecurrence(ctx, rec); err != nil {
		t.Fatalf("UpdateRecurrence: %v", err)
	}

	got, err := repo.GetRecurrenceByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetRecurrenceByID: %v", err)
	}
	if got.DayOfWeek != 6 || got.Note != "moved to Saturday" {
		t.Errorf("the update did not persist: %+v", got)
	}
	// Clearing one end of the range has to actually write NULL, not leave the old value.
	if got.EndTime != nil {
		t.Errorf("EndTime = %v, want nil after being cleared", *got.EndTime)
	}

	// Updating something that is not there is an error rather than a silent no-op: the
	// caller would otherwise report success for a recurrence it never changed.
	missing := recurrence(f.participant.ID, 1, nil, nil)
	if err := repo.UpdateRecurrence(ctx, missing); err == nil {
		t.Error("updating an unknown recurrence reported success")
	}

	if err := repo.DeleteRecurrence(ctx, rec.ID); err != nil {
		t.Fatalf("DeleteRecurrence: %v", err)
	}
	if err := repo.DeleteRecurrence(ctx, rec.ID); err == nil {
		t.Error("deleting the same recurrence twice reported success")
	}
}

// TestRecurrenceExceptions covers the opt-out path: a single skipped occurrence in an
// otherwise weekly commitment. Without it the participant would have to delete and
// recreate the whole recurrence.
func TestRecurrenceExceptions(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	rec := recurrence(f.participant.ID, 1, nil, nil)
	if err := repo.CreateRecurrence(ctx, rec); err != nil {
		t.Fatalf("CreateRecurrence: %v", err)
	}

	newException := func(date string) *models.RecurrenceException {
		exc := &models.RecurrenceException{
			RecurrenceID: rec.ID,
			ExcludedDate: date,
			CreatedAt:    time.Date(2027, 2, 1, 12, 0, 0, 0, time.UTC),
		}
		exc.ID = uuid.New()

		return exc
	}

	for _, date := range []string{"2027-03-15", "2027-03-08"} {
		if err := repo.CreateException(ctx, newException(date)); err != nil {
			t.Fatalf("CreateException %s: %v", date, err)
		}
	}

	// UNIQUE(recurrence_id, excluded_date): excluding the same day twice would make the
	// exception count disagree with the number of days actually skipped.
	if err := repo.CreateException(ctx, newException("2027-03-15")); err == nil {
		t.Error("the same date was excluded twice")
	}

	got, err := repo.GetExceptionsByRecurrence(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetExceptionsByRecurrence: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetExceptionsByRecurrence returned %d, want 2", len(got))
	}
	if got[0].ExcludedDate != "2027-03-08" || got[1].ExcludedDate != "2027-03-15" {
		t.Errorf("exceptions = %q then %q, want them in date order", got[0].ExcludedDate, got[1].ExcludedDate)
	}

	if err := repo.DeleteException(ctx, rec.ID, "2027-03-08"); err != nil {
		t.Fatalf("DeleteException: %v", err)
	}
	if err := repo.DeleteException(ctx, rec.ID, "2027-03-08"); err == nil {
		t.Error("deleting an absent exception reported success")
	}

	// Deleting the recurrence takes its exceptions with it; orphaned exclusions would
	// silently apply to whatever recurrence next took that id.
	if err := repo.DeleteRecurrence(ctx, rec.ID); err != nil {
		t.Fatalf("DeleteRecurrence: %v", err)
	}
	left, err := repo.GetExceptionsByRecurrence(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetExceptionsByRecurrence after delete: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("%d exceptions outlived their recurrence", len(left))
	}
}

// TestGetExceptionsByRecurrenceIDs covers the batched read that replaced a
// GetExceptionsByRecurrence call per recurrence on the two summary endpoints. The
// grouping is done in Go from a single result set, so the risk is a row landing under
// the wrong recurrence — which would make one participant's exclusions silently apply
// to another's weekly slot.
func TestGetExceptionsByRecurrenceIDs(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewRecurrenceRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	withExceptions := recurrence(f.participant.ID, 1, nil, nil)
	alsoWithExceptions := recurrence(f.other.ID, 2, nil, nil)
	withoutExceptions := recurrence(f.participant.ID, 3, nil, nil)
	for _, rec := range []*models.Recurrence{withExceptions, alsoWithExceptions, withoutExceptions} {
		if err := repo.CreateRecurrence(ctx, rec); err != nil {
			t.Fatalf("CreateRecurrence: %v", err)
		}
	}

	addException := func(recurrenceID uuid.UUID, date string) {
		t.Helper()

		exc := &models.RecurrenceException{
			RecurrenceID: recurrenceID,
			ExcludedDate: date,
			CreatedAt:    time.Date(2027, 2, 1, 12, 0, 0, 0, time.UTC),
		}
		exc.ID = uuid.New()

		if err := repo.CreateException(ctx, exc); err != nil {
			t.Fatalf("CreateException %s: %v", date, err)
		}
	}

	addException(withExceptions.ID, "2027-03-15")
	addException(withExceptions.ID, "2027-03-08")
	addException(alsoWithExceptions.ID, "2027-04-05")

	t.Run("groups every recurrence in one query", func(t *testing.T) {
		got, err := repo.GetExceptionsByRecurrenceIDs(ctx, []uuid.UUID{
			withExceptions.ID, alsoWithExceptions.ID, withoutExceptions.ID,
		})
		if err != nil {
			t.Fatalf("GetExceptionsByRecurrenceIDs: %v", err)
		}

		mine := got[withExceptions.ID]
		if len(mine) != 2 {
			t.Fatalf("got %d exceptions for the first recurrence, want 2", len(mine))
		}
		// Same date ordering as the single-recurrence read, which the summaries rely on.
		if mine[0].ExcludedDate != "2027-03-08" || mine[1].ExcludedDate != "2027-03-15" {
			t.Errorf("dates = %q then %q, want them in date order", mine[0].ExcludedDate, mine[1].ExcludedDate)
		}
		for _, exc := range mine {
			if exc.RecurrenceID != withExceptions.ID {
				t.Errorf("exception %s is filed under the wrong recurrence", exc.ID)
			}
		}

		theirs := got[alsoWithExceptions.ID]
		if len(theirs) != 1 || theirs[0].ExcludedDate != "2027-04-05" {
			t.Errorf("second recurrence = %v, want one exception on 2027-04-05", theirs)
		}

		// A recurrence with no exception is absent rather than present-and-empty, which
		// is what GetExceptionsByRecurrence returns too.
		if got, ok := got[withoutExceptions.ID]; ok || got != nil {
			t.Errorf("a recurrence with no exception yielded %v", got)
		}
	})

	t.Run("matches the single-recurrence read", func(t *testing.T) {
		one, err := repo.GetExceptionsByRecurrence(ctx, withExceptions.ID)
		if err != nil {
			t.Fatalf("GetExceptionsByRecurrence: %v", err)
		}

		batched, err := repo.GetExceptionsByRecurrenceIDs(ctx, []uuid.UUID{withExceptions.ID})
		if err != nil {
			t.Fatalf("GetExceptionsByRecurrenceIDs: %v", err)
		}

		got := batched[withExceptions.ID]
		if len(got) != len(one) {
			t.Fatalf("batched returned %d exceptions, single returned %d", len(got), len(one))
		}
		for i := range one {
			if got[i] != one[i] {
				t.Errorf("exception %d: batched = %+v, single = %+v", i, got[i], one[i])
			}
		}
	})

	t.Run("no ids means no query and no rows", func(t *testing.T) {
		got, err := repo.GetExceptionsByRecurrenceIDs(ctx, nil)
		if err != nil {
			t.Fatalf("GetExceptionsByRecurrenceIDs(nil): %v", err)
		}
		if len(got) != 0 {
			t.Errorf("an empty id list returned %d groups", len(got))
		}
	})

	t.Run("unknown ids contribute nothing", func(t *testing.T) {
		got, err := repo.GetExceptionsByRecurrenceIDs(ctx, []uuid.UUID{uuid.New()})
		if err != nil {
			t.Fatalf("GetExceptionsByRecurrenceIDs: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("an unknown recurrence returned %d groups", len(got))
		}
	})
}
