// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/datevalidation"
	"github.com/whento/pkg/logger"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
)

// Sentinels. handleAvailabilityError and handleRecurrenceError turn these into status
// codes with errors.Is, and anything that does not match one of them is reported as a
// 500 — so a rejection the caller could act on has to be one of these, wrapped with %w,
// rather than a bare fmt.Errorf.
var (
	ErrCalendarNotFound        = errors.New("calendar not found")
	ErrParticipantNotFound     = errors.New("participant not found")
	ErrInvalidParticipantID    = errors.New("invalid participant ID")
	ErrInvalidDate             = errors.New("invalid date format, expected YYYY-MM-DD")
	ErrInvalidDateRange        = errors.New("end date must be after start date")
	ErrDateBeforeCalendarStart = errors.New("date is before the calendar start date")
	ErrDateAfterCalendarEnd    = errors.New("date is after the calendar end date")
	ErrInvalidTime             = errors.New("invalid time format, expected HH:MM")
	ErrInvalidTimeRange        = errors.New("end time must be after start time")
	ErrTimeOutsideAllowedHours = errors.New("time range does not fit within allowed hours for this day")
	ErrDurationTooShort        = errors.New("availability duration is less than the minimum required")
	ErrAvailabilityExists      = errors.New("availability already exists for this date")
	ErrAvailabilityNotFound    = errors.New("availability not found")
	ErrInvalidRecurrenceID     = errors.New("invalid recurrence ID")
	ErrRecurrenceNotFound      = errors.New("recurrence not found")
	ErrExceptionNotFound       = errors.New("exception not found")
	ErrRecurrenceOverlap       = errors.New("recurrence overlaps with an existing recurrence on the same day")
	ErrInvalidDayOfWeek        = errors.New("day_of_week must be between 0 (Sunday) and 6 (Saturday)")
	ErrWeekdayNotAllowed       = errors.New("this day of the week is not allowed for this calendar")
	ErrDateInPast              = errors.New("cannot modify availability for past dates")
)

// AvailabilityRepository defines the interface for availability repository operations
type AvailabilityRepository interface {
	Create(ctx context.Context, availability *models.Availability) error
	GetByParticipantID(ctx context.Context, participantID uuid.UUID) ([]*models.Availability, error)
	GetByParticipantIDWithDateRange(ctx context.Context, participantID uuid.UUID, startDate, endDate *time.Time) ([]*models.Availability, error)
	GetByParticipantAndDate(ctx context.Context, participantID uuid.UUID, date time.Time) (*models.Availability, error)
	// The expansion the summary endpoints read: manual answers plus the recurrence
	// occurrences that survive them, which is a question the availabilities table alone
	// cannot answer since recurrences are never stored.
	GetOccurrencesForDate(ctx context.Context, calendarID uuid.UUID, date time.Time) ([]models.Occurrence, error)
	GetOccurrencesForRange(ctx context.Context, calendarID uuid.UUID, startDate, endDate time.Time) ([]models.Occurrence, error)
	GetParticipantCountForDate(ctx context.Context, calendarID uuid.UUID, date time.Time) (int, error)
	Update(ctx context.Context, availability *models.Availability) error
	Delete(ctx context.Context, participantID uuid.UUID, date time.Time) error
}

// CalendarRepository defines the interface for calendar repository operations
type CalendarRepository interface {
	GetByPublicToken(ctx context.Context, token string) (uuid.UUID, error)
	GetCalendarInfoByPublicToken(ctx context.Context, token string) (*repository.Calendar, error)
}

// ParticipantRepository defines the interface for participant repository operations
type ParticipantRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*repository.Participant, error)
	GetByCalendarID(ctx context.Context, calendarID uuid.UUID) ([]*repository.Participant, error)
}

// RecurrenceRepository defines the interface for recurrence repository operations
type RecurrenceRepository interface {
	CreateRecurrence(ctx context.Context, recurrence *models.Recurrence) error
	GetRecurrencesByParticipant(ctx context.Context, participantID uuid.UUID) ([]models.Recurrence, error)
	GetRecurrenceByID(ctx context.Context, id uuid.UUID) (*models.Recurrence, error)
	UpdateRecurrence(ctx context.Context, recurrence *models.Recurrence) error
	DeleteRecurrence(ctx context.Context, id uuid.UUID) error
	CreateException(ctx context.Context, exception *models.RecurrenceException) error
	GetExceptionsByRecurrenceIDs(
		ctx context.Context,
		recurrenceIDs []uuid.UUID,
	) (map[uuid.UUID][]models.RecurrenceException, error)
	DeleteException(ctx context.Context, recurrenceID uuid.UUID, excludedDate string) error
}

// NotifyService defines the interface for notification service operations
type NotifyService interface {
	CheckThresholdAndNotify(ctx context.Context, calendarID uuid.UUID, date time.Time, previousCount int) error
}

// AvailabilityService handles availability business logic
type AvailabilityService struct {
	availabilityRepo AvailabilityRepository
	calendarRepo     CalendarRepository
	participantRepo  ParticipantRepository
	recurrenceRepo   RecurrenceRepository
	notifyService    NotifyService
	cache            cache.Cache
}

// NewAvailabilityService creates a new availability service
func NewAvailabilityService(
	availabilityRepo AvailabilityRepository,
	calendarRepo CalendarRepository,
	participantRepo ParticipantRepository,
	recurrenceRepo RecurrenceRepository,
	notifyService NotifyService,
	c cache.Cache,
) *AvailabilityService {
	return &AvailabilityService{
		availabilityRepo: availabilityRepo,
		calendarRepo:     calendarRepo,
		participantRepo:  participantRepo,
		recurrenceRepo:   recurrenceRepo,
		notifyService:    notifyService,
		cache:            c,
	}
}

// notifyThresholdTimeout bounds the detached threshold check. Nothing is waiting on it —
// the request that triggered it has already been answered — so the deadline is there to
// stop a wedged SMTP or database call from pinning a goroutine for the life of the
// process, one per availability written.
const notifyThresholdTimeout = 30 * time.Second

