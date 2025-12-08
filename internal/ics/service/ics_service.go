// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/google/uuid"

	"github.com/whento/pkg/datevalidation"
	"github.com/whento/whento/internal/ics/models"
	"github.com/whento/whento/internal/ics/repository"
)

var (
	ErrCalendarNotFound = errors.New("calendar not found")
	ErrQuotaExceeded    = errors.New("calendar owner has exceeded their quota - please delete calendars or upgrade")
)

// CalendarRepository defines the interface for calendar repository operations
type CalendarRepository interface {
	GetByICSToken(ctx context.Context, icsToken string) (*repository.Calendar, error)
}

// AvailabilityRepository defines the interface for availability repository operations
type AvailabilityRepository interface {
	GetEventsAboveThreshold(ctx context.Context, calendarID uuid.UUID, threshold int) (map[time.Time][]repository.DateAvailability, error)
}

// QuotaChecker defines the interface for checking if a user is over quota
type QuotaChecker interface {
	IsOverQuota(ctx context.Context, userID uuid.UUID) (bool, error)
}

type ICSService struct {
	calendarRepo     CalendarRepository
	availabilityRepo AvailabilityRepository
	quotaChecker     QuotaChecker
	appDomain        string
}

func NewICSService(
	calendarRepo CalendarRepository,
	availabilityRepo AvailabilityRepository,
	quotaChecker QuotaChecker,
	appDomain string,
) *ICSService {
	return &ICSService{
		calendarRepo:     calendarRepo,
		availabilityRepo: availabilityRepo,
		quotaChecker:     quotaChecker,
		appDomain:        appDomain,
	}
}

// GenerateFeed generates an iCalendar feed for a calendar using its ICS token
// The host parameter should be the host from the HTTP request (e.g., "192.168.1.10:8080" or "example.com")
func (s *ICSService) GenerateFeed(ctx context.Context, icsToken string, host string) (string, error) {
	// Use provided host if available, otherwise fall back to configured appDomain
	domain := host
	if domain == "" {
		domain = s.appDomain
	}

	// Get calendar
	calendar, err := s.calendarRepo.GetByICSToken(ctx, icsToken)
	if err != nil {
		return "", ErrCalendarNotFound
	}

	// Check if calendar owner is over quota (subscription/license expired with too many calendars)
	// If over quota, block ICS feed generation until they delete calendars or upgrade
	isOverQuota, _ := s.quotaChecker.IsOverQuota(ctx, calendar.OwnerID)
	if isOverQuota {
		return "", ErrQuotaExceeded
	}

	// Get events above threshold
	eventsByDate, err := s.availabilityRepo.GetEventsAboveThreshold(ctx, calendar.ID, calendar.Threshold)
	if err != nil {
		return "", fmt.Errorf("failed to get events: %w", err)
	}

	// Convert to calendar events
	events := s.buildCalendarEvents(calendar, eventsByDate)

	// Generate ICS
	ics := s.generateICS(calendar, events, domain)

	return ics, nil
}

