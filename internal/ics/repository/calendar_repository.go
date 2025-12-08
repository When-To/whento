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
)

type Calendar struct {
	ID                uuid.UUID
	Name              string
	Description       string
	Threshold         int
	AllowedWeekdays   []int
	MinDurationHours  int
	Timezone          string
	HolidaysPolicy    string
	AllowHolidayEves  bool
	OwnerID           uuid.UUID
	TotalParticipants int
	StartDate         *time.Time
	EndDate           *time.Time
}

type CalendarRepository struct {
	db *pgxpool.Pool
}

func NewCalendarRepository(db *pgxpool.Pool) *CalendarRepository {
	return &CalendarRepository{db: db}
}

// GetByICSToken retrieves a calendar by its ICS token
func (r *CalendarRepository) GetByICSToken(ctx context.Context, icsToken string) (*Calendar, error) {
	query := `
		SELECT
			c.id,
			c.name,
			COALESCE(c.description, ''),
			c.threshold,
			c.allowed_weekdays,
			c.min_duration_hours,
			c.timezone,
			c.holidays_policy,
			c.allow_holiday_eves,
			c.owner_id,
			c.start_date,
			c.end_date,
			COUNT(p.id) as total_participants
		FROM calendars c
		LEFT JOIN participants p ON p.calendar_id = c.id
		WHERE c.ics_token = $1
		GROUP BY c.id, c.name, c.description, c.threshold, c.allowed_weekdays, c.min_duration_hours, c.timezone, c.holidays_policy, c.allow_holiday_eves, c.owner_id, c.start_date, c.end_date
	`

	var cal Calendar
	err := r.db.QueryRow(ctx, query, icsToken).Scan(
		&cal.ID,
		&cal.Name,
		&cal.Description,
		&cal.Threshold,
		&cal.AllowedWeekdays,
		&cal.MinDurationHours,
		&cal.Timezone,
		&cal.HolidaysPolicy,
		&cal.AllowHolidayEves,
		&cal.OwnerID,
		&cal.StartDate,
		&cal.EndDate,
		&cal.TotalParticipants,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get calendar by ics token: %w", err)
	}

	return &cal, nil
}
