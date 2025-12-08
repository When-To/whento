// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Default TTLs for different types of data
const (
	TTLCalendar     = 5 * time.Minute // Calendars change infrequently
	TTLParticipant  = 5 * time.Minute // Participants change infrequently
	TTLAvailability = 2 * time.Minute // Availabilities change more frequently
	TTLICSFeed      = 1 * time.Minute // ICS feeds should be fresh
	TTLDateSummary  = 2 * time.Minute // Date summaries change when availabilities change
	TTLRangeSummary = 2 * time.Minute // Range summaries change when availabilities change
)

// GetOrSet attempts to get a value from cache, and if not found, calls the provided
// function to fetch it and stores it in cache before returning
func GetOrSet(ctx context.Context, c Cache, key string, dest interface{}, ttl time.Duration, fetchFn func() (interface{}, error)) error {
	// Try to get from cache first
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil // Cache hit
	}

	// If it's not a cache miss error, return the error
	if err != redis.Nil {
		// Log the error but continue to fetch from source
	}

	// Cache miss or error - fetch from source
	value, err := fetchFn()
	if err != nil {
		return err
	}

	// Store in cache (ignore cache errors)
	_ = c.Set(ctx, key, value, ttl)

	// Copy the fetched value to dest
	// Since we're using json marshaling in cache, we need to handle the type assertion
	if v, ok := value.(interface{}); ok {
		// Use type switch or reflection if needed
		// For now, we'll assume the fetchFn returns the correct type
		// This is a simplified version - in production you might want more robust type handling
		switch destPtr := dest.(type) {
		case *interface{}:
			*destPtr = v
		}
	}

	return nil
}

// InvalidatePattern deletes all keys matching a pattern (only works with RedisCache)
// This is useful for invalidating multiple related cache entries
func InvalidatePattern(ctx context.Context, c Cache, pattern string) error {
	// Only RedisCache supports pattern-based deletion
	if rc, ok := c.(*RedisCache); ok {
		iter := rc.client.Scan(ctx, 0, pattern, 0).Iterator()
		keys := []string{}

		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}

		if err := iter.Err(); err != nil {
			return err
		}

		if len(keys) > 0 {
			return rc.client.Del(ctx, keys...).Err()
		}
	}

	return nil
}

// GetWithFallback tries to get a value from cache, and if not found or cache is disabled,
// calls the fallback function
func GetWithFallback[T any](ctx context.Context, c Cache, key string, ttl time.Duration, fallbackFn func() (T, error)) (T, error) {
	var result T

	// If cache is disabled, skip directly to fallback
	if !c.IsEnabled() {
		return fallbackFn()
	}

	// Try to get from cache
	err := c.Get(ctx, key, &result)
	if err == nil {
		return result, nil // Cache hit
	}

	// If it's not a cache miss, log but continue
	if err != redis.Nil {
		// Could log here
	}

	// Cache miss - fetch from source
	result, err = fallbackFn()
	if err != nil {
		return result, err
	}

	// Store in cache (ignore errors)
	_ = c.Set(ctx, key, result, ttl)

	return result, nil
}
