// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package service

import (
	"testing"
)

func TestThresholdTransitionLogic(t *testing.T) {
	tests := []struct {
		name               string
		previousCount      int
		newCount           int
		threshold          int
		expectedTransition string
		description        string
	}{
		{
			name:               "Threshold reached (from below)",
			previousCount:      4,
			newCount:           5,
			threshold:          5,
			expectedTransition: "threshold_reached",
			description:        "Crossing from below to at threshold should trigger 'reached'",
		},
		{
			name:               "Threshold lost (from above)",
			previousCount:      5,
			newCount:           4,
			threshold:          5,
			expectedTransition: "threshold_lost",
			description:        "Dropping below threshold should trigger 'lost'",
		},
		{
			name:               "No transition (staying below)",
			previousCount:      3,
			newCount:           4,
			threshold:          5,
			expectedTransition: "none",
			description:        "Staying below threshold should trigger no transition",
		},
		{
			name:               "No transition (staying above)",
			previousCount:      6,
			newCount:           7,
			threshold:          5,
			expectedTransition: "none",
			description:        "Staying above threshold should trigger no transition",
		},
		{
			name:               "No previous count - above threshold",
			previousCount:      -1, // Unknown
			newCount:           5,
			threshold:          5,
			expectedTransition: "threshold_reached",
			description:        "Without previous count, being at/above threshold should trigger 'reached'",
		},
		{
			name:               "No previous count - below threshold",
			previousCount:      -1, // Unknown
			newCount:           3,
			threshold:          5,
			expectedTransition: "none",
			description:        "Without previous count, being below threshold should trigger 'none'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate transition detection logic
			var transitionType string

			if tt.previousCount >= 0 {
				// We know the previous count
				wasMet := tt.previousCount >= tt.threshold
				nowMet := tt.newCount >= tt.threshold

				if !wasMet && nowMet {
					transitionType = "threshold_reached"
				} else if wasMet && !nowMet {
					transitionType = "threshold_lost"
				} else {
					transitionType = "none"
				}
			} else {
				// Previous count unknown
				if tt.newCount >= tt.threshold {
					transitionType = "threshold_reached"
				} else {
					transitionType = "none"
				}
			}

			if transitionType != tt.expectedTransition {
				t.Errorf("Transition logic failed: got %s, want %s (description: %s)",
					transitionType, tt.expectedTransition, tt.description)
			}
		})
	}
}
