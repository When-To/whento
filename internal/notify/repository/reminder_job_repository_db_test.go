// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/notify/models"
	"github.com/whento/whento/internal/notify/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// The reliability of the reminder scheduler lives in this table: atomic claiming
// across deliveries (a UUID claim token, not an instance id), idempotent
// reconciling enqueue, retry/backoff, permanent failure and the immutability of
// a sent row. None of that can be proven with fakes, so these tests need a real
// database.

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
	if claimed[0].ClaimToken == [16]byte{} {
		t.Error("ClaimDue did not hand out a claim token")
	}
	if !claimed[0].LeaseUntil.After(now) {
		t.Errorf("LeaseUntil = %v, want after claim time %v", claimed[0].LeaseUntil, now)
	}

	// A job claimed within its lease is not claimable, even by the same
	// instance: the token is the fence, not the instance id.
	same, err := repo.ClaimDue(ctx, "instance-a", now.Add(time.Minute), 10*time.Minute, 10)
	if err != nil {
		t.Fatalf("second ClaimDue: %v", err)
	}
	if len(same) != 0 {
		t.Errorf("the same instance claimed its own live lease a second time: %+v", same)
	}

	// After the lease expires the same job is claimable again (crash recovery),
	// under a fresh token.
	reclaimed, err := repo.ClaimDue(ctx, "instance-b", now.Add(11*time.Minute), 10*time.Minute, 10)
	if err != nil {
		t.Fatalf("reclaim after lock expiry: %v", err)
	}
	if len(reclaimed) != 1 || !sameDeliveryKey(reclaimed[0], *due) {
		t.Errorf("the expired-lease job was not reclaimed: %+v", reclaimed)
	}
	if reclaimed[0].ClaimToken == claimed[0].ClaimToken {
		t.Error("a re-claim reused the previous claim token; a stale worker could still fence on it")
	}
	if reclaimed[0].ClaimToken == [16]byte{} {
		t.Error("reclaim did not hand out a new claim token")
	}
}