// notifyThresholdAsync runs the threshold check outside the request that caused it.
//
// Threshold notifications are the emails this product exists to send, so a failure here
// is worth a log line: the three copies of this block that it replaces each ended in an
// empty branch under a comment reading "log only", and every failure vanished.
//
// The context is detached rather than dropped: cancellation is what must not carry over —
// the request is finished, and its context is cancelled the moment the handler returns —
// while the request ID must, or the log line has nothing to be correlated with.
func (s *AvailabilityService) notifyThresholdAsync(
	ctx context.Context,
	calendarID uuid.UUID,
	date time.Time,
	previousCount int,
) {
	log := logger.FromContext(ctx)
	detached := context.WithoutCancel(ctx)

	go func() {
		// middleware.Recoverer only wraps the request goroutine. A panic raised here
		// would take the whole process down with it, calendar and all.
		defer func() {
			if r := recover(); r != nil {
				log.Error("threshold notification panicked",
					"calendar_id", calendarID,
					"date", date.Format("2006-01-02"),
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()

		notifyCtx, cancel := context.WithTimeout(detached, notifyThresholdTimeout)
		defer cancel()

		if err := s.notifyService.CheckThresholdAndNotify(notifyCtx, calendarID, date, previousCount); err != nil {
			log.Error("threshold notification failed",
				"calendar_id", calendarID,
				"date", date.Format("2006-01-02"),
				"previous_count", previousCount,
				"error", err,
			)
		}
	}()
}

// CreateAvailability creates a new availability for a participant
func (s *AvailabilityService) CreateAvailability(ctx context.Context, token, participantID string, req *models.CreateAvailabilityRequest) (*models.AvailabilityResponse, error) {
	// Validate calendar token and get calendar info
	calendarInfo, err := s.calendarRepo.GetCalendarInfoByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}
	calendarID := calendarInfo.ID

	// Parse and validate participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant exists and belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return nil, ErrParticipantNotFound
		}
		return nil, err
	}

	if participant.CalendarID != calendarID {
		return nil, ErrParticipantNotFound
	}

	// Parse date
	date, err := parseDate(req.Date)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// Check if date is in the past
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if date.Before(today) {
		return nil, ErrDateInPast
	}

	// Validate that the date is within calendar's date range if set
	if calendarInfo.StartDate != nil && date.Before(*calendarInfo.StartDate) {
		return nil, fmt.Errorf("%w (%s)", ErrDateBeforeCalendarStart, calendarInfo.StartDate.Format("2006-01-02"))
	}
	if calendarInfo.EndDate != nil && date.After(*calendarInfo.EndDate) {
		return nil, fmt.Errorf("%w (%s)", ErrDateAfterCalendarEnd, calendarInfo.EndDate.Format("2006-01-02"))
	}

	// Validate that the date is allowed for this calendar
	// This checks weekday, holidays policy, and holiday eves
	if !datevalidation.IsDateAllowed(date, calendarInfo.Timezone, calendarInfo.AllowedWeekdays, calendarInfo.HolidaysPolicy, calendarInfo.AllowHolidayEves) {
		return nil, ErrWeekdayNotAllowed
	}

	// Parse and validate times if provided
	var startTime, endTime *string
	if req.StartTime != nil && *req.StartTime != "" {
		if !isValidTime(*req.StartTime) {
			return nil, ErrInvalidTime
		}
		startTime = req.StartTime
	}
	if req.EndTime != nil && *req.EndTime != "" {
		if !isValidTime(*req.EndTime) {
			return nil, ErrInvalidTime
		}
		endTime = req.EndTime
	}

	// Normalize time range (swap if start > end)
	startTime, endTime = normalizeTimeRange(startTime, endTime)

	// Adjust times based on allowed hours for this calendar
	startTime, endTime = adjustTimesByAllowedHours(date, startTime, endTime, calendarInfo)

	// Validate time range if both provided
	if startTime != nil && endTime != nil {
		if !isValidTimeRange(*startTime, *endTime) {
			return nil, ErrInvalidTimeRange
		}

		// Validate duration against calendar's min_duration_hours
		if calendarInfo.MinDurationHours > 0 {
			duration := calculateDuration(*startTime, *endTime)
			if duration < float64(calendarInfo.MinDurationHours) {
				return nil, ErrDurationTooShort
			}
		}
	}

	// Get participant count BEFORE creating availability (for threshold detection)
	previousCount, err := s.availabilityRepo.GetParticipantCountForDate(ctx, calendarID, date)
	if err != nil {
		// Log error but continue - notification just won't have accurate previous count
		previousCount = -1
	}

	// Create availability
	availability := &models.Availability{
		ParticipantID: partID,
		Date:          date,
		StartTime:     startTime,
		EndTime:       endTime,
		Note:          req.Note,
		Source:        "manual",
		RecurrenceID:  nil,
	}
	availability.ID = uuid.New()

	if err := s.availabilityRepo.Create(ctx, availability); err != nil {
		if errors.Is(err, repository.ErrAvailabilityExists) {
			return nil, ErrAvailabilityExists
		}
		return nil, err
	}

	// The availability is already stored: the threshold check must not be able to fail
	// the write, so it runs detached from the request.
	s.notifyThresholdAsync(ctx, calendarID, date, previousCount)

	return toAvailabilityResponse(availability, participant.Name, participant.Email, participant.EmailVerified), nil
}

// GetParticipantAvailabilities retrieves all availabilities for a participant
func (s *AvailabilityService) GetParticipantAvailabilities(ctx context.Context, token, participantID, startDateStr, endDateStr string) (*models.ParticipantAvailabilitiesResponse, error) {
	// Validate calendar token
	calendarID, err := s.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return nil, ErrParticipantNotFound
		}
		return nil, err
	}

	if participant.CalendarID != calendarID {
		return nil, ErrParticipantNotFound
	}

	// Parse optional date range
	var startDate, endDate *time.Time
	if startDateStr != "" {
		parsed, err := time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return nil, ErrInvalidDate
		}
		startDate = &parsed
	}
	if endDateStr != "" {
		parsed, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			return nil, ErrInvalidDate
		}
		endDate = &parsed
	}

	// Get availabilities with optional date filtering
	availabilities, err := s.availabilityRepo.GetByParticipantIDWithDateRange(ctx, partID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Convert to response with participant info and availability items
	items := make([]models.AvailabilityItem, len(availabilities))
	for i, avail := range availabilities {
		items[i] = models.AvailabilityItem{
			ID:        avail.ID,
			Date:      formatDate(avail.Date),
			StartTime: avail.StartTime,
			EndTime:   avail.EndTime,
			Note:      avail.Note,
			CreatedAt: avail.CreatedAt,
			UpdatedAt: avail.UpdatedAt,
		}
	}

	return &models.ParticipantAvailabilitiesResponse{
		Participant: models.ParticipantInfo{
			ID:            participant.ID,
			Name:          participant.Name,
			Email:         participant.Email,
			EmailVerified: participant.EmailVerified,
		},
		Availabilities: items,
	}, nil
}

