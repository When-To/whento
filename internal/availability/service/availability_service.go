// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/datevalidation"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
)

var (
	ErrCalendarNotFound        = errors.New("calendar not found")
	ErrParticipantNotFound     = errors.New("participant not found")
	ErrInvalidParticipantID    = errors.New("invalid participant ID")
	ErrInvalidDate             = errors.New("invalid date format, expected YYYY-MM-DD")
	ErrInvalidTime             = errors.New("invalid time format, expected HH:MM")
	ErrInvalidTimeRange        = errors.New("end time must be after start time")
	ErrTimeOutsideAllowedHours = errors.New("time range does not fit within allowed hours for this day")
	ErrDurationTooShort        = errors.New("availability duration is less than the minimum required")
	ErrAvailabilityExists      = errors.New("availability already exists for this date")
	ErrAvailabilityNotFound    = errors.New("availability not found")
	ErrRecurrenceNotFound      = errors.New("recurrence not found")
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
	GetByDate(ctx context.Context, calendarID uuid.UUID, date time.Time) ([]*models.Availability, error)
	GetByCalendarDateRange(ctx context.Context, calendarID uuid.UUID, startDate, endDate time.Time) ([]*models.Availability, error)
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
	GetRecurrencesByCalendar(ctx context.Context, calendarID uuid.UUID) ([]models.Recurrence, error)
	GetRecurrenceByID(ctx context.Context, id uuid.UUID) (*models.Recurrence, error)
	UpdateRecurrence(ctx context.Context, recurrence *models.Recurrence) error
	DeleteRecurrence(ctx context.Context, id uuid.UUID) error
	CreateException(ctx context.Context, exception *models.RecurrenceException) error
	GetExceptionsByRecurrence(ctx context.Context, recurrenceID uuid.UUID) ([]models.RecurrenceException, error)
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
		return nil, fmt.Errorf("invalid participant id: %w", err)
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
		return nil, fmt.Errorf("date is before calendar start date (%s)", calendarInfo.StartDate.Format("2006-01-02"))
	}
	if calendarInfo.EndDate != nil && date.After(*calendarInfo.EndDate) {
		return nil, fmt.Errorf("date is after calendar end date (%s)", calendarInfo.EndDate.Format("2006-01-02"))
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
		if isDuplicateError(err) {
			return nil, ErrAvailabilityExists
		}
		return nil, err
	}

	// Trigger notification check (fire-and-forget, don't block availability operation)
	go func() {
		notifyCtx := context.Background()
		if err := s.notifyService.CheckThresholdAndNotify(notifyCtx, calendarID, date, previousCount); err != nil {
			// Log only, don't fail the availability operation
		}
	}()

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
		return nil, fmt.Errorf("invalid participant id: %w", err)
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
		return nil, fmt.Errorf("invalid participant id: %w", err)
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

	// Trigger notification check (fire-and-forget)
	// Note: Update doesn't change participant count, but we still check in case threshold config changed
	go func() {
		notifyCtx := context.Background()
		if err := s.notifyService.CheckThresholdAndNotify(notifyCtx, calendarID, date, currentCount); err != nil {
			// Log only, don't fail the availability operation
		}
	}()

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
		return fmt.Errorf("invalid participant id: %w", err)
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

	// Trigger notification check (fire-and-forget)
	go func() {
		notifyCtx := context.Background()
		if err := s.notifyService.CheckThresholdAndNotify(notifyCtx, calendarID, date, previousCount); err != nil {
			// Log only, don't fail the availability operation
		}
	}()

	return nil
}

