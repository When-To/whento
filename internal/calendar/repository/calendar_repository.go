// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/dberr"
	"github.com/whento/whento/internal/calendar/models"
)

var (
	ErrCalendarNotFound = errors.New("calendar not found")
	// ErrInvalidDateRange reports a patch whose effective end_date, checked against
	// the locked row, falls before its start_date.
	ErrInvalidDateRange = errors.New("end_date must be after start_date")
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
		INSERT INTO calendars (id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, allow_anonymous_participants, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
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
		calendar.AllowAnonymousParticipants,
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
	defer func() { _ = tx.Rollback(ctx) }()

	// Create calendar
	calendarQuery := `
		INSERT INTO calendars (id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, allow_anonymous_participants, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
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
		calendar.AllowAnonymousParticipants,
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
			if dberr.IsUniqueViolation(err) {
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
		SELECT id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, allow_anonymous_participants, start_date, end_date, created_at, updated_at
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
		&calendar.AllowAnonymousParticipants,
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
		SELECT id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, allow_anonymous_participants, start_date, end_date, created_at, updated_at
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
			&calendar.AllowAnonymousParticipants,
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating calendars: %w", err)
	}

	return calendars, nil
}

// GetByPublicToken retrieves a calendar by public token
func (r *CalendarRepository) GetByPublicToken(ctx context.Context, token string) (*models.Calendar, error) {
	query := `
		SELECT id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, allow_anonymous_participants, start_date, end_date, created_at, updated_at
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
		&calendar.AllowAnonymousParticipants,
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

const calendarColumns = `id, owner_id, name, description, public_token, ics_token, threshold, allowed_weekdays, min_duration_hours, timezone, holidays_policy, allow_holiday_eves, allowed_hours, notify_on_threshold, notify_config, lock_participants, allow_anonymous_participants, start_date, end_date, created_at, updated_at`

// rowScanner is the minimal scan surface pgx.Row and pgx.Rows share, so one
// helper serves both the single-row RETURNING paths and the multi-row lists.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanCalendar scans the full calendar projection into a new model.
func scanCalendar(row rowScanner) (*models.Calendar, error) {
	calendar := &models.Calendar{}
	err := row.Scan(
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
		&calendar.AllowAnonymousParticipants,
		&calendar.StartDate,
		&calendar.EndDate,
		&calendar.CreatedAt,
		&calendar.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return calendar, nil
}

// CalendarPatch is a field-specific calendar write. Nil pointers are left
// unchanged. Notification JSON is never part of this write; that column has
// its own statement so a settings save cannot restore a stale copy of it.
// AllowedHours, when set, is applied in the same transaction/statement as the
// regular columns: one API request lands as one commit, so a partial patch
// (regular fields written but the allowed-hours merge failing) cannot happen.
type CalendarPatch struct {
	Name                       *string
	Description                *string
	Threshold                  *int
	AllowedWeekdays            []int
	MinDurationHours           *int
	Timezone                   *string
	HolidaysPolicy             *string
	AllowHolidayEves           *bool
	NotifyOnThreshold          *bool
	LockParticipants           *bool
	AllowAnonymousParticipants *bool
	StartDate                  *time.Time
	EndDate                    *time.Time
	ClearStartDate             bool
	ClearEndDate               bool
	AllowedHours               *AllowedHoursPatch
}

// Patch updates only the fields the caller set and returns the complete,
// post-write row. When AllowedHours is set the regular columns and the allowed
// hours merge are applied in one transaction (a single UPDATE ... RETURNING), so
// a failure in either half rolls the whole write back; without AllowedHours the
// existing single UPDATE is already atomic. Two disjoint patches of the same row
// both persist; neither writes the columns the other owns. Returning the full row
// — not a timestamp or the pre-write snapshot — is what lets the service answer a
// PATCH with a response that reflects a concurrent disjoint commit made before
// this one landed: the RETURNING scan is the row after this statement, not before
// it.
func (r *CalendarRepository) Patch(ctx context.Context, id uuid.UUID, patch CalendarPatch) (*models.Calendar, error) {
	sets := make([]string, 0, 16)
	args := []any{id}
	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if patch.Name != nil {
		add("name", *patch.Name)
	}
	if patch.Description != nil {
		add("description", *patch.Description)
	}
	if patch.Threshold != nil {
		add("threshold", *patch.Threshold)
	}
	if len(patch.AllowedWeekdays) > 0 {
		add("allowed_weekdays", patch.AllowedWeekdays)
	}
	if patch.MinDurationHours != nil {
		add("min_duration_hours", *patch.MinDurationHours)
	}
	if patch.Timezone != nil {
		add("timezone", *patch.Timezone)
	}
	if patch.HolidaysPolicy != nil {
		add("holidays_policy", *patch.HolidaysPolicy)
	}
	if patch.AllowHolidayEves != nil {
		add("allow_holiday_eves", *patch.AllowHolidayEves)
	}
	if patch.NotifyOnThreshold != nil {
		add("notify_on_threshold", *patch.NotifyOnThreshold)
	}
	if patch.LockParticipants != nil {
		add("lock_participants", *patch.LockParticipants)
	}
	if patch.AllowAnonymousParticipants != nil {
		add("allow_anonymous_participants", *patch.AllowAnonymousParticipants)
	}
	if patch.ClearStartDate {
		add("start_date", nil)
	} else if patch.StartDate != nil {
		add("start_date", *patch.StartDate)
	}
	if patch.ClearEndDate {
		add("end_date", nil)
	} else if patch.EndDate != nil {
		add("end_date", *patch.EndDate)
	}

	// No allowed-hours merge involved: a plain single UPDATE is already atomic, so
	// keep the fast path. An empty patch still returns the current row.
	if patch.AllowedHours == nil && patch.StartDate == nil && patch.EndDate == nil && !patch.ClearStartDate && !patch.ClearEndDate {
		if len(sets) == 0 {
			calendar, err := scanCalendar(r.Pool.QueryRow(ctx, `SELECT `+calendarColumns+` FROM calendars WHERE id = $1`, id))
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, ErrCalendarNotFound
				}
				return nil, err
			}
			return calendar, nil
		}
		query := `UPDATE calendars SET ` + strings.Join(sets, ", ") + `, updated_at = NOW() WHERE id = $1 RETURNING ` + calendarColumns
		calendar, err := scanCalendar(r.Pool.QueryRow(ctx, query, args...))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrCalendarNotFound
			}
			return nil, fmt.Errorf("failed to patch calendar: %w", err)
		}
		return calendar, nil
	}

	// Allowed-hours merge needs the persisted document (single bounds are
	// normalized against their stored counterpart) and must land atomically with
	// the regular columns, so it runs in one transaction: lock the row, merge in
	// memory, then a single UPDATE ... RETURNING, then commit. Any failure rolls
	// the whole patch back — the regular fields are never committed on their own.
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin patch: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existing *string
	var start, end *time.Time
	if err := tx.QueryRow(ctx, `SELECT allowed_hours, start_date, end_date FROM calendars WHERE id = $1 FOR UPDATE`, id).Scan(&existing, &start, &end); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("read allowed_hours for patch: %w", err)
	}

	// Check dates against the locked row. Equal dates allow one-day calendars.
	if patch.ClearStartDate {
		start = nil
	} else if patch.StartDate != nil {
		start = patch.StartDate
	}
	if patch.ClearEndDate {
		end = nil
	} else if patch.EndDate != nil {
		end = patch.EndDate
	}
	if start != nil && end != nil && end.Before(*start) {
		return nil, ErrInvalidDateRange
	}
	if patch.AllowedHours != nil {
		merged, err := MergeAllowedHours(existing, *patch.AllowedHours)
		if err != nil {
			return nil, err
		}
		args = append(args, *merged)
		sets = append(sets, fmt.Sprintf("allowed_hours = $%d", len(args)))
	}

	query := `UPDATE calendars SET ` + strings.Join(sets, ", ") + `, updated_at = NOW() WHERE id = $1 RETURNING ` + calendarColumns
	calendar, err := scanCalendar(tx.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("failed to patch calendar: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit patch: %w", err)
	}
	return calendar, nil
}