// UpdateAvailability updates an existing availability
func (s *AvailabilityService) UpdateAvailability(ctx context.Context, token, participantID, dateStr string, req *models.UpdateAvailabilityRequest) (*models.AvailabilityResponse, error) {
	// Validate calendar token and get calendar info
	calendarInfo, err := s.calendarRepo.GetCalendarInfoByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}
	calendarID := calendarInfo.ID

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return nil, ErrParticipantNotFound
		}
		return nil, err
	}

	if participant.CalendarID != calendarID {
		return nil, ErrParticipantNotFound
	}

	// Parse date
	date, err := parseDate(dateStr)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// Check if date is in the past
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if date.Before(today) {
		return nil, ErrDateInPast
	}

	// Get existing availability
	availability, err := s.availabilityRepo.GetByParticipantAndDate(ctx, partID, date)
	if err != nil {
		if errors.Is(err, repository.ErrAvailabilityNotFound) {
			return nil, ErrAvailabilityNotFound
		}
		return nil, err
	}

	// Update fields if provided
	if req.StartTime != nil {
		if *req.StartTime == "" {
			availability.StartTime = nil
		} else {
			if !isValidTime(*req.StartTime) {
				return nil, ErrInvalidTime
			}
			availability.StartTime = req.StartTime
		}
	}

	if req.EndTime != nil {
		if *req.EndTime == "" {
			availability.EndTime = nil
		} else {
			if !isValidTime(*req.EndTime) {
				return nil, ErrInvalidTime
			}
			availability.EndTime = req.EndTime
		}
	}

	// Normalize time range (swap if start > end)
	availability.StartTime, availability.EndTime = normalizeTimeRange(availability.StartTime, availability.EndTime)

	// Adjust times based on allowed hours for this calendar
	availability.StartTime, availability.EndTime = adjustTimesByAllowedHours(date, availability.StartTime, availability.EndTime, calendarInfo)

	// Validate time range if both are set
	if availability.StartTime != nil && availability.EndTime != nil && *availability.StartTime != "" && *availability.EndTime != "" {
		if !isValidTimeRange(*availability.StartTime, *availability.EndTime) {
			return nil, ErrInvalidTimeRange
		}

		// Validate duration against calendar's min_duration_hours
		calendarInfo, err := s.calendarRepo.GetCalendarInfoByPublicToken(ctx, token)
		if err == nil && calendarInfo.MinDurationHours > 0 {
			duration := calculateDuration(*availability.StartTime, *availability.EndTime)
			if duration < float64(calendarInfo.MinDurationHours) {
				return nil, ErrDurationTooShort
			}
		}
	}

	if req.Note != nil {
		availability.Note = *req.Note
	}

	// Get participant count (for threshold detection - count doesn't change on update)
	currentCount, err := s.availabilityRepo.GetParticipantCountForDate(ctx, calendarID, date)
	if err != nil {
		currentCount = -1
	}

	// Update in database
	if err := s.availabilityRepo.Update(ctx, availability); err != nil {
		return nil, err
	}

	// An update does not change the participant count, but the threshold configuration
	// may have changed since, so the check still runs.
	s.notifyThresholdAsync(ctx, calendarID, date, currentCount)

	return toAvailabilityResponse(availability, participant.Name, participant.Email, participant.EmailVerified), nil
}

// DeleteAvailability deletes an availability
func (s *AvailabilityService) DeleteAvailability(ctx context.Context, token, participantID, dateStr string) error {
	// Validate calendar token
	calendarID, err := s.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return ErrCalendarNotFound
		}
		return err
	}

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return ErrParticipantNotFound
		}
		return err
	}

	if participant.CalendarID != calendarID {
		return ErrParticipantNotFound
	}

	// Parse date
	date, err := parseDate(dateStr)
	if err != nil {
		return ErrInvalidDate
	}

	// Check if date is in the past
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if date.Before(today) {
		return ErrDateInPast
	}

	// Get participant count BEFORE deleting (for threshold detection)
	previousCount, err := s.availabilityRepo.GetParticipantCountForDate(ctx, calendarID, date)
	if err != nil {
		// Log error but continue - notification just won't have accurate previous count
		previousCount = -1
	}

	// Delete availability
	if err := s.availabilityRepo.Delete(ctx, partID, date); err != nil {
		if errors.Is(err, repository.ErrAvailabilityNotFound) {
			return ErrAvailabilityNotFound
		}
		return err
	}

	s.notifyThresholdAsync(ctx, calendarID, date, previousCount)

	return nil
}

// exceptionsFor loads the exceptions of every given recurrence in a single query.
//
// This used to be a loop issuing one query per recurrence. GetDateSummary and
// GetRangeSummary both walk every recurrence of a calendar, and both are what the SSE
// availability notification makes each open browser reload, so the fan-out multiplied
// with the number of recurrences and the number of viewers at once. Cost is now one
// round trip regardless of how many recurrences the calendar has.
func (s *AvailabilityService) exceptionsFor(
	ctx context.Context,
	recurrences []models.Recurrence,
) (map[uuid.UUID][]models.RecurrenceException, error) {
	ids := make([]uuid.UUID, 0, len(recurrences))
	for _, rec := range recurrences {
		ids = append(ids, rec.ID)
	}

	return s.recurrenceRepo.GetExceptionsByRecurrenceIDs(ctx, ids)
}

// GetDateSummary gets all participants available on a specific date.
//
// participantID is the caller's own participant id, and is the one id left unmasked
// when the calendar has lock_participants set — exactly as in GetRangeSummary. This
// endpoint used to answer with models.DateAvailabilitySummary, whose ParticipantID is
// a plain uuid.UUID and therefore always serialised, so a locked calendar handed every
// participant's id to anyone holding the public token. Since participant access here is
// capability-based (public token + participant id *is* the authorisation), that was
// both halves of the credential for every participant on the calendar.
func (s *AvailabilityService) GetDateSummary(ctx context.Context, token, dateStr, participantID string) (*models.PublicDateAvailabilitySummary, error) {
	// Validate calendar token and get calendar info (including min_duration_hours)
	calendarInfo, err := s.calendarRepo.GetCalendarInfoByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}
	calendarID := calendarInfo.ID

	// Parse date
	date, err := parseDate(dateStr)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// One question, one answer. This used to read the availabilities table and then
	// expand the recurrences again in Go — a third copy of a rule that the threshold
	// count and the ICS feed each had their own version of, and the copies disagreed.
	occurrences, err := s.availabilityRepo.GetOccurrencesForDate(ctx, calendarID, date)
	if err != nil {
		return nil, err
	}

	participantSummaries := summariesFrom(occurrences)

	// The minimum duration bounds which overlaps count, not whether the date is worth
	// answering about. It used to blank the whole summary — count and participants —
	// whenever the window shared by *everyone* fell short, so a day where two people
	// were free at different hours reported nobody available at all, with nothing to
	// say why. The feed never worked that way: it drops a slot that is too short and
	// keeps the rest.
	return &models.PublicDateAvailabilitySummary{
		Date:         dateStr,
		TotalCount:   countThatCanMeet(participantSummaries, calendarInfo.MinDurationHours),
		Participants: filterParticipantSummaries(calendarInfo.LockParticipants, participantID, participantSummaries),
	}, nil
}

