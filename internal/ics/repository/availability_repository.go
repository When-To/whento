// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// CalendarThreshold names a calendar and the number of available participants its feed
// treats as an event, so that several calendars can be asked for at once.
type CalendarThreshold struct {
	CalendarID uuid.UUID
	Threshold  int
}

// eventsAboveThresholdQuery expands every recurrence of a calendar over its date range,
// subtracts the exceptions and the dates already covered by a manual availability, then
// keeps only the dates that reach the threshold. $1 is the calendar, $2 the threshold.
//
// It is shared verbatim by the single-calendar and the batched read, so the unified feed
// cannot drift from the per-calendar feed.
const eventsAboveThresholdQuery = `
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

// GetEventsAboveThreshold retrieves all dates with availability >= threshold for a
// calendar. This includes both manual availabilities and computed availabilities from
// recurrences.
func (r *AvailabilityRepository) GetEventsAboveThreshold(
	ctx context.Context,
	calendarID uuid.UUID,
	threshold int,
) (map[time.Time][]DateAvailability, error) {
	rows, err := r.db.Query(ctx, eventsAboveThresholdQuery, calendarID, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get events above threshold: %w", err)
	}

	return scanEventsByDate(rows)
}

// GetEventsAboveThresholdForCalendars retrieves the events of several calendars, each
// with its own threshold, in a single round trip, keyed by calendar.
//
// The unified feed used to call GetEventsAboveThreshold once per included calendar: a
// user with 20 calendars paid 20 sequential round trips on every poll, and ICS clients
// poll on a timer. The queries are pipelined into one batch instead, so the cost is one
// round trip whatever the number of calendars. The SQL each one runs is unchanged, which
// is what keeps the recurrence and threshold semantics identical to the single-calendar
// feed — this is a transport change, not a query change.
func (r *AvailabilityRepository) GetEventsAboveThresholdForCalendars(
	ctx context.Context,
	calendars []CalendarThreshold,
) (map[uuid.UUID]map[time.Time][]DateAvailability, error) {
	eventsByCalendar := make(map[uuid.UUID]map[time.Time][]DateAvailability, len(calendars))
	if len(calendars) == 0 {
		return eventsByCalendar, nil
	}

	batch := &pgx.Batch{}
	for _, cal := range calendars {
		batch.Queue(eventsAboveThresholdQuery, cal.CalendarID, cal.Threshold)
	}

	br := r.db.SendBatch(ctx, batch)

	for _, cal := range calendars {
		rows, err := br.Query()
		if err != nil {
			_ = br.Close()

			return nil, fmt.Errorf("failed to get events for calendar %s: %w", cal.CalendarID, err)
		}

		eventsByDate, err := scanEventsByDate(rows)
		if err != nil {
			_ = br.Close()

			return nil, fmt.Errorf("failed to read events for calendar %s: %w", cal.CalendarID, err)
		}

		eventsByCalendar[cal.CalendarID] = eventsByDate
	}

	// Close reports the first error from any result the loop above did not consume.
	// Swallowing it would let a half-read batch pass for a complete feed.
	if err := br.Close(); err != nil {
		return nil, fmt.Errorf("failed to close events batch: %w", err)
	}

	return eventsByCalendar, nil
}

// scanEventsByDate drains one result set of eventsAboveThresholdQuery and groups it by
// date. It takes ownership of rows and closes them.
func scanEventsByDate(rows pgx.Rows) (map[time.Time][]DateAvailability, error) {
	defer rows.Close()

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
