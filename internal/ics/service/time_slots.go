// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"sort"
	"time"

	"github.com/whento/whento/internal/ics/models"
	"github.com/whento/whento/internal/ics/repository"
)

// TimeSlot represents a time segment during which a set of participants are available
type TimeSlot struct {
	StartTime    string // HH:MM format
	EndTime      string // HH:MM format
	Participants []models.ParticipantAvailability
}

// parseTimeToMinutes converts a time string "HH:MM" to minutes since midnight
func parseTimeToMinutes(timeStr string) int {
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return 0
	}
	return t.Hour()*60 + t.Minute()
}

// minutesToTimeString converts minutes since midnight to "HH:MM" format
func minutesToTimeString(minutes int) string {
	h := minutes / 60
	m := minutes % 60
	t := time.Date(0, 1, 1, h, m, 0, 0, time.UTC)
	return t.Format("15:04")
}

// normalizeParticipantTime normalizes participant times, treating nil as full day
func normalizeParticipantTime(p *models.ParticipantAvailability) (start, end int) {
	startStr := "00:00"
	endStr := "23:59"

	if p.StartTime != nil {
		startStr = *p.StartTime
	}
	if p.EndTime != nil {
		endStr = *p.EndTime
	}

	return parseTimeToMinutes(startStr), parseTimeToMinutes(endStr)
}

// isParticipantAvailableAt checks if a participant is available during a specific time range
func isParticipantAvailableAt(p *models.ParticipantAvailability, slotStart, slotEnd int) bool {
	pStart, pEnd := normalizeParticipantTime(p)
	// Participant is available if their range completely covers the slot
	return pStart <= slotStart && pEnd >= slotEnd
}

// computeTimeSlots analyzes participant availabilities and computes time slots
// where the threshold is met, merging consecutive slots into continuous events
func computeTimeSlots(availabilities []repository.DateAvailability, threshold int) []TimeSlot {
	if len(availabilities) == 0 {
		return nil
	}

	// Convert availabilities to participant model
	participants := make([]models.ParticipantAvailability, len(availabilities))
	for i, av := range availabilities {
		participants[i] = models.ParticipantAvailability{
			Name:      av.ParticipantName,
			StartTime: av.StartTime,
			EndTime:   av.EndTime,
			Note:      av.Note,
		}
	}

	// Collect all unique time boundaries
	boundarySet := make(map[int]bool)
	boundarySet[0] = true    // Start of day
	boundarySet[1439] = true // End of day (23:59)

	for _, p := range participants {
		start, end := normalizeParticipantTime(&p)
		boundarySet[start] = true
		boundarySet[end] = true
	}

	// Sort boundaries
	boundaries := make([]int, 0, len(boundarySet))
	for b := range boundarySet {
		boundaries = append(boundaries, b)
	}
	sort.Ints(boundaries)

	// Analyze each segment between consecutive boundaries
	type segment struct {
		start        int
		end          int
		count        int
		participants []models.ParticipantAvailability
	}

	var segments []segment

	for i := 0; i < len(boundaries)-1; i++ {
		segStart := boundaries[i]
		segEnd := boundaries[i+1]

		// Count participants available for this entire segment
		var availableParticipants []models.ParticipantAvailability
		for j := range participants {
			if isParticipantAvailableAt(&participants[j], segStart, segEnd) {
				availableParticipants = append(availableParticipants, participants[j])
			}
		}

		segments = append(segments, segment{
			start:        segStart,
			end:          segEnd,
			count:        len(availableParticipants),
			participants: availableParticipants,
		})
	}

	// Merge consecutive segments where threshold is met
	var result []TimeSlot
	var currentSlot *TimeSlot

	for _, seg := range segments {
		if seg.count >= threshold {
			if currentSlot == nil {
				// Start new slot
				currentSlot = &TimeSlot{
					StartTime:    minutesToTimeString(seg.start),
					EndTime:      minutesToTimeString(seg.end),
					Participants: seg.participants,
				}
			} else {
				// Extend current slot
				currentSlot.EndTime = minutesToTimeString(seg.end)
				// Update participants to include all who are available for ANY part of this merged slot
				// We keep track of unique participants
				participantMap := make(map[string]models.ParticipantAvailability)
				for _, p := range currentSlot.Participants {
					participantMap[p.Name] = p
				}
				for _, p := range seg.participants {
					participantMap[p.Name] = p
				}
				currentSlot.Participants = make([]models.ParticipantAvailability, 0, len(participantMap))
				for _, p := range participantMap {
					currentSlot.Participants = append(currentSlot.Participants, p)
				}
			}
		} else {
			// Threshold not met - close current slot if exists
			if currentSlot != nil {
				result = append(result, *currentSlot)
				currentSlot = nil
			}
		}
	}

	// Don't forget the last slot
	if currentSlot != nil {
		result = append(result, *currentSlot)
	}

	return result
}

// isAllDaySlot checks if a time slot covers the entire day
func isAllDaySlot(slot *TimeSlot) bool {
	return slot.StartTime == "00:00" && slot.EndTime == "23:59"
}
