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

	"github.com/whento/pkg/dberr"
	"github.com/whento/whento/internal/availability/models"
)

// Sentinels, so that callers can tell a request the user got wrong from a database
// that refused. Compare with errors.Is — never with the error text, which is what the
// service layer used to do and which made these messages part of the API by accident.
var (
	ErrAvailabilityNotFound = errors.New("availability not found")
	ErrAvailabilityExists   = errors.New("availability already exists for this date")
)

// AvailabilityRepository handles availability database operations
type AvailabilityRepository struct {
	pool *pgxpool.Pool
}

// NewAvailabilityRepository creates a new availability repository
func NewAvailabilityRepository(pool *pgxpool.Pool) *AvailabilityRepository {
	return &AvailabilityRepository{pool: pool}
}

// Create creates a new availability
func (r *AvailabilityRepository) Create(ctx context.Context, availability *models.Availability) error {
	query := `
		INSERT INTO availabilities (id, participant_id, date, start_time, end_time, note, source, recurrence_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		availability.ID,
		availability.ParticipantID,
		availability.Date,
		availability.StartTime,
		availability.EndTime,
		availability.Note,
		availability.Source,
		availability.RecurrenceID,
	).Scan(&availability.CreatedAt, &availability.UpdatedAt)

	if err != nil {
		if dberr.IsUniqueViolation(err) {
			return ErrAvailabilityExists
		}
		return fmt.Errorf("failed to create availability: %w", err)
	}

	return nil
}

// GetByParticipantAndDate retrieves an availability by participant ID and date
func (r *AvailabilityRepository) GetByParticipantAndDate(ctx context.Context, participantID uuid.UUID, date time.Time) (*models.Availability, error) {
	query := `
		SELECT id, participant_id, date,
		       TO_CHAR(start_time, 'HH24:MI') as start_time,
		       TO_CHAR(end_time, 'HH24:MI') as end_time,
		       note, source, recurrence_id, created_at, updated_at
		FROM availabilities
		WHERE participant_id = $1 AND date = $2`

	availability := &models.Availability{}
	err := r.pool.QueryRow(ctx, query, participantID, date).Scan(
		&availability.ID,
		&availability.ParticipantID,
		&availability.Date,
		&availability.StartTime,
		&availability.EndTime,
		&availability.Note,
		&availability.Source,
		&availability.RecurrenceID,
		&availability.CreatedAt,
		&availability.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAvailabilityNotFound
		}
		return nil, fmt.Errorf("failed to get availability: %w", err)
	}

	return availability, nil
}

// GetByParticipantID retrieves all availabilities for a participant
func (r *AvailabilityRepository) GetByParticipantID(ctx context.Context, participantID uuid.UUID) ([]*models.Availability, error) {
	return r.GetByParticipantIDWithDateRange(ctx, participantID, nil, nil)
}

// GetByParticipantIDWithDateRange retrieves availabilities for a participant, optionally filtered by date range
func (r *AvailabilityRepository) GetByParticipantIDWithDateRange(ctx context.Context, participantID uuid.UUID, startDate, endDate *time.Time) ([]*models.Availability, error) {
	query := `
		SELECT id, participant_id, date,
		       TO_CHAR(start_time, 'HH24:MI') as start_time,
		       TO_CHAR(end_time, 'HH24:MI') as end_time,
		       note, source, recurrence_id, created_at, updated_at
		FROM availabilities
		WHERE participant_id = $1`

	args := []interface{}{participantID}
	paramCount := 1

	if startDate != nil {
		paramCount++
		query += fmt.Sprintf(" AND date >= $%d", paramCount)
		args = append(args, *startDate)
	}

	if endDate != nil {
		paramCount++
		query += fmt.Sprintf(" AND date <= $%d", paramCount)
		args = append(args, *endDate)
	}

	query += " ORDER BY date ASC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get availabilities: %w", err)
	}
	defer rows.Close()

	var availabilities []*models.Availability
	for rows.Next() {
		availability := &models.Availability{}
		err := rows.Scan(
			&availability.ID,
			&availability.ParticipantID,
			&availability.Date,
			&availability.StartTime,
			&availability.EndTime,
			&availability.Note,
			&availability.Source,
			&availability.RecurrenceID,
			&availability.CreatedAt,
			&availability.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan availability: %w", err)
		}
		availabilities = append(availabilities, availability)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
	}

	return availabilities, nil
}

// GetByDateRange retrieves availabilities for a participant within a date range
func (r *AvailabilityRepository) GetByDateRange(ctx context.Context, participantID uuid.UUID, startDate, endDate time.Time) ([]*models.Availability, error) {
	query := `
		SELECT id, participant_id, date,
		       TO_CHAR(start_time, 'HH24:MI') as start_time,
		       TO_CHAR(end_time, 'HH24:MI') as end_time,
		       note, source, recurrence_id, created_at, updated_at
		FROM availabilities
		WHERE participant_id = $1 AND date >= $2 AND date <= $3
		ORDER BY date ASC`

	rows, err := r.pool.Query(ctx, query, participantID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get availabilities by date range: %w", err)
	}
	defer rows.Close()

	var availabilities []*models.Availability
	for rows.Next() {
		availability := &models.Availability{}
		err := rows.Scan(
			&availability.ID,
			&availability.ParticipantID,
			&availability.Date,
			&availability.StartTime,
			&availability.EndTime,
			&availability.Note,
			&availability.Source,
			&availability.RecurrenceID,
			&availability.CreatedAt,
			&availability.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan availability: %w", err)
		}
		availabilities = append(availabilities, availability)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
	}

	return availabilities, nil
}

// GetByDate retrieves all availabilities for a specific date across all participants in a calendar
func (r *AvailabilityRepository) GetByDate(ctx context.Context, calendarID uuid.UUID, date time.Time) ([]*models.Availability, error) {
	query := `
		SELECT a.id, a.participant_id, a.date,
		       TO_CHAR(a.start_time, 'HH24:MI') as start_time,
		       TO_CHAR(a.end_time, 'HH24:MI') as end_time,
		       a.note, a.source, a.recurrence_id, a.created_at, a.updated_at
		FROM availabilities a
		JOIN participants p ON a.participant_id = p.id
		WHERE p.calendar_id = $1 AND a.date = $2
		ORDER BY a.participant_id ASC`

	rows, err := r.pool.Query(ctx, query, calendarID, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get availabilities by date: %w", err)
	}
	defer rows.Close()

	var availabilities []*models.Availability
	for rows.Next() {
		availability := &models.Availability{}
		err := rows.Scan(
			&availability.ID,
			&availability.ParticipantID,
			&availability.Date,
			&availability.StartTime,
			&availability.EndTime,
			&availability.Note,
			&availability.Source,
			&availability.RecurrenceID,
			&availability.CreatedAt,
			&availability.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan availability: %w", err)
		}
		availabilities = append(availabilities, availability)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
	}

	return availabilities, nil
}

// GetByCalendarDateRange retrieves all availabilities for a calendar within a date range
func (r *AvailabilityRepository) GetByCalendarDateRange(ctx context.Context, calendarID uuid.UUID, startDate, endDate time.Time) ([]*models.Availability, error) {
	query := `
		SELECT a.id, a.participant_id, a.date,
		       TO_CHAR(a.start_time, 'HH24:MI') as start_time,
		       TO_CHAR(a.end_time, 'HH24:MI') as end_time,
		       a.note, a.source, a.recurrence_id, a.created_at, a.updated_at
		FROM availabilities a
		JOIN participants p ON a.participant_id = p.id
		WHERE p.calendar_id = $1 AND a.date >= $2 AND a.date <= $3
		ORDER BY a.date ASC, a.participant_id ASC`

	rows, err := r.pool.Query(ctx, query, calendarID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get availabilities by calendar date range: %w", err)
	}
	defer rows.Close()

	var availabilities []*models.Availability
	for rows.Next() {
		availability := &models.Availability{}
		err := rows.Scan(
			&availability.ID,
			&availability.ParticipantID,
			&availability.Date,
			&availability.StartTime,
			&availability.EndTime,
			&availability.Note,
			&availability.Source,
			&availability.RecurrenceID,
			&availability.CreatedAt,
			&availability.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan availability: %w", err)
		}
		availabilities = append(availabilities, availability)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
	}

	return availabilities, nil
}

// Update updates an availability
func (r *AvailabilityRepository) Update(ctx context.Context, availability *models.Availability) error {
	query := `
		UPDATE availabilities
		SET start_time = $2, end_time = $3, note = $4, updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`

	err := r.pool.QueryRow(ctx, query,
		availability.ID,
		availability.StartTime,
		availability.EndTime,
		availability.Note,
	).Scan(&availability.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAvailabilityNotFound
		}
		return fmt.Errorf("failed to update availability: %w", err)
	}

	return nil
}

// Delete deletes an availability
func (r *AvailabilityRepository) Delete(ctx context.Context, participantID uuid.UUID, date time.Time) error {
	query := `DELETE FROM availabilities WHERE participant_id = $1 AND date = $2`

	result, err := r.pool.Exec(ctx, query, participantID, date)
	if err != nil {
		return fmt.Errorf("failed to delete availability: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrAvailabilityNotFound
	}

	return nil
}

// GetParticipantCountForDate counts unique participants with availability for a specific date
// GetAvailableParticipantsForDate returns everyone who counts as available on a date,
// by name, whether they said so directly or a recurrence says it for them.
//
// This is the single answer to "who is available on this date". It exists because there
// were several: the threshold count expanded recurrences in SQL, the notification list
// read the availabilities table raw, and they disagreed. A participant available only
// through a recurrence was counted towards the threshold, left out of the participant
// list in the email, and — worse, and silently — dropped from the recipients of the
// notification about the very date they had made themselves available for.
//
// Two conditions that used to differ between the two queries are settled here. The
// availabilities half no longer filters on source = 'manual': nothing writes any other
// value, so the filter selected nothing today while quietly discarding rows written by
// an older version. And the recurrence half no longer tests start_date IS NULL, because
// the column is NOT NULL — that branch could never be taken.
func (r *AvailabilityRepository) GetAvailableParticipantsForDate(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
) ([]models.AvailableParticipant, error) {
	// Written as two EXISTS against participants rather than a UNION of ids followed by
	// a name lookup: one participant matching both halves is one row either way, and the
	// name comes back with it.
	query := `
		SELECT p.id, p.name
		FROM participants p
		WHERE p.calendar_id = $1
		  AND (
			EXISTS (
				SELECT 1 FROM availabilities a
				WHERE a.participant_id = p.id AND a.date = $2::DATE
			)
			OR EXISTS (
				SELECT 1 FROM recurrences r
				WHERE r.participant_id = p.id
				  AND EXTRACT(DOW FROM $2::DATE)::int = r.day_of_week
				  AND $2::DATE >= r.start_date
				  AND (r.end_date IS NULL OR $2::DATE <= r.end_date)
				  AND NOT EXISTS (
					SELECT 1 FROM recurrence_exceptions re
					WHERE re.recurrence_id = r.id
					  AND re.excluded_date = $2::DATE
				  )
			)
		  )
		ORDER BY p.name ASC`

	rows, err := r.pool.Query(ctx, query, calendarID, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get available participants for date: %w", err)
	}
	defer rows.Close()

	var participants []models.AvailableParticipant
	for rows.Next() {
		var participant models.AvailableParticipant
		if err := rows.Scan(&participant.ID, &participant.Name); err != nil {
			return nil, fmt.Errorf("failed to scan available participant: %w", err)
		}
		participants = append(participants, participant)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating available participants: %w", err)
	}

	return participants, nil
}

// GetParticipantCountForDate returns how many participants are available on a date.
//
// Derived from the list rather than counted by a query of its own, so the number in
// "3/3 participants available" and the names printed underneath it cannot disagree.
// They used to.
func (r *AvailabilityRepository) GetParticipantCountForDate(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
) (int, error) {
	participants, err := r.GetAvailableParticipantsForDate(ctx, calendarID, date)
	if err != nil {
		return 0, err
	}

	return len(participants), nil
}
