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
	ErrCalendarNotFound = errors.New("calendar not found")
)

// CalendarRepository handles calendar database operations
type CalendarRepository struct {
	Pool *pgxpool.Pool
}

// NewCalendarRepository creates a new calendar repository
func NewCalendarRepository(pool *pgxpool.Pool) *CalendarRepository {
	return &CalendarRepository{Pool: pool}
}

// Create creates a new calendar
func (r *CalendarRepository) Create(ctx context.Context, calendar *models.Calendar) error {
	query := `
		INSERT INTO calendars (id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING created_at, updated_at`

	err := r.Pool.QueryRow(ctx, query,
		calendar.ID,
		calendar.OwnerID,
		calendar.Name,
		calendar.Description,
		calendar.PublicToken,
		calendar.ICSToken,
		calendar.Threshold,
		calendar.AllowedWeekdays,
		calendar.MinDurationHours,
		calendar.Timezone,
		calendar.HolidaysPolicy,
		calendar.AllowHolidayEves,
		calendar.AllowedHours,
		calendar.NotifyOnThreshold,
		calendar.NotifyConfig,
		calendar.LockParticipants,
		calendar.StartDate,
		calendar.EndDate,
	).Scan(&calendar.CreatedAt, &calendar.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create calendar: %w", err)
	}

	return nil
}

// ParticipantInput represents participant creation data
type ParticipantInput struct {
	Name          string
	Email         *string
	EmailVerified bool
	Locale        string
}

// CreateWithParticipants creates a calendar and its participants in a transaction
func (r *CalendarRepository) CreateWithParticipants(ctx context.Context, calendar *models.Calendar, participants []ParticipantInput) ([]models.Participant, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Create calendar
	calendarQuery := `
		INSERT INTO calendars (id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING created_at, updated_at`

	err = tx.QueryRow(ctx, calendarQuery,
		calendar.ID,
		calendar.OwnerID,
		calendar.Name,
		calendar.Description,
		calendar.PublicToken,
		calendar.ICSToken,
		calendar.Threshold,
		calendar.AllowedWeekdays,
		calendar.MinDurationHours,
		calendar.Timezone,
		calendar.HolidaysPolicy,
		calendar.AllowHolidayEves,
		calendar.AllowedHours,
		calendar.NotifyOnThreshold,
		calendar.NotifyConfig,
		calendar.LockParticipants,
		calendar.StartDate,
		calendar.EndDate,
	).Scan(&calendar.CreatedAt, &calendar.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create calendar: %w", err)
	}

	// Create participants
	var createdParticipants []models.Participant
	participantQuery := `
		INSERT INTO participants (id, calendar_id, name, email, email_verified, locale)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`

	for _, input := range participants {
		// Skip empty names
		if input.Name == "" {
			continue
		}

		participant := models.Participant{
			CalendarID:    calendar.ID,
			Name:          input.Name,
			Email:         input.Email,
			EmailVerified: input.EmailVerified,
			Locale:        input.Locale,
		}
		participant.ID = uuid.New()

		err = tx.QueryRow(ctx, participantQuery,
			participant.ID,
			participant.CalendarID,
			participant.Name,
			participant.Email,
			participant.EmailVerified,
			participant.Locale,
		).Scan(&participant.CreatedAt)

		if err != nil {
			// Check for duplicate participant name
			if isDuplicateKeyError(err) {
				return nil, ErrParticipantAlreadyExists
			}
			return nil, fmt.Errorf("failed to create participant: %w", err)
		}

		createdParticipants = append(createdParticipants, participant)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return createdParticipants, nil
}

// GetByID retrieves a calendar by ID
func (r *CalendarRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Calendar, error) {
	query := `
		SELECT id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, start_date, end_date, created_at, updated_at
		FROM calendars
		WHERE id = $1`

	calendar := &models.Calendar{}
	err := r.Pool.QueryRow(ctx, query, id).Scan(
		&calendar.ID,
		&calendar.OwnerID,
		&calendar.Name,
		&calendar.Description,
		&calendar.PublicToken,
		&calendar.ICSToken,
		&calendar.Threshold,
		&calendar.AllowedWeekdays,
		&calendar.MinDurationHours,
		&calendar.Timezone,
		&calendar.HolidaysPolicy,
		&calendar.AllowHolidayEves,
		&calendar.AllowedHours,
		&calendar.NotifyOnThreshold,
		&calendar.NotifyConfig,
		&calendar.LockParticipants,
		&calendar.StartDate,
		&calendar.EndDate,
		&calendar.CreatedAt,
		&calendar.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("failed to get calendar by id: %w", err)
	}

	return calendar, nil
}

// GetByOwnerID retrieves all calendars owned by a user
func (r *CalendarRepository) GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]*models.Calendar, error) {
	query := `
		SELECT id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, start_date, end_date, created_at, updated_at
		FROM calendars
		WHERE owner_id = $1
		ORDER BY created_at DESC`

	rows, err := r.Pool.Query(ctx, query, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get calendars by owner: %w", err)
	}
	defer rows.Close()

	var calendars []*models.Calendar
	for rows.Next() {
		calendar := &models.Calendar{}
		err := rows.Scan(
			&calendar.ID,
			&calendar.OwnerID,
			&calendar.Name,
			&calendar.Description,
			&calendar.PublicToken,
			&calendar.ICSToken,
			&calendar.Threshold,
			&calendar.AllowedWeekdays,
			&calendar.MinDurationHours,
			&calendar.Timezone,
			&calendar.HolidaysPolicy,
			&calendar.AllowHolidayEves,
			&calendar.AllowedHours,
			&calendar.NotifyOnThreshold,
			&calendar.NotifyConfig,
			&calendar.LockParticipants,
			&calendar.StartDate,
			&calendar.EndDate,
			&calendar.CreatedAt,
			&calendar.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan calendar: %w", err)
		}
		calendars = append(calendars, calendar)
	}

	return calendars, nil
}