// filterParticipantSummaries masks participant IDs based on lock_participants setting and participant_id
func filterParticipantSummaries(lockParticipants bool, participantID string, summaries []models.ParticipantAvailabilitySummary) []models.PublicParticipantAvailabilitySummary {
	publicSummaries := make([]models.PublicParticipantAvailabilitySummary, len(summaries))

	var parsedID uuid.UUID
	var err error
	if participantID != "" {
		parsedID, err = uuid.Parse(participantID)
		if err != nil {
			parsedID = uuid.Nil
		}
	}

	for i, summary := range summaries {
		if !lockParticipants {
			// Return all participants with their IDs visible
			publicSummaries[i] = models.PublicParticipantAvailabilitySummary{
				ParticipantID:   &summary.ParticipantID,
				ParticipantName: summary.ParticipantName,
				StartTime:       summary.StartTime,
				EndTime:         summary.EndTime,
				Note:            summary.Note,
			}
		} else if participantID != "" && summary.ParticipantID == parsedID {
			// Keep this participant with their ID
			publicSummaries[i] = models.PublicParticipantAvailabilitySummary{
				ParticipantID:   &summary.ParticipantID,
				ParticipantName: summary.ParticipantName,
				StartTime:       summary.StartTime,
				EndTime:         summary.EndTime,
				Note:            summary.Note,
			}
		} else {
			// Mask the ID
			publicSummaries[i] = models.PublicParticipantAvailabilitySummary{
				ParticipantID:   nil,
				ParticipantName: summary.ParticipantName,
				StartTime:       summary.StartTime,
				EndTime:         summary.EndTime,
				Note:            summary.Note,
			}
		}
	}

	return publicSummaries
}

// GetRangeSummary gets availability summary over a date range
func (s *AvailabilityService) GetRangeSummary(ctx context.Context, token, startDateStr, endDateStr, participantID string) ([]models.PublicDateAvailabilitySummary, error) {
	// Validate calendar token and get calendar info (including min_duration_hours)
	calendarInfo, err := s.calendarRepo.GetCalendarInfoByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}
	calendarID := calendarInfo.ID

	// Parse dates
	startDate, err := parseDate(startDateStr)
	if err != nil {
		return nil, ErrInvalidDate
	}

	endDate, err := parseDate(endDateStr)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// Validate range
	if endDate.Before(startDate) {
		return nil, fmt.Errorf("%w (%s..%s)", ErrInvalidDateRange, startDateStr, endDateStr)
	}

	occurrences, err := s.availabilityRepo.GetOccurrencesForRange(ctx, calendarID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Grouped here rather than expanded here: the repository already decided which
	// recurrence occurrences survive a manual answer, so this only has to bucket them.
	dateMap := make(map[string][]models.ParticipantAvailabilitySummary)
	for _, occurrence := range occurrences {
		dateKey := formatDate(occurrence.Date)
		dateMap[dateKey] = append(dateMap[dateKey], summaryFrom(occurrence))
	}

	// Build response (with min_duration_hours filter if configured)
	// Initialised rather than left nil so an empty range serialises as [] and not null.
	// The frontend carried an Array.isArray guard purely to absorb that, and any client
	// calling .map() on the payload would have thrown.
	summaries := make([]models.PublicDateAvailabilitySummary, 0, len(dateMap))
	for date, participants := range dateMap {
		summaries = append(summaries, models.PublicDateAvailabilitySummary{
			Date:         date,
			TotalCount:   countThatCanMeet(participants, calendarInfo.MinDurationHours),
			Participants: filterParticipantSummaries(calendarInfo.LockParticipants, participantID, participants),
		})
	}

	return summaries, nil
}

// Helper functions

func parseDate(dateStr string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, err
	}
	return date, nil
}

func formatDate(date time.Time) string {
	return date.Format("2006-01-02")
}

func isValidTime(timeStr string) bool {
	_, err := time.Parse("15:04", timeStr)
	return err == nil
}

func isValidTimeRange(startTime, endTime string) bool {
	start, err1 := time.Parse("15:04", startTime)
	end, err2 := time.Parse("15:04", endTime)
	if err1 != nil || err2 != nil {
		return false
	}
	return end.After(start)
}

// normalizeTimeRange ensures start time is always before end time by swapping if necessary
// Returns the normalized start and end times
func normalizeTimeRange(startTime, endTime *string) (*string, *string) {
	// If either is nil or empty, return as-is
	if startTime == nil || endTime == nil || *startTime == "" || *endTime == "" {
		return startTime, endTime
	}

	// Parse times
	start, err1 := time.Parse("15:04", *startTime)
	end, err2 := time.Parse("15:04", *endTime)

	// If parsing fails, return as-is (validation will catch this later)
	if err1 != nil || err2 != nil {
		return startTime, endTime
	}

	// If start is after end, swap them
	if start.After(end) {
		return endTime, startTime
	}

	// Otherwise return as-is
	return startTime, endTime
}

// calculateDuration calculates the duration in hours between two time strings (format "HH:MM")
func calculateDuration(startTime, endTime string) float64 {
	start, err1 := time.Parse("15:04", startTime)
	end, err2 := time.Parse("15:04", endTime)
	if err1 != nil || err2 != nil {
		return 0.0
	}

	duration := end.Sub(start).Hours()
	if duration < 0 {
		return 0.0
	}

	return duration
}

func toAvailabilityResponse(availability *models.Availability, participantName string, participantEmail *string, participantEmailVerified bool) *models.AvailabilityResponse {
	return &models.AvailabilityResponse{
		ID:                       availability.ID,
		ParticipantID:            availability.ParticipantID,
		ParticipantName:          participantName,
		ParticipantEmail:         participantEmail,
		ParticipantEmailVerified: participantEmailVerified,
		Date:                     formatDate(availability.Date),
		StartTime:                availability.StartTime,
		EndTime:                  availability.EndTime,
		Note:                     availability.Note,
		CreatedAt:                availability.CreatedAt,
		UpdatedAt:                availability.UpdatedAt,
	}
}

