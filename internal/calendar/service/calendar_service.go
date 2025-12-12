// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/repository"
)

var (
	ErrCalendarNotFound    = errors.New("calendar not found")
	ErrParticipantNotFound = errors.New("participant not found")
	ErrUnauthorized        = errors.New("you don't have permission to access this calendar")
	ErrParticipantExists   = errors.New("participant with this name already exists")
	ErrInvalidTokenType    = errors.New("invalid token type, must be 'public' or 'ics'")
)

// CalendarRepository defines the interface for calendar repository operations
type CalendarRepository interface {
	CreateWithParticipants(ctx context.Context, calendar *models.Calendar, participants []repository.ParticipantInput) ([]models.Participant, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Calendar, error)
	GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]*models.Calendar, error)
	GetByPublicToken(ctx context.Context, token string) (*models.Calendar, error)
	Update(ctx context.Context, calendar *models.Calendar) error
	Delete(ctx context.Context, id uuid.UUID) error
	RegenerateToken(ctx context.Context, id uuid.UUID, tokenType, newToken string) error
}

// ParticipantRepository defines the interface for participant repository operations
type ParticipantRepository interface {
	Create(ctx context.Context, participant *models.Participant) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Participant, error)
	GetByCalendarID(ctx context.Context, calendarID uuid.UUID) ([]models.Participant, error)
	Update(ctx context.Context, id uuid.UUID, name string) error
	Delete(ctx context.Context, id uuid.UUID) error
	SetEmailAsVerified(ctx context.Context, participantID uuid.UUID, email string) error
}

// CalendarService handles calendar business logic
type CalendarService struct {
	calendarRepo    CalendarRepository
	participantRepo ParticipantRepository
	userRepo        *authRepo.UserRepository
	cache           cache.Cache
}

// NewCalendarService creates a new calendar service
func NewCalendarService(
	calendarRepo CalendarRepository,
	participantRepo ParticipantRepository,
	userRepo *authRepo.UserRepository,
	c cache.Cache,
) *CalendarService {
	return &CalendarService{
		calendarRepo:    calendarRepo,
		participantRepo: participantRepo,
		userRepo:        userRepo,
		cache:           c,
	}
}

