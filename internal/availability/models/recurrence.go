// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/models"
)

// Recurrence represents a recurring availability pattern
type Recurrence struct {
	models.Entity
	ParticipantID uuid.UUID `json:"participant_id"`
	DayOfWeek     int       `json:"day_of_week"`          // 0=Sunday, 1=Monday, ..., 6=Saturday
	StartTime     *string   `json:"start_time,omitempty"` // Optional, format "HH:MM"
	EndTime       *string   `json:"end_time,omitempty"`   // Optional, format "HH:MM"
	Note          string    `json:"note,omitempty"`
	StartDate     string    `json:"start_date"`         // Format: "YYYY-MM-DD"
	EndDate       *string   `json:"end_date,omitempty"` // Optional, format: "YYYY-MM-DD"
	CreatedAt     time.Time `json:"created_at"`
}

// RecurrenceException represents a date excluded from a recurrence
type RecurrenceException struct {
	models.Entity
	RecurrenceID uuid.UUID `json:"recurrence_id"`
	ExcludedDate string    `json:"excluded_date"` // Format: "YYYY-MM-DD"
	CreatedAt    time.Time `json:"created_at"`
}

// CreateRecurrenceRequest represents the request to create a recurrence
type CreateRecurrenceRequest struct {
	DayOfWeek *int    `json:"day_of_week" validate:"required,min=0,max=6"`
	StartTime *string `json:"start_time,omitempty" validate:"omitempty,len=5"` // Format: "HH:MM"
	EndTime   *string `json:"end_time,omitempty" validate:"omitempty,len=5"`
	Note      string  `json:"note,omitempty" validate:"omitempty,max=500"`
	StartDate string  `json:"start_date" validate:"required"` // Format: "YYYY-MM-DD"
	EndDate   *string `json:"end_date,omitempty"`             // Format: "YYYY-MM-DD"
}

// UpdateRecurrenceRequest represents the request to update a recurrence
type UpdateRecurrenceRequest struct {
	DayOfWeek *int    `json:"day_of_week" validate:"required,min=0,max=6"`
	StartTime *string `json:"start_time,omitempty" validate:"omitempty,len=5"` // Format: "HH:MM"
	EndTime   *string `json:"end_time,omitempty" validate:"omitempty,len=5"`
	Note      string  `json:"note,omitempty" validate:"omitempty,max=500"`
	StartDate string  `json:"start_date" validate:"required"` // Format: "YYYY-MM-DD"
	EndDate   *string `json:"end_date,omitempty"`             // Format: "YYYY-MM-DD"
}

// CreateExceptionRequest represents the request to create an exception
type CreateExceptionRequest struct {
	ExcludedDate string `json:"excluded_date" validate:"required"` // Format: "YYYY-MM-DD"
}

// RecurrenceWithExceptions includes a recurrence and its exceptions
type RecurrenceWithExceptions struct {
	Recurrence
	Exceptions []RecurrenceException `json:"exceptions"`
}
