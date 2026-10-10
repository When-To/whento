// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/notify/models"
)

// ReminderJobRepository owns the reminder_jobs table: the persisted delivery
// queue behind the reminder scheduler.
//
// The scheduling loop is a happy-path reader/writer here; the reliability lives
// in this table and in the atomicity of ClaimDue, which is what keeps several
// deliveries from sending the same reminder twice.
//
// The claim model is a per-acquisition UUID plus a lease, not an instance id: a
// single instance can claim the same job twice across a lease expiry, so only
// the token distinguishes the worker that owns the current delivery from one
// whose claim lapsed. Every fenced worker operation (MarkSent, MarkFailed,
// per-job cancel, lease renewal) requires the token ClaimDue handed out and a
// 'pending' status; a row that is 'sent' can never be turned into another
// state, which is what makes a late worker's failure report a recognizable
// no-op instead of a resurrected reminder.
type ReminderJobRepository struct {
	pool *pgxpool.Pool
}

// NewReminderJobRepository creates a reminder job repository.
func NewReminderJobRepository(pool *pgxpool.Pool) *ReminderJobRepository {
	return &ReminderJobRepository{pool: pool}
}

// Enqueue inserts a reminder job, reconciling with whatever already exists for
// the same delivery key (calendar, event date, recipient class, channel).
//
// The upsert is conditional rather than a blind ON CONFLICT DO NOTHING:
//
//   - an absent row is inserted as pending;
//   - a 'pending' row that still has a live claim is left untouched (repeated
//     scans must not reset failures or steal leases);
//   - a 'pending' row without a claim keeps its attempt/backoff history and
//     only takes a recomputed schedule when the old one drifted;
//   - a 'canceled' row is rearmed within the delivery window: cancellation and
//     error metadata are reset and the new schedule set;
//   - a 'sent' row is never rearmed by a routine scan or a settings change;
//   - an exhausted 'failed' row keeps its terminal failure.
//
// The whole decision is one atomic statement, so a restart mid-scan cannot
// double-schedule nor leave a half-reconciled row behind.
func (r *ReminderJobRepository) Enqueue(
	ctx context.Context,
	job *models.ReminderJob,
) error {
	query := `
		INSERT INTO reminder_jobs
			(calendar_id, event_date, recipient_type, channel, scheduled_at, max_attempts, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (calendar_id, event_date, recipient_type, channel) DO UPDATE SET
			scheduled_at = EXCLUDED.scheduled_at,
			max_attempts = EXCLUDED.max_attempts,
			status = 'pending',
			attempt = CASE WHEN reminder_jobs.status = 'canceled' THEN 0 ELSE reminder_jobs.attempt END,
			canceled_at = NULL,
			sent_at = NULL,
			last_error = CASE WHEN reminder_jobs.status = 'canceled' THEN NULL ELSE reminder_jobs.last_error END,
			claim_token = NULL,
			lease_until = NULL,
			locked_by = NULL,
			locked_at = NULL,
			next_attempt_at = CASE
				WHEN reminder_jobs.status = 'canceled' THEN EXCLUDED.next_attempt_at
				WHEN reminder_jobs.scheduled_at = EXCLUDED.scheduled_at THEN reminder_jobs.next_attempt_at
				ELSE GREATEST(EXCLUDED.scheduled_at, LEAST(reminder_jobs.next_attempt_at, EXCLUDED.next_attempt_at))
			END,
			updated_at = now()
		WHERE reminder_jobs.status IN ('pending', 'canceled')
		  AND (reminder_jobs.claim_token IS NULL OR reminder_jobs.lease_until IS NULL OR reminder_jobs.lease_until < now())`

	_, err := r.pool.Exec(ctx, query,
		job.CalendarID,
		job.EventDate,
		job.RecipientType,
		job.Channel,
		job.ScheduledAt,
		job.MaxAttempts,
		job.NextAttemptAt,
	)
	if err != nil {
		return fmt.Errorf("failed to enqueue reminder job: %w", err)
	}
	return nil
}