// CreateCalendar creates a new calendar with optional participants
func (s *CalendarService) CreateCalendar(ctx context.Context, userID string, req *models.CreateCalendarRequest) (*models.CalendarResponse, error) {
	ownerUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	// Generate tokens
	publicToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate public token: %w", err)
	}

	icsToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ics token: %w", err)
	}

	// Set default threshold
	threshold := req.Threshold
	if threshold == 0 {
		threshold = 1
	}

	// Set default allowed weekdays (all days if not specified)
	allowedWeekdays := req.AllowedWeekdays
	if len(allowedWeekdays) == 0 {
		allowedWeekdays = []int{0, 1, 2, 3, 4, 5, 6}
	}

	// Set default timezone (Europe/Paris if not specified)
	timezone := req.Timezone
	if timezone == "" {
		timezone = "Europe/Paris"
	}

	// Set default holidays_policy (ignore if not specified)
	holidaysPolicy := req.HolidaysPolicy
	if holidaysPolicy == "" {
		holidaysPolicy = "ignore"
	}

	// Normalize weekday times (swap if min > max)
	normalizedWeekdayTimes := models.NormalizeWeekdayTimes(req.WeekdayTimes)

	// Normalize holiday times
	normalizedHolidayMinTime, normalizedHolidayMaxTime := models.NormalizeHolidayTimes(req.HolidayMinTime, req.HolidayMaxTime)

	// Normalize holiday eve times
	normalizedHolidayEveMinTime, normalizedHolidayEveMaxTime := models.NormalizeHolidayTimes(req.HolidayEveMinTime, req.HolidayEveMaxTime)

	// Build allowed_hours JSONB from normalized request fields
	allowedHoursJSON, err := models.BuildAllowedHoursJSON(
		normalizedWeekdayTimes,
		normalizedHolidayMinTime,
		normalizedHolidayMaxTime,
		normalizedHolidayEveMinTime,
		normalizedHolidayEveMaxTime,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build allowed_hours: %w", err)
	}

	// Parse dates if provided
	var startDate, endDate *time.Time
	if req.StartDate != "" {
		parsed, err := time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			return nil, fmt.Errorf("invalid start_date format, expected YYYY-MM-DD: %w", err)
		}
		startDate = &parsed
	}
	if req.EndDate != "" {
		parsed, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			return nil, fmt.Errorf("invalid end_date format, expected YYYY-MM-DD: %w", err)
		}
		endDate = &parsed
	}

	// Validate that end_date is after start_date if both are set
	if startDate != nil && endDate != nil && endDate.Before(*startDate) {
		return nil, fmt.Errorf("end_date must be after start_date")
	}

	calendar := &models.Calendar{
		OwnerID:           ownerUUID,
		Name:              req.Name,
		Description:       req.Description,
		PublicToken:       publicToken,
		ICSToken:          icsToken,
		Threshold:         threshold,
		AllowedWeekdays:   allowedWeekdays,
		MinDurationHours:  req.MinDurationHours,
		Timezone:          timezone,
		HolidaysPolicy:    holidaysPolicy,
		AllowHolidayEves:  req.AllowHolidayEves,
		AllowedHours:      allowedHoursJSON,
		NotifyOnThreshold: req.NotifyOnThreshold,
		NotifyConfig:      req.NotifyConfig,
		LockParticipants:  req.LockParticipants,
		StartDate:         startDate,
		EndDate:           endDate,
	}
	calendar.ID = uuid.New()

	// Determine participant locale (use request locale or fall back to owner's locale)
	participantLocale := req.ParticipantLocale
	var ownerEmail string
	if participantLocale == "" || s.userRepo != nil {
		// Get owner information for participant email matching and locale
		if s.userRepo != nil {
			owner, err := s.userRepo.GetByID(ctx, ownerUUID)
			if err != nil {
				return nil, fmt.Errorf("failed to get owner information: %w", err)
			}
			if participantLocale == "" {
				participantLocale = owner.Locale
			}
			if owner.EmailVerified {
				ownerEmail = owner.Email
			}
		}
	}
	if participantLocale == "" {
		participantLocale = "en" // Default fallback
	}

	// Build participant inputs with locale and owner email if applicable
	participantInputs := make([]repository.ParticipantInput, 0, len(req.Participants))
	for _, name := range req.Participants {
		input := repository.ParticipantInput{
			Name:   name,
			Locale: participantLocale,
		}

		// If participant name matches owner's email is available, pre-populate email
		if ownerEmail != "" {
			input.Email = &ownerEmail
			input.EmailVerified = true
		}

		participantInputs = append(participantInputs, input)
	}

	// Create calendar and participants in a transaction
	participants, err := s.calendarRepo.CreateWithParticipants(ctx, calendar, participantInputs)
	if err != nil {
		if errors.Is(err, repository.ErrParticipantAlreadyExists) {
			return nil, fmt.Errorf("duplicate participant name in request")
		}
		return nil, fmt.Errorf("failed to create calendar: %w", err)
	}

	// Auto-populate owner participant's email if user has verified email
	if len(participants) > 0 && s.userRepo != nil {
		// Get owner user information
		owner, err := s.userRepo.GetByID(ctx, ownerUUID)
		if err == nil && owner != nil && owner.EmailVerified && owner.Email != "" {
			// Find participant matching owner's display name (frontend auto-adds owner as first participant)
			for _, participant := range participants {
				if participant.Name == owner.DisplayName {
					// Set email as already verified for the owner participant
					if err := s.participantRepo.SetEmailAsVerified(ctx, participant.ID, owner.Email); err != nil {
						// Log error but don't fail calendar creation
						// The owner can manually add their email later if this fails
					}
					break
				}
			}
		}
	}

	return buildCalendarResponse(calendar, participants)
}