// TestReminderJobTwoWorkersSameInstanceSingleWinner is the fence in its sharpest
// form: two workers that share one instance id (two goroutines, two pods on one
// host, a redeploy overlapping the old process) claim concurrently, and exactly
// one of them wins the delivery.
func TestReminderJobTwoWorkersSameInstanceSingleWinner(t *testing.T) {
	pool := dbtest.Pool(t)
	repo := repository.NewReminderJobRepository(pool)
	ctx := dbtest.Context(t)

	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)
	job := newReminderJob(t, pool)
	job.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	start := make(chan struct{})
	var mu sync.Mutex
	claims := make([]int, 0, 2)
	errs := make([]error, 0, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := repo.ClaimDue(ctx, "same-instance", now, 10*time.Minute, 10)
			mu.Lock()
			claims = append(claims, len(got))
			errs = append(errs, err)
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent ClaimDue: %v", err)
		}
	}
	winners := 0
	for _, n := range claims {
		winners += n
	}
	if winners != 1 {
		t.Errorf("two concurrent same-instance workers claimed %d deliveries total, want exactly 1", winners)
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

	if err := repo.MarkSent(ctx, claimed[0].ID, claimed[0].ClaimToken); err != nil {
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

	// A stale worker with the old token cannot touch a sent row.
	if err := repo.MarkFailed(ctx, claimed[0].ID, uuid.New(), 1, 5, now, "late worker"); err == nil {
		t.Error("MarkFailed with a foreign token on a sent row succeeded; a sent row must be immutable")
	}

	// Fail the same job again (reset to pending), retry with backoff, then
	// exhaust. MarkSent cleared the claim, so the retry path re-claims to
	// obtain the live token every fenced failure has to present.
	next := now.Add(time.Hour)
	if _, err := pool.Exec(ctx,
		`UPDATE reminder_jobs SET status = 'pending', attempt = 0, next_attempt_at = $2,
		        claim_token = NULL, lease_until = NULL WHERE id = $1`,
		claimed[0].ID, now); err != nil {
		t.Fatalf("reset job: %v", err)
	}

	reclaimed, err := repo.ClaimDue(ctx, "instance-a", now.Add(time.Minute), 10*time.Minute, 10)
	if err != nil || len(reclaimed) != 1 {
		t.Fatalf("re-claim after reset: %v (%d)", err, len(reclaimed))
	}
	if err := repo.MarkFailed(ctx, reclaimed[0].ID, reclaimed[0].ClaimToken, 1, 5, next, "smtp down"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT status FROM reminder_jobs WHERE id = $1`, reclaimed[0].ID).Scan(&status); err != nil {
		t.Fatalf("read status after failure: %v", err)
	}
	if status != models.ReminderJobPending {
		t.Errorf("status after a retryable failure = %q, want %q", status, models.ReminderJobPending)
	}

	// Exhaust: reset to a high attempt count and a fresh claim, then fail past
	// max_attempts -> the row is permanently failed.
	if _, err := pool.Exec(ctx,
		`UPDATE reminder_jobs SET status = 'pending', attempt = 4, next_attempt_at = $2,
		        claim_token = NULL, lease_until = NULL WHERE id = $1`,
		reclaimed[0].ID, now); err != nil {
		t.Fatalf("reset job to near-max: %v", err)
	}
	reclaimed2, err := repo.ClaimDue(ctx, "instance-a", now.Add(2*time.Minute), 10*time.Minute, 10)
	if err != nil || len(reclaimed2) != 1 {
		t.Fatalf("re-claim before exhausting: %v (%d)", err, len(reclaimed2))
	}
	if err := repo.MarkFailed(ctx, reclaimed2[0].ID, reclaimed2[0].ClaimToken, 5, 5, next, "smtp still down"); err != nil {
		t.Fatalf("MarkFailed (permanent): %v", err)
	}

	if err := pool.QueryRow(ctx, `SELECT status FROM reminder_jobs WHERE id = $1`, reclaimed2[0].ID).Scan(&status); err != nil {
		t.Fatalf("read status after failures: %v", err)
	}
	if status != models.ReminderJobFailed {
		t.Errorf("status after exhausting attempts = %q, want %q", status, models.ReminderJobFailed)
	}

	// An exhausted failed row stays terminal: a later scan must not rearm it.
	if err := repo.Enqueue(ctx, &models.ReminderJob{
		CalendarID:    job.CalendarID,
		EventDate:     job.EventDate,
		RecipientType: job.RecipientType,
		Channel:       job.Channel,
		ScheduledAt:   job.ScheduledAt,
		MaxAttempts:   5,
		NextAttemptAt: job.ScheduledAt,
	}); err != nil {
		t.Fatalf("Enqueue after permanent failure: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM reminder_jobs WHERE id = $1`, claimed[0].ID).Scan(&status); err != nil {
		t.Fatalf("read status after re-enqueue: %v", err)
	}
	if status != models.ReminderJobFailed {
		t.Errorf("a routine scan rearmed an exhausted failed row: %q", status)
	}
}

// TestReminderStaleWorkerMustNotResurrectSentJob is the audit repository probe:
// the old worker's claim expires, a new one claims and sends; when the late
// worker reports failure for the job it lost, the row must stay 'sent' and no
// third worker may reclaim it.
func TestReminderStaleWorkerMustNotResurrectSentJob(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)

	job := newReminderJob(t, pool)
	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)
	job.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}

	// A claims, B claims after A's lease expired.
	a, err := repo.ClaimDue(ctx, "worker-a", now, 10*time.Minute, 10)
	if err != nil || len(a) != 1 {
		t.Fatalf("claim A: %v (%d)", err, len(a))
	}
	b, err := repo.ClaimDue(ctx, "worker-b", now.Add(11*time.Minute), 10*time.Minute, 10)
	if err != nil || len(b) != 1 {
		t.Fatalf("claim B: %v (%d)", err, len(b))
	}

	// B delivers; A (late) reports a failure for the same job.
	if err := repo.MarkSent(ctx, b[0].ID, b[0].ClaimToken); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkFailed(ctx, a[0].ID, a[0].ClaimToken, 1, 5, now, "late worker A"); err == nil {
		t.Fatal("late worker A's MarkFailed succeeded; a lost claim must be a recognizable no-op")
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM reminder_jobs WHERE id = $1`, a[0].ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "sent" {
		t.Errorf("late A changed B's sent result to %s", status)
	}

	// A sent job is not claimable by anyone.
	c, err := repo.ClaimDue(ctx, "worker-c", now.Add(12*time.Minute), 10*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) > 0 {
		t.Errorf("worker C reclaimed already-sent job: %d", len(c))
	}
}

// TestReminderClaimFenceOnMarkFailedWithoutInterveningSend pins the token fence
// itself, not just the 'sent' guard: while B holds the live claim, A's fenced
// failure report must no-op, leaving the row pending and B's delivery intact.
func TestReminderClaimFenceOnMarkFailed(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)

	job := newReminderJob(t, pool)
	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)
	job.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}

	// A's lease expires, B claims while A is still mid-delivery.
	a, err := repo.ClaimDue(ctx, "worker-a", now, 10*time.Minute, 10)
	if err != nil || len(a) != 1 {
		t.Fatalf("claim A: %v (%d)", err, len(a))
	}
	b, err := repo.ClaimDue(ctx, "worker-b", now.Add(11*time.Minute), 10*time.Minute, 10)
	if err != nil || len(b) != 1 {
		t.Fatalf("claim B: %v (%d)", err, len(b))
	}

	if err := repo.MarkFailed(ctx, a[0].ID, a[0].ClaimToken, 2, 5, now.Add(time.Hour), "late worker A"); err == nil {
		t.Fatal("A's stale token mutated B's live claim; the token fence failed")
	}

	var status string
	var attempt int
	if err := pool.QueryRow(ctx,
		`SELECT status, attempt FROM reminder_jobs WHERE id = $1`, b[0].ID).Scan(&status, &attempt); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempt != 0 {
		t.Errorf("B's live claim was disturbed by A's stale write: status=%s attempt=%d", status, attempt)
	}

	// B completes its delivery.
	if err := repo.MarkSent(ctx, b[0].ID, b[0].ClaimToken); err != nil {
		t.Fatal(err)
	}
}

// TestReminderRenewLeaseIsFenced pins the pre-send lease renewal: a worker whose
// lease has lapsed cannot renew (its delivery is cancelled at the scheduler
// level, and the claim is reaped), and a stale token cannot renew either.
func TestReminderRenewLeaseIsFenced(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)

	job := newReminderJob(t, pool)
	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)
	job.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}

	a, err := repo.ClaimDue(ctx, "worker-a", now, 10*time.Minute, 10)
	if err != nil || len(a) != 1 {
		t.Fatalf("claim A: %v (%d)", err, len(a))
	}
	// A's lease is still valid, so its renewal works...
	if err := repo.RenewLease(ctx, a[0].ID, a[0].ClaimToken, now.Add(time.Minute), 5*time.Minute); err != nil {
		t.Fatalf("RenewLease within lease: %v", err)
	}
	// ...but a stale token cannot renew a live claim...
	if err := repo.RenewLease(ctx, a[0].ID, uuid.New(), now.Add(time.Minute), 5*time.Minute); err == nil {
		t.Fatal("RenewLease with a foreign token succeeded; the fence failed")
	}

	// ...and after the lease expires, even the original worker cannot renew.
	if err := repo.RenewLease(ctx, a[0].ID, a[0].ClaimToken, now.Add(11*time.Minute), 5*time.Minute); err == nil {
		t.Fatal("RenewLease with an expired lease succeeded; a lapsed worker must not deliver")
	}
}

// TestReminderCancelClaimIsFenced: a per-delivery cancel requires the claim
// token; a lost claim is a no-op, and the event's other deliveries live on.
func TestReminderCancelClaimIsFenced(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)

	job := newReminderJob(t, pool)
	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)
	job.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}

	a, err := repo.ClaimDue(ctx, "worker-a", now, 10*time.Minute, 10)
	if err != nil || len(a) != 1 {
		t.Fatalf("claim A: %v (%d)", err, len(a))
	}
	if err := repo.CancelClaim(ctx, a[0].ID, uuid.New()); err == nil {
		t.Fatal("CancelClaim with a foreign token succeeded; the fence failed")
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM reminder_jobs WHERE id = $1`, a[0].ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Errorf("foreign-token cancel disturbed the claim: %s", status)
	}

	if err := repo.CancelClaim(ctx, a[0].ID, a[0].ClaimToken); err != nil {
		t.Fatalf("CancelClaim with the live token: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM reminder_jobs WHERE id = $1`, a[0].ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "canceled" {
		t.Errorf("status after per-delivery cancel = %s, want canceled", status)
	}
}

