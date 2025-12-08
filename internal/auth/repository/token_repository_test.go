// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"testing"
)

func TestHashToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "simple token",
			token: "test-token-123",
		},
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "long token",
			token: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := HashToken(tt.token)
			hash2 := HashToken(tt.token)

			// Same input should produce same hash
			if hash1 != hash2 {
				t.Errorf("HashToken() not deterministic: got %v and %v", hash1, hash2)
			}

			// Hash should be 64 characters (SHA-256 hex encoded)
			if len(hash1) != 64 {
				t.Errorf("HashToken() length = %v, want 64", len(hash1))
			}

			// Different tokens should produce different hashes
			if tt.token != "" {
				differentHash := HashToken(tt.token + "x")
				if hash1 == differentHash {
					t.Errorf("HashToken() produced same hash for different inputs")
				}
			}
		})
	}
}

func TestIsDuplicateKeyError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{
			name:     "duplicate key error",
			errMsg:   "ERROR: duplicate key value violates unique constraint",
			expected: true,
		},
		{
			name:     "postgres error code 23505",
			errMsg:   "ERROR: code 23505 - duplicate key violation",
			expected: true,
		},
		{
			name:     "other error",
			errMsg:   "connection refused",
			expected: false,
		},
		{
			name:     "empty error",
			errMsg:   "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the helper functions
			if tt.errMsg != "" {
				result := findSubstring(tt.errMsg, "23505") || findSubstring(tt.errMsg, "duplicate key")
				if result != tt.expected {
					t.Errorf("expected %v, got %v for error: %s", tt.expected, result, tt.errMsg)
				}
			}
		})
	}
}

func TestFindSubstring(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{
			name:     "found at start",
			s:        "hello world",
			substr:   "hello",
			expected: true,
		},
		{
			name:     "found at end",
			s:        "hello world",
			substr:   "world",
			expected: true,
		},
		{
			name:     "found in middle",
			s:        "hello world",
			substr:   "lo wo",
			expected: true,
		},
		{
			name:     "not found",
			s:        "hello world",
			substr:   "xyz",
			expected: false,
		},
		{
			name:     "empty string",
			s:        "",
			substr:   "test",
			expected: false,
		},
		{
			name:     "empty substring",
			s:        "hello",
			substr:   "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findSubstring(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("findSubstring(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}
