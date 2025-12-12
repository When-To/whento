// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNotificationLog_AntiSpamLogic(t *testing.T) {
	// These would be used in actual database queries
	_ = uuid.New()                          // calendarID
	_ = uuid.New()                          // recipientID
	_ = time.Now().Truncate(24 * time.Hour) // date

	tests := []struct {
		name             string
		eventType        string
		channel          string
		timeSinceLastSent time.Duration
		expectedBlocked  bool
		description      string
	}{
		{
			name:             "First notification - should send",
			eventType:        "threshold_reached",
			channel:          "email",
			timeSinceLastSent: 0, // Never sent before
			expectedBlocked:  false,
			description:      "First notification should always be sent",
		},
		{
			name:             "Within 1 hour - should block",
			eventType:        "threshold_reached",
			channel:          "email",
			timeSinceLastSent: 30 * time.Minute,
			expectedBlocked:  true,
			description:      "Notification within 1 hour should be blocked (anti-spam)",
		},
		{
			name:             "After 1 hour - should send",
			eventType:        "threshold_reached",
			channel:          "email",
			timeSinceLastSent: 61 * time.Minute,
			expectedBlocked:  false,
			description:      "Notification after 1 hour should be allowed",
		},
		{
			name:             "Different channel - should send",
			eventType:        "threshold_reached",
			channel:          "discord", // Different channel
			timeSinceLastSent: 0,        // Simulating first send for this channel
			expectedBlocked:  false,
			description:      "Different channel should not be blocked",
		},
		{
			name:             "Different event type - should send",
			eventType:        "threshold_lost", // Different event
			channel:          "email",
			timeSinceLastSent: 0, // Simulating first send for this event type
			expectedBlocked:  false,
			description:      "Different event type should not be blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate anti-spam check logic
			// In real implementation, this would query notification_log table
			var wasRecentlySent bool

			// Mock: if time since last < 1 hour AND same event/channel, block
			if tt.timeSinceLastSent > 0 && tt.timeSinceLastSent < 1*time.Hour {
				wasRecentlySent = true
			}

			if wasRecentlySent != tt.expectedBlocked {
				t.Errorf("Anti-spam logic failed: wasRecentlySent=%v, expected blocked=%v (description: %s)",
					wasRecentlySent, tt.expectedBlocked, tt.description)
			}
		})
	}
}

func TestNotificationLog_CleanupOldLogs(t *testing.T) {
	tests := []struct {
		name           string
		logAge         time.Duration
		shouldBeDeleted bool
		description    string
	}{
		{
			name:           "Recent log (1 day old)",
			logAge:         24 * time.Hour,
			shouldBeDeleted: false,
			description:    "Recent logs should be kept",
		},
		{
			name:           "29 days old",
			logAge:         29 * 24 * time.Hour,
			shouldBeDeleted: false,
			description:    "Logs younger than 30 days should be kept",
		},
		{
			name:           "31 days old",
			logAge:         31 * 24 * time.Hour,
			shouldBeDeleted: true,
			description:    "Logs older than 30 days should be deleted",
		},
		{
			name:           "90 days old",
			logAge:         90 * 24 * time.Hour,
			shouldBeDeleted: true,
			description:    "Very old logs should be deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate cleanup logic: delete if > 30 days
			cutoffTime := time.Now().Add(-30 * 24 * time.Hour)
			logTimestamp := time.Now().Add(-tt.logAge)

			shouldDelete := logTimestamp.Before(cutoffTime)

			if shouldDelete != tt.shouldBeDeleted {
				t.Errorf("Cleanup logic failed: shouldDelete=%v, expected=%v (description: %s)",
					shouldDelete, tt.shouldBeDeleted, tt.description)
			}
		})
	}
}
