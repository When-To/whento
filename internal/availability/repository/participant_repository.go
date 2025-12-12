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
)

var (
	ErrParticipantNotFound = errors.New("participant not found")
)

// Participant represents a participant with basic info
type Participant struct {
	ID            uuid.UUID
	CalendarID    uuid.UUID
	Name          string
	Email         *string
	EmailVerified bool
}

// ParticipantRepository handles participant database operations
type ParticipantRepository struct {
	pool *pgxpool.Pool
}

// NewParticipantRepository creates a new participant repository
func NewParticipantRepository(pool *pgxpool.Pool) *ParticipantRepository {
	return &ParticipantRepository{pool: pool}
}

// GetByID retrieves a participant by ID
func (r *ParticipantRepository) GetByID(ctx context.Context, id uuid.UUID) (*Participant, error) {
	query := `
		SELECT id, calendar_id, name, email, email_verified
		FROM participants
		WHERE id = $1`

	participant := &Participant{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&participant.ID,
		&participant.CalendarID,
		&participant.Name,
		&participant.Email,
		&participant.EmailVerified,
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
func (r *ParticipantRepository) GetByCalendarID(ctx context.Context, calendarID uuid.UUID) ([]*Participant, error) {
	query := `
		SELECT id, calendar_id, name, email, email_verified
		FROM participants
		WHERE calendar_id = $1
		ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query, calendarID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants by calendar: %w", err)
	}
	defer rows.Close()

	var participants []*Participant
	for rows.Next() {
		participant := &Participant{}
		err := rows.Scan(
			&participant.ID,
			&participant.CalendarID,
			&participant.Name,
			&participant.Email,
			&participant.EmailVerified,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, participant)
	}

	return participants, nil
}
