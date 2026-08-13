// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/whento/whento/internal/auth/models"
)

// The failed-login counter lives in Redis for fifteen minutes. Keying it by the
// address turned `KEYS login_attempts:*` into a list of the people who recently
// mistyped their password here — readable by anyone with a Redis console, and
// written to disk by the default RDB save points.

// TestLoginAttemptsKeyNeverHoldsTheAddress drives the real Login path and looks
// at what the cache was actually asked to store.
func TestLoginAttemptsKeyNeverHoldsTheAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"an ordinary address", "ada@example.test"},
		{"an address with a plus tag", "ada+calendar@example.test"},
		{"an address for an account that does not exist", "nobody@example.test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFixture(t, nil)
			fixture.withUser(t, "ada@example.test", "Str0ng!Passw0rd", models.RoleUser)

			_, _ = fixture.service.Login(context.Background(), &models.LoginRequest{
				Email: tt.address, Password: "wrong",
			})

			keys := make([]string, 0, len(fixture.cache.values))
			for key := range fixture.cache.values {
				keys = append(keys, key)
			}
			if len(keys) == 0 {
				t.Fatal("nothing was written to the cache; the lockout counter no longer runs")
			}

			for _, key := range keys {
				if strings.Contains(key, tt.address) {
					t.Errorf("the cache key %q holds the address in clear", key)
				}
				// The local part on its own is enough to identify someone.
				if local, _, found := strings.Cut(tt.address, "@"); found && strings.Contains(key, local) {
					t.Errorf("the cache key %q holds the local part %q", key, local)
				}
				if !strings.HasPrefix(key, loginAttemptsPrefix) {
					t.Errorf("the cache key %q lost its prefix; operators can no longer tell what it is", key)
				}
			}
		})
	}
}

// TestLoginAttemptsKeyStillSeparatesAccounts: hashing must not merge two
// accounts into one bucket, which would let one person's failures lock out
// another's.
func TestLoginAttemptsKeyStillSeparatesAccounts(t *testing.T) {
	fixture := newFixture(t, nil)
	fixture.withUser(t, "ada@example.test", "Str0ng!Passw0rd", models.RoleUser)

	for _, address := range []string{"ada@example.test", "grace@example.test"} {
		_, _ = fixture.service.Login(context.Background(), &models.LoginRequest{
			Email: address, Password: "wrong",
		})
	}

	if got := len(fixture.cache.values); got != 2 {
		t.Errorf("two accounts produced %d counter(s), want 2: %v", got, fixture.cache.values)
	}
}
