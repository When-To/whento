// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"testing"

	"github.com/whento/whento/internal/ics/repository"
)

func ptr(s string) *string {
	return &s
}

func TestComputeTimeSlots_SimpleOverlap(t *testing.T) {
	// All participants available for the same time range
	availabilities := []repository.DateAvailability{
		{ParticipantName: "Alice", StartTime: ptr("10:00"), EndTime: ptr("18:00")},
		{ParticipantName: "Bob", StartTime: ptr("10:00"), EndTime: ptr("18:00")},
	}

	slots := computeTimeSlots(availabilities, 2)

	if len(slots) != 1 {
		t.Fatalf("Expected 1 slot, got %d", len(slots))
	}

	if slots[0].StartTime != "10:00" || slots[0].EndTime != "18:00" {
		t.Errorf("Expected slot 10:00-18:00, got %s-%s", slots[0].StartTime, slots[0].EndTime)
	}

	if len(slots[0].Participants) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(slots[0].Participants))
	}
}

func TestComputeTimeSlots_NonOverlapping(t *testing.T) {
	// P1 all day, P2 until 12:00, P3 from 14:00
	// Threshold 2 should create 2 separate events
	availabilities := []repository.DateAvailability{
		{ParticipantName: "P1", StartTime: ptr("00:00"), EndTime: ptr("23:59")},
		{ParticipantName: "P2", StartTime: ptr("00:00"), EndTime: ptr("12:00")},
		{ParticipantName: "P3", StartTime: ptr("14:00"), EndTime: ptr("23:59")},
	}

	slots := computeTimeSlots(availabilities, 2)

	if len(slots) != 2 {
		t.Fatalf("Expected 2 slots, got %d", len(slots))
	}

	// First slot: 00:00 - 12:00 (P1 + P2)
	if slots[0].StartTime != "00:00" || slots[0].EndTime != "12:00" {
		t.Errorf("Expected first slot 00:00-12:00, got %s-%s", slots[0].StartTime, slots[0].EndTime)
	}

	// Second slot: 14:00 - 23:59 (P1 + P3)
	if slots[1].StartTime != "14:00" || slots[1].EndTime != "23:59" {
		t.Errorf("Expected second slot 14:00-23:59, got %s-%s", slots[1].StartTime, slots[1].EndTime)
	}
}

func TestComputeTimeSlots_ContinuousCoverage(t *testing.T) {
	// P1 all day, P2 until 12:00, P3 from 12:00
	// No gap, should create 1 continuous event
	availabilities := []repository.DateAvailability{
		{ParticipantName: "P1", StartTime: ptr("00:00"), EndTime: ptr("23:59")},
		{ParticipantName: "P2", StartTime: ptr("00:00"), EndTime: ptr("12:00")},
		{ParticipantName: "P3", StartTime: ptr("12:00"), EndTime: ptr("23:59")},
	}

	slots := computeTimeSlots(availabilities, 2)

	if len(slots) != 1 {
		t.Fatalf("Expected 1 slot (continuous coverage), got %d", len(slots))
	}

	if slots[0].StartTime != "00:00" || slots[0].EndTime != "23:59" {
		t.Errorf("Expected slot 00:00-23:59, got %s-%s", slots[0].StartTime, slots[0].EndTime)
	}
}

func TestComputeTimeSlots_ThresholdNotMet(t *testing.T) {
	// Only 1 participant, threshold 2
	availabilities := []repository.DateAvailability{
		{ParticipantName: "Alice", StartTime: ptr("10:00"), EndTime: ptr("18:00")},
	}

	slots := computeTimeSlots(availabilities, 2)

	if len(slots) != 0 {
		t.Fatalf("Expected 0 slots (threshold not met), got %d", len(slots))
	}
}

func TestComputeTimeSlots_NilTimes(t *testing.T) {
	// Participants with nil times should be treated as full day
	availabilities := []repository.DateAvailability{
		{ParticipantName: "Alice", StartTime: nil, EndTime: nil},
		{ParticipantName: "Bob", StartTime: nil, EndTime: nil},
	}

	slots := computeTimeSlots(availabilities, 2)

	if len(slots) != 1 {
		t.Fatalf("Expected 1 slot, got %d", len(slots))
	}

	if slots[0].StartTime != "00:00" || slots[0].EndTime != "23:59" {
		t.Errorf("Expected full day slot 00:00-23:59, got %s-%s", slots[0].StartTime, slots[0].EndTime)
	}
}

