// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/notify/models"
	"github.com/whento/whento/internal/notify/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// The reliability of the reminder scheduler lives in this table: atomic claiming
// across instances, idempotent enqueue, retry/backoff and permanent failure.
// None of that can be proven with fakes, so these tests need a real database.

func newReminderJob(t *testing.T, pool *pgxpool.Pool) *models.ReminderJob {
	t.Helper()
	calendar := newCalendar(t, pool)
	return &models.ReminderJob{
		CalendarID:    calendar.ID,
		EventDate:     time.Date(2027, 4, 10, 0, 0, 0, 0, time.UTC),
		RecipientType: models.ReminderRecipientOwner,
		Channel:       "email",
		ScheduledAt:   time.Date(2027, 4, 9, 0, 0, 0, 0, time.UTC),
		MaxAttempts:   5,
		NextAttemptAt: time.Date(2027, 4, 9, 0, 0, 0, 0, time.UTC),
	}
}

func TestReminderJobEnqueueIsIdempotent(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewReminderJobRepository(pool)
	ctx := dbtest.Context(t)

	job := newReminderJob(t, pool)

	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue (duplicate): %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reminder_jobs WHERE calendar_id = $1 AND event_date = $2`,
		job.CalendarID, job.EventDate).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("%d rows exist for the same delivery key, want exactly 1 (idempotent enqueue)", count)
	}
}

func TestReminderJobClaimIsAtomicAndScopedToDue(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewReminderJobRepository(pool)
	ctx := dbtest.Context(t)

	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)

	due := newReminderJob(t, pool)
	due.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, due); err != nil {
		t.Fatalf("Enqueue due: %v", err)
	}

	notYet := newReminderJob(t, pool)
	notYet.RecipientType = models.ReminderRecipientParticipants
	notYet.NextAttemptAt = now.Add(time.Hour)
	if err := repo.Enqueue(ctx, notYet); err != nil {
		t.Fatalf("Enqueue not-yet: %v", err)
	}

	claimed, err := repo.ClaimDue(ctx, "instance-a", now, 10*time.Minute, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d jobs, want 1 (only the due one)", len(claimed))
	}
	if !sameDeliveryKey(claimed[0], *due) {
		t.Errorf("claimed the wrong job: %+v", claimed[0])
	}
	if claimed[0].LockedBy != "instance-a" {
		t.Errorf("LockedBy = %q, want instance-a", claimed[0].LockedBy)
	}

	// A job locked by another instance within the TTL is not claimable.
	also, err := repo.ClaimDue(ctx, "instance-b", now.Add(time.Minute), 10*time.Minute, 10)
	if err != nil {
		t.Fatalf("second ClaimDue: %v", err)
	}
	if len(also) != 0 {
		t.Errorf("a job locked by another instance within its TTL was claimed: %+v", also)
	}

	// After the lock TTL expires the same job is claimable again (crash recovery).
	reclaimed, err := repo.ClaimDue(ctx, "instance-b", now.Add(11*time.Minute), 10*time.Minute, 10)
	if err != nil {
		t.Fatalf("reclaim after lock expiry: %v", err)
	}
	if len(reclaimed) != 1 || !sameDeliveryKey(reclaimed[0], *due) {
		t.Errorf("the expired-lock job was not reclaimed: %+v", reclaimed)
	}
}

// sameDeliveryKey reports whether two jobs describe the same delivery: the four
// columns that form reminder_jobs' unique constraint. The ID itself is generated
// by the database on insert, so a test cannot own it ahead of time — comparing
// the delivery key is the identity the enqueueing test actually controls.
func sameDeliveryKey(a, b models.ReminderJob) bool {
	return a.CalendarID == b.CalendarID &&
		a.EventDate.Equal(b.EventDate) &&
		a.RecipientType == b.RecipientType &&
		a.Channel == b.Channel
}

func TestReminderJobLifecycle(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewReminderJobRepository(pool)
	ctx := dbtest.Context(t)

	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)
	job := newReminderJob(t, pool)
	job.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	claimed, err := repo.ClaimDue(ctx, "instance-a", now, 10*time.Minute, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimDue: %v (%d)", err, len(claimed))
	}

	if err := repo.MarkSent(ctx, claimed[0].ID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM reminder_jobs WHERE id = $1`, claimed[0].ID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != models.ReminderJobSent {
		t.Errorf("status after MarkSent = %q, want %q", status, models.ReminderJobSent)
	}

	// Fail the same job again (reset to pending), retry with backoff, then exhaust.
	if _, err := pool.Exec(ctx, `UPDATE reminder_jobs SET status = 'pending', attempt = 0 WHERE id = $1`, claimed[0].ID); err != nil {
		t.Fatalf("reset job: %v", err)
	}

	next := now.Add(time.Hour)
	if err := repo.MarkFailed(ctx, claimed[0].ID, 1, 5, next, "smtp down"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if err := repo.MarkFailed(ctx, claimed[0].ID, 5, 5, next, "smtp still down"); err != nil {
		t.Fatalf("MarkFailed (permanent): %v", err)
	}

	if err := pool.QueryRow(ctx, `SELECT status FROM reminder_jobs WHERE id = $1`, claimed[0].ID).Scan(&status); err != nil {
		t.Fatalf("read status after failures: %v", err)
	}
	if status != models.ReminderJobFailed {
		t.Errorf("status after exhausting attempts = %q, want %q", status, models.ReminderJobFailed)
	}
}

func TestReminderJobCancelPendingForEvent(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewReminderJobRepository(pool)
	ctx := dbtest.Context(t)

	job := newReminderJob(t, pool)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := repo.Enqueue(ctx, &models.ReminderJob{
		CalendarID:    job.CalendarID,
		EventDate:     job.EventDate.AddDate(0, 0, 1),
		RecipientType: models.ReminderRecipientParticipants,
		Channel:       "email",
		ScheduledAt:   job.ScheduledAt.AddDate(0, 0, 1),
		MaxAttempts:   5,
		NextAttemptAt: job.NextAttemptAt.AddDate(0, 0, 1),
	}); err != nil {
		t.Fatalf("Enqueue other event: %v", err)
	}

	if err := repo.CancelPendingForEvent(ctx, job.CalendarID, job.EventDate); err != nil {
		t.Fatalf("CancelPendingForEvent: %v", err)
	}

	var pending int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reminder_jobs WHERE calendar_id = $1 AND status = 'pending'`,
		job.CalendarID).Scan(&pending); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	// One stays (the other event date), the target event was canceled.
	if pending != 1 {
		t.Errorf("%d jobs still pending, want 1 (the cancel only hit its own event)", pending)
	}
}