// GetByPublicToken retrieves a calendar by public token
func (r *CalendarRepository) GetByPublicToken(ctx context.Context, token string) (*models.Calendar, error) {
	query := `
		SELECT id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, start_date, end_date, created_at, updated_at
		FROM calendars
		WHERE public_token = $1`

	calendar := &models.Calendar{}
	err := r.Pool.QueryRow(ctx, query, token).Scan(
		&calendar.ID,
		&calendar.OwnerID,
		&calendar.Name,
		&calendar.Description,
		&calendar.PublicToken,
		&calendar.ICSToken,
		&calendar.Threshold,
		&calendar.AllowedWeekdays,
		&calendar.MinDurationHours,
		&calendar.Timezone,
		&calendar.HolidaysPolicy,
		&calendar.AllowHolidayEves,
		&calendar.AllowedHours,
		&calendar.NotifyOnThreshold,
		&calendar.NotifyConfig,
		&calendar.LockParticipants,
		&calendar.StartDate,
		&calendar.EndDate,
		&calendar.CreatedAt,
		&calendar.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("failed to get calendar by public token: %w", err)
	}

	return calendar, nil
}

// Update updates a calendar
func (r *CalendarRepository) Update(ctx context.Context, calendar *models.Calendar) error {
	query := `
		UPDATE calendars
		SET name = $2, description = $3, threshold = $4, allowed_weekdays = $5, min_duration_hours = $6, timezone = $7, holidays_policy = $8, allow_holiday_eves = $9, allowed_hours = $10, notify_on_threshold = $11, notify_config = $12, lock_participants = $13, start_date = $14, end_date = $15, updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`

	err := r.Pool.QueryRow(ctx, query,
		calendar.ID,
		calendar.Name,
		calendar.Description,
		calendar.Threshold,
		calendar.AllowedWeekdays,
		calendar.MinDurationHours,
		calendar.Timezone,
		calendar.HolidaysPolicy,
		calendar.AllowHolidayEves,
		calendar.AllowedHours,
		calendar.NotifyOnThreshold,
		calendar.NotifyConfig,
		calendar.LockParticipants,
		calendar.StartDate,
		calendar.EndDate,
	).Scan(&calendar.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCalendarNotFound
		}
		return fmt.Errorf("failed to update calendar: %w", err)
	}

	return nil
}

// Delete deletes a calendar
func (r *CalendarRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM calendars WHERE id = $1`

	result, err := r.Pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete calendar: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrCalendarNotFound
	}

	return nil
}

// RegenerateToken regenerates either the public_token or ics_token
func (r *CalendarRepository) RegenerateToken(ctx context.Context, id uuid.UUID, tokenType, newToken string) error {
	var query string
	if tokenType == "public" {
		query = `UPDATE calendars SET public_token = $2, updated_at = NOW() WHERE id = $1`
	} else if tokenType == "ics" {
		query = `UPDATE calendars SET ics_token = $2, updated_at = NOW() WHERE id = $1`
	} else {
		return fmt.Errorf("invalid token type: %s", tokenType)
	}

	result, err := r.Pool.Exec(ctx, query, id, newToken)
	if err != nil {
		return fmt.Errorf("failed to regenerate token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrCalendarNotFound
	}

	return nil
}

// CountByUser returns the number of calendars owned by a user
func (r *CalendarRepository) CountByUser(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM calendars WHERE owner_id = $1`

	var count int
	err := r.Pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count calendars by user: %w", err)
	}

	return count, nil
}

// CountAll returns the total number of calendars across all users
func (r *CalendarRepository) CountAll(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM calendars`

	var count int
	err := r.Pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count all calendars: %w", err)
	}

	return count, nil
}

// UpdateNotifyConfig updates the notify_config field and notify_on_threshold flag for a calendar
func (r *CalendarRepository) UpdateNotifyConfig(ctx context.Context, id uuid.UUID, notifyConfig string, enabled bool) error {
	query := `
		UPDATE calendars
		SET notify_config = $1, notify_on_threshold = $2, updated_at = NOW()
		WHERE id = $3`

	result, err := r.Pool.Exec(ctx, query, notifyConfig, enabled, id)
	if err != nil {
		return fmt.Errorf("failed to update notify config: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrCalendarNotFound
	}

	return nil
}