// Recurrence methods

// CreateRecurrence creates a new recurrence for a participant
func (s *AvailabilityService) CreateRecurrence(ctx context.Context, token, participantID string, req *models.CreateRecurrenceRequest) (*models.Recurrence, error) {
	// Validate calendar token and get calendar info
	calendarInfo, err := s.calendarRepo.GetCalendarInfoByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}
	calendarID := calendarInfo.ID

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return nil, ErrParticipantNotFound
		}
		return nil, err
	}

	if participant.CalendarID != calendarID {
		return nil, ErrParticipantNotFound
	}

	// Validate day of week (already validated by struct tags, but double-check)
	if req.DayOfWeek == nil || *req.DayOfWeek < 0 || *req.DayOfWeek > 6 {
		return nil, ErrInvalidDayOfWeek
	}

	// Validate that this weekday is allowed for this calendar
	if !datevalidation.IsWeekdayAllowed(*req.DayOfWeek, calendarInfo.AllowedWeekdays) {
		return nil, ErrWeekdayNotAllowed
	}

	// Validate times if provided
	if req.StartTime != nil && !isValidTime(*req.StartTime) {
		return nil, ErrInvalidTime
	}
	if req.EndTime != nil && !isValidTime(*req.EndTime) {
		return nil, ErrInvalidTime
	}

	// Normalize time range (swap if start > end)
	normalizedStart, normalizedEnd := normalizeTimeRange(req.StartTime, req.EndTime)

	// Parse start date for validation
	startDate, err := parseDate(req.StartDate)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// Parse end date if provided (for validation)
	if req.EndDate != nil {
		endDate, err := parseDate(*req.EndDate)
		if err != nil {
			return nil, ErrInvalidDate
		}
		if endDate.Before(startDate) {
			return nil, fmt.Errorf("%w (%s..%s)", ErrInvalidDateRange, req.StartDate, *req.EndDate)
		}
	}

	// Adjust times based on allowed hours for this day of week
	adjustedStartTime, adjustedEndTime := adjustTimesByAllowedHoursForWeekday(*req.DayOfWeek, normalizedStart, normalizedEnd, calendarInfo)

	// Validate time range if both times are provided (end must be after start)
	// After normalization and adjustment, an invalid range means the time doesn't fit within allowed hours
	if adjustedStartTime != nil && adjustedEndTime != nil {
		if !isValidTimeRange(*adjustedStartTime, *adjustedEndTime) {
			return nil, ErrTimeOutsideAllowedHours
		}
	}

	// Validate duration against calendar's min_duration_hours (after adjustment)
	if adjustedStartTime != nil && adjustedEndTime != nil && calendarInfo.MinDurationHours > 0 {
		duration := calculateDuration(*adjustedStartTime, *adjustedEndTime)
		if duration < float64(calendarInfo.MinDurationHours) {
			return nil, ErrDurationTooShort
		}
	}

	// Check for overlapping recurrences on the same day of week
	if err := s.checkRecurrenceOverlap(ctx, partID, *req.DayOfWeek, req.StartDate, req.EndDate, nil); err != nil {
		return nil, err
	}

	// Create recurrence (use string dates from request)
	recurrence := &models.Recurrence{
		ParticipantID: partID,
		DayOfWeek:     *req.DayOfWeek,
		StartTime:     adjustedStartTime,
		EndTime:       adjustedEndTime,
		Note:          req.Note,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
		CreatedAt:     time.Now(),
	}
	recurrence.ID = uuid.New()

	if err := s.recurrenceRepo.CreateRecurrence(ctx, recurrence); err != nil {
		return nil, err
	}

	return recurrence, nil
}

// GetParticipantRecurrences retrieves all recurrences for a participant
func (s *AvailabilityService) GetParticipantRecurrences(ctx context.Context, token, participantID string) ([]models.RecurrenceWithExceptions, error) {
	// Validate calendar token
	calendarID, err := s.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return nil, ErrParticipantNotFound
		}
		return nil, err
	}

	if participant.CalendarID != calendarID {
		return nil, ErrParticipantNotFound
	}

	// Get recurrences
	recurrences, err := s.recurrenceRepo.GetRecurrencesByParticipant(ctx, partID)
	if err != nil {
		return nil, err
	}

	// Get the exceptions of every recurrence in one query rather than one per recurrence
	exceptionsMap, err := s.exceptionsFor(ctx, recurrences)
	if err != nil {
		return nil, err
	}

	var result []models.RecurrenceWithExceptions
	for _, rec := range recurrences {
		exceptions := exceptionsMap[rec.ID]
		if exceptions == nil {
			exceptions = []models.RecurrenceException{}
		}
		result = append(result, models.RecurrenceWithExceptions{
			Recurrence: rec,
			Exceptions: exceptions,
		})
	}

	return result, nil
}