// ClaimDue atomically claims up to limit due pending jobs, skipping rows another
// acquisition already holds (FOR UPDATE SKIP LOCKED). Each claimed job receives
// a new UUID claim token and a lease expiry, which are returned with the job;
// that token is the fence every fenced worker operation below checks.
//
// A job whose lease has expired is claimable again, which is what lets a crashed
// worker's work be handed back — always under a fresh token, so the previous
// worker can no longer mutate the row.
func (r *ReminderJobRepository) ClaimDue(
	ctx context.Context,
	instanceID string,
	now time.Time,
	leaseTTL time.Duration,
	limit int,
) ([]models.ReminderJob, error) {
	token := uuid.New()
	leaseUntil := now.Add(leaseTTL)

	query := `
		UPDATE reminder_jobs
		SET claim_token = $1, lease_until = $4, locked_by = $2, locked_at = $3, updated_at = $3
		WHERE id IN (
			SELECT id FROM reminder_jobs
			WHERE status = 'pending'
			  AND scheduled_at <= $3
			  AND next_attempt_at <= $3
			  AND (claim_token IS NULL OR lease_until IS NULL OR lease_until < $3)
			ORDER BY next_attempt_at
			LIMIT $5
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, calendar_id, event_date, recipient_type, channel, scheduled_at, status,
		          attempt, max_attempts, next_attempt_at, claim_token, lease_until,
		          COALESCE(last_error, ''), COALESCE(locked_by, '')`

	rows, err := r.pool.Query(ctx, query, token, instanceID, now, leaseUntil, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to claim reminder jobs: %w", err)
	}
	defer rows.Close()

	var jobs []models.ReminderJob
	for rows.Next() {
		var job models.ReminderJob
		if err := rows.Scan(
			&job.ID,
			&job.CalendarID,
			&job.EventDate,
			&job.RecipientType,
			&job.Channel,
			&job.ScheduledAt,
			&job.Status,
			&job.Attempt,
			&job.MaxAttempts,
			&job.NextAttemptAt,
			&job.ClaimToken,
			&job.LeaseUntil,
			&job.LastError,
			&job.LockedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reminder job: %w", err)
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reminder jobs: %w", err)
	}

	return jobs, nil
}

// RenewLease extends the lease of the job the caller still owns, immediately
// before an external send. It is fenced by the claim token, and requires the
// current lease to still be valid: a worker whose lease lapsed has lost the
// right to deliver, and letting it renew would race the worker that re-claimed
// the job into a double send. A no-op result is models.ErrClaimLost.
func (r *ReminderJobRepository) RenewLease(
	ctx context.Context,
	id uuid.UUID,
	token uuid.UUID,
	now time.Time,
	ttl time.Duration,
) error {
	leaseUntil := now.Add(ttl)
	tag, err := r.pool.Exec(ctx, `
		UPDATE reminder_jobs
		SET lease_until = GREATEST(lease_until, $4), locked_at = $3, updated_at = now()
		WHERE id = $1 AND status = 'pending' AND claim_token = $2 AND lease_until >= $3`,
		id, token, now, leaseUntil)
	if err != nil {
		return fmt.Errorf("failed to renew reminder job lease: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrClaimLost
	}
	return nil
}

// MarkSent records a successful delivery. It is fenced by the claim token and a
// 'pending' status, so a worker whose claim was lost — or a row already sent by
// the worker that won the re-claim — is a recognizable no-op (models.ErrClaimLost)
// rather than a corrupted outcome.
func (r *ReminderJobRepository) MarkSent(ctx context.Context, id, token uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE reminder_jobs
		SET status = 'sent', sent_at = now(), claim_token = NULL, lease_until = NULL,
		    locked_by = NULL, locked_at = NULL, canceled_at = NULL, last_error = NULL, updated_at = now()
		WHERE id = $1 AND status = 'pending' AND claim_token = $2 AND lease_until >= now()`,
		id, token)
	if err != nil {
		return fmt.Errorf("failed to mark reminder job sent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrClaimLost
	}
	return nil
}

// RecordMissed creates a canceled audit row once, without changing an existing
// delivery or claim. Enqueue may rearm it if an explicit schedule change brings
// the event back into the catch-up window.
func (r *ReminderJobRepository) RecordMissed(ctx context.Context, job *models.ReminderJob) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO reminder_jobs
			(calendar_id, event_date, recipient_type, channel, scheduled_at, max_attempts,
			 next_attempt_at, status, canceled_at, last_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'canceled', now(), 'catch_up_window_missed')
		ON CONFLICT (calendar_id, event_date, recipient_type, channel) DO NOTHING`,
		job.CalendarID, job.EventDate, job.RecipientType, job.Channel, job.ScheduledAt,
		job.MaxAttempts, job.NextAttemptAt)
	if err != nil {
		return false, fmt.Errorf("failed to record missed reminder: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Cleanup scrubs legacy provider error strings and bounds queue retention without
// removing future-event tombstones or live claims. At most limit rows are touched
// per pass; a full batch tells the scheduler to continue on its next pass.
func (r *ReminderJobRepository) Cleanup(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	scrubbed, err := r.pool.Exec(ctx, `
		UPDATE reminder_jobs SET last_error = CASE WHEN status = 'sent' THEN NULL ELSE 'reminder_internal_failure' END
		WHERE id IN (
			SELECT id FROM reminder_jobs WHERE last_error IS NOT NULL
			  AND (status = 'sent' OR last_error NOT IN (
				'email_delivery_failed', 'discord_delivery_failed', 'slack_delivery_failed',
				'telegram_delivery_failed', 'reminder_internal_failure', 'catch_up_window_missed'))
			ORDER BY updated_at, id LIMIT $1 FOR UPDATE SKIP LOCKED
		)`, limit)
	if err != nil {
		return 0, fmt.Errorf("failed to scrub reminder errors: %w", err)
	}
	changed := scrubbed.RowsAffected()
	if changed >= int64(limit) {
		return changed, nil
	}
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM reminder_jobs WHERE id IN (
			SELECT id FROM reminder_jobs
			WHERE event_date < ($1::timestamptz AT TIME ZONE 'UTC')::date AND updated_at < $1::timestamptz
			  AND (lease_until IS NULL OR lease_until < now())
			ORDER BY event_date, id LIMIT $2
			FOR UPDATE SKIP LOCKED
	)`, before.UTC(), int64(limit)-changed)
	if err != nil {
		return 0, fmt.Errorf("failed to clean reminder jobs: %w", err)
	}
	return changed + tag.RowsAffected(), nil
}

// MarkFailed records a delivery failure and schedules the next attempt, or
// permanently fails the job once attempts are exhausted. Fenced by the claim
// token and a 'pending' status: a 'sent' row cannot be walked backwards into a
// retry, and a late worker's failure report for a job somebody else sent is a
// no-op.
func (r *ReminderJobRepository) MarkFailed(
	ctx context.Context,
	id uuid.UUID,
	token uuid.UUID,
	attempt int,
	maxAttempts int,
	nextAttemptAt time.Time,
	reason string,
) error {
	status := "pending"
	if attempt >= maxAttempts {
		status = "failed"
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE reminder_jobs
		SET status = $3, attempt = $4, next_attempt_at = $5, last_error = $6,
		    claim_token = NULL, lease_until = NULL, locked_by = NULL, locked_at = NULL,
		    canceled_at = NULL, updated_at = now()
		WHERE id = $1 AND status = 'pending' AND claim_token = $2 AND lease_until >= now()`,
		id, token, status, attempt, nextAttemptAt, reason)
	if err != nil {
		return fmt.Errorf("failed to record reminder job failure: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrClaimLost
	}
	return nil
}

// CancelClaim cancels a single pending delivery under the caller's claim token.
// Used when an individual delivery is no longer wanted (channel or recipient
// disabled, reminders turned off, SMTP vanished) without touching the other
// deliveries of the same event. The canceled row stays rearmable by Enqueue
// within the delivery window.
func (r *ReminderJobRepository) CancelClaim(ctx context.Context, id, token uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE reminder_jobs
		SET status = 'canceled', canceled_at = now(), claim_token = NULL, lease_until = NULL,
		    locked_by = NULL, locked_at = NULL, updated_at = now()
		WHERE id = $1 AND status = 'pending' AND claim_token = $2 AND lease_until >= now()`,
		id, token)
	if err != nil {
		return fmt.Errorf("failed to cancel reminder job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrClaimLost
	}
	return nil
}
