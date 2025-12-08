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

type DateAvailability struct {
	Date              time.Time
	ParticipantName   string
	StartTime         *string
	EndTime           *string
	Note              string
	AvailableCount    int
	TotalParticipants int
}

type AvailabilityRepository struct {
	db *pgxpool.Pool
}

func NewAvailabilityRepository(db *pgxpool.Pool) *AvailabilityRepository {
	return &AvailabilityRepository{db: db}
}

// GetEventsAboveThreshold retrieves all dates with availability >= threshold for a calendar
// This includes both manual availabilities and computed availabilities from recurrences
func (r *AvailabilityRepository) GetEventsAboveThreshold(ctx context.Context, calendarID uuid.UUID, threshold int) (map[time.Time][]DateAvailability, error) {
	query := `
		WITH
		-- Generate all dates in the calendar's recurrence range
		date_series AS (
			SELECT
				MIN(COALESCE(r.start_date, CURRENT_DATE)) as start_date,
				MAX(COALESCE(r.end_date, CURRENT_DATE + INTERVAL '1 year')) as end_date
			FROM recurrences r
			JOIN participants p ON p.id = r.participant_id
			WHERE p.calendar_id = $1
		),
		all_dates AS (
			SELECT generate_series(
				(SELECT start_date FROM date_series),
				(SELECT end_date FROM date_series),
				'1 day'::interval
			)::date as date
		),
		-- Get all availabilities (manual + computed from recurrences)
		all_availabilities AS (
			-- Manual availabilities
			SELECT
				a.date,
				a.participant_id,
				p.name as participant_name,
				a.start_time,
				a.end_time,
				COALESCE(a.note, '') as note
			FROM availabilities a
			JOIN participants p ON p.id = a.participant_id
			WHERE p.calendar_id = $1

			UNION

			-- Computed availabilities from recurrences
			SELECT
				d.date,
				r.participant_id,
				p.name as participant_name,
				r.start_time,
				r.end_time,
				COALESCE(r.note, '') as note
			FROM recurrences r
			JOIN participants p ON p.id = r.participant_id
			CROSS JOIN all_dates d
			WHERE p.calendar_id = $1
				AND d.date >= r.start_date
				AND (r.end_date IS NULL OR d.date <= r.end_date)
				AND EXTRACT(DOW FROM d.date)::int = r.day_of_week
				-- Exclude dates with exceptions
				AND NOT EXISTS (
					SELECT 1 FROM recurrence_exceptions re
					WHERE re.recurrence_id = r.id
					AND re.excluded_date = d.date
				)
				-- Exclude dates that already have manual availabilities
				AND NOT EXISTS (
					SELECT 1 FROM availabilities a
					WHERE a.participant_id = r.participant_id
					AND a.date = d.date
				)
		),
		-- Count availabilities per date
		date_counts AS (
			SELECT
				date,
				COUNT(DISTINCT participant_id) as available_count,
				(SELECT COUNT(*) FROM participants WHERE calendar_id = $1) as total_participants
			FROM all_availabilities
			GROUP BY date
			HAVING COUNT(DISTINCT participant_id) >= $2
		)
		-- Final result
		SELECT
			aa.date,
			aa.participant_name,
			aa.start_time,
			aa.end_time,
			aa.note,
			dc.available_count,
			dc.total_participants
		FROM all_availabilities aa
		JOIN date_counts dc ON dc.date = aa.date
		ORDER BY aa.date, aa.participant_name
	`

	rows, err := r.db.Query(ctx, query, calendarID, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get events above threshold: %w", err)
	}
	defer rows.Close()

	// Group by date
	eventsByDate := make(map[time.Time][]DateAvailability)

	for rows.Next() {
		var da DateAvailability
		var startTime, endTime *time.Time

		err := rows.Scan(
			&da.Date,
			&da.ParticipantName,
			&startTime,
			&endTime,
			&da.Note,
			&da.AvailableCount,
			&da.TotalParticipants,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan availability: %w", err)
		}

		// Convert time.Time to string format HH:MM
		if startTime != nil {
			timeStr := startTime.Format("15:04")
			da.StartTime = &timeStr
		}
		if endTime != nil {
			timeStr := endTime.Format("15:04")
			da.EndTime = &timeStr
		}

		eventsByDate[da.Date] = append(eventsByDate[da.Date], da)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
	}

	return eventsByDate, nil
}
