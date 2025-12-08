// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	client *redis.Client
}

// NewRateLimiter creates a new rate limiter
// If client is nil, rate limiting is disabled (allows all requests)
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// RateLimitConfig holds rate limit configuration
type RateLimitConfig struct {
	Requests int                          // Number of requests allowed
	Window   time.Duration                // Time window
	KeyFunc  func(r *http.Request) string // Function to extract rate limit key
}

// Limit creates a rate limiting middleware
// If Redis client is nil, this middleware does nothing (allows all requests)
func (rl *RateLimiter) Limit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If Redis is not available, skip rate limiting
			if rl.client == nil {
				next.ServeHTTP(w, r)
				return
			}

			key := cfg.KeyFunc(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed, remaining, resetAt, err := rl.check(r.Context(), key, cfg.Requests, cfg.Window)
			if err != nil {
				// On error, allow the request but log it
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.Requests))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt))

			if !allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", resetAt-time.Now().Unix()))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) check(ctx context.Context, key string, limit int, window time.Duration) (bool, int, int64, error) {
	redisKey := fmt.Sprintf("ratelimit:%s", key)
	now := time.Now()
	windowStart := now.Add(-window).UnixMicro()

	pipe := rl.client.Pipeline()

	// Remove old entries
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart))

	// Add current request
	pipe.ZAdd(ctx, redisKey, redis.Z{
		Score:  float64(now.UnixMicro()),
		Member: now.UnixMicro(),
	})

	// Count requests in window
	pipe.ZCard(ctx, redisKey)

	// Set expiry
	pipe.Expire(ctx, redisKey, window)

	results, err := pipe.Exec(ctx)
	if err != nil {
		return true, limit, now.Add(window).Unix(), err
	}

	count := results[2].(*redis.IntCmd).Val()
	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	resetAt := now.Add(window).Unix()

	return count <= int64(limit), remaining, resetAt, nil
}

// IPKeyFunc returns client IP as rate limit key
func IPKeyFunc(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// UserKeyFunc returns user ID as rate limit key (requires Auth middleware)
func UserKeyFunc(r *http.Request) string {
	return GetUserID(r.Context())
}

// CombinedKeyFunc combines path and IP for endpoint-specific limiting
func CombinedKeyFunc(r *http.Request) string {
	return fmt.Sprintf("%s:%s", r.URL.Path, IPKeyFunc(r))
}
