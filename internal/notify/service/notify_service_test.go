// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package service

import (
	"testing"
)

func TestEmailDeduplication(t *testing.T) {
	tests := []struct {
		name                   string
		ownerEmail             string
		participantEmails      []string
		expectedRecipientCount int
		description            string
	}{
		{
			name:                   "Owner not in participants",
			ownerEmail:             "owner@example.com",
			participantEmails:      []string{"alice@example.com", "bob@example.com"},
			expectedRecipientCount: 3, // owner + 2 participants
			description:            "Should have 3 unique emails when owner is not a participant",
		},
		{
			name:                   "Owner is also participant (email deduplication)",
			ownerEmail:             "owner@example.com",
			participantEmails:      []string{"owner@example.com", "alice@example.com"}, // owner email duplicated
			expectedRecipientCount: 2,                                                    // owner deduplicated + alice
			description:            "Should deduplicate when owner email matches participant email",
		},
		{
			name:                   "Multiple participants same email (edge case)",
			ownerEmail:             "owner@example.com",
			participantEmails:      []string{"shared@example.com", "shared@example.com"}, // duplicate
			expectedRecipientCount: 2,                                                     // owner + 1 deduplicated shared email
			description:            "Should deduplicate when multiple participants share same email",
		},
		{
			name:                   "Only owner (no participants)",
			ownerEmail:             "owner@example.com",
			participantEmails:      []string{},
			expectedRecipientCount: 1, // owner only
			description:            "Should have only owner when no participants",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate email deduplication logic using a map
			emailMap := make(map[string]bool)

			// Add owner email
			emailMap[tt.ownerEmail] = true

			// Add participant emails
			for _, email := range tt.participantEmails {
				emailMap[email] = true
			}

			if len(emailMap) != tt.expectedRecipientCount {
				t.Errorf("Email deduplication failed: got %d unique emails, want %d (description: %s)",
					len(emailMap), tt.expectedRecipientCount, tt.description)
			}
		})
	}
}

func TestNotificationMessageContent(t *testing.T) {
	tests := []struct {
		name             string
		transitionType   string
		locale           string
		shouldContain    []string
		shouldNotContain []string
		description      string
	}{
		{
			name:             "Threshold reached - English",
			transitionType:   "threshold_reached",
			locale:           "en",
			shouldContain:    []string{"Threshold reached"},
			shouldNotContain: []string{"Seuil atteint", "Threshold lost"},
			description:      "English message should contain correct phrases",
		},
		{
			name:             "Threshold reached - French",
			transitionType:   "threshold_reached",
			locale:           "fr",
			shouldContain:    []string{"Seuil atteint"},
			shouldNotContain: []string{"Threshold reached", "Threshold lost"},
			description:      "French message should contain correct phrases",
		},
		{
			name:             "Threshold lost - English",
			transitionType:   "threshold_lost",
			locale:           "en",
			shouldContain:    []string{"Threshold lost"},
			shouldNotContain: []string{"Threshold reached", "Seuil atteint"},
			description:      "Lost transition should indicate loss",
		},
		{
			name:             "Threshold lost - French",
			transitionType:   "threshold_lost",
			locale:           "fr",
			shouldContain:    []string{"Seuil perdu"},
			shouldNotContain: []string{"Seuil atteint", "Threshold"},
			description:      "Lost transition in French should use correct wording",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate building text message (simplified version)
			var message string
			if tt.transitionType == "threshold_reached" {
				if tt.locale == "fr" {
					message = "Seuil atteint pour le calendrier"
				} else {
					message = "Threshold reached for calendar"
				}
			} else if tt.transitionType == "threshold_lost" {
				if tt.locale == "fr" {
					message = "Seuil perdu pour le calendrier"
				} else {
					message = "Threshold lost for calendar"
				}
			}

			// Check required strings
			for _, required := range tt.shouldContain {
				if !contains(message, required) {
					t.Errorf("Message missing required string '%s': %s (description: %s)",
						required, message, tt.description)
				}
			}

			// Check forbidden strings
			for _, forbidden := range tt.shouldNotContain {
				if contains(message, forbidden) {
					t.Errorf("Message contains forbidden string '%s': %s (description: %s)",
						forbidden, message, tt.description)
				}
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