// UpdateRecurrence updates an existing recurrence
func (s *AvailabilityService) UpdateRecurrence(ctx context.Context, token, participantID, recurrenceID string, req *models.UpdateRecurrenceRequest) (*models.Recurrence, error) {
	// Validate calendar token and get calendar info
	calendarInfo, err := s.calendarRepo.GetCalendarInfoByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}
	calendarID := calendarInfo.ID

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return nil, ErrInvalidParticipantID
	}

	// Parse recurrence ID
	recID, err := uuid.Parse(recurrenceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRecurrenceID, err)
	}

	// Verify participant belongs to calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		return nil, ErrParticipantNotFound
	}
	if participant.CalendarID != calendarID {
		return nil, ErrParticipantNotFound
	}

	// Verify recurrence exists and belongs to participant
	existingRec, err := s.recurrenceRepo.GetRecurrenceByID(ctx, recID)
	if err != nil {
		return nil, ErrRecurrenceNotFound
	}
	// A recurrence belonging to someone else is reported as missing rather than as a
	// refusal: the holder of the link learns nothing about what the ID names.
	if existingRec.ParticipantID != partID {
		return nil, ErrRecurrenceNotFound
	}

	// Validate day of week
	if req.DayOfWeek == nil || *req.DayOfWeek < 0 || *req.DayOfWeek > 6 {
		return nil, ErrInvalidDayOfWeek
	}

	// Validate that this weekday is allowed for this calendar
	if !datevalidation.IsWeekdayAllowed(*req.DayOfWeek, calendarInfo.AllowedWeekdays) {
		return nil, ErrWeekdayNotAllowed
	}

	// Validate times if provided
	if req.StartTime != nil && !isValidTime(*req.StartTime) {
		return nil, ErrInvalidTime
	}
	if req.EndTime != nil && !isValidTime(*req.EndTime) {
		return nil, ErrInvalidTime
	}

	// Normalize time range (swap if start > end)
	normalizedStart, normalizedEnd := normalizeTimeRange(req.StartTime, req.EndTime)

	// Parse start date for validation
	startDate, err := parseDate(req.StartDate)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// Parse end date if provided (for validation)
	if req.EndDate != nil {
		endDate, err := parseDate(*req.EndDate)
		if err != nil {
			return nil, ErrInvalidDate
		}
		if endDate.Before(startDate) {
			return nil, fmt.Errorf("%w (%s..%s)", ErrInvalidDateRange, req.StartDate, *req.EndDate)
		}
	}

	// Adjust times based on allowed hours for this day of week
	adjustedStartTime, adjustedEndTime := adjustTimesByAllowedHoursForWeekday(*req.DayOfWeek, normalizedStart, normalizedEnd, calendarInfo)

	// Validate time range if both times are provided (end must be after start)
	// After normalization and adjustment, an invalid range means the time doesn't fit within allowed hours
	if adjustedStartTime != nil && adjustedEndTime != nil {
		if !isValidTimeRange(*adjustedStartTime, *adjustedEndTime) {
			return nil, ErrTimeOutsideAllowedHours
		}
	}

	// Validate duration against calendar's min_duration_hours (after adjustment)
	if adjustedStartTime != nil && adjustedEndTime != nil && calendarInfo.MinDurationHours > 0 {
		duration := calculateDuration(*adjustedStartTime, *adjustedEndTime)
		if duration < float64(calendarInfo.MinDurationHours) {
			return nil, ErrDurationTooShort
		}
	}

	// Check for overlapping recurrences on the same day of week (excluding the current one)
	if err := s.checkRecurrenceOverlap(ctx, partID, *req.DayOfWeek, req.StartDate, req.EndDate, &recID); err != nil {
		return nil, err
	}

	// Update recurrence
	recurrence := &models.Recurrence{
		ParticipantID: partID,
		DayOfWeek:     *req.DayOfWeek,
		StartTime:     adjustedStartTime,
		EndTime:       adjustedEndTime,
		Note:          req.Note,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
		CreatedAt:     existingRec.CreatedAt,
	}
	recurrence.ID = recID

	if err := s.recurrenceRepo.UpdateRecurrence(ctx, recurrence); err != nil {
		if errors.Is(err, repository.ErrRecurrenceNotFound) {
			return nil, ErrRecurrenceNotFound
		}
		return nil, err
	}

	return recurrence, nil
}

// DeleteRecurrence deletes a recurrence
func (s *AvailabilityService) DeleteRecurrence(ctx context.Context, token, participantID, recurrenceID string) error {
	// Validate calendar token
	calendarID, err := s.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return ErrCalendarNotFound
		}
		return err
	}

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return ErrParticipantNotFound
		}
		return err
	}

	if participant.CalendarID != calendarID {
		return ErrParticipantNotFound
	}

	// Parse recurrence ID
	recID, err := uuid.Parse(recurrenceID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRecurrenceID, err)
	}

	// Verify recurrence belongs to this participant
	recurrence, err := s.recurrenceRepo.GetRecurrenceByID(ctx, recID)
	if err != nil {
		return ErrRecurrenceNotFound
	}

	if recurrence.ParticipantID != partID {
		return ErrRecurrenceNotFound
	}

	// Delete recurrence
	if err := s.recurrenceRepo.DeleteRecurrence(ctx, recID); err != nil {
		if errors.Is(err, repository.ErrRecurrenceNotFound) {
			return ErrRecurrenceNotFound
		}
		return err
	}

	return nil
}

// CreateException creates an exception for a recurrence
func (s *AvailabilityService) CreateException(ctx context.Context, token, participantID, recurrenceID string, req *models.CreateExceptionRequest) (*models.RecurrenceException, error) {
	// Validate calendar token
	calendarID, err := s.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return nil, ErrParticipantNotFound
		}
		return nil, err
	}

	if participant.CalendarID != calendarID {
		return nil, ErrParticipantNotFound
	}

	// Parse recurrence ID
	recID, err := uuid.Parse(recurrenceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRecurrenceID, err)
	}

	// Verify recurrence belongs to this participant
	recurrence, err := s.recurrenceRepo.GetRecurrenceByID(ctx, recID)
	if err != nil {
		return nil, ErrRecurrenceNotFound
	}

	if recurrence.ParticipantID != partID {
		return nil, ErrRecurrenceNotFound
	}

	// Parse excluded date (validate format)
	_, err = parseDate(req.ExcludedDate)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// Create exception
	exception := &models.RecurrenceException{
		RecurrenceID: recID,
		ExcludedDate: req.ExcludedDate, // Keep as string in YYYY-MM-DD format
		CreatedAt:    time.Now(),
	}
	exception.ID = uuid.New()

	if err := s.recurrenceRepo.CreateException(ctx, exception); err != nil {
		return nil, err
	}

	return exception, nil
}

// DeleteException deletes an exception from a recurrence
func (s *AvailabilityService) DeleteException(ctx context.Context, token, participantID, recurrenceID, dateStr string) error {
	// Validate calendar token
	calendarID, err := s.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return ErrCalendarNotFound
		}
		return err
	}

	// Parse participant ID
	partID, err := uuid.Parse(participantID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidParticipantID, err)
	}

	// Verify participant belongs to this calendar
	participant, err := s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantNotFound) {
			return ErrParticipantNotFound
		}
		return err
	}

	if participant.CalendarID != calendarID {
		return ErrParticipantNotFound
	}

	// Parse recurrence ID
	recID, err := uuid.Parse(recurrenceID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRecurrenceID, err)
	}

	// Verify recurrence belongs to this participant
	recurrence, err := s.recurrenceRepo.GetRecurrenceByID(ctx, recID)
	if err != nil {
		return ErrRecurrenceNotFound
	}

	if recurrence.ParticipantID != partID {
		return ErrRecurrenceNotFound
	}

	// Parse date (validate format)
	_, err = parseDate(dateStr)
	if err != nil {
		return ErrInvalidDate
	}

	// Delete exception
	if err := s.recurrenceRepo.DeleteException(ctx, recID, dateStr); err != nil {
		if errors.Is(err, repository.ErrExceptionNotFound) {
			return ErrExceptionNotFound
		}
		return err
	}

	return nil
}

