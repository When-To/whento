// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMemoryBackend_AllowsUpToLimit(t *testing.T) {
	b := newMemoryBackend()
	defer b.stop()

	const limit = 3
	for i := 0; i < limit; i++ {
		allowed, _, _, err := b.Allow(context.Background(), "k1", limit, time.Minute)
		if err != nil {
			t.Fatalf("unexpected error on req %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("req %d should be allowed", i+1)
		}
	}

	allowed, _, _, _ := b.Allow(context.Background(), "k1", limit, time.Minute)
	if allowed {
		t.Fatalf("req %d should be denied (over limit)", limit+1)
	}
}

func TestMemoryBackend_KeysAreIsolated(t *testing.T) {
	b := newMemoryBackend()
	defer b.stop()

	for i := 0; i < 2; i++ {
		_, _, _, _ = b.Allow(context.Background(), "k1", 2, time.Minute)
	}
	allowed, _, _, _ := b.Allow(context.Background(), "k1", 2, time.Minute)
	if allowed {
		t.Fatalf("k1 should be denied")
	}

	allowed, _, _, _ = b.Allow(context.Background(), "k2", 2, time.Minute)
	if !allowed {
		t.Fatalf("k2 should be allowed (different key)")
	}
}

func TestMemoryBackend_Refills(t *testing.T) {
	b := newMemoryBackend()
	defer b.stop()

	const limit = 2
	window := 100 * time.Millisecond

	for i := 0; i < limit; i++ {
		_, _, _, _ = b.Allow(context.Background(), "refill", limit, window)
	}
	allowed, _, _, _ := b.Allow(context.Background(), "refill", limit, window)
	if allowed {
		t.Fatalf("should be denied right after exhausting limit")
	}

	time.Sleep(window)
	allowed, _, _, _ = b.Allow(context.Background(), "refill", limit, window)
	if !allowed {
		t.Fatalf("should be allowed after refill")
	}
}

func TestMemoryBackend_Sweep(t *testing.T) {
	b := newMemoryBackend()
	defer b.stop()

	_, _, _, _ = b.Allow(context.Background(), "fresh", 5, time.Minute)
	_, _, _, _ = b.Allow(context.Background(), "stale", 5, time.Minute)

	b.mu.Lock()
	b.entries["stale"].lastUsed = time.Now().Add(-2 * evictAfter)
	b.mu.Unlock()

	b.sweep(time.Now())

	b.mu.Lock()
	_, hasFresh := b.entries["fresh"]
	_, hasStale := b.entries["stale"]
	b.mu.Unlock()

	if !hasFresh {
		t.Fatalf("fresh entry should not be swept")
	}
	if hasStale {
		t.Fatalf("stale entry should be swept")
	}
}

// fakePrimary stands in for redisBackend so we can exercise failure paths
// without a real Redis instance. It records the keys it was handed, which is
// what the privacy tests below inspect: those keys are exactly what a real
// Redis would have persisted.
type fakePrimary struct {
	err     error
	calls   int
	allowed bool
	keys    []string
}

func (f *fakePrimary) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, int, int64, error) {
	f.calls++
	f.keys = append(f.keys, key)
	if f.err != nil {
		return false, 0, 0, f.err
	}
	return f.allowed, limit, time.Now().Add(time.Minute).Unix(), nil
}

func TestRateLimiter_FallsBackOnPrimaryError(t *testing.T) {
	rl := NewRateLimiter(nil)
	defer rl.Stop()
	rl.primary = &fakePrimary{err: errors.New("connection refused")}

	// First two requests: memory backend allows under the limit of 2.
	for i := 0; i < 2; i++ {
		allowed, _, _, _, err := rl.check(context.Background(), "1.2.3.4", 2, time.Minute)
		if err != nil {
			t.Fatalf("req %d: unexpected error: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("req %d: memory backend should allow", i+1)
		}
	}

	// Third request: memory limit reached -> denied (429), never 503.
	allowed, _, _, _, err := rl.check(context.Background(), "1.2.3.4", 2, time.Minute)
	if err != nil {
		t.Fatalf("memory backend should never error, got %v", err)
	}
	if allowed {
		t.Fatalf("third request should be denied by memory limiter")
	}
}

// slowPrimary blocks until ctx is cancelled, simulating a hung Redis client
// that go-redis would otherwise retry against for ~19s by default.
type slowPrimary struct {
	calls int
}

func (s *slowPrimary) Allow(ctx context.Context, _ string, _ int, _ time.Duration) (bool, int, int64, error) {
	s.calls++
	<-ctx.Done()
	return false, 0, 0, ctx.Err()
}

func TestRateLimiter_TimeoutCapsRedisCallDuration(t *testing.T) {
	rl := NewRateLimiter(nil)
	defer rl.Stop()
	rl.redisTimeout = 20 * time.Millisecond

	primary := &slowPrimary{}
	rl.primary = primary

	start := time.Now()
	_, _, _, _, _ = rl.check(context.Background(), "k", 5, time.Minute)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Fatalf("check() took %v, expected <200ms (timeout=%v)", elapsed, rl.redisTimeout)
	}
	if primary.calls != 1 {
		t.Fatalf("primary should be tried once, got %d", primary.calls)
	}
}

