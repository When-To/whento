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

// EventTimes returns the event start and end times from its slot times
func (e *CalendarEvent) EventTimes() (start, end *time.Time) {
	if e.SlotStartTime == nil || e.SlotEndTime == nil {
		return nil, nil
	}

	// Load the timezone for this event
	loc, err := time.LoadLocation(e.Timezone)
	if err != nil {
		// Fallback to UTC if timezone loading fails
		loc = time.UTC
	}

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

// IsAllDay returns true if the event's slot covers the full day (00:00-23:59)
func (e *CalendarEvent) IsAllDay() bool {
	return e.SlotStartTime != nil && e.SlotEndTime != nil &&
		*e.SlotStartTime == "00:00" && *e.SlotEndTime == "23:59"
}