// TestReminderCanceledDeliveryMustBeRearmable is the audit repository probe: a
// delivery canceled by a reconciliation action must come back to 'pending' the
// moment it is enqueued again within the delivery window.
func TestReminderCanceledDeliveryMustBeRearmable(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)

	job := newReminderJob(t, pool)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	cancelPending(ctx, t, pool, job)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}

	var status string
	var attempt int
	if err := pool.QueryRow(ctx,
		`SELECT status, attempt FROM reminder_jobs WHERE calendar_id = $1 AND event_date = $2`,
		job.CalendarID, job.EventDate).Scan(&status, &attempt); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Errorf("re-enqueue cannot rearm canceled delivery: %s", status)
	}
	if attempt != 0 {
		t.Errorf("rearmed delivery kept old failure state: attempt=%d", attempt)
	}
}

// TestReminderReconcilePreservesLiveClaimAndBackoff: a routine scan that
// re-enqueues jobs must neither steal a live claim nor reset an in-flight
// retry's backoff.
func TestReminderReconcilePreservesLiveClaimAndBackoff(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)

	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)
	job := newReminderJob(t, pool)
	job.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}

	claimed, err := repo.ClaimDue(ctx, "worker", now, 10*time.Minute, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v (%d)", err, len(claimed))
	}

	// A scan re-enqueues the same delivery key while the claim is live.
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatalf("re-enqueue over a live claim: %v", err)
	}

	var token uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT claim_token FROM reminder_jobs WHERE id = $1`, claimed[0].ID).Scan(&token); err != nil {
		t.Fatal(err)
	}
	if token != claimed[0].ClaimToken {
		t.Error("a routine scan stole a live claim")
	}

	// A pending row with an in-flight backoff keeps its next_attempt_at.
	if _, err := pool.Exec(ctx,
		`UPDATE reminder_jobs SET status = 'pending', attempt = 2, next_attempt_at = $2, claim_token = NULL,
		        lease_until = NULL, last_error = 'smtp down'
		 WHERE id = $1`, claimed[0].ID, now.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatalf("re-enqueue over backoff: %v", err)
	}

	var attempt int
	var next time.Time
	var lastErr string
	if err := pool.QueryRow(ctx,
		`SELECT attempt, next_attempt_at, COALESCE(last_error, '') FROM reminder_jobs WHERE id = $1`,
		claimed[0].ID).Scan(&attempt, &next, &lastErr); err != nil {
		t.Fatal(err)
	}
	if attempt != 2 || !next.Equal(now.Add(8*time.Minute)) || lastErr != "smtp down" {
		t.Errorf("a routine scan reset the retry history: attempt=%d next=%v err=%q", attempt, next, lastErr)
	}
}

// TestReminderReconcileRearmsCanceledButNeverSent pins the two immutable states:
// a canceled delivery re-arms on re-enqueue, a sent one never does.
func TestReminderReconcileRearmsCanceledButNeverSent(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)

	makePair := func() (*models.ReminderJob, *models.ReminderJob) {
		out := newReminderJob(t, pool)
		sibling := &models.ReminderJob{
			CalendarID:    out.CalendarID,
			EventDate:     out.EventDate,
			RecipientType: models.ReminderRecipientParticipants,
			Channel:       "email",
			ScheduledAt:   out.ScheduledAt,
			MaxAttempts:   5,
			NextAttemptAt: out.NextAttemptAt,
		}
		return out, sibling
	}

	// Canceled: re-enqueue rearms.
	canceledJob, canceledOther := makePair()
	if err := repo.Enqueue(ctx, canceledJob); err != nil {
		t.Fatal(err)
	}
	if err := repo.Enqueue(ctx, canceledOther); err != nil {
		t.Fatal(err)
	}
	cancelPending(ctx, t, pool, canceledJob)
	if err := repo.Enqueue(ctx, canceledJob); err != nil {
		t.Fatal(err)
	}
	if err := repo.Enqueue(ctx, canceledOther); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reminder_jobs WHERE calendar_id = $1 AND status = 'pending'`,
		canceledJob.CalendarID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 2 {
		t.Errorf("canceled deliveries were not both rearmed: pending=%d", pending)
	}

	// Sent: re-enqueue is a no-op that must not resurrect the row. The earlier
	// canceled-rearm phase left two pending rows behind, so the sent-path job is
	// located among the claimed batch by its calendar rather than assumed to be
	// the only claim.
	sentJob, _ := makePair()
	if err := repo.Enqueue(ctx, sentJob); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2027, 4, 9, 12, 0, 0, 0, time.UTC)
	sentJob.NextAttemptAt = now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, sentJob); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimDue(ctx, "worker", now, 10*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	var sentClaim *models.ReminderJob
	for i := range claimed {
		if claimed[i].CalendarID == sentJob.CalendarID {
			sentClaim = &claimed[i]
			break
		}
	}
	if sentClaim == nil {
		t.Fatalf("sent-path job was not claimed: %+v", claimed)
	}
	if err := repo.MarkSent(ctx, sentClaim.ID, sentClaim.ClaimToken); err != nil {
		t.Fatal(err)
	}
	if err := repo.Enqueue(ctx, sentJob); err != nil {
		t.Fatalf("re-enqueue over a sent row: %v", err)
	}
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM reminder_jobs WHERE id = $1`, sentClaim.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "sent" {
		t.Errorf("a routine scan rearmed a sent delivery: %s", status)
	}
}

func TestReminderMovedLaterCannotBeClaimedEarly(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)
	job := newReminderJob(t, pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	job.ScheduledAt, job.NextAttemptAt = now.Add(-time.Minute), now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	job.ScheduledAt, job.NextAttemptAt = now.Add(4*time.Minute), now.Add(4*time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	var next time.Time
	if err := pool.QueryRow(ctx, `SELECT next_attempt_at FROM reminder_jobs WHERE calendar_id=$1`, job.CalendarID).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next.Before(job.ScheduledAt) {
		t.Fatalf("moved reminder next_attempt_at=%s precedes scheduled_at=%s", next, job.ScheduledAt)
	}
	claimed, err := repo.ClaimDue(ctx, "early-worker", now, time.Minute, 10)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("future job claimed early: %+v error=%v", claimed, err)
	}
	claimed, err = repo.ClaimDue(ctx, "due-worker", now.Add(5*time.Minute), time.Minute, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("moved job not claimed when due: %+v error=%v", claimed, err)
	}
}

func TestReminderExpiredClaimCannotMutateWithoutAReplacement(t *testing.T) {
	for operation, mutate := range map[string]func(context.Context, *repository.ReminderJobRepository, models.ReminderJob) error{
		"sent": func(ctx context.Context, repo *repository.ReminderJobRepository, job models.ReminderJob) error {
			return repo.MarkSent(ctx, job.ID, job.ClaimToken)
		},
		"failed": func(ctx context.Context, repo *repository.ReminderJobRepository, job models.ReminderJob) error {
			return repo.MarkFailed(ctx, job.ID, job.ClaimToken, 1, 5, time.Now(), "expired")
		},
		"cancel": func(ctx context.Context, repo *repository.ReminderJobRepository, job models.ReminderJob) error {
			return repo.CancelClaim(ctx, job.ID, job.ClaimToken)
		},
	} {
		t.Run(operation, func(t *testing.T) {
			pool := dbtest.Pool(t)
			ctx := dbtest.Context(t)
			repo := repository.NewReminderJobRepository(pool)
			job := newReminderJob(t, pool)
			now := time.Now().UTC().Truncate(time.Microsecond)
			job.ScheduledAt, job.NextAttemptAt = now.Add(-time.Minute), now.Add(-time.Minute)
			if err := repo.Enqueue(ctx, job); err != nil {
				t.Fatal(err)
			}
			claimed, err := repo.ClaimDue(ctx, "expired-worker", now, time.Minute, 1)
			if err != nil || len(claimed) != 1 {
				t.Fatalf("claim: %v jobs=%v", err, claimed)
			}
			old := claimed[0]
			if _, err := pool.Exec(ctx, `UPDATE reminder_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, old.ID); err != nil {
				t.Fatal(err)
			}
			if err := mutate(ctx, repo, old); !errors.Is(err, models.ErrClaimLost) {
				t.Errorf("expired %s=%v, want ErrClaimLost", operation, err)
			}
		})
	}
}