// buildCalendarEvents converts repository data to calendar events
// It uses time slot segmentation to correctly handle cases where different
// participants are available at different times of the day
func (s *ICSService) buildCalendarEvents(calendar *repository.Calendar, eventsByDate map[time.Time][]repository.DateAvailability) []models.CalendarEvent {
	var events []models.CalendarEvent

	// Sort dates
	dates := make([]time.Time, 0, len(eventsByDate))
	for date := range eventsByDate {
		dates = append(dates, date)
	}
	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Before(dates[j])
	})

	// Build events with sequential numbering
	eventNumber := 0
	for _, date := range dates {
		availabilities := eventsByDate[date]
		if len(availabilities) == 0 {
			continue
		}

		// Filter by calendar date range if set
		if calendar.StartDate != nil && date.Before(*calendar.StartDate) {
			// Skip events before calendar start date
			continue
		}
		if calendar.EndDate != nil && date.After(*calendar.EndDate) {
			// Skip events after calendar end date
			continue
		}

		// Filter by allowed weekdays, holidays policy, and holiday eves
		if !datevalidation.IsDateAllowed(date, calendar.Timezone, calendar.AllowedWeekdays, calendar.HolidaysPolicy, calendar.AllowHolidayEves) {
			// Skip this event if the date is not allowed
			continue
		}

		// Compute time slots where threshold is met
		timeSlots := computeTimeSlots(availabilities, calendar.Threshold)

		// Create an event for each time slot
		for slotIdx, slot := range timeSlots {
			startTime := slot.StartTime
			endTime := slot.EndTime

			event := models.CalendarEvent{
				Date:                date,
				CalendarID:          calendar.ID,
				CalendarName:        calendar.Name,
				CalendarDescription: calendar.Description,
				EventNumber:         eventNumber + 1, // Will be set properly after filter
				AvailableCount:      len(slot.Participants),
				TotalParticipants:   calendar.TotalParticipants,
				Threshold:           calendar.Threshold,
				Participants:        slot.Participants,
				Timezone:            calendar.Timezone,
				SlotStartTime:       &startTime,
				SlotEndTime:         &endTime,
				SlotIndex:           slotIdx,
			}

			// Apply min_duration_hours filter if configured
			if calendar.MinDurationHours > 0 {
				duration := s.calculateEventDuration(&event)
				if duration < float64(calendar.MinDurationHours) {
					// Skip this event if duration is less than minimum
					continue
				}
			}

			// Only increment event number for events that pass the filter
			eventNumber++
			event.EventNumber = eventNumber
			events = append(events, event)
		}
	}

	return events
}

// calculateEventDuration calculates the duration of an event in hours
func (s *ICSService) calculateEventDuration(event *models.CalendarEvent) float64 {
	// If it's an all-day event, return 24 hours
	if event.IsAllDay() {
		return 24.0
	}

	// Calculate duration based on event times
	start, end := event.EventTimes()
	if start == nil || end == nil {
		// If we can't determine times, consider it as all-day
		return 24.0
	}

	// Calculate duration in hours
	duration := end.Sub(*start).Hours()
	return duration
}

// generateICS generates the iCalendar string from events
func (s *ICSService) generateICS(calendar *repository.Calendar, events []models.CalendarEvent, domain string) string {
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	cal.SetProductId("-//WhenTo//WhenTo Calendar//EN")
	cal.SetName(calendar.Name)
	cal.SetXWRCalName(calendar.Name)
	cal.SetXWRTimezone(calendar.Timezone) // Hint for calendar clients about the intended timezone
	cal.SetRefreshInterval("PT1H")        // Refresh every hour

	// Add events using floating time (RFC 5545 FORM #1)
	// Events are interpreted in the local timezone of the viewer
	// X-WR-TIMEZONE provides context about the calendar's timezone without requiring VTIMEZONE
	for _, event := range events {
		s.addEvent(cal, event, domain)
	}

	// Serialize and convert LF to CRLF as required by RFC 5545 section 3.1
	icsContent := cal.Serialize()
	icsContent = strings.ReplaceAll(icsContent, "\n", "\r\n")

	return icsContent
}