// buildCalendarResponse converts a Calendar model to CalendarResponse with parsed allowed_hours
func buildCalendarResponse(calendar *models.Calendar, participants []models.Participant) (*models.CalendarResponse, error) {
	// Parse allowed_hours JSONB to extract separate fields
	weekdayTimes, holidayMinTime, holidayMaxTime, holidayEveMinTime, holidayEveMaxTime, err := models.ParseAllowedHoursJSON(calendar.AllowedHours)
	if err != nil {
		return nil, fmt.Errorf("failed to parse allowed_hours: %w", err)
	}

	return &models.CalendarResponse{
		ID:                calendar.ID,
		OwnerID:           calendar.OwnerID,
		Name:              calendar.Name,
		Description:       calendar.Description,
		PublicToken:       calendar.PublicToken,
		ICSToken:          calendar.ICSToken,
		Threshold:         calendar.Threshold,
		AllowedWeekdays:   calendar.AllowedWeekdays,
		MinDurationHours:  calendar.MinDurationHours,
		Timezone:          calendar.Timezone,
		HolidaysPolicy:    calendar.HolidaysPolicy,
		AllowHolidayEves:  calendar.AllowHolidayEves,
		WeekdayTimes:      weekdayTimes,
		HolidayMinTime:    holidayMinTime,
		HolidayMaxTime:    holidayMaxTime,
		HolidayEveMinTime: holidayEveMinTime,
		HolidayEveMaxTime: holidayEveMaxTime,
		NotifyOnThreshold: calendar.NotifyOnThreshold,
		LockParticipants:  calendar.LockParticipants,
		StartDate:         calendar.StartDate,
		EndDate:           calendar.EndDate,
		Participants:      participants,
		CreatedAt:         calendar.CreatedAt,
		UpdatedAt:         calendar.UpdatedAt,
	}, nil
}

// buildPublicCalendarResponse converts a Calendar model to PublicCalendarResponse with parsed allowed_hours
func buildPublicCalendarResponse(calendar *models.Calendar, participants []models.PublicParticipant) (*models.PublicCalendarResponse, error) {
	// Parse allowed_hours JSONB to extract separate fields
	weekdayTimes, holidayMinTime, holidayMaxTime, holidayEveMinTime, holidayEveMaxTime, err := models.ParseAllowedHoursJSON(calendar.AllowedHours)
	if err != nil {
		return nil, fmt.Errorf("failed to parse allowed_hours: %w", err)
	}

	// Check if participant notifications are enabled
	notifyParticipants := false
	if calendar.NotifyConfig != nil && *calendar.NotifyConfig != "" {
		// We need to import the notify models package to parse the config
		// For now, use a simple JSON parsing approach
		var notifyConfig struct {
			Enabled            bool `json:"enabled"`
			NotifyParticipants bool `json:"notify_participants"`
		}
		if err := json.Unmarshal([]byte(*calendar.NotifyConfig), &notifyConfig); err == nil {
			notifyParticipants = notifyConfig.Enabled && notifyConfig.NotifyParticipants
		}
	}

	return &models.PublicCalendarResponse{
		ID:                 calendar.ID,
		Name:               calendar.Name,
		Description:        calendar.Description,
		Threshold:          calendar.Threshold,
		AllowedWeekdays:    calendar.AllowedWeekdays,
		MinDurationHours:   calendar.MinDurationHours,
		Timezone:           calendar.Timezone,
		HolidaysPolicy:     calendar.HolidaysPolicy,
		AllowHolidayEves:   calendar.AllowHolidayEves,
		WeekdayTimes:       weekdayTimes,
		HolidayMinTime:     holidayMinTime,
		HolidayMaxTime:     holidayMaxTime,
		HolidayEveMinTime:  holidayEveMinTime,
		HolidayEveMaxTime:  holidayEveMaxTime,
		LockParticipants:   calendar.LockParticipants,
		NotifyParticipants: notifyParticipants,
		ICSToken:           calendar.ICSToken,
		StartDate:          calendar.StartDate,
		EndDate:            calendar.EndDate,
		Participants:       participants,
		CreatedAt:          calendar.CreatedAt,
	}, nil
}

// isDuplicateKeyError checks if an error is a duplicate key constraint violation
func isDuplicateKeyError(err error) bool {
	return err != nil && (err.Error() == "ERROR: duplicate key value violates unique constraint" ||
		containsCode(err.Error(), "23505"))
}

