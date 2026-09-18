// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/availability/models"
)

// ActivityLogRepository is the per-date activity journal: at most one row per
// (calendar_id, date), holding two independent slots that are each overwritten by the
// next event of their own kind.
//
// The two writes below are deliberately not one method with a flag. They touch disjoint
// columns, and that is the whole contract: recording a withdrawal must not disturb who
// joined last, and recording a join must not disturb the withdrawal. A single upsert
// writing all four columns would silently clear the other slot on every call.
type ActivityLogRepository struct {
	pool *pgxpool.Pool
}

// NewActivityLogRepository creates a new activity log repository
func NewActivityLogRepository(pool *pgxpool.Pool) *ActivityLogRepository {
	return &ActivityLogRepository{pool: pool}
}

// RecordJoin remembers who joined this date last, replacing whoever held the slot.
func (r *ActivityLogRepository) RecordJoin(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
	participantID uuid.UUID,
	at time.Time,
) error {
	query := `
		INSERT INTO date_activity_log (calendar_id, date, last_joined_participant_id, last_joined_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (calendar_id, date) DO UPDATE
		SET last_joined_participant_id = EXCLUDED.last_joined_participant_id,
		    last_joined_at             = EXCLUDED.last_joined_at`

	if _, err := r.pool.Exec(ctx, query, calendarID, date, participantID, at); err != nil {
		return fmt.Errorf("failed to record the join in the activity log: %w", err)
	}

	return nil
}

// RecordWithdrawal remembers who withdrew from this date last, replacing whoever held
// the slot. The caller decides whether a withdrawal is worth journalling at all — see
// the service, where the rule is that the date had already reached its threshold.
func (r *ActivityLogRepository) RecordWithdrawal(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
	participantID uuid.UUID,
	at time.Time,
) error {
	query := `
		INSERT INTO date_activity_log (calendar_id, date, last_withdrawn_participant_id, last_withdrawn_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (calendar_id, date) DO UPDATE
		SET last_withdrawn_participant_id = EXCLUDED.last_withdrawn_participant_id,
		    last_withdrawn_at             = EXCLUDED.last_withdrawn_at`

	if _, err := r.pool.Exec(ctx, query, calendarID, date, participantID, at); err != nil {
		return fmt.Errorf("failed to record the withdrawal in the activity log: %w", err)
	}

	return nil
}

// activityLogColumns is the projection both reads share.
//
// The participants table is joined twice, once per slot, and the join is what supplies
// the name: nothing in date_activity_log holds one. An id whose participant has been
// deleted is NULL by then, the join yields nothing, and toModel below reads that as
// "no entry" — timestamp included, so a stranded last_joined_at never escapes.
const activityLogColumns = `
		l.date,
		l.last_joined_participant_id, j.name, l.last_joined_at,
		l.last_withdrawn_participant_id, w.name, l.last_withdrawn_at
	FROM date_activity_log l
	LEFT JOIN participants j ON j.id = l.last_joined_participant_id
	LEFT JOIN participants w ON w.id = l.last_withdrawn_participant_id`

// activityRow is one journal row as it comes out of the database, before the two
// nullable slots are resolved into refs.
type activityRow struct {
	date          time.Time
	joinedID      *uuid.UUID
	joinedName    *string
	joinedAt      *time.Time
	withdrawnID   *uuid.UUID
	withdrawnName *string
	withdrawnAt   *time.Time
}

// toModel resolves the two slots. A slot counts only when the id, the name and the
// timestamp are all there: anything less is a deleted participant or a row that has
// only ever held the other slot.
func (row activityRow) toModel() *models.DateActivity {
	activity := &models.DateActivity{Date: row.date}

	if row.joinedID != nil && row.joinedName != nil && row.joinedAt != nil {
		activity.LastJoined = &models.ParticipantRef{
			ID:   *row.joinedID,
			Name: *row.joinedName,
			At:   *row.joinedAt,
		}
	}

	if row.withdrawnID != nil && row.withdrawnName != nil && row.withdrawnAt != nil {
		activity.LastWithdrawn = &models.ParticipantRef{
			ID:   *row.withdrawnID,
			Name: *row.withdrawnName,
			At:   *row.withdrawnAt,
		}
	}

	return activity
}

// GetForDate returns the journal entry for one date, or nil when there is none.
//
// A row all of whose slots have been emptied by participant deletion comes back as a
// DateActivity with two nil refs rather than as nil: the row exists, it just no longer
// remembers anybody. Callers render both slots as absent either way.
func (r *ActivityLogRepository) GetForDate(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
) (*models.DateActivity, error) {
	query := `SELECT ` + activityLogColumns + `
	WHERE l.calendar_id = $1 AND l.date = $2`

	var row activityRow
	err := r.pool.QueryRow(ctx, query, calendarID, date).Scan(
		&row.date,
		&row.joinedID, &row.joinedName, &row.joinedAt,
		&row.withdrawnID, &row.withdrawnName, &row.withdrawnAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read the activity log for the date: %w", err)
	}

	return row.toModel(), nil
}

// GetForRange returns the journal entries covering a date range, keyed by "2006-01-02".
//
// One query for the whole range: this is read by the owner-facing activity endpoint,
// which would otherwise issue one per date in the range.
func (r *ActivityLogRepository) GetForRange(
	ctx context.Context,
	calendarID uuid.UUID,
	startDate, endDate time.Time,
) (map[string]*models.DateActivity, error) {
	query := `SELECT ` + activityLogColumns + `
	WHERE l.calendar_id = $1 AND l.date >= $2 AND l.date <= $3`

	rows, err := r.pool.Query(ctx, query, calendarID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to read the activity log for the range: %w", err)
	}
	defer rows.Close()

	entries := make(map[string]*models.DateActivity)
	for rows.Next() {
		var row activityRow
		if err := rows.Scan(
			&row.date,
			&row.joinedID, &row.joinedName, &row.joinedAt,
			&row.withdrawnID, &row.withdrawnName, &row.withdrawnAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan an activity log row: %w", err)
		}

		entries[row.date.Format("2006-01-02")] = row.toModel()
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating activity log rows: %w", err)
	}

	return entries, nil
}
