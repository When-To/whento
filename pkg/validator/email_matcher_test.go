// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package validator

import "testing"

func TestEmailMatches(t *testing.T) {
	tests := []struct {
		name            string
		email           string
		allowedPatterns []string
		expectedMatch   bool
	}{
		// Wildcard "*" - matches all
		{
			name:            "wildcard * matches any email",
			email:           "user@example.com",
			allowedPatterns: []string{"*"},
			expectedMatch:   true,
		},
		{
			name:            "wildcard * matches another email",
			email:           "admin@company.org",
			allowedPatterns: []string{"*"},
			expectedMatch:   true,
		},

		// Domain wildcard *@domain.com
		{
			name:            "domain wildcard matches email from domain",
			email:           "user@example.com",
			allowedPatterns: []string{"*@example.com"},
			expectedMatch:   true,
		},
		{
			name:            "domain wildcard matches another user from domain",
			email:           "admin@example.com",
			allowedPatterns: []string{"*@example.com"},
			expectedMatch:   true,
		},
		{
			name:            "domain wildcard does not match different domain",
			email:           "user@otherdomain.com",
			allowedPatterns: []string{"*@example.com"},
			expectedMatch:   false,
		},

		// Local wildcard user@*
		{
			name:            "local wildcard matches user from any domain",
			email:           "admin@example.com",
			allowedPatterns: []string{"admin@*"},
			expectedMatch:   true,
		},
		{
			name:            "local wildcard matches user from another domain",
			email:           "admin@company.org",
			allowedPatterns: []string{"admin@*"},
			expectedMatch:   true,
		},
		{
			name:            "local wildcard does not match different user",
			email:           "user@example.com",
			allowedPatterns: []string{"admin@*"},
			expectedMatch:   false,
		},

		// Exact match
		{
			name:            "exact match",
			email:           "admin@example.com",
			allowedPatterns: []string{"admin@example.com"},
			expectedMatch:   true,
		},
		{
			name:            "exact match does not match different email",
			email:           "user@example.com",
			allowedPatterns: []string{"admin@example.com"},
			expectedMatch:   false,
		},

		// Multiple patterns
		{
			name:            "multiple patterns - matches first",
			email:           "admin@example.com",
			allowedPatterns: []string{"admin@example.com", "*@company.org"},
			expectedMatch:   true,
		},
		{
			name:            "multiple patterns - matches second",
			email:           "user@company.org",
			allowedPatterns: []string{"admin@example.com", "*@company.org"},
			expectedMatch:   true,
		},
		{
			name:            "multiple patterns - no match",
			email:           "user@otherdomain.com",
			allowedPatterns: []string{"admin@example.com", "*@company.org"},
			expectedMatch:   false,
		},

		// Edge cases
		{
			name:            "empty pattern list does not match",
			email:           "user@example.com",
			allowedPatterns: []string{},
			expectedMatch:   false,
		},
		{
			name:            "case insensitive matching",
			email:           "User@Example.COM",
			allowedPatterns: []string{"user@example.com"},
			expectedMatch:   true,
		},
		{
			name:            "whitespace trimming",
			email:           " user@example.com ",
			allowedPatterns: []string{" user@example.com "},
			expectedMatch:   true,
		},
		{
			name:            "invalid email format does not match",
			email:           "invalid-email",
			allowedPatterns: []string{"*@example.com"},
			expectedMatch:   false,
		},
		{
			name:            "empty email does not match",
			email:           "",
			allowedPatterns: []string{"*"},
			expectedMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EmailMatches(tt.email, tt.allowedPatterns)
			if result != tt.expectedMatch {
				t.Errorf("EmailMatches(%q, %v) = %v, want %v",
					tt.email, tt.allowedPatterns, result, tt.expectedMatch)
			}
		})
	}
}
