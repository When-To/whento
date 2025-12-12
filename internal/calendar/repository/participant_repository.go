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
		INSERT INTO participants (id, calendar_id, name, locale)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`

	err := r.pool.QueryRow(ctx, query,
		participant.ID,
		participant.CalendarID,
		participant.Name,
		participant.Locale,
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
		SELECT id, calendar_id, name, email, email_verified,
		       email_verification_token, email_verification_token_expires_at, locale, created_at
		FROM participants
		WHERE id = $1`

	participant := &models.Participant{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&participant.ID,
		&participant.CalendarID,
		&participant.Name,
		&participant.Email,
		&participant.EmailVerified,
		&participant.EmailVerificationToken,
		&participant.EmailVerificationTokenExpiresAt,
		&participant.Locale,
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
		SELECT id, calendar_id, name, email, email_verified,
		       email_verification_token, email_verification_token_expires_at, locale, created_at
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
			&participant.Email,
			&participant.EmailVerified,
			&participant.EmailVerificationToken,
			&participant.EmailVerificationTokenExpiresAt,
			&participant.Locale,
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
		SELECT id, calendar_id, name, email, email_verified,
		       email_verification_token, email_verification_token_expires_at, locale, created_at
		FROM participants
		WHERE calendar_id = $1 AND name = $2`

	participant := &models.Participant{}
	err := r.pool.QueryRow(ctx, query, calendarID, name).Scan(
		&participant.ID,
		&participant.CalendarID,
		&participant.Name,
		&participant.Email,
		&participant.EmailVerified,
		&participant.EmailVerificationToken,
		&participant.EmailVerificationTokenExpiresAt,
		&participant.Locale,
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

// UpdateLocale updates the locale for a participant
func (r *ParticipantRepository) UpdateLocale(ctx context.Context, id uuid.UUID, locale string) error {
	query := `UPDATE participants SET locale = $1 WHERE id = $2`

	result, err := r.pool.Exec(ctx, query, locale, id)
	if err != nil {
		return fmt.Errorf("failed to update participant locale: %w", err)
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

// SetEmailVerificationToken sets the email verification token for a participant
func (r *ParticipantRepository) SetEmailVerificationToken(
	ctx context.Context,
	participantID uuid.UUID,
	email, token string,
	expiresAt time.Time,
) error {
	query := `
		UPDATE participants
		SET email = $1,
		    email_verification_token = $2,
		    email_verification_token_expires_at = $3,
		    email_verified = false
		WHERE id = $4`

	result, err := r.pool.Exec(ctx, query, email, token, expiresAt, participantID)
	if err != nil {
		if isDuplicateKeyError(err) {
			return errors.New("email already in use for this calendar")
		}
		return fmt.Errorf("failed to set email verification token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrParticipantNotFound
	}

	return nil
}

// GetByVerificationToken retrieves a participant by verification token
func (r *ParticipantRepository) GetByVerificationToken(
	ctx context.Context,
	token string,
) (*models.Participant, error) {
	query := `
		SELECT id, calendar_id, name, email, email_verified,
		       email_verification_token, email_verification_token_expires_at, locale, created_at
		FROM participants
		WHERE email_verification_token = $1
		  AND email_verification_token_expires_at > NOW()`

	participant := &models.Participant{}
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&participant.ID,
		&participant.CalendarID,
		&participant.Name,
		&participant.Email,
		&participant.EmailVerified,
		&participant.EmailVerificationToken,
		&participant.EmailVerificationTokenExpiresAt,
		&participant.Locale,
		&participant.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid or expired verification token")
		}
		return nil, fmt.Errorf("failed to get participant by verification token: %w", err)
	}

	return participant, nil
}

// VerifyEmail marks a participant's email as verified and clears the token
func (r *ParticipantRepository) VerifyEmail(ctx context.Context, participantID uuid.UUID) error {
	query := `
		UPDATE participants
		SET email_verified = true,
		    email_verification_token = NULL,
		    email_verification_token_expires_at = NULL
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, participantID)
	if err != nil {
		return fmt.Errorf("failed to verify email: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrParticipantNotFound
	}

	return nil
}

// ClearEmailVerificationToken clears the verification token
func (r *ParticipantRepository) ClearEmailVerificationToken(
	ctx context.Context,
	participantID uuid.UUID,
) error {
	query := `
		UPDATE participants
		SET email_verification_token = NULL,
		    email_verification_token_expires_at = NULL
		WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, participantID)
	if err != nil {
		return fmt.Errorf("failed to clear email verification token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrParticipantNotFound
	}

	return nil
}

// SetEmailAsVerified sets an email for a participant and marks it as verified immediately (used for owner participant)
func (r *ParticipantRepository) SetEmailAsVerified(
	ctx context.Context,
	participantID uuid.UUID,
	email string,
) error {
	query := `
		UPDATE participants
		SET email = $1,
		    email_verified = true,
		    email_verification_token = NULL,
		    email_verification_token_expires_at = NULL
		WHERE id = $2`

	result, err := r.pool.Exec(ctx, query, email, participantID)
	if err != nil {
		if isDuplicateKeyError(err) {
			return errors.New("email already in use for this calendar")
		}
		return fmt.Errorf("failed to set email as verified: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrParticipantNotFound
	}

	return nil
}

// GetVerifiedParticipantsByCalendar retrieves participants with verified emails
func (r *ParticipantRepository) GetVerifiedParticipantsByCalendar(
	ctx context.Context,
	calendarID uuid.UUID,
) ([]models.Participant, error) {
	query := `
		SELECT id, calendar_id, name, email, email_verified,
		       email_verification_token, email_verification_token_expires_at, locale, created_at
		FROM participants
		WHERE calendar_id = $1
		  AND email IS NOT NULL
		  AND email_verified = true
		ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query, calendarID)
	if err != nil {
		return nil, fmt.Errorf("failed to get verified participants: %w", err)
	}
	defer rows.Close()

	var participants []models.Participant
	for rows.Next() {
		participant := models.Participant{}
		err := rows.Scan(
			&participant.ID,
			&participant.CalendarID,
			&participant.Name,
			&participant.Email,
			&participant.EmailVerified,
			&participant.EmailVerificationToken,
			&participant.EmailVerificationTokenExpiresAt,
			&participant.Locale,
			&participant.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, participant)
	}

	return participants, nil
}