func TestReminderRescheduleInvalidatesExpiredClaimToken(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := dbtest.Context(t)
	repo := repository.NewReminderJobRepository(pool)
	job := newReminderJob(t, pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	job.ScheduledAt, job.NextAttemptAt = now.Add(-time.Minute), now.Add(-time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimDue(ctx, "old-worker", now, time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v jobs=%v", err, claimed)
	}
	old := claimed[0]
	if _, err := pool.Exec(ctx, `UPDATE reminder_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, old.ID); err != nil {
		t.Fatal(err)
	}
	job.ScheduledAt, job.NextAttemptAt = now.Add(4*time.Minute), now.Add(4*time.Minute)
	if err := repo.Enqueue(ctx, job); err != nil {
		t.Fatal(err)
	}
	var clear bool
	if err := pool.QueryRow(ctx, `SELECT claim_token IS NULL AND lease_until IS NULL AND locked_by IS NULL AND locked_at IS NULL FROM reminder_jobs WHERE id=$1`, old.ID).Scan(&clear); err != nil {
		t.Fatal(err)
	}
	if !clear {
		t.Fatal("rescheduled row still carries expired ownership")
	}
	if err := repo.MarkSent(ctx, old.ID, old.ClaimToken); !errors.Is(err, models.ErrClaimLost) {
		t.Fatalf("late completion of rescheduled row: %v", err)
	}
}

// cancelPending puts a job's pending row in the canceled state, as a reconciled
// cancel would.
func cancelPending(ctx context.Context, t *testing.T, pool *pgxpool.Pool, job *models.ReminderJob) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE reminder_jobs SET status = 'canceled', canceled_at = now(),
		claim_token = NULL, lease_until = NULL, updated_at = now()
		WHERE calendar_id = $1 AND event_date = $2 AND status = 'pending'`, job.CalendarID, job.EventDate); err != nil {
		t.Fatal(err)
	}
}
