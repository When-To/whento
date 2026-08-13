// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"github.com/whento/whento/internal/mfa/models"
)

// The MFA lockout counter used to be stored under `mfa_attempts:<user uuid>`, so `KEYS *`
// or a `dump.rdb` named every account that had recently failed a second factor.
//
// Hiding that is one assertion. The one that matters more is the other one: the counter
// has to go on counting. Check, increment and clear each build the key separately as far
// as the compiler is concerned, and if hashing had been applied to two of the three the
// brute-force limit would have gone on returning 401 for ever — no error, no log line,
// and a six-digit second factor reduced to decoration. Both are asserted below.

// TestTheAttemptKeyCarriesNoUserID covers the stored form.
func TestTheAttemptKeyCarriesNoUserID(t *testing.T) {
	for _, tt := range []struct {
		name   string
		userID uuid.UUID
	}{
		{"a random account", uuid.MustParse("6f2c1d3e-4a5b-4c6d-8e9f-0a1b2c3d4e5f")},
		{"another account", uuid.MustParse("11111111-2222-3333-4444-555555555555")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := mfaAttemptsKey(tt.userID)

			if !strings.HasPrefix(key, mfaAttemptsPrefix) {
				t.Errorf("key %q lost the %q prefix an operator diagnoses it by", key, mfaAttemptsPrefix)
			}
			if strings.Contains(key, tt.userID.String()) {
				t.Errorf("key %q still spells out the user id", key)
			}
			if digest := strings.TrimPrefix(key, mfaAttemptsPrefix); digest == "" {
				t.Error("the digest is empty, so every account would share one lockout bucket")
			}
			// Deterministic, because a lockout must survive a restart and must follow the
			// account across the instances sharing one Redis.
			if again := mfaAttemptsKey(tt.userID); again != key {
				t.Errorf("the key is not deterministic: %q then %q", key, again)
			}
		})
	}
}

// TestTwoAccountsGetTwoBuckets: a collision would let one account under attack lock out
// another.
func TestTwoAccountsGetTwoBuckets(t *testing.T) {
	first := mfaAttemptsKey(uuid.MustParse("6f2c1d3e-4a5b-4c6d-8e9f-0a1b2c3d4e5f"))
	second := mfaAttemptsKey(uuid.MustParse("11111111-2222-3333-4444-555555555555"))

	if first == second {
		t.Errorf("two accounts share the lockout key %q", first)
	}
}

// TestTheLockoutStillCountsAndClearsUnderTheHashedKey is the functional half: it drives
// the handler and watches the counter rise, reset on a correct code, and rise again to a
// lockout. That is check, increment and clear all agreeing on one key — which is the
// thing a hashing mistake would silently break.
func TestTheLockoutStillCountsAndClearsUnderTheHashedKey(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"

	user := testUser()
	appCache := newCountingCache(true)
	h := newHarness(t,
		&fakeMFAStore{record: &models.UserMFA{UserID: user.ID, Secret: secret, Enabled: true}},
		&fakeUserLookup{user: user}, appCache)

	key := mfaAttemptsKey(user.ID)

	verify := func(code string) int {
		// A fresh temp token each time: the auth service consumes the jti on the
		// successful call, and a replayed one would be refused for the wrong reason.
		token := tempToken(t, h.manager, map[string]interface{}{
			"user_id": user.ID.String(), "mfa_pending": true,
			"exp": time.Now().Add(5 * time.Minute).Unix(),
		})
		rec := httptest.NewRecorder()
		h.handler.VerifyLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify",
			strings.NewReader(`{"temp_token":"`+token+`","code":"`+code+`"}`)))

		return rec.Code
	}

	// Three wrong codes: the counter exists, and under the digest rather than the id.
	for attempt := 1; attempt <= 3; attempt++ {
		if got := verify("000000"); got != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d, want 401", attempt, got)
		}
	}
	if got := appCache.values[key]; got != 3 {
		t.Fatalf("the counter under the hashed key is %d after three failures, want 3", got)
	}

	// A correct code clears it. The login itself may or may not complete here — what is
	// under test is that clearMFAAttempts reached the same bucket the increments did.
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	verify(code)

	if _, still := appCache.values[key]; still {
		t.Error("a correct code left the failure counter standing")
	}
	if !slices.Contains(appCache.deleted, key) {
		t.Errorf("the counter was cleared under %v, none of which is the key the increments used", appCache.deleted)
	}

	// And the limit still engages afterwards, from zero: five more wrong codes are each
	// merely wrong, the sixth is refused before the code is checked.
	for attempt := 1; attempt <= 5; attempt++ {
		if got := verify("000000"); got != http.StatusUnauthorized {
			t.Fatalf("post-reset attempt %d returned %d, want 401", attempt, got)
		}
	}
	if got := verify("000000"); got != http.StatusTooManyRequests {
		t.Errorf("the sixth post-reset attempt returned %d, want 429 — the limit no longer engages", got)
	}

	// Nothing the handler wrote or deleted names the account.
	for key := range appCache.values {
		if strings.Contains(key, user.ID.String()) {
			t.Errorf("cache key %q carries the user id", key)
		}
	}
	for _, key := range appCache.deleted {
		if strings.Contains(key, user.ID.String()) {
			t.Errorf("deleted cache key %q carries the user id", key)
		}
	}
}