// GetDateSummary gets all participants available on a specific date
func (s *AvailabilityService) GetDateSummary(ctx context.Context, token, dateStr string) (*models.DateAvailabilitySummary, error) {
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

	// Get all availabilities for this date
	availabilities, err := s.availabilityRepo.GetByDate(ctx, calendarID, date)
	if err != nil {
		return nil, err
	}

	// Get participant info
	participants, err := s.participantRepo.GetByCalendarID(ctx, calendarID)
	if err != nil {
		return nil, err
	}

	// Create participant map for quick lookup
	participantMap := make(map[uuid.UUID]*repository.Participant)
	for _, p := range participants {
		participantMap[p.ID] = p
	}

	// Get all recurrences for the calendar
	recurrences, err := s.recurrenceRepo.GetRecurrencesByCalendar(ctx, calendarID)
	if err != nil {
		return nil, err
	}

	// Get exceptions for all recurrences
	exceptionsMap := make(map[uuid.UUID][]models.RecurrenceException)
	for _, rec := range recurrences {
		exceptions, err := s.recurrenceRepo.GetExceptionsByRecurrence(ctx, rec.ID)
		if err != nil {
			return nil, err
		}
		exceptionsMap[rec.ID] = exceptions
	}

	// Create a set of participants that have explicit availabilities
	explicitParticipants := make(map[uuid.UUID]bool)
	for _, avail := range availabilities {
		explicitParticipants[avail.ParticipantID] = true
	}

	// Build summary - start with explicit availabilities
	var participantSummaries []models.ParticipantAvailabilitySummary
	for _, avail := range availabilities {
		if participant, ok := participantMap[avail.ParticipantID]; ok {
			participantSummaries = append(participantSummaries, models.ParticipantAvailabilitySummary{
				ParticipantID:   avail.ParticipantID,
				ParticipantName: participant.Name,
				StartTime:       avail.StartTime,
				EndTime:         avail.EndTime,
				Note:            avail.Note,
			})
		}
	}

	// Add recurrence-based availabilities
	dayOfWeek := int(date.Weekday())
	for _, rec := range recurrences {
		// Skip if day of week doesn't match
		if rec.DayOfWeek != dayOfWeek {
			continue
		}

		// Compare dates as strings to avoid timezone issues
		// Skip if date is before recurrence start
		if dateStr < rec.StartDate {
			continue
		}

		// Skip if date is after recurrence end (if end date is set)
		if rec.EndDate != nil && dateStr > *rec.EndDate {
			continue
		}

		// Check if this date is in the exceptions
		isException := false
		for _, exc := range exceptionsMap[rec.ID] {
			if exc.ExcludedDate == dateStr {
				isException = true
				break
			}
		}
		if isException {
			continue
		}

		// Skip if there's already an explicit availability for this participant
		if explicitParticipants[rec.ParticipantID] {
			continue
		}

		// Add this participant to the summary
		if participant, ok := participantMap[rec.ParticipantID]; ok {
			participantSummaries = append(participantSummaries, models.ParticipantAvailabilitySummary{
				ParticipantID:   rec.ParticipantID,
				ParticipantName: participant.Name,
				StartTime:       rec.StartTime,
				EndTime:         rec.EndTime,
				Note:            rec.Note,
			})
		}
	}

	// Apply min_duration_hours filter if configured
	if calendarInfo.MinDurationHours > 0 && len(participantSummaries) > 0 {
		duration := calculateDurationForDate(participantSummaries)
		if duration < float64(calendarInfo.MinDurationHours) {
			// Return empty summary if duration is less than minimum
			return &models.DateAvailabilitySummary{
				Date:         dateStr,
				TotalCount:   0,
				Participants: []models.ParticipantAvailabilitySummary{},
			}, nil
		}
	}

	return &models.DateAvailabilitySummary{
		Date:         dateStr,
		TotalCount:   calculateMaxSimultaneousParticipants(participantSummaries),
		Participants: participantSummaries,
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
		return nil, fmt.Errorf("end date must be after start date")
	}

	// Get all availabilities in range
	availabilities, err := s.availabilityRepo.GetByCalendarDateRange(ctx, calendarID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Get participant info
	participants, err := s.participantRepo.GetByCalendarID(ctx, calendarID)
	if err != nil {
		return nil, err
	}

	// Create participant map
	participantMap := make(map[uuid.UUID]*repository.Participant)
	for _, p := range participants {
		participantMap[p.ID] = p
	}

	// Get all recurrences for the calendar
	recurrences, err := s.recurrenceRepo.GetRecurrencesByCalendar(ctx, calendarID)
	if err != nil {
		return nil, err
	}

	// Get exceptions for all recurrences
	exceptionsMap := make(map[uuid.UUID][]models.RecurrenceException)
	for _, rec := range recurrences {
		exceptions, err := s.recurrenceRepo.GetExceptionsByRecurrence(ctx, rec.ID)
		if err != nil {
			return nil, err
		}
		exceptionsMap[rec.ID] = exceptions
	}

	// Create a set of dates that have explicit availabilities per participant
	// This prevents counting the same participant twice (once from recurrence, once from explicit)
	explicitAvailabilities := make(map[string]map[uuid.UUID]bool)
	for _, avail := range availabilities {
		dateKey := formatDate(avail.Date)
		if explicitAvailabilities[dateKey] == nil {
			explicitAvailabilities[dateKey] = make(map[uuid.UUID]bool)
		}
		explicitAvailabilities[dateKey][avail.ParticipantID] = true
	}

	// Group by date - start with explicit availabilities
	dateMap := make(map[string][]models.ParticipantAvailabilitySummary)
	for _, avail := range availabilities {
		dateKey := formatDate(avail.Date)
		if participant, ok := participantMap[avail.ParticipantID]; ok {
			dateMap[dateKey] = append(dateMap[dateKey], models.ParticipantAvailabilitySummary{
				ParticipantID:   avail.ParticipantID,
				ParticipantName: participant.Name,
				StartTime:       avail.StartTime,
				EndTime:         avail.EndTime,
				Note:            avail.Note,
			})
		}
	}

	// Add recurrence-based availabilities for each date in range
	currentDate := startDate
	for !currentDate.After(endDate) {
		dateKey := formatDate(currentDate)
		dayOfWeek := int(currentDate.Weekday())

		// Check each recurrence
		for _, rec := range recurrences {
			// Skip if day of week doesn't match
			if rec.DayOfWeek != dayOfWeek {
				continue
			}

			// Compare dates as strings to avoid timezone issues
			// Skip if date is before recurrence start
			if dateKey < rec.StartDate {
				continue
			}

			// Skip if date is after recurrence end (if end date is set)
			if rec.EndDate != nil && dateKey > *rec.EndDate {
				continue
			}

			// Check if this date is in the exceptions
			isException := false
			for _, exc := range exceptionsMap[rec.ID] {
				if exc.ExcludedDate == dateKey {
					isException = true
					break
				}
			}
			if isException {
				continue
			}

			// Skip if there's already an explicit availability for this participant on this date
			if explicitAvailabilities[dateKey] != nil && explicitAvailabilities[dateKey][rec.ParticipantID] {
				continue
			}

			// Add this participant to the date
			if participant, ok := participantMap[rec.ParticipantID]; ok {
				dateMap[dateKey] = append(dateMap[dateKey], models.ParticipantAvailabilitySummary{
					ParticipantID:   rec.ParticipantID,
					ParticipantName: participant.Name,
					StartTime:       rec.StartTime,
					EndTime:         rec.EndTime,
					Note:            rec.Note,
				})
			}
		}

		// Move to next day
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Build response (with min_duration_hours filter if configured)
	var summaries []models.PublicDateAvailabilitySummary
	for date, participants := range dateMap {
		// Apply min_duration_hours filter if configured
		if calendarInfo.MinDurationHours > 0 {
			duration := calculateDurationForDate(participants)
			if duration < float64(calendarInfo.MinDurationHours) {
				// Skip this date if duration is less than minimum
				continue
			}
		}

		summaries = append(summaries, models.PublicDateAvailabilitySummary{
			Date:         date,
			TotalCount:   calculateMaxSimultaneousParticipants(participants),
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

func isDuplicateError(err error) bool {
	return err != nil && (err.Error() == "availability already exists for this date")
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

// calculateDurationForDate calculates the event duration for a date based on participant availabilities
// Returns duration in hours
func calculateDurationForDate(participants []models.ParticipantAvailabilitySummary) float64 {
	// Check if all participants have no times (all-day event)
	hasAnyTime := false
	for _, p := range participants {
		if p.StartTime != nil || p.EndTime != nil {
			hasAnyTime = true
			break
		}
	}

	// If all-day event, return 24 hours
	if !hasAnyTime {
		return 24.0
	}

	// Calculate MAX(start_time) and MIN(end_time)
	var maxStart, minEnd *time.Time

	for _, p := range participants {
		if p.StartTime != nil && *p.StartTime != "" {
			t, err := time.Parse("15:04", *p.StartTime)
			if err == nil {
				if maxStart == nil || t.After(*maxStart) {
					maxStart = &t
				}
			}
		}

		if p.EndTime != nil && *p.EndTime != "" {
			t, err := time.Parse("15:04", *p.EndTime)
			if err == nil {
				if minEnd == nil || t.Before(*minEnd) {
					minEnd = &t
				}
			}
		}
	}

	// If we couldn't determine both times, treat as all-day
	if maxStart == nil || minEnd == nil {
		return 24.0
	}

	// Calculate duration in hours
	duration := minEnd.Sub(*maxStart).Hours()

	// Handle negative or zero duration (invalid case)
	if duration <= 0 {
		return 0.0
	}

	return duration
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
		return nil, fmt.Errorf("invalid participant id: %w", err)
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
			return nil, fmt.Errorf("end date must be after start date")
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
		return nil, fmt.Errorf("invalid participant id: %w", err)
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

	// Get exceptions for each recurrence
	var result []models.RecurrenceWithExceptions
	for _, rec := range recurrences {
		exceptions, err := s.recurrenceRepo.GetExceptionsByRecurrence(ctx, rec.ID)
		if err != nil {
			return nil, err
		}
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
		return nil, fmt.Errorf("invalid recurrence ID")
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
		return nil, fmt.Errorf("recurrence not found")
	}
	if existingRec.ParticipantID != partID {
		return nil, fmt.Errorf("recurrence does not belong to participant")
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
			return nil, fmt.Errorf("end date must be after start date")
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
		return fmt.Errorf("invalid participant id: %w", err)
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
		return fmt.Errorf("invalid recurrence id: %w", err)
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
		return nil, fmt.Errorf("invalid participant id: %w", err)
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
		return nil, fmt.Errorf("invalid recurrence id: %w", err)
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
		return fmt.Errorf("invalid participant id: %w", err)
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
		return fmt.Errorf("invalid recurrence id: %w", err)
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

// calculateMaxSimultaneousParticipants calculates the maximum number of participants
// that are available at the same time on a given date.
// This uses the same logic as the ICS feed generation (time slot segmentation).
func calculateMaxSimultaneousParticipants(participants []models.ParticipantAvailabilitySummary) int {
	if len(participants) == 0 {
		return 0
	}

	// Normalize participant times (treat nil as 00:00-23:59)
	type normalizedParticipant struct {
		startMinutes int
		endMinutes   int
		valid        bool
	}

	normalized := make([]normalizedParticipant, len(participants))
	validCount := 0
	for i, p := range participants {
		startStr := "00:00"
		endStr := "23:59"

		if p.StartTime != nil && *p.StartTime != "" {
			startStr = *p.StartTime
		}
		if p.EndTime != nil && *p.EndTime != "" {
			endStr = *p.EndTime
		}

		startTime, err1 := time.Parse("15:04", startStr)
		endTime, err2 := time.Parse("15:04", endStr)

		if err1 != nil || err2 != nil {
			// If parsing fails, mark as invalid
			normalized[i] = normalizedParticipant{valid: false}
			continue
		}

		normalized[i] = normalizedParticipant{
			startMinutes: startTime.Hour()*60 + startTime.Minute(),
			endMinutes:   endTime.Hour()*60 + endTime.Minute(),
			valid:        true,
		}
		validCount++
	}

	if validCount == 0 {
		return 0
	}

	// Collect all unique time boundaries
	boundarySet := make(map[int]bool)
	for _, p := range normalized {
		if p.valid {
			boundarySet[p.startMinutes] = true
			boundarySet[p.endMinutes] = true
		}
	}

	if len(boundarySet) == 0 {
		return 0
	}

	// Sort boundaries
	boundaries := make([]int, 0, len(boundarySet))
	for b := range boundarySet {
		boundaries = append(boundaries, b)
	}
	sortInts := func(arr []int) {
		for i := 0; i < len(arr); i++ {
			for j := i + 1; j < len(arr); j++ {
				if arr[i] > arr[j] {
					arr[i], arr[j] = arr[j], arr[i]
				}
			}
		}
	}
	sortInts(boundaries)

	// Calculate the maximum number of participants for each segment
	maxCount := 0

	for i := 0; i < len(boundaries)-1; i++ {
		segStart := boundaries[i]
		segEnd := boundaries[i+1]

		// Count participants available for this entire segment
		count := 0
		for _, p := range normalized {
			if p.valid {
				// Participant is available if their range completely covers the segment
				if p.startMinutes <= segStart && p.endMinutes >= segEnd {
					count++
				}
			}
		}

		if count > maxCount {
			maxCount = count
		}
	}

	return maxCount
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
	weekdayStr := fmt.Sprintf("%d", dayOfWeek)

	// Check if this weekday has specific allowed hours configured
	if tr, ok := calendarInfo.AllowedHours.Weekdays[weekdayStr]; ok {
		return tr
	}

	// No specific hours configured for this day, return empty (no restrictions)
	return repository.TimeRange{}
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
