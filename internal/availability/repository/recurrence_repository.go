// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/availability/models"
)

type RecurrenceRepository struct {
	db *pgxpool.Pool
}

func NewRecurrenceRepository(db *pgxpool.Pool) *RecurrenceRepository {
	return &RecurrenceRepository{db: db}
}

// CreateRecurrence creates a new recurrence
func (r *RecurrenceRepository) CreateRecurrence(ctx context.Context, recurrence *models.Recurrence) error {
	query := `
		INSERT INTO recurrences (id, participant_id, day_of_week, start_time, end_time, note, start_date, end_date, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Exec(ctx, query,
		recurrence.ID,
		recurrence.ParticipantID,
		recurrence.DayOfWeek,
		recurrence.StartTime,
		recurrence.EndTime,
		recurrence.Note,
		recurrence.StartDate,
		recurrence.EndDate,
		recurrence.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create recurrence: %w", err)
	}

	return nil
}

// GetRecurrenceByID retrieves a recurrence by ID
func (r *RecurrenceRepository) GetRecurrenceByID(ctx context.Context, id uuid.UUID) (*models.Recurrence, error) {
	query := `
		SELECT id, participant_id, day_of_week,
		       TO_CHAR(start_time, 'HH24:MI') as start_time,
		       TO_CHAR(end_time, 'HH24:MI') as end_time,
		       note,
		       TO_CHAR(start_date, 'YYYY-MM-DD') as start_date,
		       TO_CHAR(end_date, 'YYYY-MM-DD') as end_date,
		       created_at
		FROM recurrences
		WHERE id = $1
	`

	var recurrence models.Recurrence
	var endDate *string

	err := r.db.QueryRow(ctx, query, id).Scan(
		&recurrence.ID,
		&recurrence.ParticipantID,
		&recurrence.DayOfWeek,
		&recurrence.StartTime,
		&recurrence.EndTime,
		&recurrence.Note,
		&recurrence.StartDate,
		&endDate,
		&recurrence.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get recurrence: %w", err)
	}

	recurrence.EndDate = endDate

	return &recurrence, nil
}

// GetRecurrencesByParticipant retrieves all recurrences for a participant
func (r *RecurrenceRepository) GetRecurrencesByParticipant(ctx context.Context, participantID uuid.UUID) ([]models.Recurrence, error) {
	query := `
		SELECT id, participant_id, day_of_week,
		       TO_CHAR(start_time, 'HH24:MI') as start_time,
		       TO_CHAR(end_time, 'HH24:MI') as end_time,
		       note,
		       TO_CHAR(start_date, 'YYYY-MM-DD') as start_date,
		       TO_CHAR(end_date, 'YYYY-MM-DD') as end_date,
		       created_at
		FROM recurrences
		WHERE participant_id = $1
		ORDER BY day_of_week, start_time
	`

	rows, err := r.db.Query(ctx, query, participantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get recurrences: %w", err)
	}
	defer rows.Close()

	var recurrences []models.Recurrence
	for rows.Next() {
		var rec models.Recurrence

		err := rows.Scan(
			&rec.ID,
			&rec.ParticipantID,
			&rec.DayOfWeek,
			&rec.StartTime,
			&rec.EndTime,
			&rec.Note,
			&rec.StartDate,
			&rec.EndDate,
			&rec.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recurrence: %w", err)
		}

		recurrences = append(recurrences, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating recurrences: %w", err)
	}

	return recurrences, nil
}

// GetRecurrencesByCalendar retrieves all recurrences for all participants in a calendar
func (r *RecurrenceRepository) GetRecurrencesByCalendar(ctx context.Context, calendarID uuid.UUID) ([]models.Recurrence, error) {
	query := `
		SELECT r.id, r.participant_id, r.day_of_week,
		       TO_CHAR(r.start_time, 'HH24:MI') as start_time,
		       TO_CHAR(r.end_time, 'HH24:MI') as end_time,
		       r.note,
		       TO_CHAR(r.start_date, 'YYYY-MM-DD') as start_date,
		       TO_CHAR(r.end_date, 'YYYY-MM-DD') as end_date,
		       r.created_at
		FROM recurrences r
		JOIN participants p ON r.participant_id = p.id
		WHERE p.calendar_id = $1
		ORDER BY r.day_of_week, r.start_time
	`

	rows, err := r.db.Query(ctx, query, calendarID)
	if err != nil {
		return nil, fmt.Errorf("failed to get recurrences: %w", err)
	}
	defer rows.Close()

	var recurrences []models.Recurrence
	for rows.Next() {
		var rec models.Recurrence

		err := rows.Scan(
			&rec.ID,
			&rec.ParticipantID,
			&rec.DayOfWeek,
			&rec.StartTime,
			&rec.EndTime,
			&rec.Note,
			&rec.StartDate,
			&rec.EndDate,
			&rec.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recurrence: %w", err)
		}

		recurrences = append(recurrences, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating recurrences: %w", err)
	}

	return recurrences, nil
}

// UpdateRecurrence updates an existing recurrence
func (r *RecurrenceRepository) UpdateRecurrence(ctx context.Context, recurrence *models.Recurrence) error {
	query := `
		UPDATE recurrences
		SET day_of_week = $1,
		    start_time = $2,
		    end_time = $3,
		    note = $4,
		    start_date = $5,
		    end_date = $6
		WHERE id = $7
	`

	result, err := r.db.Exec(ctx, query,
		recurrence.DayOfWeek,
		recurrence.StartTime,
		recurrence.EndTime,
		recurrence.Note,
		recurrence.StartDate,
		recurrence.EndDate,
		recurrence.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update recurrence: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("recurrence not found")
	}

	return nil
}

// DeleteRecurrence deletes a recurrence
func (r *RecurrenceRepository) DeleteRecurrence(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM recurrences WHERE id = $1`

	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete recurrence: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("recurrence not found")
	}

	return nil
}