func containsCode(errMsg, code string) bool {
	return len(errMsg) > 0 && len(code) > 0 && findSubstring(errMsg, code)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// filterParticipants masks participant IDs based on lock_participants setting and participant_id
func filterParticipants(lockParticipants bool, participantID string, participants []models.Participant) []models.PublicParticipant {
	// Parse participant ID if provided
	var parsedID uuid.UUID
	var err error
	if participantID != "" {
		parsedID, err = uuid.Parse(participantID)
		if err != nil {
			parsedID = uuid.Nil
		}
	}

	publicParticipants := make([]models.PublicParticipant, len(participants))

	if !lockParticipants {
		// Return all participants with their IDs visible
		// Email info is only included for the specified participant
		for i, p := range participants {
			isCurrentParticipant := participantID != "" && p.ID == parsedID
			publicParticipants[i] = models.PublicParticipant{
				ID:            &p.ID,
				CalendarID:    p.CalendarID,
				Name:          p.Name,
				Email:         conditionalEmail(isCurrentParticipant, p.Email),
				EmailVerified: isCurrentParticipant && p.EmailVerified,
				CreatedAt:     p.CreatedAt,
			}
		}
		return publicParticipants
	}

	// If lock_participants is true, mask IDs except for the specified participant
	// Email info is still only shown for the specified participant
	for i, p := range participants {
		isCurrentParticipant := participantID != "" && p.ID == parsedID
		if isCurrentParticipant {
			// Keep the participant with their ID and email
			publicParticipants[i] = models.PublicParticipant{
				ID:            &p.ID,
				CalendarID:    p.CalendarID,
				Name:          p.Name,
				Email:         p.Email,
				EmailVerified: p.EmailVerified,
				CreatedAt:     p.CreatedAt,
			}
		} else {
			// Mask the ID and email by setting them to nil/false
			publicParticipants[i] = models.PublicParticipant{
				ID:            nil,
				CalendarID:    p.CalendarID,
				Name:          p.Name,
				Email:         nil,
				EmailVerified: false,
				CreatedAt:     p.CreatedAt,
			}
		}
	}

	return publicParticipants
}

// conditionalEmail returns the email if condition is true, otherwise nil
func conditionalEmail(condition bool, email *string) *string {
	if condition {
		return email
	}
	return nil
}

// GetCalendar retrieves a calendar by ID (requires ownership or admin role)
func (s *CalendarService) GetCalendar(ctx context.Context, userID, userRole, calendarID string) (*models.CalendarResponse, error) {
	id, err := uuid.Parse(calendarID)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar id: %w", err)
	}

	calendar, err := s.calendarRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Check ownership or admin role
	if calendar.OwnerID.String() != userID && userRole != "admin" {
		return nil, ErrUnauthorized
	}

	// Get participants
	participants, err := s.participantRepo.GetByCalendarID(ctx, calendar.ID)
	if err != nil {
		return nil, err
	}

	return buildCalendarResponse(calendar, participants)
}

// ListMyCalendars lists all calendars owned by the user
func (s *CalendarService) ListMyCalendars(ctx context.Context, userID string) ([]*models.CalendarResponse, error) {
	ownerUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	calendars, err := s.calendarRepo.GetByOwnerID(ctx, ownerUUID)
	if err != nil {
		return nil, err
	}

	var responses []*models.CalendarResponse
	for _, calendar := range calendars {
		// Get participants for each calendar
		participants, err := s.participantRepo.GetByCalendarID(ctx, calendar.ID)
		if err != nil {
			return nil, err
		}

		response, err := buildCalendarResponse(calendar, participants)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}

	return responses, nil
}

