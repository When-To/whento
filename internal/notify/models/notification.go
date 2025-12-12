// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

import (
	"time"

	"github.com/google/uuid"
)

// ThresholdTransition represents a change in threshold status for a calendar date
type ThresholdTransition struct {
	CalendarID     uuid.UUID
	Date           time.Time
	PreviousCount  int
	NewCount       int
	Threshold      int
	TransitionType string // "reached", "lost", "none"
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
