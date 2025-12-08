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

// AvailabilityResponse represents the response for availability
type AvailabilityResponse struct {
	ID              uuid.UUID `json:"id"`
	ParticipantID   uuid.UUID `json:"participant_id"`
	ParticipantName string    `json:"participant_name"`
	Date            string    `json:"date"`                 // Format: "2006-01-02"
	StartTime       *string   `json:"start_time,omitempty"` // Format: "15:04"
	EndTime         *string   `json:"end_time,omitempty"`   // Format: "15:04"
	Note            string    `json:"note,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ParticipantAvailabilitySummary represents availability summary for a participant
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

// DateAvailabilitySummary represents all participants available on a specific date
type DateAvailabilitySummary struct {
	Date         string                           `json:"date"`
	TotalCount   int                              `json:"total_count"`
	Participants []ParticipantAvailabilitySummary `json:"participants"`
}

// PublicDateAvailabilitySummary represents all participants available on a specific date (public view)
type PublicDateAvailabilitySummary struct {
	Date         string                                 `json:"date"`
	TotalCount   int                                    `json:"total_count"`
	Participants []PublicParticipantAvailabilitySummary `json:"participants"`
}