// UpdateCalendar updates a calendar (requires ownership or admin role)
func (s *CalendarService) UpdateCalendar(ctx context.Context, userID, userRole, calendarID string, req *models.UpdateCalendarRequest) (*models.CalendarResponse, error) {
	id, err := uuid.Parse(calendarID)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar id: %w", err)
	}

	calendar, err := s.calendarRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Check ownership or admin role
	if calendar.OwnerID.String() != userID && userRole != "admin" {
		return nil, ErrUnauthorized
	}

	// Update fields if provided
	if req.Name != nil {
		calendar.Name = *req.Name
	}
	if req.Description != nil {
		calendar.Description = *req.Description
	}
	if req.Threshold != nil {
		calendar.Threshold = *req.Threshold
	}
	if len(req.AllowedWeekdays) > 0 {
		calendar.AllowedWeekdays = req.AllowedWeekdays
	}
	if req.MinDurationHours != nil {
		calendar.MinDurationHours = *req.MinDurationHours
	}
	if req.Timezone != nil {
		calendar.Timezone = *req.Timezone
	}
	if req.HolidaysPolicy != nil {
		calendar.HolidaysPolicy = *req.HolidaysPolicy
	}
	if req.AllowHolidayEves != nil {
		calendar.AllowHolidayEves = *req.AllowHolidayEves
	}
	if req.NotifyOnThreshold != nil {
		calendar.NotifyOnThreshold = *req.NotifyOnThreshold
	}
	if req.NotifyConfig != nil {
		calendar.NotifyConfig = req.NotifyConfig
	}
	if req.LockParticipants != nil {
		calendar.LockParticipants = *req.LockParticipants
	}

	// Update start_date if provided
	if req.StartDate != nil {
		if *req.StartDate == "" {
			// Empty string means clear the date
			calendar.StartDate = nil
		} else {
			parsed, err := time.Parse("2006-01-02", *req.StartDate)
			if err != nil {
				return nil, fmt.Errorf("invalid start_date format, expected YYYY-MM-DD: %w", err)
			}
			calendar.StartDate = &parsed
		}
	}

	// Update end_date if provided
	if req.EndDate != nil {
		if *req.EndDate == "" {
			// Empty string means clear the date
			calendar.EndDate = nil
		} else {
			parsed, err := time.Parse("2006-01-02", *req.EndDate)
			if err != nil {
				return nil, fmt.Errorf("invalid end_date format, expected YYYY-MM-DD: %w", err)
			}
			calendar.EndDate = &parsed
		}
	}

	// Validate that end_date is after start_date if both are set
	if calendar.StartDate != nil && calendar.EndDate != nil && calendar.EndDate.Before(*calendar.StartDate) {
		return nil, fmt.Errorf("end_date must be after start_date")
	}

	// Update allowed_hours if any time-related fields are provided
	if len(req.WeekdayTimes) > 0 || req.HolidayMinTime != nil || req.HolidayMaxTime != nil || req.HolidayEveMinTime != nil || req.HolidayEveMaxTime != nil {
		// Parse current allowed_hours first to keep unchanged values
		existingWeekdayTimes, existingHolidayMinTime, existingHolidayMaxTime, existingHolidayEveMinTime, existingHolidayEveMaxTime, err := models.ParseAllowedHoursJSON(calendar.AllowedHours)
		if err != nil {
			return nil, fmt.Errorf("failed to parse existing allowed_hours: %w", err)
		}

		// Use new values if provided, otherwise keep existing
		weekdayTimes := req.WeekdayTimes
		if len(weekdayTimes) == 0 {
			weekdayTimes = existingWeekdayTimes
		}

		holidayMinTime := existingHolidayMinTime
		if req.HolidayMinTime != nil {
			holidayMinTime = *req.HolidayMinTime
		}

		holidayMaxTime := existingHolidayMaxTime
		if req.HolidayMaxTime != nil {
			holidayMaxTime = *req.HolidayMaxTime
		}

		holidayEveMinTime := existingHolidayEveMinTime
		if req.HolidayEveMinTime != nil {
			holidayEveMinTime = *req.HolidayEveMinTime
		}

		holidayEveMaxTime := existingHolidayEveMaxTime
		if req.HolidayEveMaxTime != nil {
			holidayEveMaxTime = *req.HolidayEveMaxTime
		}

		// Normalize weekday times (swap if min > max)
		normalizedWeekdayTimes := models.NormalizeWeekdayTimes(weekdayTimes)

		// Normalize holiday times
		normalizedHolidayMinTime, normalizedHolidayMaxTime := models.NormalizeHolidayTimes(holidayMinTime, holidayMaxTime)

		// Normalize holiday eve times
		normalizedHolidayEveMinTime, normalizedHolidayEveMaxTime := models.NormalizeHolidayTimes(holidayEveMinTime, holidayEveMaxTime)

		// Build new allowed_hours JSONB from normalized values
		allowedHoursJSON, err := models.BuildAllowedHoursJSON(
			normalizedWeekdayTimes,
			normalizedHolidayMinTime,
			normalizedHolidayMaxTime,
			normalizedHolidayEveMinTime,
			normalizedHolidayEveMaxTime,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build allowed_hours: %w", err)
		}

		calendar.AllowedHours = allowedHoursJSON
	}

	if err := s.calendarRepo.Update(ctx, calendar); err != nil {
		return nil, err
	}

	// Invalidate the public calendar cache
	cacheKey := cache.CalendarByPublicTokenKey(calendar.PublicToken)
	_ = s.cache.Delete(ctx, cacheKey)

	// Get participants
	participants, err := s.participantRepo.GetByCalendarID(ctx, calendar.ID)
	if err != nil {
		return nil, err
	}

	return buildCalendarResponse(calendar, participants)
}

