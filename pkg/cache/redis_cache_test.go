// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisCache marshals to JSON on the way in and unmarshals on the way out, which is
// exactly the part a stub cannot check: a struct that round-trips through a map is not
// evidence that it round-trips through Redis. These skip when REDIS_URL is unset, the
// same rule the database harness follows.
//
// Every key is namespaced with a fresh UUID, so nothing collides with a dev server or
// with another test running at the same time, and each test cleans up after itself.

func redisCache(t *testing.T) (Cache, *redis.Client) {
	t.Helper()

	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL is not set; skipping the Redis-backed tests")
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("REDIS_URL is set but unparseable: %v", err)
	}
	opts.DialTimeout = 2 * time.Second

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("REDIS_URL is set but the server is unreachable: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	return NewRedisCache(client), client
}

// scopedKey keeps one test's keys away from every other user of the same server.
func scopedKey(t *testing.T, client *redis.Client, name string) string {
	t.Helper()

	key := fmt.Sprintf("test:%s:%s", uuid.New(), name)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Del(ctx, key).Err()
	})

	return key
}

func TestRedisCacheIsEnabled(t *testing.T) {
	c, _ := redisCache(t)

	if !c.IsEnabled() {
		t.Error("a Redis-backed cache reports itself disabled")
	}
}

func TestRedisCacheRoundTripsAStruct(t *testing.T) {
	c, client := redisCache(t)
	ctx := context.Background()
	key := scopedKey(t, client, "struct")

	type participant struct {
		Name  string   `json:"name"`
		Count int      `json:"count"`
		Days  []string `json:"days"`
	}

	stored := participant{Name: "Ada", Count: 3, Days: []string{"2026-03-05", "2026-03-06"}}
	if err := c.Set(ctx, key, stored, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got participant
	if err := c.Get(ctx, key, &got); err != nil {
		t.Fatalf("Get: %v", err)
	}

	// The JSON hop is the whole point: a field without a tag, or a type that does not
	// marshal, would be lost here and nowhere else.
	if got.Name != stored.Name || got.Count != stored.Count || len(got.Days) != len(stored.Days) {
		t.Errorf("round trip lost data: %+v, want %+v", got, stored)
	}
}

func TestRedisCacheMissIsRedisNil(t *testing.T) {
	c, client := redisCache(t)
	ctx := context.Background()
	key := scopedKey(t, client, "absent")

	var dest string
	err := c.Get(ctx, key, &dest)

	// GetOrSet and GetWithFallback both compare against redis.Nil to tell a miss from
	// a real failure, so the sentinel has to survive.
	if !errors.Is(err, redis.Nil) {
		t.Errorf("Get on a missing key = %v, want redis.Nil", err)
	}
}

func TestRedisCacheDeleteAndExists(t *testing.T) {
	c, client := redisCache(t)
	ctx := context.Background()
	first := scopedKey(t, client, "one")
	second := scopedKey(t, client, "two")

	for _, key := range []string{first, second} {
		if err := c.Set(ctx, key, "value", time.Minute); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}

	exists, err := c.Exists(ctx, first)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("a key just written does not exist")
	}

	// Delete takes a variadic list, which is how the calendar invalidation bundles are
	// applied in one call.
	if err := c.Delete(ctx, first, second); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	for _, key := range []string{first, second} {
		exists, err := c.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		if exists {
			t.Errorf("%s survived deletion", key)
		}
	}
}

func TestRedisCacheHonoursTheTTL(t *testing.T) {
	c, client := redisCache(t)
	ctx := context.Background()
	key := scopedKey(t, client, "ttl")

	if err := c.Set(ctx, key, "value", 50*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// PTTL, not TTL: the latter answers in whole seconds and rounds a sub-second
	// expiry down to zero, which is indistinguishable from "no expiry set".
	ttl, err := client.PTTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("PTTL: %v", err)
	}
	// A negative value means the key never expires, which would let a stale summary
	// outlive the data it summarises.
	if ttl <= 0 {
		t.Fatalf("PTTL = %v; the entry would never expire", ttl)
	}

	time.Sleep(120 * time.Millisecond)

	var dest string
	if err := c.Get(ctx, key, &dest); !errors.Is(err, redis.Nil) {
		t.Errorf("the entry outlived its TTL: %v", err)
	}
}

func TestRedisCacheDeletingNothingIsFine(t *testing.T) {
	c, client := redisCache(t)

	if err := c.Delete(context.Background(), scopedKey(t, client, "never-written")); err != nil {
		t.Errorf("deleting an absent key returned %v", err)
	}
}

func TestInvalidatePatternSweepsMatchingKeys(t *testing.T) {
	c, client := redisCache(t)
	ctx := context.Background()

	// A shared prefix stands in for the calendar:* pattern used in production, scoped
	// to this test so nothing else is swept.
	prefix := fmt.Sprintf("test:%s", uuid.New())
	matching := []string{prefix + ":a", prefix + ":b"}
	other := prefix + "-untouched"

	t.Cleanup(func() {
		_ = client.Del(context.Background(), append(matching, other)...).Err()
	})

	for _, key := range append(append([]string{}, matching...), other) {
		if err := c.Set(ctx, key, "value", time.Minute); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}

	if err := InvalidatePattern(ctx, c, prefix+":*"); err != nil {
		t.Fatalf("InvalidatePattern: %v", err)
	}

	for _, key := range matching {
		exists, err := c.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists: %v", err)
		}
		if exists {
			t.Errorf("%s matched the pattern but survived", key)
		}
	}

	// The neighbouring key differs only after the prefix; sweeping it too would mean
	// an invalidation reaching further than intended.
	exists, err := c.Exists(ctx, other)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("a key outside the pattern was swept")
	}
}

func TestGetWithFallbackAgainstRedis(t *testing.T) {
	c, client := redisCache(t)
	ctx := context.Background()
	key := scopedKey(t, client, "fallback")

	calls := 0
	fetch := func() (string, error) {
		calls++

		return "from source", nil
	}

	first, err := GetWithFallback(ctx, c, key, time.Minute, fetch)
	if err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}
	if first != "from source" || calls != 1 {
		t.Fatalf("first call: value %q after %d fetches", first, calls)
	}

	second, err := GetWithFallback(ctx, c, key, time.Minute, fetch)
	if err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}
	if second != "from source" {
		t.Errorf("second call returned %q", second)
	}
	if calls != 1 {
		t.Errorf("the source was consulted %d times; the value was not cached", calls)
	}
}
