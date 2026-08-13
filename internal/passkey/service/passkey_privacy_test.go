// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	authModels "github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/config"
)

// WebAuthn ceremony state used to be stored under `passkey:registration:<user uuid>` and
// `passkey:authentication:challenge:<id>`, so the Redis key space named every account
// currently enrolling a passkey and every login ceremony in flight.
//
// The dangerous part of hiding that is not the digest, it is the pairing: Begin writes
// the session and Finish reads it, from two different methods and — in a scaled
// deployment — from two different instances. If they stopped agreeing on the key, every
// registration and every passkey login would fail with ErrInvalidChallenge, which looks
// exactly like an expired ceremony and would be blamed on the user's browser.
//
// The ceremonies themselves cannot be completed without a real authenticator, so the
// tests below stop one step short: they assert that Finish gets *past* the cache lookup
// and fails on the credential instead. ErrInvalidCredential means the session was found
// under the key Begin wrote it to; ErrInvalidChallenge means it was not.

// recordingCache is the cache.Cache the service is given: it round-trips values through
// JSON exactly as RedisCache does, so a session that would not survive Redis does not
// survive this either.
type recordingCache struct {
	mu     sync.Mutex
	values map[string][]byte
	keys   []string
}

func newRecordingCache() *recordingCache {
	return &recordingCache{values: map[string][]byte{}}
}

func (c *recordingCache) Get(_ context.Context, key string, dest interface{}) error {
	c.mu.Lock()
	c.keys = append(c.keys, key)
	raw, ok := c.values[key]
	c.mu.Unlock()

	if !ok {
		return errors.New("cache miss")
	}

	return json.Unmarshal(raw, dest)
}

func (c *recordingCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.keys = append(c.keys, key)
	c.values[key] = raw
	c.mu.Unlock()

	return nil
}

func (c *recordingCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keys {
		c.keys = append(c.keys, key)
		delete(c.values, key)
	}

	return nil
}

func (c *recordingCache) Exists(context.Context, string) (bool, error) { return false, nil }
func (c *recordingCache) IsEnabled() bool                              { return true }

func (c *recordingCache) stored() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]string, 0, len(c.values))
	for key := range c.values {
		out = append(out, key)
	}

	return out
}

func (c *recordingCache) touched() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]string(nil), c.keys...)
}

func testWebAuthnUser() *authModels.User {
	user := &authModels.User{Email: "someone@example.test", DisplayName: "Someone"}
	user.ID = uuid.MustParse("7c9e6679-7425-40de-944b-e07fc1f90ae7")

	return user
}

// newCeremonyService builds the real service — real WebAuthn, real key derivation — over
// fake storage.
func newCeremonyService(t *testing.T, appCache *recordingCache, user *authModels.User) *PasskeyService {
	t.Helper()

	cfg := &config.Config{
		WebAuthnRPName:   "WhenTo",
		WebAuthnRPID:     "localhost",
		WebAuthnRPOrigin: "http://localhost:8080",
	}

	svc, err := NewPasskeyService(newFakeStore(), &fakeUserLookup{user: user}, cfg, appCache,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewPasskeyService: %v", err)
	}

	return svc
}

// assertionRequest is a syntactically valid WebAuthn response that no authenticator
// signed. It gets the ceremony past parsing and fails it on the credential, which is the
// step after the cache lookup.
func assertionRequest() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")

	return r
}

// TestTheCeremonyKeysCarryNoIdentifier covers the stored form.
func TestTheCeremonyKeysCarryNoIdentifier(t *testing.T) {
	userID := uuid.MustParse("7c9e6679-7425-40de-944b-e07fc1f90ae7")
	challengeID := uuid.MustParse("3f333df6-90a4-4fda-8dd3-9485d27cee36").String()

	for _, tt := range []struct {
		name     string
		key      string
		prefix   string
		variable string
	}{
		{"a registration", registrationSessionKey(userID), registrationPrefix, userID.String()},
		{"a login challenge", challengeSessionKey(challengeID), challengePrefix, challengeID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.HasPrefix(tt.key, tt.prefix) {
				t.Errorf("key %q lost the %q prefix an operator diagnoses it by", tt.key, tt.prefix)
			}
			if strings.Contains(tt.key, tt.variable) {
				t.Errorf("key %q still spells out %q", tt.key, tt.variable)
			}
			if digest := strings.TrimPrefix(tt.key, tt.prefix); digest == "" {
				t.Error("the digest is empty, so every ceremony would share one entry")
			}
		})
	}

	// Deterministic and distinct: the instance that finishes a ceremony is not
	// necessarily the one that began it, and two users must not share an entry.
	// (The determinism of the digest itself is asserted end to end below, where a
	// session written by one call is read back by another.)
	if registrationSessionKey(userID) == registrationSessionKey(uuid.New()) {
		t.Error("two users share a registration entry")
	}
	if challengeSessionKey(challengeID) == challengeSessionKey(uuid.New().String()) {
		t.Error("two login ceremonies share an entry")
	}
}

