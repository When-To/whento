// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/models"
)

// Availability represents a participant's availability for a specific date
type Availability struct {
	models.TimestampedEntity
	ParticipantID uuid.UUID  `json:"participant_id"`
	Date          time.Time  `json:"date"`                 // DATE type in DB
	StartTime     *string    `json:"start_time,omitempty"` // TIME type in DB (optional, format "15:04")
	EndTime       *string    `json:"end_time,omitempty"`   // TIME type in DB (optional, format "15:04")
	Note          string     `json:"note,omitempty"`
	Source        string     `json:"source"` // 'manual' or 'recurrence'
	RecurrenceID  *uuid.UUID `json:"recurrence_id,omitempty"`
}

// Occurrence is one participant being available on one date, whether they said so
// directly or a recurrence says it for them.
//
// This is what the several expansions of "who is available" had each been deriving
// separately — the day view and the range view in Go, the threshold count in SQL — and
// disagreeing about. A recurrence occurrence and a manual answer are the same thing to
// everyone who reads them; which record explains it matters only to the query that
// produced it, and it applies the one rule they all shared: a manual answer suppresses
// the recurrence for that participant on that date, times and all.
type Occurrence struct {
	Date            time.Time
	ParticipantID   uuid.UUID
	ParticipantName string
	// StartTime and EndTime are "15:04", or nil for an all-day answer.
	StartTime *string
	EndTime   *string
	Note      string
}

// AvailableParticipant is one participant who counts as available on a given date,
// whether they said so directly or a recurrence says it for them.
//
// The distinction matters nowhere it is used, which is the point: a recurrence is how
// someone answers "every Friday" once instead of fifty times, not a lesser kind of
// answer. Reading it any other way is what kept recurrence-only participants out of
// their own threshold notifications.
type AvailableParticipant struct {
	ID   uuid.UUID
	Name string
}

// CreateAvailabilityRequest represents a request to create availability
type CreateAvailabilityRequest struct {
	Date      string  `json:"date" validate:"required"`                  // Format: "2006-01-02"
	StartTime *string `json:"start_time,omitempty" validate:"omitempty"` // Format: "15:04"
	EndTime   *string `json:"end_time,omitempty" validate:"omitempty"`   // Format: "15:04"
	Note      string  `json:"note,omitempty" validate:"max=1000"`
}

// UpdateAvailabilityRequest represents a request to update availability
type UpdateAvailabilityRequest struct {
	StartTime *string `json:"start_time,omitempty" validate:"omitempty"` // Format: "15:04" or null
	EndTime   *string `json:"end_time,omitempty" validate:"omitempty"`   // Format: "15:04" or null
	Note      *string `json:"note,omitempty" validate:"omitempty,max=1000"`
}

