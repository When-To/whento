// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrCalendarNotFound = errors.New("calendar not found")
)

// CalendarRepository handles calendar database operations
type CalendarRepository struct {
	pool *pgxpool.Pool
}

// NewCalendarRepository creates a new calendar repository
func NewCalendarRepository(pool *pgxpool.Pool) *CalendarRepository {
	return &CalendarRepository{pool: pool}
}

// AllowedHours represents the allowed hours configuration for a calendar
type AllowedHours struct {
	Weekdays    map[string]TimeRange `json:"weekdays"`
	Holidays    TimeRange            `json:"holidays"`
	HolidayEves TimeRange            `json:"holiday_eves"`
}

// TimeRange represents a time range with start and end times
type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Calendar represents calendar information needed for availability filtering
type Calendar struct {
	ID               uuid.UUID
	Threshold        int
	AllowedWeekdays  []int
	MinDurationHours int
	Timezone         string
	HolidaysPolicy   string
	AllowHolidayEves bool
	AllowedHours     AllowedHours
	LockParticipants bool
	StartDate        *time.Time
	EndDate          *time.Time
}

// GetByPublicToken retrieves a calendar ID by public token (for validation)
func (r *CalendarRepository) GetByPublicToken(ctx context.Context, token string) (uuid.UUID, error) {
	query := `SELECT id FROM calendars WHERE public_token = $1`

	var calendarID uuid.UUID
	err := r.pool.QueryRow(ctx, query, token).Scan(&calendarID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrCalendarNotFound
		}
		return uuid.Nil, fmt.Errorf("failed to get calendar by public token: %w", err)
	}

	return calendarID, nil
}

// GetCalendarInfoByPublicToken retrieves calendar information by public token
func (r *CalendarRepository) GetCalendarInfoByPublicToken(ctx context.Context, token string) (*Calendar, error) {
	query := `SELECT id, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, lock_participants, start_date, end_date FROM calendars WHERE public_token = $1`

	var cal Calendar
	var allowedHoursJSON []byte
	err := r.pool.QueryRow(ctx, query, token).Scan(
		&cal.ID,
		&cal.Threshold,
		&cal.AllowedWeekdays,
		&cal.MinDurationHours,
		&cal.Timezone,
		&cal.HolidaysPolicy,
		&cal.AllowHolidayEves,
		&allowedHoursJSON,
		&cal.LockParticipants,
		&cal.StartDate,
		&cal.EndDate,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("failed to get calendar info by public token: %w", err)
	}

	// Parse allowed_hours JSONB
	if err := parseAllowedHours(allowedHoursJSON, &cal.AllowedHours); err != nil {
		return nil, fmt.Errorf("failed to parse allowed_hours: %w", err)
	}

	return &cal, nil
}

// parseAllowedHours parses the allowed_hours JSONB field
func parseAllowedHours(data []byte, allowedHours *AllowedHours) error {
	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, allowedHours); err != nil {
		return fmt.Errorf("failed to unmarshal allowed_hours: %w", err)
	}

	return nil
}
