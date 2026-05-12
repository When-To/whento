// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
		b.Allow(context.Background(), "k1", 2, time.Minute)
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
		b.Allow(context.Background(), "refill", limit, window)
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

	b.Allow(context.Background(), "fresh", 5, time.Minute)
	b.Allow(context.Background(), "stale", 5, time.Minute)

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
// without a real Redis instance.
type fakePrimary struct {
	err     error
	calls   int
	allowed bool
}

func (f *fakePrimary) Allow(_ context.Context, _ string, limit int, _ time.Duration) (bool, int, int64, error) {
	f.calls++
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
	rl.check(context.Background(), "k", 5, time.Minute)
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