// CreateException creates a new exception for a recurrence
func (r *RecurrenceRepository) CreateException(ctx context.Context, exception *models.RecurrenceException) error {
	query := `
		INSERT INTO recurrence_exceptions (id, recurrence_id, excluded_date, created_at)
		VALUES ($1, $2, $3, $4)
	`

	_, err := r.db.Exec(ctx, query,
		exception.ID,
		exception.RecurrenceID,
		exception.ExcludedDate,
		exception.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create exception: %w", err)
	}

	return nil
}

// GetExceptionsByRecurrence retrieves all exceptions for a recurrence
func (r *RecurrenceRepository) GetExceptionsByRecurrence(ctx context.Context, recurrenceID uuid.UUID) ([]models.RecurrenceException, error) {
	query := `
		SELECT id, recurrence_id, TO_CHAR(excluded_date, 'YYYY-MM-DD') as excluded_date, created_at
		FROM recurrence_exceptions
		WHERE recurrence_id = $1
		ORDER BY excluded_date
	`

	rows, err := r.db.Query(ctx, query, recurrenceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get exceptions: %w", err)
	}
	defer rows.Close()

	var exceptions []models.RecurrenceException
	for rows.Next() {
		var exc models.RecurrenceException

		err := rows.Scan(
			&exc.ID,
			&exc.RecurrenceID,
			&exc.ExcludedDate,
			&exc.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan exception: %w", err)
		}

		exceptions = append(exceptions, exc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating exceptions: %w", err)
	}

	return exceptions, nil
}

// DeleteException deletes an exception
func (r *RecurrenceRepository) DeleteException(ctx context.Context, recurrenceID uuid.UUID, excludedDate string) error {
	query := `DELETE FROM recurrence_exceptions WHERE recurrence_id = $1 AND excluded_date = $2`

	result, err := r.db.Exec(ctx, query, recurrenceID, excludedDate)
	if err != nil {
		return fmt.Errorf("failed to delete exception: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("exception not found")
	}

	return nil
}
