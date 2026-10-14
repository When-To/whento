// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

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