// AvailabilityResponse represents the response for availability (single operation)
type AvailabilityResponse struct {
	ID                       uuid.UUID `json:"id"`
	ParticipantID            uuid.UUID `json:"participant_id"`
	ParticipantName          string    `json:"participant_name"`
	ParticipantEmail         *string   `json:"participant_email,omitempty"`
	ParticipantEmailVerified bool      `json:"participant_email_verified"`
	Date                     string    `json:"date"`                 // Format: "2006-01-02"
	StartTime                *string   `json:"start_time,omitempty"` // Format: "15:04"
	EndTime                  *string   `json:"end_time,omitempty"`   // Format: "15:04"
	Note                     string    `json:"note,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// AvailabilityItem represents a single availability without participant info
type AvailabilityItem struct {
	ID        uuid.UUID `json:"id"`
	Date      string    `json:"date"`                 // Format: "2006-01-02"
	StartTime *string   `json:"start_time,omitempty"` // Format: "15:04"
	EndTime   *string   `json:"end_time,omitempty"`   // Format: "15:04"
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ParticipantInfo represents participant information for availabilities response
type ParticipantInfo struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Email         *string   `json:"email,omitempty"`
	EmailVerified bool      `json:"email_verified"`
}

// ParticipantAvailabilitiesResponse represents a participant with their availabilities
type ParticipantAvailabilitiesResponse struct {
	Participant    ParticipantInfo    `json:"participant"`
	Availabilities []AvailabilityItem `json:"availabilities"`
}

// ParticipantAvailabilitySummary represents availability summary for a participant.
//
// Internal to the service layer: its ParticipantID is a plain uuid.UUID and is
// therefore always serialised, so this type must never be handed to a public
// endpoint. Both summary endpoints build these, then convert with
// filterParticipantSummaries into the Public* pair below, which is what actually
// crosses the wire.
type ParticipantAvailabilitySummary struct {
	ParticipantID   uuid.UUID `json:"participant_id"`
	ParticipantName string    `json:"participant_name"`
	StartTime       *string   `json:"start_time,omitempty"`
	EndTime         *string   `json:"end_time,omitempty"`
	Note            string    `json:"note,omitempty"`
}

// PublicParticipantAvailabilitySummary represents availability summary for a participant in public views
// The ParticipantID field is nullable to support masking when lock_participants is enabled
type PublicParticipantAvailabilitySummary struct {
	ParticipantID   *uuid.UUID `json:"participant_id,omitempty"`
	ParticipantName string     `json:"participant_name"`
	StartTime       *string    `json:"start_time,omitempty"`
	EndTime         *string    `json:"end_time,omitempty"`
	Note            string     `json:"note,omitempty"`
}

// PublicDateAvailabilitySummary represents all participants available on a specific date (public view)
type PublicDateAvailabilitySummary struct {
	Date         string                                 `json:"date"`
	TotalCount   int                                    `json:"total_count"`
	Participants []PublicParticipantAvailabilitySummary `json:"participants"`
}

// TimeWindow is one availability's span, "15:04" or nil for all day.
type TimeWindow struct {
	Start *string
	End   *string
}

// Window is how an occurrence enters the overlap calculation.
func (o Occurrence) Window() TimeWindow {
	return TimeWindow{Start: o.StartTime, End: o.EndTime}
}

// Window is how a summary row enters it.
func (p ParticipantAvailabilitySummary) Window() TimeWindow {
	return TimeWindow{Start: p.StartTime, End: p.EndTime}
}

// MaxSimultaneousFor returns the largest number of the given windows that are open
// together for at least minMinutes. A nil start or end means all day, so a calendar answered entirely in whole
// days gives the same number as simply counting the answers.
//
// It lives here rather than in the availability service because the notification threshold
// needs it too, and the dependency runs availability -> notify: the detector cannot reach
// back into the service without a cycle. Both packages already import this one.
func MaxSimultaneousFor(windows []TimeWindow, minMinutes int) int {
	if len(windows) == 0 {
		return 0
	}

	// Normalize participant times (treat nil as 00:00-23:59)
	type normalizedParticipant struct {
		startMinutes int
		endMinutes   int
		valid        bool
	}

	normalized := make([]normalizedParticipant, len(windows))
	validCount := 0
	for i, p := range windows {
		startStr := "00:00"
		endStr := "23:59"

		if p.Start != nil && *p.Start != "" {
			startStr = *p.Start
		}
		if p.End != nil && *p.End != "" {
			endStr = *p.End
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

	// The largest group covering any window long enough to be worth meeting in.
	//
	// Every candidate window runs from one boundary to a later one, rather than only
	// between consecutive ones: a group has to cover the whole of a window at least
	// minMinutes long, and that window generally spans several elementary segments. With
	// minMinutes zero the longer candidates can only ever have fewer coverers than the
	// segments they contain, so the answer is the same one this always gave.
	maxCount := 0

	for i := 0; i < len(boundaries)-1; i++ {
		for j := i + 1; j < len(boundaries); j++ {
			if boundaries[j]-boundaries[i] < minMinutes {
				continue
			}

			count := 0
			for _, p := range normalized {
				// Available if their range completely covers the candidate window.
				if p.valid && p.startMinutes <= boundaries[i] && p.endMinutes >= boundaries[j] {
					count++
				}
			}

			if count > maxCount {
				maxCount = count
			}
		}
	}

	return maxCount
}

// MaxSimultaneous is MaxSimultaneousFor with no minimum: the largest number of windows
// open at the same moment, however briefly.
func MaxSimultaneous(windows []TimeWindow) int {
	return MaxSimultaneousFor(windows, 0)
}
