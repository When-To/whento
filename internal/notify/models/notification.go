// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"

	availabilityModels "github.com/whento/whento/internal/availability/models"
)

// ThresholdTransition represents a change in threshold status for a calendar date
type ThresholdTransition struct {
	CalendarID     uuid.UUID
	Date           time.Time
	PreviousCount  int
	NewCount       int
	Threshold      int
	TransitionType string // "reached", "lost", "none"

	// LastJoined and LastWithdrawn come from the activity journal: who joined this
	// date last, and who withdrew last while it had reached its threshold. They are
	// what lets a notification say who, not just how many.
	//
	// The journal's own type rather than a copy of its three fields. A second
	// ParticipantRef declared here would be a struct that has to agree with that one
	// for ever, plus the conversion keeping them in step — and swaggo, which flattens
	// every `models` package into one namespace, would see two types with one name and
	// qualify the exported schema to disambiguate them.
	//
	// Either is nil when the journal has no entry — a date whose whole history predates
	// the journal, or whose participant has since been deleted. The messages then keep
	// exactly the shape they had before this existed, rather than naming nobody.
	LastJoined    *availabilityModels.ParticipantRef
	LastWithdrawn *availabilityModels.ParticipantRef
}

// NotificationEvent represents a notification event to be sent
type NotificationEvent struct {
	CalendarID   uuid.UUID
	CalendarName string
	Date         time.Time
	EventType    string // "threshold_reached", "threshold_lost", "reminder"
	Message      string
	Participants []string
	TimeSlotInfo string
	CalendarURL  string
	PublicToken  string
}

// NotificationRecipient represents a recipient of a notification
type NotificationRecipient struct {
	RecipientID   uuid.UUID
	RecipientType string // "owner", "participant"
	Email         *string
	Name          string
}