// adjustTimesByAllowedHours adjusts the start and end times based on the calendar's allowed hours configuration
func adjustTimesByAllowedHours(date time.Time, requestedStart, requestedEnd *string, calendarInfo *repository.Calendar) (*string, *string) {
	// Get the appropriate time range for this date
	allowedRange := getAllowedTimeRangeForDate(date, calendarInfo)

	// If no allowed range is configured at all, return the requested times as-is
	if allowedRange.Start == "" && allowedRange.End == "" {
		return requestedStart, requestedEnd
	}

	// Adjust start time
	var adjustedStart *string
	if allowedRange.Start != "" {
		// We have a configured minimum start time
		if requestedStart == nil || *requestedStart == "" {
			// No start time specified, use the allowed minimum
			adjustedStart = &allowedRange.Start
		} else {
			// Compare requested start with allowed minimum
			if compareTime(*requestedStart, allowedRange.Start) < 0 {
				// Requested start is before allowed minimum, use allowed minimum
				adjustedStart = &allowedRange.Start
			} else {
				// Requested start is within allowed range, use it
				adjustedStart = requestedStart
			}
		}
	} else if allowedRange.End != "" && (requestedStart == nil || *requestedStart == "") {
		// No minimum configured but maximum is configured, and no start time requested
		// Use "00:00" as default start to create a proper time range
		defaultStart := "00:00"
		adjustedStart = &defaultStart
	} else {
		// No minimum configured, use requested start (may be nil)
		adjustedStart = requestedStart
	}

	// Adjust end time
	var adjustedEnd *string
	if allowedRange.End != "" {
		// We have a configured maximum end time
		if requestedEnd == nil || *requestedEnd == "" {
			// No end time specified, use the allowed maximum
			adjustedEnd = &allowedRange.End
		} else {
			// Compare requested end with allowed maximum
			if compareTime(*requestedEnd, allowedRange.End) > 0 {
				// Requested end is after allowed maximum, use allowed maximum
				adjustedEnd = &allowedRange.End
			} else {
				// Requested end is within allowed range, use it
				adjustedEnd = requestedEnd
			}
		}
	} else if allowedRange.Start != "" && (requestedEnd == nil || *requestedEnd == "") {
		// No maximum configured but minimum is configured, and no end time requested
		// Use "23:59" as default end to create a proper time range
		defaultEnd := "23:59"
		adjustedEnd = &defaultEnd
	} else {
		// No maximum configured, use requested end (may be nil)
		adjustedEnd = requestedEnd
	}

	return adjustedStart, adjustedEnd
}

// getAllowedTimeRangeForDate gets the allowed time range for a specific date based on calendar configuration
// For holidays and holiday eves that fall on non-allowed weekdays, it uses ONLY the special hours.
// For holidays and holiday eves that fall on allowed weekdays, it combines the hours:
// - Start time: MIN(holiday/holiday_eve start, weekday start)
// - End time: MAX(holiday/holiday_eve end, weekday end)
func getAllowedTimeRangeForDate(date time.Time, calendarInfo *repository.Calendar) repository.TimeRange {
	// Get the weekday
	weekday := int(date.Weekday())
	weekdayStr := fmt.Sprintf("%d", weekday)

	// Check if this weekday is allowed in the calendar
	weekdayAllowed := datevalidation.IsWeekdayAllowed(weekday, calendarInfo.AllowedWeekdays)

	// Get the weekday time range (if weekday is allowed)
	var weekdayRange repository.TimeRange
	if weekdayAllowed {
		if tr, ok := calendarInfo.AllowedHours.Weekdays[weekdayStr]; ok {
			weekdayRange = tr
		} else {
			// Weekday is allowed but no specific hours configured, use full day
			weekdayRange = repository.TimeRange{Start: "00:00", End: "23:59"}
		}
	}

	// Determine the country code for holiday checking
	countryCode := datevalidation.GetCountryFromTimezone(calendarInfo.Timezone)

	// Check if it's a holiday and policy is "allow"
	if calendarInfo.HolidaysPolicy == "allow" && countryCode != "" && datevalidation.IsHoliday(date, countryCode) {
		// If weekday is also allowed, combine the ranges
		if weekdayAllowed {
			return combineTimeRanges(calendarInfo.AllowedHours.Holidays, weekdayRange)
		}
		// Otherwise, use ONLY the holiday hours (weekday is not allowed)
		return calendarInfo.AllowedHours.Holidays
	}

	// Check if it's a holiday eve
	if calendarInfo.AllowHolidayEves && countryCode != "" && datevalidation.IsHolidayEve(date, countryCode) {
		// If weekday is also allowed, combine the ranges
		if weekdayAllowed {
			return combineTimeRanges(calendarInfo.AllowedHours.HolidayEves, weekdayRange)
		}
		// Otherwise, use ONLY the holiday eve hours (weekday is not allowed)
		return calendarInfo.AllowedHours.HolidayEves
	}

	// It's a regular weekday
	return weekdayRange
}

// combineTimeRanges combines two time ranges by taking:
// - MIN of start times (earliest allowed start)
// - MAX of end times (latest allowed end)
func combineTimeRanges(specialRange, weekdayRange repository.TimeRange) repository.TimeRange {
	// If special range is not configured, use weekday range
	if specialRange.Start == "" || specialRange.End == "" {
		return weekdayRange
	}

	// If weekday range is not configured, use special range
	if weekdayRange.Start == "" || weekdayRange.End == "" {
		return specialRange
	}

	// Combine: MIN(start times), MAX(end times)
	combinedStart := specialRange.Start
	if compareTime(weekdayRange.Start, specialRange.Start) < 0 {
		combinedStart = weekdayRange.Start
	}

	combinedEnd := specialRange.End
	if compareTime(weekdayRange.End, specialRange.End) > 0 {
		combinedEnd = weekdayRange.End
	}

	return repository.TimeRange{
		Start: combinedStart,
		End:   combinedEnd,
	}
}

// compareTime compares two time strings in "HH:MM" format
// Returns: -1 if t1 < t2, 0 if t1 == t2, 1 if t1 > t2
func compareTime(t1, t2 string) int {
	time1, err1 := time.Parse("15:04", t1)
	time2, err2 := time.Parse("15:04", t2)

	if err1 != nil || err2 != nil {
		return 0
	}

	if time1.Before(time2) {
		return -1
	} else if time1.After(time2) {
		return 1
	}
	return 0
}

