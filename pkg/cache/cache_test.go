// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisCache(nil) is the self-hosted path: no Redis configured, so every call has to
// be a well-behaved no-op rather than a nil dereference. Several services call the cache
// unconditionally, so this is the configuration most installs actually run.

func TestNewRedisCacheFallsBackToNoOp(t *testing.T) {
	c := NewRedisCache(nil)

	if _, ok := c.(*NoOpCache); !ok {
		t.Fatalf("NewRedisCache(nil) returned %T, want *NoOpCache", c)
	}
	if c.IsEnabled() {
		t.Error("the no-op cache reports itself enabled; callers use this to skip work")
	}
}

func TestNoOpCacheIsAlwaysAMiss(t *testing.T) {
	c := NewRedisCache(nil)
	ctx := context.Background()

	var dest string
	err := c.Get(ctx, "any-key", &dest)

	// redis.Nil specifically: callers compare against it to tell a miss from a real
	// failure.
	if !errors.Is(err, redis.Nil) {
		t.Errorf("Get error = %v, want redis.Nil", err)
	}

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Errorf("Set on a no-op cache returned %v", err)
	}
	if err := c.Delete(ctx, "k", "j"); err != nil {
		t.Errorf("Delete on a no-op cache returned %v", err)
	}
}

func TestKeysAreDistinctAndPrefixed(t *testing.T) {
	// Two different keys colliding would serve one calendar's data for another, so the
	// namespacing matters more than the exact format.
	calendar := CalendarByPublicTokenKey("abc")
	password := UserPasswordChangedKey("abc")

	if calendar == password {
		t.Errorf("both keys are %q", calendar)
	}
	if !strings.HasPrefix(calendar, PrefixCalendar+":") {
		t.Errorf("calendar key %q carries no calendar prefix", calendar)
	}
	if !strings.HasPrefix(password, PrefixAuth+":") {
		t.Errorf("password key %q carries no auth prefix", password)
	}
}
