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
// instances from delivering the same reminder twice.
type ReminderJobRepository struct {
	pool *pgxpool.Pool
}

// NewReminderJobRepository creates a reminder job repository.
func NewReminderJobRepository(pool *pgxpool.Pool) *ReminderJobRepository {
	return &ReminderJobRepository{pool: pool}
}

// Enqueue inserts a reminder job, ignoring a job that already exists for the
// same delivery key (calendar, event date, recipient class, channel). Idempotent
// by construction: a scheduler restart mid-scan cannot create duplicates.
func (r *ReminderJobRepository) Enqueue(
	ctx context.Context,
	job *models.ReminderJob,
) error {
	query := `
		INSERT INTO reminder_jobs
			(calendar_id, event_date, recipient_type, channel, scheduled_at, max_attempts, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (calendar_id, event_date, recipient_type, channel) DO NOTHING`

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

// ClaimDue atomically claims up to limit pending jobs whose next attempt is due,
// skipping rows another instance already locked (FOR UPDATE SKIP LOCKED). A job
// whose lock has expired is claimable again, which is what lets a crashed
// instance hand its work back.
func (r *ReminderJobRepository) ClaimDue(
	ctx context.Context,
	instanceID string,
	now time.Time,
	lockTTL time.Duration,
	limit int,
) ([]models.ReminderJob, error) {
	query := `
		UPDATE reminder_jobs
		SET locked_by = $1, locked_at = $2, updated_at = $2
		WHERE id IN (
			SELECT id FROM reminder_jobs
			WHERE status = 'pending'
			  AND next_attempt_at <= $2
			  AND (locked_at IS NULL OR locked_at < $2 - $3::interval)
			ORDER BY next_attempt_at
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, calendar_id, event_date, recipient_type, channel, scheduled_at, status, attempt, max_attempts, next_attempt_at, COALESCE(last_error, ''), COALESCE(locked_by, '')`

	rows, err := r.pool.Query(ctx, query, instanceID, now, lockTTL, limit)
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

// MarkSent records a successful delivery and takes the job out of the loop.
func (r *ReminderJobRepository) MarkSent(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE reminder_jobs
		SET status = 'sent', locked_by = NULL, locked_at = NULL, updated_at = now()
		WHERE id = $1`
	if _, err := r.pool.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("failed to mark reminder job sent: %w", err)
	}
	return nil
}

// MarkFailed records a delivery failure and schedules the next attempt, or
// permanently fails the job once attempts are exhausted.
func (r *ReminderJobRepository) MarkFailed(
	ctx context.Context,
	id uuid.UUID,
	attempt int,
	maxAttempts int,
	nextAttemptAt time.Time,
	reason string,
) error {
	status := "pending"
	if attempt >= maxAttempts {
		status = "failed"
	}
	query := `
		UPDATE reminder_jobs
		SET status = $2, attempt = $3, next_attempt_at = $4, last_error = $5,
		    locked_by = NULL, locked_at = NULL, updated_at = now()
		WHERE id = $1`
	if _, err := r.pool.Exec(ctx, query, id, status, attempt, nextAttemptAt, reason); err != nil {
		return fmt.Errorf("failed to record reminder job failure: %w", err)
	}
	return nil
}

// CancelPendingForEvent marks every pending job for an event date as canceled.
// The scheduler calls this when the event no longer qualifies (availability
// dropped below threshold, date left the calendar range, reminders switched
// off), so a stale reminder cannot slip out on a later tick.
func (r *ReminderJobRepository) CancelPendingForEvent(
	ctx context.Context,
	calendarID uuid.UUID,
	eventDate time.Time,
) error {
	query := `
		UPDATE reminder_jobs
		SET status = 'canceled', locked_by = NULL, locked_at = NULL, updated_at = now()
		WHERE calendar_id = $1 AND event_date = $2 AND status = 'pending'`
	if _, err := r.pool.Exec(ctx, query, calendarID, eventDate); err != nil {
		return fmt.Errorf("failed to cancel reminder jobs: %w", err)
	}
	return nil
}