// AllowedHoursPatch names the allowed_hours JSON subpaths a request actually
// set. A nil field means "leave that subpath alone"; a provided field is merged
// into the stored document (see MergeAllowedHours), so disjoint updates to
// different allowed-hour subfields (say holidays.start from one request and
// holidays.end from another) both persist.
type AllowedHoursPatch struct {
	WeekdayTimes      *string // JSON object for the weekdays key, e.g. {"1":{"start":"09:00","end":"18:00"}}
	HolidayMinTime    *string
	HolidayMaxTime    *string
	HolidayEveMinTime *string
	HolidayEveMaxTime *string
}

// MergeAllowedHours merges an allowed-hours sub-path update into the persisted
// document and returns the complete JSON to store. Weekdays replace wholesale
// (that is how the settings form removes days; "weekday map replaces rather than
// merges"). A single holiday or holiday-eve bound is first merged with its
// persisted counterpart and the pair then normalized, so a minimum-only or
// maximum-only update can never persist an inverted range (e.g. a stored
// 11:00-15:00 window combined with min 16:00 lands as 15:00-16:00, not
// 16:00-15:00). The service level no longer reads the column at all; the merge
// happens here, against the row locked by the enclosing transaction.
func MergeAllowedHours(existing *string, patch AllowedHoursPatch) (*string, error) {
	weekdayTimes, holidayMin, holidayMax, holidayEveMin, holidayEveMax, err := models.ParseAllowedHoursJSON(existing)
	if err != nil {
		return nil, err
	}

	if patch.WeekdayTimes != nil {
		weekdayTimes, err = models.ParseWeekdayTimesJSON(*patch.WeekdayTimes)
		if err != nil {
			return nil, err
		}
	}

	if patch.HolidayMinTime != nil || patch.HolidayMaxTime != nil {
		minTime := holidayMin
		maxTime := holidayMax
		if patch.HolidayMinTime != nil {
			minTime = *patch.HolidayMinTime
		}
		if patch.HolidayMaxTime != nil {
			maxTime = *patch.HolidayMaxTime
		}
		holidayMin, holidayMax = models.NormalizeHolidayTimes(minTime, maxTime)
	}

	if patch.HolidayEveMinTime != nil || patch.HolidayEveMaxTime != nil {
		minTime := holidayEveMin
		maxTime := holidayEveMax
		if patch.HolidayEveMinTime != nil {
			minTime = *patch.HolidayEveMinTime
		}
		if patch.HolidayEveMaxTime != nil {
			maxTime = *patch.HolidayEveMaxTime
		}
		holidayEveMin, holidayEveMax = models.NormalizeHolidayTimes(minTime, maxTime)
	}

	// Build the document directly (not through BuildAllowedHoursJSON) so absent
	// weekdays stay absent: sub-path patching must not inject default 00:00-23:59
	// entries for days the request never mentioned.
	allowedHours := models.AllowedHours{
		Weekdays: make(map[string]models.TimeSlot),
		Holidays: models.TimeSlot{Start: holidayMin, End: holidayMax},
		HolidayEves: models.TimeSlot{
			Start: holidayEveMin,
			End:   holidayEveMax,
		},
	}
	for day, tr := range weekdayTimes {
		allowedHours.Weekdays[day] = models.TimeSlot{Start: tr.MinTime, End: tr.MaxTime}
	}
	jsonBytes, err := json.Marshal(allowedHours)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal allowed_hours: %w", err)
	}
	out := string(jsonBytes)

	return &out, nil
}

// UpdateThreshold changes only the threshold. A participant removal must not
// rewrite the rest of the row from a snapshot taken before the delete.
func (r *CalendarRepository) UpdateThreshold(ctx context.Context, id uuid.UUID, threshold int) error {
	result, err := r.Pool.Exec(ctx, `
		UPDATE calendars SET threshold = $2, updated_at = NOW() WHERE id = $1`, id, threshold)
	if err != nil {
		return fmt.Errorf("failed to update calendar threshold: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrCalendarNotFound
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
	switch tokenType {
	case "public":
		query = `UPDATE calendars SET public_token = $2, updated_at = NOW() WHERE id = $1`
	case "ics":
		query = `UPDATE calendars SET ics_token = $2, updated_at = NOW() WHERE id = $1`
	default:
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
