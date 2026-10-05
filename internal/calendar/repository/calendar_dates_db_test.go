// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

func TestConcurrentDatePatchesCannotInvertTheStoredRange(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewCalendarRepository(pool)
	owner := newOwner(t, pool)
	calendar := newCalendar(owner.ID)
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 10)
	calendar.StartDate, calendar.EndDate = &start, &end
	if err := repo.Create(ctx, calendar); err != nil {
		t.Fatal(err)
	}
	dbtest.CleanupContext(ctx, t, pool, `DELETE FROM calendars WHERE id = $1`, calendar.ID)
	newStart, newEnd := start.AddDate(0, 0, 8), start.AddDate(0, 0, 5)
	// Both requests are valid against the initial snapshot, but not together.
	begin := make(chan struct{})
	results := make(chan error, 2)
	for _, patch := range []repository.CalendarPatch{{StartDate: &newStart}, {EndDate: &newEnd}} {
		go func() { <-begin; _, err := repo.Patch(ctx, calendar.ID, patch); results <- err }()
	}
	close(begin)
	failures := 0
	for range 2 {
		if err := <-results; err != nil {
			if !errors.Is(err, repository.ErrInvalidDateRange) {
				t.Fatalf("expected domain validation, not a raw database constraint error: %v", err)
			}
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("rejected %d patches, want exactly one", failures)
	}
	got, err := repo.GetByID(ctx, calendar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EndDate.Before(*got.StartDate) {
		t.Fatalf("stored inverted dates: %v..%v", got.StartDate, got.EndDate)
	}
}

func TestInvalidDatePatchRollsBackEverySuppliedField(t *testing.T) {
	for _, withHours := range []bool{false, true} {
		t.Run(map[bool]string{false: "dates only", true: "dates and allowed hours"}[withHours], func(t *testing.T) {
			pool := dbtest.Pool(t)
			ctx := dbtest.Context(t)
			repo := repository.NewCalendarRepository(pool)
			owner := newOwner(t, pool)
			calendar := newCalendar(owner.ID)
			start := time.Date(2030, 1, 8, 0, 0, 0, 0, time.UTC)
			calendar.StartDate = &start
			if err := repo.Create(ctx, calendar); err != nil {
				t.Fatal(err)
			}
			dbtest.CleanupContext(ctx, t, pool, `DELETE FROM calendars WHERE id = $1`, calendar.ID)
			end, name, hour := start.AddDate(0, 0, -1), "must not persist", "10:00"
			patch := repository.CalendarPatch{EndDate: &end, Name: &name}
			if withHours {
				patch.AllowedHours = &repository.AllowedHoursPatch{HolidayMinTime: &hour}
			}
			if _, err := repo.Patch(ctx, calendar.ID, patch); err == nil {
				t.Fatal("invalid dates were accepted")
			} else if !errors.Is(err, repository.ErrInvalidDateRange) {
				t.Fatalf("expected domain validation, not a raw database constraint error: %v", err)
			}
			got, err := repo.GetByID(ctx, calendar.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != calendar.Name || got.EndDate != nil || got.AllowedHours != nil {
				t.Fatalf("partial patch persisted: %+v", got)
			}
		})
	}
}