// DeleteCalendar deletes a calendar (requires ownership or admin role)
func (s *CalendarService) DeleteCalendar(ctx context.Context, userID, userRole, calendarID string) error {
	id, err := uuid.Parse(calendarID)
	if err != nil {
		return fmt.Errorf("invalid calendar id: %w", err)
	}

	calendar, err := s.calendarRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return ErrCalendarNotFound
		}
		return err
	}

	// Check ownership or admin role
	if calendar.OwnerID.String() != userID && userRole != "admin" {
		return ErrUnauthorized
	}

	// Invalidate the public calendar cache before deletion
	cacheKey := cache.CalendarByPublicTokenKey(calendar.PublicToken)
	_ = s.cache.Delete(ctx, cacheKey)

	return s.calendarRepo.Delete(ctx, id)
}

// RegenerateToken regenerates a token (public or ics)
func (s *CalendarService) RegenerateToken(ctx context.Context, userID, userRole, calendarID, tokenType string) (*models.CalendarResponse, error) {
	if tokenType != "public" && tokenType != "ics" {
		return nil, ErrInvalidTokenType
	}

	id, err := uuid.Parse(calendarID)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar id: %w", err)
	}

	calendar, err := s.calendarRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Check ownership or admin role
	if calendar.OwnerID.String() != userID && userRole != "admin" {
		return nil, ErrUnauthorized
	}

	// If regenerating public token, invalidate the old cache first
	if tokenType == "public" {
		oldCacheKey := cache.CalendarByPublicTokenKey(calendar.PublicToken)
		_ = s.cache.Delete(ctx, oldCacheKey)
	}

	// Generate new token
	newToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Update the appropriate token
	if err := s.calendarRepo.RegenerateToken(ctx, id, tokenType, newToken); err != nil {
		return nil, err
	}

	// Update calendar object
	if tokenType == "public" {
		calendar.PublicToken = newToken
	} else {
		calendar.ICSToken = newToken
	}

	// Get participants
	participants, err := s.participantRepo.GetByCalendarID(ctx, calendar.ID)
	if err != nil {
		return nil, err
	}

	return buildCalendarResponse(calendar, participants)
}

// AddParticipant adds a participant to a calendar
func (s *CalendarService) AddParticipant(ctx context.Context, userID, userRole, calendarID string, req *models.AddParticipantRequest) (*models.Participant, error) {
	id, err := uuid.Parse(calendarID)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar id: %w", err)
	}

	calendar, err := s.calendarRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Check ownership or admin role
	if calendar.OwnerID.String() != userID && userRole != "admin" {
		return nil, ErrUnauthorized
	}

	participant := &models.Participant{
		CalendarID: id,
		Name:       req.Name,
	}
	participant.ID = uuid.New()

	if err := s.participantRepo.Create(ctx, participant); err != nil {
		if errors.Is(err, repository.ErrParticipantAlreadyExists) {
			return nil, ErrParticipantExists
		}
		return nil, err
	}

	// Invalidate the public calendar cache since participants list changed
	cacheKey := cache.CalendarByPublicTokenKey(calendar.PublicToken)
	_ = s.cache.Delete(ctx, cacheKey)

	return participant, nil
}

