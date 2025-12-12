// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"testing"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/availability/models"
)

func TestCalculateMaxSimultaneousParticipants(t *testing.T) {
	tests := []struct {
		name         string
		participants []models.ParticipantAvailabilitySummary
		expected     int
		description  string
	}{
		{
			name:         "Empty participants",
			participants: []models.ParticipantAvailabilitySummary{},
			expected:     0,
			description:  "No participants should return 0",
		},
		{
			name: "Two participants with no overlap",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("08:00"),
					EndTime:         stringPtr("12:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("14:00"),
					EndTime:         stringPtr("18:00"),
				},
			},
			expected:    1,
			description: "Two participants without overlap should have max 1 simultaneous",
		},
		{
			name: "Three participants with partial overlap",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("08:00"),
					EndTime:         stringPtr("18:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("08:00"),
					EndTime:         stringPtr("12:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Charlie",
					StartTime:       stringPtr("14:00"),
					EndTime:         stringPtr("18:00"),
				},
			},
			expected:    2,
			description: "Three participants with one all-day and two partial should have max 2 simultaneous",
		},
		{
			name: "All participants available all day",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       nil,
					EndTime:         nil,
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       nil,
					EndTime:         nil,
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Charlie",
					StartTime:       nil,
					EndTime:         nil,
				},
			},
			expected:    3,
			description: "Three participants all day should have max 3 simultaneous",
		},
		{
			name: "Complete overlap",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("10:00"),
					EndTime:         stringPtr("16:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("10:00"),
					EndTime:         stringPtr("16:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Charlie",
					StartTime:       stringPtr("10:00"),
					EndTime:         stringPtr("16:00"),
				},
			},
			expected:    3,
			description: "Three participants with complete overlap should have max 3 simultaneous",
		},
		{
			name: "Staggered availability",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("08:00"),
					EndTime:         stringPtr("14:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("10:00"),
					EndTime:         stringPtr("16:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Charlie",
					StartTime:       stringPtr("12:00"),
					EndTime:         stringPtr("18:00"),
				},
			},
			expected:    3,
			description: "Three participants with staggered times should have max 3 simultaneous during overlap (12:00-14:00)",
		},
		{
			name: "Available from midnight",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("00:00"),
					EndTime:         stringPtr("12:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("00:00"),
					EndTime:         stringPtr("08:00"),
				},
			},
			expected:    2,
			description: "Two participants from midnight should be counted correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateMaxSimultaneousParticipants(tt.participants)
			if result != tt.expected {
				t.Errorf("%s: expected %d, got %d", tt.description, tt.expected, result)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestRecurrencesOverlap(t *testing.T) {
	tests := []struct {
		name       string
		startDateA string
		endDateA   string // empty string means no end date (infinite)
		startDateB string
		endDateB   string // empty string means no end date (infinite)
		expected   bool
	}{
		{
			name:       "Both infinite (no end dates) - always overlap",
			startDateA: "2025-01-01",
			endDateA:   "",
			startDateB: "2025-06-01",
			endDateB:   "",
			expected:   true,
		},
		{
			name:       "A infinite, B ends before A starts - no overlap",
			startDateA: "2025-06-01",
			endDateA:   "",
			startDateB: "2025-01-01",
			endDateB:   "2025-05-31",
			expected:   false,
		},
		{
			name:       "A infinite, B ends after A starts - overlap",
			startDateA: "2025-06-01",
			endDateA:   "",
			startDateB: "2025-01-01",
			endDateB:   "2025-12-31",
			expected:   true,
		},
		{
			name:       "A infinite, B ends exactly when A starts - overlap",
			startDateA: "2025-06-01",
			endDateA:   "",
			startDateB: "2025-01-01",
			endDateB:   "2025-06-01",
			expected:   true,
		},
		{
			name:       "B infinite, A ends before B starts - no overlap",
			startDateA: "2025-01-01",
			endDateA:   "2025-05-31",
			startDateB: "2025-06-01",
			endDateB:   "",
			expected:   false,
		},
		{
			name:       "B infinite, A ends after B starts - overlap",
			startDateA: "2025-01-01",
			endDateA:   "2025-12-31",
			startDateB: "2025-06-01",
			endDateB:   "",
			expected:   true,
		},
		{
			name:       "Both have end dates, no overlap (A before B)",
			startDateA: "2025-01-01",
			endDateA:   "2025-03-31",
			startDateB: "2025-06-01",
			endDateB:   "2025-08-31",
			expected:   false,
		},
		{
			name:       "Both have end dates, no overlap (B before A)",
			startDateA: "2025-06-01",
			endDateA:   "2025-08-31",
			startDateB: "2025-01-01",
			endDateB:   "2025-03-31",
			expected:   false,
		},
		{
			name:       "Both have end dates, overlap",
			startDateA: "2025-01-01",
			endDateA:   "2025-06-30",
			startDateB: "2025-04-01",
			endDateB:   "2025-12-31",
			expected:   true,
		},
		{
			name:       "Both have end dates, A contains B",
			startDateA: "2025-01-01",
			endDateA:   "2025-12-31",
			startDateB: "2025-03-01",
			endDateB:   "2025-06-30",
			expected:   true,
		},
		{
			name:       "Both have end dates, touching (A ends when B starts)",
			startDateA: "2025-01-01",
			endDateA:   "2025-06-01",
			startDateB: "2025-06-01",
			endDateB:   "2025-12-31",
			expected:   true,
		},
		{
			name:       "Both have end dates, adjacent (A ends before B starts)",
			startDateA: "2025-01-01",
			endDateA:   "2025-05-31",
			startDateB: "2025-06-01",
			endDateB:   "2025-12-31",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recurrencesOverlap(tt.startDateA, tt.endDateA, tt.startDateB, tt.endDateB)
			if result != tt.expected {
				t.Errorf("recurrencesOverlap(%s, %s, %s, %s) = %v, expected %v",
					tt.startDateA, tt.endDateA, tt.startDateB, tt.endDateB, result, tt.expected)
			}
		})
	}
}
