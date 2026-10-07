// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"testing"
	"time"

	"github.com/whento/whento/internal/availability/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// What only a real database shows here is the upsert itself. The whole design of this
// table is that two writers touch one row without clobbering each other — a join
// replaces the join slot and leaves the withdrawal alone, and vice versa — and that is
// a property of the ON CONFLICT clauses, not of any Go code. A fake would assert the
// shape of a struct and prove nothing.

func TestRecordJoinOverwritesInPlace(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewActivityLogRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	date := day(0)

	first := time.Date(2027, 2, 1, 10, 0, 0, 0, time.UTC)
	second := time.Date(2027, 2, 1, 11, 0, 0, 0, time.UTC)

	if err := repo.RecordJoin(ctx, f.calendar.ID, date, f.participant.ID, first); err != nil {
		t.Fatalf("RecordJoin (first): %v", err)
	}
	if err := repo.RecordJoin(ctx, f.calendar.ID, date, f.other.ID, second); err != nil {
		t.Fatalf("RecordJoin (second): %v", err)
	}

	// One row, not two: the primary key is (calendar_id, date), and the journal is
	// deliberately not a history.
	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM date_activity_log WHERE calendar_id = $1 AND date = $2`,
		f.calendar.ID, date,
	).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}

	activity, err := repo.GetForDate(ctx, f.calendar.ID, date)
	if err != nil {
		t.Fatalf("GetForDate: %v", err)
	}
	if activity == nil || activity.LastJoined == nil {
		t.Fatalf("LastJoined = nil, want the second participant")
	}
	if activity.LastJoined.ID != f.other.ID {
		t.Errorf("LastJoined.ID = %v, want %v", activity.LastJoined.ID, f.other.ID)
	}
	// The name is joined at read time; nothing in the table holds one.
	if activity.LastJoined.Name != f.other.Name {
		t.Errorf("LastJoined.Name = %q, want %q", activity.LastJoined.Name, f.other.Name)
	}
	if !activity.LastJoined.At.Equal(second) {
		t.Errorf("LastJoined.At = %v, want %v", activity.LastJoined.At, second)
	}
}

func TestRecordWithdrawalLeavesTheJoinSlotAlone(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewActivityLogRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	date := day(1)

	joinedAt := time.Date(2027, 2, 2, 9, 0, 0, 0, time.UTC)
	withdrewAt := time.Date(2027, 2, 2, 18, 0, 0, 0, time.UTC)

	if err := repo.RecordJoin(ctx, f.calendar.ID, date, f.participant.ID, joinedAt); err != nil {
		t.Fatalf("RecordJoin: %v", err)
	}
	if err := repo.RecordWithdrawal(ctx, f.calendar.ID, date, f.other.ID, withdrewAt); err != nil {
		t.Fatalf("RecordWithdrawal: %v", err)
	}

	activity, err := repo.GetForDate(ctx, f.calendar.ID, date)
	if err != nil {
		t.Fatalf("GetForDate: %v", err)
	}
	if activity == nil {
		t.Fatal("GetForDate = nil, want a row")
	}

	// The two slots are independent. A single upsert writing all four columns would
	// have silently cleared this one.
	if activity.LastJoined == nil || activity.LastJoined.ID != f.participant.ID {
		t.Errorf("LastJoined = %v, want %v", activity.LastJoined, f.participant.ID)
	}
	if activity.LastWithdrawn == nil || activity.LastWithdrawn.ID != f.other.ID {
		t.Errorf("LastWithdrawn = %v, want %v", activity.LastWithdrawn, f.other.ID)
	}
	if activity.LastWithdrawn != nil && !activity.LastWithdrawn.At.Equal(withdrewAt) {
		t.Errorf("LastWithdrawn.At = %v, want %v", activity.LastWithdrawn.At, withdrewAt)
	}
}

func TestDeletingAParticipantForgetsTheEntry(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewActivityLogRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	date := day(2)

	at := time.Date(2027, 2, 3, 9, 0, 0, 0, time.UTC)
	if err := repo.RecordJoin(ctx, f.calendar.ID, date, f.participant.ID, at); err != nil {
		t.Fatalf("RecordJoin: %v", err)
	}
	if err := repo.RecordWithdrawal(ctx, f.calendar.ID, date, f.other.ID, at); err != nil {
		t.Fatalf("RecordWithdrawal: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM participants WHERE id = $1`, f.participant.ID); err != nil {
		t.Fatalf("delete participant: %v", err)
	}

	activity, err := repo.GetForDate(ctx, f.calendar.ID, date)
	if err != nil {
		t.Fatalf("GetForDate: %v", err)
	}
	if activity == nil {
		t.Fatal("GetForDate = nil, want the row to survive")
	}

	// ON DELETE SET NULL, not CASCADE: the deleted participant is forgotten and the
	// other slot of the same row is untouched. The stranded last_joined_at is never
	// exposed, because a ref needs all three of id, name and timestamp.
	if activity.LastJoined != nil {
		t.Errorf("LastJoined = %v, want nil after the participant was deleted", activity.LastJoined)
	}
	if activity.LastWithdrawn == nil || activity.LastWithdrawn.ID != f.other.ID {
		t.Errorf("LastWithdrawn = %v, want %v", activity.LastWithdrawn, f.other.ID)
	}
}

func TestGetForRangeIsBoundedByTheRange(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewActivityLogRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)
	at := time.Date(2027, 2, 4, 9, 0, 0, 0, time.UTC)

	for _, offset := range []int{0, 3, 10} {
		if err := repo.RecordJoin(ctx, f.calendar.ID, day(offset), f.participant.ID, at); err != nil {
			t.Fatalf("RecordJoin(%d): %v", offset, err)
		}
	}

	entries, err := repo.GetForRange(ctx, f.calendar.ID, day(0), day(3))
	if err != nil {
		t.Fatalf("GetForRange: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	for _, offset := range []int{0, 3} {
		key := day(offset).Format("2006-01-02")
		if entries[key] == nil {
			t.Errorf("entries[%s] = nil, want an entry", key)
		}
	}
	if entries[day(10).Format("2006-01-02")] != nil {
		t.Error("entries contains a date outside the range")
	}
}

func TestGetForDateWithoutARow(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewActivityLogRepository(pool)
	ctx := dbtest.Context(t)

	f := seed(t, pool)

	activity, err := repo.GetForDate(ctx, f.calendar.ID, day(30))
	if err != nil {
		t.Fatalf("GetForDate: %v", err)
	}
	if activity != nil {
		t.Errorf("GetForDate = %v, want nil for a date with no entry", activity)
	}
}