// addEvent adds a single event to the calendar
func (s *ICSService) addEvent(cal *ics.Calendar, event models.CalendarEvent, domain string) {
	vevent := cal.AddEvent(s.generateUID(event, domain))

	// Set timestamp
	vevent.SetDtStampTime(time.Now())

	// Set status
	vevent.SetStatus(ics.ObjectStatusConfirmed)

	// Set summary: "{CalendarName} #{EventNumber} ({available}/{total})"
	summary := fmt.Sprintf("%s #%d (%d/%d)",
		event.CalendarName,
		event.EventNumber,
		event.AvailableCount,
		event.TotalParticipants,
	)
	vevent.SetSummary(summary)

	// Set description with participant list
	description := s.buildDescription(event)
	vevent.SetDescription(description)

	// Add participants as ATTENDEE fields
	s.addAttendees(vevent, event)

	// Set date/time using floating time (RFC 5545 FORM #1: DATE WITH LOCAL TIME)
	// Floating times are not bound to any timezone - they represent the same
	// hour/minute/second regardless of which timezone the viewer is in
	if event.IsAllDay() {
		// All-day event (use VALUE=DATE format)
		vevent.SetAllDayStartAt(event.Date)
		vevent.SetAllDayEndAt(event.Date.AddDate(0, 0, 1)) // Next day for all-day events
	} else {
		// Timed event using floating time (no TZID, no Z suffix)
		// Format: DTSTART:19970714T133000
		start, end := event.EventTimes()

		if start != nil {
			dtstart := start.Format("20060102T150405")
			vevent.AddProperty(ics.ComponentProperty("DTSTART"), dtstart)
		} else {
			// Fallback to date if no start time
			vevent.SetAllDayStartAt(event.Date)
		}

		if end != nil {
			dtend := end.Format("20060102T150405")
			vevent.AddProperty(ics.ComponentProperty("DTEND"), dtend)
		} else if start != nil {
			// If we have start but no end, make it 1 hour
			endTime := start.Add(1 * time.Hour)
			dtend := endTime.Format("20060102T150405")
			vevent.AddProperty(ics.ComponentProperty("DTEND"), dtend)
		} else {
			// Fallback to next day
			vevent.SetAllDayEndAt(event.Date.AddDate(0, 0, 1))
		}
	}
}

// generateUID generates a stable UID for an event
// When multiple events exist for the same date (different time slots), the SlotIndex is included
func (s *ICSService) generateUID(event models.CalendarEvent, domain string) string {
	dateStr := event.Date.Format("20060102")
	if event.SlotIndex > 0 {
		return fmt.Sprintf("%s-%d-whento-%s@%s", dateStr, event.SlotIndex, event.CalendarID.String(), domain)
	}
	return fmt.Sprintf("%s-whento-%s@%s", dateStr, event.CalendarID.String(), domain)
}

// buildDescription builds the event description with participant list and calendar description
func (s *ICSService) buildDescription(event models.CalendarEvent) string {
	desc := "Participants disponibles:\n"

	for _, p := range event.Participants {
		line := fmt.Sprintf("- %s", p.Name)

		// Only show time range if it's not a full day (00:00-23:59)
		if p.StartTime != nil || p.EndTime != nil {
			start := "00:00"
			if p.StartTime != nil {
				start = *p.StartTime
			}
			end := "23:59"
			if p.EndTime != nil {
				end = *p.EndTime
			}

			// Don't show time if it's the full day range
			if start != "00:00" || end != "23:59" {
				line += fmt.Sprintf(" (%s-%s)", start, end)
			}
		}

		if p.Note != "" {
			line += fmt.Sprintf(": %s", p.Note)
		}

		desc += line + "\n"
	}

	// Add calendar description at the end if present
	if event.CalendarDescription != "" {
		desc += "\n---\n" + event.CalendarDescription
	}

	return desc
}

// addAttendees adds participants as ATTENDEE fields in the iCalendar event
func (s *ICSService) addAttendees(vevent *ics.VEvent, event models.CalendarEvent) {
	for _, p := range event.Participants {
		// Add ATTENDEE property with parameters
		// Format: ATTENDEE;CN="Name";ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED:MAILTO:noreply@whento.be
		vevent.AddProperty(
			ics.ComponentProperty("ATTENDEE"),
			"MAILTO:noreply@whento.be",
			&ics.KeyValues{Key: "CN", Value: []string{p.Name}},
			&ics.KeyValues{Key: "ROLE", Value: []string{"REQ-PARTICIPANT"}},
			&ics.KeyValues{Key: "PARTSTAT", Value: []string{"ACCEPTED"}},
			&ics.KeyValues{Key: "CUTYPE", Value: []string{"INDIVIDUAL"}},
		)
	}
}