func TestComputeTimeSlots_MixedNilAndExplicit(t *testing.T) {
	// P1 all day (nil), P2 until 12:00
	availabilities := []repository.DateAvailability{
		{ParticipantName: "P1", StartTime: nil, EndTime: nil},
		{ParticipantName: "P2", StartTime: ptr("00:00"), EndTime: ptr("12:00")},
	}

	slots := computeTimeSlots(availabilities, 2)

	if len(slots) != 1 {
		t.Fatalf("Expected 1 slot, got %d", len(slots))
	}

	// P1 (full day) + P2 (00:00-12:00) should give 00:00-12:00
	if slots[0].StartTime != "00:00" || slots[0].EndTime != "12:00" {
		t.Errorf("Expected slot 00:00-12:00, got %s-%s", slots[0].StartTime, slots[0].EndTime)
	}
}

func TestComputeTimeSlots_ThreeParticipantsPartialOverlap(t *testing.T) {
	// P1: 09:00-18:00
	// P2: 10:00-14:00
	// P3: 12:00-20:00
	// Threshold 2: should get continuous coverage from 09:00-20:00 where at least 2 overlap
	availabilities := []repository.DateAvailability{
		{ParticipantName: "P1", StartTime: ptr("09:00"), EndTime: ptr("18:00")},
		{ParticipantName: "P2", StartTime: ptr("10:00"), EndTime: ptr("14:00")},
		{ParticipantName: "P3", StartTime: ptr("12:00"), EndTime: ptr("20:00")},
	}

	slots := computeTimeSlots(availabilities, 2)

	if len(slots) != 1 {
		t.Fatalf("Expected 1 slot, got %d", len(slots))
	}

	// 09:00-10:00: only P1 (1) - threshold NOT met
	// 10:00-12:00: P1 + P2 (2) - threshold met
	// 12:00-14:00: P1 + P2 + P3 (3) - threshold met
	// 14:00-18:00: P1 + P3 (2) - threshold met
	// 18:00-20:00: only P3 (1) - threshold NOT met
	// So the slot should be 10:00-18:00
	if slots[0].StartTime != "10:00" || slots[0].EndTime != "18:00" {
		t.Errorf("Expected slot 10:00-18:00, got %s-%s", slots[0].StartTime, slots[0].EndTime)
	}
}

func TestComputeTimeSlots_HigherThreshold(t *testing.T) {
	// P1: all day
	// P2: 10:00-16:00
	// P3: 12:00-14:00
	// Threshold 3: only 12:00-14:00 has all 3
	availabilities := []repository.DateAvailability{
		{ParticipantName: "P1", StartTime: ptr("00:00"), EndTime: ptr("23:59")},
		{ParticipantName: "P2", StartTime: ptr("10:00"), EndTime: ptr("16:00")},
		{ParticipantName: "P3", StartTime: ptr("12:00"), EndTime: ptr("14:00")},
	}

	slots := computeTimeSlots(availabilities, 3)

	if len(slots) != 1 {
		t.Fatalf("Expected 1 slot, got %d", len(slots))
	}

	if slots[0].StartTime != "12:00" || slots[0].EndTime != "14:00" {
		t.Errorf("Expected slot 12:00-14:00, got %s-%s", slots[0].StartTime, slots[0].EndTime)
	}

	if len(slots[0].Participants) != 3 {
		t.Errorf("Expected 3 participants, got %d", len(slots[0].Participants))
	}
}

func TestComputeTimeSlots_EmptyAvailabilities(t *testing.T) {
	slots := computeTimeSlots(nil, 2)

	if len(slots) != 0 {
		t.Errorf("Expected 0 slots for empty input, got %d", len(slots))
	}
}

func TestIsAllDaySlot(t *testing.T) {
	tests := []struct {
		name     string
		slot     TimeSlot
		expected bool
	}{
		{
			name:     "full day",
			slot:     TimeSlot{StartTime: "00:00", EndTime: "23:59"},
			expected: true,
		},
		{
			name:     "partial day",
			slot:     TimeSlot{StartTime: "09:00", EndTime: "18:00"},
			expected: false,
		},
		{
			name:     "starts at midnight",
			slot:     TimeSlot{StartTime: "00:00", EndTime: "12:00"},
			expected: false,
		},
		{
			name:     "ends at 23:59",
			slot:     TimeSlot{StartTime: "12:00", EndTime: "23:59"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAllDaySlot(&tt.slot)
			if result != tt.expected {
				t.Errorf("isAllDaySlot(%v) = %v, want %v", tt.slot, result, tt.expected)
			}
		})
	}
}

func TestParseTimeToMinutes(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"00:00", 0},
		{"00:30", 30},
		{"01:00", 60},
		{"12:00", 720},
		{"23:59", 1439},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseTimeToMinutes(tt.input)
			if result != tt.expected {
				t.Errorf("parseTimeToMinutes(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMinutesToTimeString(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "00:00"},
		{30, "00:30"},
		{60, "01:00"},
		{720, "12:00"},
		{1439, "23:59"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := minutesToTimeString(tt.input)
			if result != tt.expected {
				t.Errorf("minutesToTimeString(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
