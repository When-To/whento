// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/calendar/models"
)

var (
	ErrParticipantNotFound      = errors.New("participant not found")
	ErrParticipantAlreadyExists = errors.New("participant with this name already exists in this calendar")
)

// ParticipantRepository handles participant database operations
type ParticipantRepository struct {
	pool *pgxpool.Pool
}

// NewParticipantRepository creates a new participant repository
func NewParticipantRepository(pool *pgxpool.Pool) *ParticipantRepository {
	return &ParticipantRepository{pool: pool}
}

// Create creates a new participant
func (r *ParticipantRepository) Create(ctx context.Context, participant *models.Participant) error {
	query := `
		INSERT INTO participants (id, calendar_id, name)
		VALUES ($1, $2, $3)
		RETURNING created_at`

	err := r.pool.QueryRow(ctx, query,
		participant.ID,
		participant.CalendarID,
		participant.Name,
	).Scan(&participant.CreatedAt)

	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrParticipantAlreadyExists
		}
		return fmt.Errorf("failed to create participant: %w", err)
	}

	return nil
}

// GetByID retrieves a participant by ID
func (r *ParticipantRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Participant, error) {
	query := `
		SELECT id, calendar_id, name, created_at
		FROM participants
		WHERE id = $1`

	participant := &models.Participant{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&participant.ID,
		&participant.CalendarID,
		&participant.Name,
		&participant.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrParticipantNotFound
		}
		return nil, fmt.Errorf("failed to get participant by id: %w", err)
	}

	return participant, nil
}

// GetByCalendarID retrieves all participants for a calendar
func (r *ParticipantRepository) GetByCalendarID(ctx context.Context, calendarID uuid.UUID) ([]models.Participant, error) {
	query := `
		SELECT id, calendar_id, name, created_at
		FROM participants
		WHERE calendar_id = $1
		ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query, calendarID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants by calendar: %w", err)
	}
	defer rows.Close()

	var participants []models.Participant
	for rows.Next() {
		participant := models.Participant{}
		err := rows.Scan(
			&participant.ID,
			&participant.CalendarID,
			&participant.Name,
			&participant.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, participant)
	}

	return participants, nil
}

// GetByCalendarIDAndName retrieves a participant by calendar ID and name
func (r *ParticipantRepository) GetByCalendarIDAndName(ctx context.Context, calendarID uuid.UUID, name string) (*models.Participant, error) {
	query := `
		SELECT id, calendar_id, name, created_at
		FROM participants
		WHERE calendar_id = $1 AND name = $2`

	participant := &models.Participant{}
	err := r.pool.QueryRow(ctx, query, calendarID, name).Scan(
		&participant.ID,
		&participant.CalendarID,
		&participant.Name,
		&participant.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrParticipantNotFound
		}
		return nil, fmt.Errorf("failed to get participant by name: %w", err)
	}

	return participant, nil
}

// Update updates a participant's name
func (r *ParticipantRepository) Update(ctx context.Context, id uuid.UUID, name string) error {
	query := `
		UPDATE participants
		SET name = $1
		WHERE id = $2`

	result, err := r.pool.Exec(ctx, query, name, id)
	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrParticipantAlreadyExists
		}
		return fmt.Errorf("failed to update participant: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrParticipantNotFound
	}

	return nil
}

// Delete deletes a participant
func (r *ParticipantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM participants WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete participant: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrParticipantNotFound
	}

	return nil
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
