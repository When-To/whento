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

var (
	ErrAvailabilityNotFound = errors.New("availability not found")
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
		if isDuplicateKeyError(err) {
			return fmt.Errorf("availability already exists for this date")
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
func (r *AvailabilityRepository) GetParticipantCountForDate(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
) (int, error) {
	// Query counts distinct participants who have availability on this date
	// This includes both manual availabilities and recurrence-generated ones
	query := `
		WITH calendar_participants AS (
			SELECT id as participant_id
			FROM participants
			WHERE calendar_id = $1
		),
		date_availabilities AS (
			-- Manual availabilities for this date
			SELECT DISTINCT a.participant_id
			FROM availabilities a
			JOIN calendar_participants cp ON a.participant_id = cp.participant_id
			WHERE a.date = $2
			  AND a.source = 'manual'

			UNION

			-- Recurrence-generated availabilities for this date
			SELECT DISTINCT r.participant_id
			FROM recurrences r
			JOIN calendar_participants cp ON r.participant_id = cp.participant_id
			WHERE EXTRACT(DOW FROM $2::DATE) = r.day_of_week
			  AND (r.start_date IS NULL OR $2::DATE >= r.start_date)
			  AND (r.end_date IS NULL OR $2::DATE <= r.end_date)
			  -- Exclude if there's an exception for this date
			  AND NOT EXISTS (
				SELECT 1 FROM recurrence_exceptions re
				WHERE re.recurrence_id = r.id
				  AND re.excluded_date = $2::DATE
			  )
		)
		SELECT COUNT(*) FROM date_availabilities`

	var count int
	err := r.pool.QueryRow(ctx, query, calendarID, date).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get participant count for date: %w", err)
	}

	return count, nil
}

func isDuplicateKeyError(err error) bool {
	return err != nil && (
	// PostgreSQL unique constraint violation
	err.Error() == "ERROR: duplicate key value violates unique constraint" ||
		// pgx specific error code check
		containsCode(err.Error(), "23505"))
}

func containsCode(errMsg, code string) bool {
	return len(errMsg) > 0 && len(code) > 0 &&
		(errMsg[0:min(len(errMsg), 100)] != "" &&
			findSubstring(errMsg, code))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