// TestARegistrationSessionIsFoundAgain is the functional half for enrolment.
func TestARegistrationSessionIsFoundAgain(t *testing.T) {
	user := testWebAuthnUser()
	appCache := newRecordingCache()
	svc := newCeremonyService(t, appCache, user)

	if _, err := svc.BeginRegistration(context.Background(), user.ID); err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}

	stored := appCache.stored()
	if len(stored) != 1 {
		t.Fatalf("BeginRegistration stored %d entries, want 1", len(stored))
	}
	if stored[0] != registrationSessionKey(user.ID) {
		t.Errorf("stored under %q, want %q", stored[0], registrationSessionKey(user.ID))
	}

	// The session is read back by the other half of the ceremony: reaching
	// ErrInvalidCredential means FinishRegistration got the session and rejected the
	// unsigned attestation, which is the failure a test without an authenticator can
	// legitimately reach. ErrInvalidChallenge would mean the two halves disagree.
	_, err := svc.FinishRegistration(context.Background(), user.ID, assertionRequest())
	if errors.Is(err, ErrInvalidChallenge) {
		t.Fatal("FinishRegistration could not find the session BeginRegistration stored: the two halves derive different keys")
	}
	if !errors.Is(err, ErrInvalidCredential) {
		t.Errorf("FinishRegistration error = %v, want ErrInvalidCredential", err)
	}

	// And a different account's registration is a different entry, so one user cannot
	// finish another's ceremony.
	other := &authModels.User{Email: "other@example.test", DisplayName: "Other"}
	other.ID = uuid.New()
	otherSvc := newCeremonyService(t, appCache, other)
	if _, err := otherSvc.FinishRegistration(context.Background(), other.ID, assertionRequest()); !errors.Is(err, ErrInvalidChallenge) {
		t.Errorf("another account's FinishRegistration error = %v, want ErrInvalidChallenge", err)
	}

	for _, key := range appCache.touched() {
		if strings.Contains(key, user.ID.String()) {
			t.Errorf("cache key %q carries the user id", key)
		}
	}
}

// TestALoginChallengeIsFoundAgain is the functional half for passwordless login, which
// is the path a broken key would lock every passkey user out of.
func TestALoginChallengeIsFoundAgain(t *testing.T) {
	user := testWebAuthnUser()
	appCache := newRecordingCache()
	svc := newCeremonyService(t, appCache, user)

	_, challengeID, err := svc.BeginDiscoverableAuthentication(context.Background())
	if err != nil {
		t.Fatalf("BeginDiscoverableAuthentication: %v", err)
	}
	if challengeID == "" {
		t.Fatal("no challenge id was handed to the browser")
	}

	stored := appCache.stored()
	if len(stored) != 1 {
		t.Fatalf("BeginDiscoverableAuthentication stored %d entries, want 1", len(stored))
	}
	if stored[0] != challengeSessionKey(challengeID) {
		t.Errorf("stored under %q, want %q", stored[0], challengeSessionKey(challengeID))
	}
	if strings.Contains(stored[0], challengeID) {
		t.Errorf("the stored key %q still carries the challenge id", stored[0])
	}

	_, err = svc.FinishAuthentication(context.Background(), challengeID, assertionRequest())
	if errors.Is(err, ErrInvalidChallenge) {
		t.Fatal("FinishAuthentication could not find the challenge BeginDiscoverableAuthentication stored: the two halves derive different keys")
	}
	if !errors.Is(err, ErrInvalidCredential) {
		t.Errorf("FinishAuthentication error = %v, want ErrInvalidCredential", err)
	}

	// An id nobody was given resolves to nothing, so the digest has not collapsed
	// distinct ceremonies into one entry.
	if _, err := svc.FinishAuthentication(context.Background(), uuid.New().String(), assertionRequest()); !errors.Is(err, ErrInvalidChallenge) {
		t.Errorf("an unknown challenge id gave %v, want ErrInvalidChallenge", err)
	}
}
