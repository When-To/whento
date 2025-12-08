// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"
)

// CalendarEvent represents a calendar event to be exported to ICS
type CalendarEvent struct {
	Date                time.Time
	CalendarID          uuid.UUID
	CalendarName        string
	CalendarDescription string
	EventNumber         int
	AvailableCount      int
	TotalParticipants   int
	Threshold           int
	Participants        []ParticipantAvailability
	Timezone            string
	// SlotStartTime and SlotEndTime define the time slot for this event
	// When set, these override the calculated EventTimes()
	SlotStartTime *string // HH:MM format
	SlotEndTime   *string // HH:MM format
	// SlotIndex is used when multiple events exist for the same date (e.g., 0, 1, 2)
	SlotIndex int
}

// ParticipantAvailability represents a participant's availability for an event
type ParticipantAvailability struct {
	Name      string
	StartTime *string
	EndTime   *string
	Note      string
}

// EventTimes calculates the event start and end times based on slot times or participants
func (e *CalendarEvent) EventTimes() (start, end *time.Time) {
	// Load the timezone for this event
	loc, err := time.LoadLocation(e.Timezone)
	if err != nil {
		// Fallback to UTC if timezone loading fails
		loc = time.UTC
	}

	// If slot times are explicitly set, use them
	if e.SlotStartTime != nil && e.SlotEndTime != nil {
		if t, err := time.Parse("15:04", *e.SlotStartTime); err == nil {
			startTime := time.Date(e.Date.Year(), e.Date.Month(), e.Date.Day(),
				t.Hour(), t.Minute(), 0, 0, loc)
			start = &startTime
		}
		if t, err := time.Parse("15:04", *e.SlotEndTime); err == nil {
			endTime := time.Date(e.Date.Year(), e.Date.Month(), e.Date.Day(),
				t.Hour(), t.Minute(), 0, 0, loc)
			end = &endTime
		}
		return start, end
	}

	// Fallback to calculating from participants (legacy behavior)
	var maxStart, minEnd *time.Time

	for _, p := range e.Participants {
		if p.StartTime != nil {
			t, err := time.Parse("15:04", *p.StartTime)
			if err == nil {
				// Combine date with time in the event's timezone
				startTime := time.Date(e.Date.Year(), e.Date.Month(), e.Date.Day(),
					t.Hour(), t.Minute(), 0, 0, loc)

				if maxStart == nil || startTime.After(*maxStart) {
					maxStart = &startTime
				}
			}
		}

		if p.EndTime != nil {
			t, err := time.Parse("15:04", *p.EndTime)
			if err == nil {
				// Combine date with time in the event's timezone
				endTime := time.Date(e.Date.Year(), e.Date.Month(), e.Date.Day(),
					t.Hour(), t.Minute(), 0, 0, loc)

				if minEnd == nil || endTime.Before(*minEnd) {
					minEnd = &endTime
				}
			}
		}
	}

	return maxStart, minEnd
}

// IsAllDay returns true if this is an all-day event (no times specified or all times are 00:00-23:59)
func (e *CalendarEvent) IsAllDay() bool {
	// If slot times are explicitly set, check if they cover the full day
	if e.SlotStartTime != nil && e.SlotEndTime != nil {
		return *e.SlotStartTime == "00:00" && *e.SlotEndTime == "23:59"
	}

	// Fallback to checking participants
	for _, p := range e.Participants {
		if p.StartTime != nil || p.EndTime != nil {
			// Check if this participant has a time range that is NOT full day (00:00-23:59)
			if !isFullDayTimeRange(p.StartTime, p.EndTime) {
				return false
			}
		}
	}
	// If no one has times, it's all day
	// If everyone has 00:00-23:59, it's also all day
	return true
}

// isFullDayTimeRange checks if a time range represents a full day (00:00-23:59)
func isFullDayTimeRange(startTime, endTime *string) bool {
	// If both are nil, consider it as full day
	if startTime == nil && endTime == nil {
		return true
	}

	start := "00:00"
	if startTime != nil {
		start = *startTime
	}

	end := "23:59"
	if endTime != nil {
		end = *endTime
	}

	return start == "00:00" && end == "23:59"
}
