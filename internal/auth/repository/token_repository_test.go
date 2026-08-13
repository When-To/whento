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