// UpdateParticipant updates a participant's name
func (s *CalendarService) UpdateParticipant(ctx context.Context, userID, userRole, calendarID, participantID string, req *models.UpdateParticipantRequest) (*models.Participant, error) {
	calID, err := uuid.Parse(calendarID)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar id: %w", err)
	}

	calendar, err := s.calendarRepo.GetByID(ctx, calID)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Check ownership or admin role
	if calendar.OwnerID.String() != userID && userRole != "admin" {
		return nil, ErrUnauthorized
	}

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

	if participant.CalendarID != calID {
		return nil, ErrParticipantNotFound
	}

	// Update the participant's name
	if err := s.participantRepo.Update(ctx, partID, req.Name); err != nil {
		if errors.Is(err, repository.ErrParticipantAlreadyExists) {
			return nil, ErrParticipantExists
		}
		return nil, err
	}

	// Invalidate the public calendar cache since participants list changed
	cacheKey := cache.CalendarByPublicTokenKey(calendar.PublicToken)
	_ = s.cache.Delete(ctx, cacheKey)

	// Get updated participant
	participant, err = s.participantRepo.GetByID(ctx, partID)
	if err != nil {
		return nil, err
	}

	return participant, nil
}

// RemoveParticipant removes a participant from a calendar
func (s *CalendarService) RemoveParticipant(ctx context.Context, userID, userRole, calendarID, participantID string) error {
	calID, err := uuid.Parse(calendarID)
	if err != nil {
		return fmt.Errorf("invalid calendar id: %w", err)
	}

	calendar, err := s.calendarRepo.GetByID(ctx, calID)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return ErrCalendarNotFound
		}
		return err
	}

	// Check ownership or admin role
	if calendar.OwnerID.String() != userID && userRole != "admin" {
		return ErrUnauthorized
	}

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

	if participant.CalendarID != calID {
		return ErrParticipantNotFound
	}

	if err := s.participantRepo.Delete(ctx, partID); err != nil {
		return err
	}

	// Count remaining participants
	remainingParticipants, err := s.participantRepo.GetByCalendarID(ctx, calID)
	if err != nil {
		return fmt.Errorf("failed to count remaining participants: %w", err)
	}

	// If threshold exceeds remaining participants, reduce it automatically
	remainingCount := len(remainingParticipants)
	if calendar.Threshold > remainingCount && remainingCount > 0 {
		calendar.Threshold = remainingCount
		if err := s.calendarRepo.Update(ctx, calendar); err != nil {
			return fmt.Errorf("failed to update calendar threshold: %w", err)
		}
	}

	// Invalidate the public calendar cache since participants list changed
	cacheKey := cache.CalendarByPublicTokenKey(calendar.PublicToken)
	_ = s.cache.Delete(ctx, cacheKey)

	return nil
}

// GetPublicCalendar retrieves a calendar by public token (no auth required)
// Uses cache if available (skip cache when participant filtering is needed)
func (s *CalendarService) GetPublicCalendar(ctx context.Context, token, participantID string) (*models.PublicCalendarResponse, error) {
	// Fetch calendar from database
	calendar, err := s.calendarRepo.GetByPublicToken(ctx, token)
	if err != nil {
		if errors.Is(err, repository.ErrCalendarNotFound) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	// Get participants
	participants, err := s.participantRepo.GetByCalendarID(ctx, calendar.ID)
	if err != nil {
		return nil, err
	}

	// Filter/mask participants based on lock_participants and participantID
	filteredParticipants := filterParticipants(calendar.LockParticipants, participantID, participants)

	// Build response with parsed allowed_hours
	response, err := buildPublicCalendarResponse(calendar, filteredParticipants)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// ListUserCalendars lists all calendars owned by a specific user (admin only)
func (s *CalendarService) ListUserCalendars(ctx context.Context, targetUserID string) ([]*models.CalendarResponse, error) {
	ownerUUID, err := uuid.Parse(targetUserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	calendars, err := s.calendarRepo.GetByOwnerID(ctx, ownerUUID)
	if err != nil {
		return nil, err
	}

	var responses []*models.CalendarResponse
	for _, calendar := range calendars {
		// Get participants for each calendar
		participants, err := s.participantRepo.GetByCalendarID(ctx, calendar.ID)
		if err != nil {
			return nil, err
		}

		response, err := buildCalendarResponse(calendar, participants)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}

	return responses, nil
}

// generateToken generates a random 64-character hex token
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