func TestRateLimiter_Middleware_NoRedis_Allows200Then429(t *testing.T) {
	rl := NewRateLimiter(nil)
	defer rl.Stop()

	handler := rl.Limit(RateLimitConfig{
		Requests: 2,
		Window:   time.Minute,
		KeyFunc:  func(r *http.Request) string { return "ip" },
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("req %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("must never return 503")
	}
}

func TestRateLimiter_Middleware_PrimaryError_NoFiveHundredThree(t *testing.T) {
	rl := NewRateLimiter(nil)
	defer rl.Stop()
	rl.primary = &fakePrimary{err: errors.New("redis: connection refused")}

	handler := rl.Limit(RateLimitConfig{
		Requests: 5,
		Window:   time.Minute,
		KeyFunc:  func(r *http.Request) string { return "ip" },
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("must never return 503, even on primary error")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 via memory fallback, got %d", rec.Code)
	}
}

func TestRateLimiter_Middleware_BackendHeader(t *testing.T) {
	tests := []struct {
		name    string
		primary rateLimitBackend
		want    string
	}{
		{
			name:    "no Redis client",
			primary: nil,
			want:    backendMemory,
		},
		{
			name:    "healthy Redis",
			primary: &fakePrimary{allowed: true},
			want:    backendRedis,
		},
		{
			name:    "Redis errors -> memory fallback",
			primary: &fakePrimary{err: errors.New("connection refused")},
			want:    backendMemory,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rl := NewRateLimiter(nil)
			defer rl.Stop()
			rl.primary = tc.primary

			handler := rl.Limit(RateLimitConfig{
				Requests: 5,
				Window:   time.Minute,
				KeyFunc:  func(r *http.Request) string { return "ip" },
			})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			handler.ServeHTTP(rec, req)

			if got := rec.Header().Get("X-RateLimit-Backend"); got != tc.want {
				t.Fatalf("X-RateLimit-Backend = %q, want %q", got, tc.want)
			}
		})
	}
}

// The rate limiter is the one component that sees every request from every visitor, so
// whatever it writes down is a record of who came and when. The owner's constraint is
// that nothing personal is stored: not an IP, not a user id, and above all not a
// participant link, which in this API is the credential itself.

// withKeySalt pins the key salt for one test and restores it afterwards. The salt is
// process-wide, so tests touching it cannot run in parallel.
func withKeySalt(t *testing.T, secret string) {
	t.Helper()

	previous := rateLimitKeySalt
	t.Cleanup(func() { rateLimitKeySalt = previous })

	SetRateLimitKeySalt(secret)
}

func TestRateLimiterStoresNothingIdentifying(t *testing.T) {
	const (
		calendarToken = "Xk3f9QvR2mNbT7wLpZ4sYd8H"
		participantID = "6f9619ff-8b86-d011-b42d-00c04fc964ff"
		clientIP      = "203.0.113.7"
	)

	tests := []struct {
		name    string
		key     string
		secrets []string
	}{
		{
			name:    "an IP address",
			key:     clientIP,
			secrets: []string{clientIP},
		},
		{
			name:    "a user id",
			key:     "018f3b6c-7c9a-7d2e-9b17-1c2d3e4f5a6b",
			secrets: []string{"018f3b6c-7c9a-7d2e-9b17-1c2d3e4f5a6b"},
		},
		{
			// CombinedKeyFunc produces exactly this: the participant link, whose token
			// and UUID together are the whole authorisation, joined to the caller's IP.
			name:    "a participant link joined to an IP",
			key:     "/api/v1/availabilities/calendar/" + calendarToken + "/participant/" + participantID + ":" + clientIP,
			secrets: []string{calendarToken, participantID, clientIP},
		},
		{
			name:    "a magic link",
			key:     "/api/v1/magic-link/verify/a94a8fe5ccb19ba61c4c0873d391e987:" + clientIP,
			secrets: []string{"a94a8fe5ccb19ba61c4c0873d391e987", clientIP},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(nil)
			defer rl.Stop()
			primary := &fakePrimary{allowed: true}
			rl.primary = primary

			handler := rl.Limit(RateLimitConfig{
				Requests: 5,
				Window:   time.Minute,
				KeyFunc:  func(*http.Request) string { return tt.key },
			})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

			if len(primary.keys) != 1 {
				t.Fatalf("the backend saw %d keys, want 1", len(primary.keys))
			}
			stored := primary.keys[0]
			for _, secret := range tt.secrets {
				if strings.Contains(stored, secret) {
					t.Errorf("the stored key %q contains %q", stored, secret)
				}
			}
			// A digest, not a truncation or an encoding: reversing it must need the salt.
			if len(stored) != 32 {
				t.Errorf("stored key = %q (%d chars), want a 32-char digest", stored, len(stored))
			}
			if _, err := hex.DecodeString(stored); err != nil {
				t.Errorf("stored key = %q, want hex: %v", stored, err)
			}
		})
	}
}

// TestMemoryFallbackStoresNothingIdentifying closes the other half of the hole: when
// Redis is down the keys live in this process's map instead, where a heap dump or a
// debugger reads them just as easily.
func TestMemoryFallbackStoresNothingIdentifying(t *testing.T) {
	const clientIP = "203.0.113.7"

	rl := NewRateLimiter(nil)
	defer rl.Stop()

	handler := rl.Limit(RateLimitConfig{
		Requests: 5,
		Window:   time.Minute,
		KeyFunc:  func(*http.Request) string { return clientIP },
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	rl.memory.mu.Lock()
	defer rl.memory.mu.Unlock()
	if len(rl.memory.entries) != 1 {
		t.Fatalf("the memory backend holds %d entries, want 1", len(rl.memory.entries))
	}
	for key := range rl.memory.entries {
		if strings.Contains(key, clientIP) {
			t.Errorf("the in-memory key %q contains the caller's IP", key)
		}
	}
}

func TestHashRateLimitKey(t *testing.T) {
	withKeySalt(t, "a fixed salt for this test")

	tests := []struct {
		name string
		a    string
		b    string
		same bool
	}{
		// Determinism is what makes the limit work at all: the same caller has to land
		// in the same bucket on every request.
		{name: "the same key twice", a: "203.0.113.7", b: "203.0.113.7", same: true},
		{name: "two IPs", a: "203.0.113.7", b: "203.0.113.8"},
		{name: "two endpoints for one IP", a: "/login:203.0.113.7", b: "/register:203.0.113.7"},
		{name: "two calendars", a: "/calendar/aaa:203.0.113.7", b: "/calendar/bbb:203.0.113.7"},
		// Neighbouring keys must not collapse into one bucket, or one visitor's traffic
		// would spend another's budget.
		{name: "a prefix of another key", a: "203.0.113.7", b: "203.0.113.70"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hashRateLimitKey(tt.a) == hashRateLimitKey(tt.b); got != tt.same {
				t.Errorf("hash(%q) == hash(%q) is %v, want %v", tt.a, tt.b, got, tt.same)
			}
		})
	}
}

// TestHashRateLimitKeyDependsOnTheSalt is what stops an attacker with a copy of the
// Redis keyspace from recovering the inputs: the IPv4 space is small enough to hash
// exhaustively, so an unsalted digest of an IP is the IP.
func TestHashRateLimitKeyDependsOnTheSalt(t *testing.T) {
	withKeySalt(t, "salt one")
	first := hashRateLimitKey("203.0.113.7")

	withKeySalt(t, "salt two")
	second := hashRateLimitKey("203.0.113.7")

	if first == second {
		t.Error("the digest does not depend on the salt, so it can be brute-forced offline")
	}

	// A default salt is random per process, so it must not be all zeroes either.
	if bytes.Equal(newRateLimitKeySalt(), make([]byte, 32)) {
		t.Error("the generated salt is all zeroes")
	}
	if bytes.Equal(newRateLimitKeySalt(), newRateLimitKeySalt()) {
		t.Error("two generated salts are identical, so they are not random")
	}
}

// TestSetRateLimitKeySaltIgnoresAnEmptySecret keeps a missing configuration value from
// silently replacing the random salt with a known constant.
func TestSetRateLimitKeySaltIgnoresAnEmptySecret(t *testing.T) {
	withKeySalt(t, "the configured salt")
	before := hashRateLimitKey("203.0.113.7")

	SetRateLimitKeySalt("")

	if hashRateLimitKey("203.0.113.7") != before {
		t.Error("an empty secret changed the salt")
	}
}

// TestRateLimitedResponseUsesTheEnvelope pins the 429 to the same {data, error} shape as
// every other response, so a client can read the reason instead of a bare text/plain body.
func TestRateLimitedResponseUsesTheEnvelope(t *testing.T) {
	rl := NewRateLimiter(nil)
	defer rl.Stop()

	handler := rl.Limit(RateLimitConfig{
		Requests: 1,
		Window:   time.Minute,
		KeyFunc:  func(*http.Request) string { return "ip" },
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	// The headers are what a well-behaved client backs off on; hashing the key must not
	// have disturbed them.
	for _, header := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After"} {
		if rec.Header().Get(header) == "" {
			t.Errorf("%s is missing from the 429", header)
		}
	}

	var body struct {
		Success bool `json:"success"`
		Error   *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the body is not JSON: %v (%q)", err, rec.Body.String())
	}
	if body.Success || body.Error == nil || body.Error.Code != "RATE_LIMITED" {
		t.Errorf("body = %q, want the standard envelope with RATE_LIMITED", rec.Body.String())
	}
}
