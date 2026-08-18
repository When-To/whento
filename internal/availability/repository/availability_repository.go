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

	"github.com/whento/pkg/dberr"
	"github.com/whento/whento/internal/availability/models"
)

// Sentinels, so that callers can tell a request the user got wrong from a database
// that refused. Compare with errors.Is — never with the error text, which is what the
// service layer used to do and which made these messages part of the API by accident.
var (
	ErrAvailabilityNotFound = errors.New("availability not found")
	ErrAvailabilityExists   = errors.New("availability already exists for this date")
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
		if dberr.IsUniqueViolation(err) {
			return ErrAvailabilityExists
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating availabilities: %w", err)
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

// occurrencesQuery expands a calendar's availability over a date range.
//
// The single answer to "who is available, when". It exists because there were several and
// they disagreed: the threshold count expanded recurrences in SQL, the day and range views
// expanded them again in Go, and the notification list read the availabilities table raw —
// so a participant available every Friday was counted towards the threshold, missing from
// the list in the email, and never told about their own Friday.
//
// Recurrences are expanded here rather than stored, which is why reading the table alone
// was never the same question. $1 is the calendar, $2 and $3 the inclusive bounds.
//
// The manual half comes back untouched. The recurrence half drops any date already covered
// by a manual answer for the same participant — the rule all three implementations already
// shared, and the one worth stating once: an answer someone typed wins over the pattern
// they set earlier, times and all, even when the typed one is all-day and the pattern is
// not.
//
// Two conditions that used to differ between the implementations are settled. The
// availabilities half does not filter on source = 'manual': nothing writes any other value,
// so it selected nothing today while quietly discarding rows an older version had written.
// And the recurrence half does not test start_date IS NULL, because the column is NOT NULL.
const occurrencesQuery = `
		SELECT a.date, a.participant_id, p.name,
		       TO_CHAR(a.start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(a.end_time, 'HH24:MI') AS end_time,
		       COALESCE(a.note, '') AS note
		FROM availabilities a
		JOIN participants p ON p.id = a.participant_id
		WHERE p.calendar_id = $1
		  AND a.date BETWEEN $2::DATE AND $3::DATE

		UNION ALL

		SELECT d.date, r.participant_id, p.name,
		       TO_CHAR(r.start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(r.end_time, 'HH24:MI') AS end_time,
		       COALESCE(r.note, '') AS note
		FROM recurrences r
		JOIN participants p ON p.id = r.participant_id
		CROSS JOIN (
			SELECT generate_series($2::DATE, $3::DATE, '1 day'::INTERVAL)::DATE AS date
		) d
		WHERE p.calendar_id = $1
		  AND EXTRACT(DOW FROM d.date)::int = r.day_of_week
		  AND d.date >= r.start_date
		  AND (r.end_date IS NULL OR d.date <= r.end_date)
		  AND NOT EXISTS (
			SELECT 1 FROM recurrence_exceptions re
			WHERE re.recurrence_id = r.id AND re.excluded_date = d.date
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM availabilities a
			WHERE a.participant_id = r.participant_id AND a.date = d.date
		  )
		ORDER BY 1 ASC, 3 ASC`

// GetOccurrencesForRange returns every availability in the range, recurrences expanded.
func (r *AvailabilityRepository) GetOccurrencesForRange(
	ctx context.Context,
	calendarID uuid.UUID,
	startDate, endDate time.Time,
) ([]models.Occurrence, error) {
	rows, err := r.pool.Query(ctx, occurrencesQuery, calendarID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get occurrences: %w", err)
	}
	defer rows.Close()

	var occurrences []models.Occurrence
	for rows.Next() {
		var occurrence models.Occurrence
		if err := rows.Scan(
			&occurrence.Date,
			&occurrence.ParticipantID,
			&occurrence.ParticipantName,
			&occurrence.StartTime,
			&occurrence.EndTime,
			&occurrence.Note,
		); err != nil {
			return nil, fmt.Errorf("failed to scan occurrence: %w", err)
		}
		occurrences = append(occurrences, occurrence)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating occurrences: %w", err)
	}

	return occurrences, nil
}

// GetOccurrencesForDate is the single-date grain of the same question.
func (r *AvailabilityRepository) GetOccurrencesForDate(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
) ([]models.Occurrence, error) {
	return r.GetOccurrencesForRange(ctx, calendarID, date, date)
}

// GetAvailableParticipantsForDate returns who is available on a date, by name.
//
// A projection of the occurrences rather than a query of its own, so it cannot answer a
// different question from the one the day view and the feed are answering. The notify
// service wants identities and deliberately not times — see the comment where it builds
// the recipient list.
func (r *AvailabilityRepository) GetAvailableParticipantsForDate(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
) ([]models.AvailableParticipant, error) {
	occurrences, err := r.GetOccurrencesForDate(ctx, calendarID, date)
	if err != nil {
		return nil, err
	}

	seen := make(map[uuid.UUID]struct{}, len(occurrences))
	participants := make([]models.AvailableParticipant, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if _, ok := seen[occurrence.ParticipantID]; ok {
			continue
		}
		seen[occurrence.ParticipantID] = struct{}{}
		participants = append(participants, models.AvailableParticipant{
			ID:   occurrence.ParticipantID,
			Name: occurrence.ParticipantName,
		})
	}

	return participants, nil
}

// GetParticipantCountForDate returns how many participants are available at once.
//
// The number the notification threshold is compared against, and it now means what the
// interface has always shown: participants whose windows actually overlap, not merely
// participants who answered for that day. Those were different numbers, so a calendar
// could send "3/3 available" for three people who were never free at the same moment,
// while the page for that date showed 1 and the feed emitted no event at all.
//
// An all-day answer spans the whole day, so a calendar answered in whole days — the
// common case — gives exactly the count it always did.
func (r *AvailabilityRepository) GetParticipantCountForDate(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
) (int, error) {
	occurrences, err := r.GetOccurrencesForDate(ctx, calendarID, date)
	if err != nil {
		return 0, err
	}

	// How long a meeting has to last is the calendar's policy, not the caller's, so it
	// is read here rather than threaded through every caller. Without it this counted
	// any overlap however brief, while the page for the same date counted only overlaps
	// long enough to hold the event — so a notification could announce a gathering the
	// interface was already saying could not happen.
	var minDurationHours int
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(min_duration_hours, 0) FROM calendars WHERE id = $1`,
		calendarID,
	).Scan(&minDurationHours); err != nil {
		return 0, fmt.Errorf("failed to read the calendar's minimum duration: %w", err)
	}

	windows := make([]models.TimeWindow, 0, len(occurrences))
	for _, occurrence := range occurrences {
		windows = append(windows, occurrence.Window())
	}

	return models.MaxSimultaneousFor(windows, minDurationHours*60), nil
}