// recurrencesOverlap checks if two recurrences on the same day of week have overlapping date ranges.
// Returns true if the date ranges overlap.
// Rules:
// - If both have no end_date (infinite): they always overlap
// - If one has no end_date: overlap if its start_date <= the other's end_date
// - If both have end_dates: overlap if start_A <= end_B AND start_B <= end_A
func recurrencesOverlap(startDateA, endDateA, startDateB, endDateB string) bool {
	// Both have no end date (infinite) - they always overlap
	if endDateA == "" && endDateB == "" {
		return true
	}

	// A has no end date (infinite from startDateA)
	if endDateA == "" {
		// A overlaps with B if A starts before or when B ends
		return startDateA <= endDateB
	}

	// B has no end date (infinite from startDateB)
	if endDateB == "" {
		// B overlaps with A if B starts before or when A ends
		return startDateB <= endDateA
	}

	// Both have end dates - classic range overlap check
	// Ranges [startA, endA] and [startB, endB] overlap if startA <= endB AND startB <= endA
	return startDateA <= endDateB && startDateB <= endDateA
}

// getAllowedTimeRangeForWeekday gets the allowed time range for a specific weekday.
// This is used for recurrences where we know the day of week directly.
// Unlike getAllowedTimeRangeForDate, this doesn't check for holidays since recurrences
// span multiple dates and holiday logic can't be consistently applied.
func getAllowedTimeRangeForWeekday(dayOfWeek int, calendarInfo *repository.Calendar) repository.TimeRange {
	// A weekday the calendar does not accept has no window at all. Returning an empty
	// range here means "do not clamp", which is right: the weekday is rejected earlier,
	// by validation, and a range invented for it would only obscure that.
	if !datevalidation.IsWeekdayAllowed(dayOfWeek, calendarInfo.AllowedWeekdays) {
		return repository.TimeRange{}
	}

	weekdayStr := fmt.Sprintf("%d", dayOfWeek)

	if tr, ok := calendarInfo.AllowedHours.Weekdays[weekdayStr]; ok {
		return tr
	}

	// An allowed weekday with no configured hours is open all day — the same answer
	// getAllowedTimeRangeForDate gives. It used to return an empty range instead, so an
	// untimed recurrence was stored with NULL times while an untimed one-off on the very
	// same day became 00:00-23:59. Both render as "all day", which is why it went
	// unnoticed, but the two were not the same row.
	return repository.TimeRange{Start: "00:00", End: "23:59"}
}

// adjustTimesByAllowedHoursForWeekday adjusts times based on the calendar's allowed hours for a specific weekday.
// This is used for recurrences where we know the day of week directly.
func adjustTimesByAllowedHoursForWeekday(dayOfWeek int, requestedStart, requestedEnd *string, calendarInfo *repository.Calendar) (*string, *string) {
	allowedRange := getAllowedTimeRangeForWeekday(dayOfWeek, calendarInfo)

	// If no allowed range is configured, return the requested times as-is
	if allowedRange.Start == "" && allowedRange.End == "" {
		return requestedStart, requestedEnd
	}

	// Adjust start time
	var adjustedStart *string
	if allowedRange.Start != "" {
		if requestedStart == nil || *requestedStart == "" {
			adjustedStart = &allowedRange.Start
		} else if compareTime(*requestedStart, allowedRange.Start) < 0 {
			adjustedStart = &allowedRange.Start
		} else {
			adjustedStart = requestedStart
		}
	} else if allowedRange.End != "" && (requestedStart == nil || *requestedStart == "") {
		defaultStart := "00:00"
		adjustedStart = &defaultStart
	} else {
		adjustedStart = requestedStart
	}

	// Adjust end time
	var adjustedEnd *string
	if allowedRange.End != "" {
		if requestedEnd == nil || *requestedEnd == "" {
			adjustedEnd = &allowedRange.End
		} else if compareTime(*requestedEnd, allowedRange.End) > 0 {
			adjustedEnd = &allowedRange.End
		} else {
			adjustedEnd = requestedEnd
		}
	} else if allowedRange.Start != "" && (requestedEnd == nil || *requestedEnd == "") {
		defaultEnd := "23:59"
		adjustedEnd = &defaultEnd
	} else {
		adjustedEnd = requestedEnd
	}

	return adjustedStart, adjustedEnd
}

// checkRecurrenceOverlap checks if a new/updated recurrence overlaps with existing recurrences
// for the same participant on the same day of week.
// excludeID is used during updates to exclude the recurrence being updated from the check.
func (s *AvailabilityService) checkRecurrenceOverlap(ctx context.Context, participantID uuid.UUID, dayOfWeek int, startDate string, endDate *string, excludeID *uuid.UUID) error {
	// Get all existing recurrences for this participant
	existingRecurrences, err := s.recurrenceRepo.GetRecurrencesByParticipant(ctx, participantID)
	if err != nil {
		return err
	}

	// Convert endDate pointer to string for comparison
	endDateStr := ""
	if endDate != nil {
		endDateStr = *endDate
	}

	// Check for overlaps with existing recurrences on the same day
	for _, existing := range existingRecurrences {
		// Skip if different day of week
		if existing.DayOfWeek != dayOfWeek {
			continue
		}

		// Skip the recurrence being updated (for update operations)
		if excludeID != nil && existing.ID == *excludeID {
			continue
		}

		// Convert existing end date to string
		existingEndDate := ""
		if existing.EndDate != nil {
			existingEndDate = *existing.EndDate
		}

		// Check if date ranges overlap
		if recurrencesOverlap(startDate, endDateStr, existing.StartDate, existingEndDate) {
			return ErrRecurrenceOverlap
		}
	}

	return nil
}

// summaryFrom is the one conversion between what the repository expands and what the
// summary endpoints return.
func summaryFrom(occurrence models.Occurrence) models.ParticipantAvailabilitySummary {
	return models.ParticipantAvailabilitySummary{
		ParticipantID:   occurrence.ParticipantID,
		ParticipantName: occurrence.ParticipantName,
		StartTime:       occurrence.StartTime,
		EndTime:         occurrence.EndTime,
		Note:            occurrence.Note,
	}
}

func summariesFrom(occurrences []models.Occurrence) []models.ParticipantAvailabilitySummary {
	summaries := make([]models.ParticipantAvailabilitySummary, 0, len(occurrences))
	for _, occurrence := range occurrences {
		summaries = append(summaries, summaryFrom(occurrence))
	}

	return summaries
}

// countThatCanMeet is how many participants are free together for long enough to hold
// the event, which is what both summary endpoints report and what the grid badge shows.
//
// minDurationHours of zero means any overlap counts, however brief.
func countThatCanMeet(participants []models.ParticipantAvailabilitySummary, minDurationHours int) int {
	windows := make([]models.TimeWindow, 0, len(participants))
	for _, p := range participants {
		windows = append(windows, p.Window())
	}

	return models.MaxSimultaneousFor(windows, minDurationHours*60)
}
