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

	// redis.Nil specifically: GetOrSet and GetWithFallback both compare against it to
	// tell a miss from a real failure.
	if !errors.Is(err, redis.Nil) {
		t.Errorf("Get error = %v, want redis.Nil", err)
	}

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Errorf("Set on a no-op cache returned %v", err)
	}
	if err := c.Delete(ctx, "k", "j"); err != nil {
		t.Errorf("Delete on a no-op cache returned %v", err)
	}

	exists, err := c.Exists(ctx, "k")
	if err != nil {
		t.Errorf("Exists returned %v", err)
	}
	if exists {
		t.Error("Exists reported true on a no-op cache")
	}
}

func TestKeysAreDistinctAndPrefixed(t *testing.T) {
	// Two different keys colliding would serve one calendar's data for another, so the
	// namespacing matters more than the exact format.
	keys := map[string]string{
		"CalendarByIDKey":           CalendarByIDKey("abc"),
		"CalendarByPublicTokenKey":  CalendarByPublicTokenKey("abc"),
		"CalendarByICSTokenKey":     CalendarByICSTokenKey("abc"),
		"CalendarParticipantsKey":   CalendarParticipantsKey("abc"),
		"UserCalendarsKey":          UserCalendarsKey("abc"),
		"ParticipantAvailabilities": ParticipantAvailabilitiesKey("abc"),
		"CalendarDateSummaryKey":    CalendarDateSummaryKey("abc", "2026-03-05"),
		"CalendarRangeSummaryKey":   CalendarRangeSummaryKey("abc", "2026-03-01", "2026-03-31"),
		"ICSFeedKey":                ICSFeedKey("abc"),
		"UserPasswordChangedKey":    UserPasswordChangedKey("abc"),
	}

	seen := map[string]string{}
	for name, key := range keys {
		if previous, clash := seen[key]; clash {
			t.Errorf("%s and %s produce the same key %q", name, previous, key)
		}
		seen[key] = name

		if !strings.Contains(key, "abc") {
			t.Errorf("%s = %q, which does not include its identifier", name, key)
		}
	}

	// Every key belongs to a known namespace, so a pattern invalidation cannot sweep
	// an unrelated one.
	prefixes := []string{PrefixCalendar, PrefixParticipant, PrefixAvailability, PrefixICS, PrefixAuth}
	for name, key := range keys {
		matched := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix+":") {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("%s = %q carries no known prefix", name, key)
		}
	}
}

func TestKeysVaryWithEveryArgument(t *testing.T) {
	// A key that ignored one of its inputs would serve March's summary for April.
	if CalendarDateSummaryKey("cal", "2026-03-05") == CalendarDateSummaryKey("cal", "2026-03-06") {
		t.Error("the date summary key ignores the date")
	}
	if CalendarDateSummaryKey("a", "2026-03-05") == CalendarDateSummaryKey("b", "2026-03-05") {
		t.Error("the date summary key ignores the calendar")
	}

	base := CalendarRangeSummaryKey("cal", "2026-03-01", "2026-03-31")
	if base == CalendarRangeSummaryKey("cal", "2026-03-02", "2026-03-31") {
		t.Error("the range key ignores the start date")
	}
	if base == CalendarRangeSummaryKey("cal", "2026-03-01", "2026-04-30") {
		t.Error("the range key ignores the end date")
	}
}

func TestCacheKeyBundles(t *testing.T) {
	calendar := CalendarCacheKeys("cal-1")
	if len(calendar) != 2 {
		t.Fatalf("got %d calendar keys, want 2", len(calendar))
	}
	// The bundle exists so an invalidation cannot forget one of them.
	want := map[string]bool{
		CalendarByIDKey("cal-1"):         false,
		CalendarParticipantsKey("cal-1"): false,
	}
	for _, key := range calendar {
		if _, expected := want[key]; !expected {
			t.Errorf("unexpected key %q in the calendar bundle", key)
		}
		want[key] = true
	}
	for key, found := range want {
		if !found {
			t.Errorf("the calendar bundle is missing %q", key)
		}
	}

	participant := ParticipantCacheKeys("p-1")
	if len(participant) != 1 || participant[0] != ParticipantAvailabilitiesKey("p-1") {
		t.Errorf("participant bundle = %v", participant)
	}
}

func TestGetWithFallbackSkipsADisabledCache(t *testing.T) {
	calls := 0
	value, err := GetWithFallback(context.Background(), NewRedisCache(nil), "k", time.Minute,
		func() (string, error) {
			calls++

			return "from source", nil
		})
	if err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}
	if value != "from source" {
		t.Errorf("value = %q, want the fallback's", value)
	}
	if calls != 1 {
		t.Errorf("the fallback ran %d times, want once", calls)
	}
}

func TestGetWithFallbackPropagatesAFailure(t *testing.T) {
	wantErr := errors.New("source unavailable")

	_, err := GetWithFallback(context.Background(), NewRedisCache(nil), "k", time.Minute,
		func() (string, error) { return "", wantErr })

	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want the source failure", err)
	}
}

// stubCache lets the enabled path be exercised without a Redis server.
type stubCache struct {
	values  map[string][]byte
	getErr  error
	enabled bool
	sets    int
}

var _ Cache = (*stubCache)(nil)

func (s *stubCache) Get(_ context.Context, key string, dest interface{}) error {
	if s.getErr != nil {
		return s.getErr
	}
	raw, ok := s.values[key]
	if !ok {
		return redis.Nil
	}
	if target, ok := dest.(*string); ok {
		*target = string(raw)
	}

	return nil
}

func (s *stubCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	s.sets++
	if str, ok := value.(string); ok {
		s.values[key] = []byte(str)
	}

	return nil
}

func (s *stubCache) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		delete(s.values, key)
	}

	return nil
}

func (s *stubCache) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.values[key]

	return ok, nil
}

func (s *stubCache) IsEnabled() bool { return s.enabled }

func TestGetWithFallbackUsesTheCachedValue(t *testing.T) {
	stub := &stubCache{values: map[string][]byte{"k": []byte("cached")}, enabled: true}

	calls := 0
	value, err := GetWithFallback(context.Background(), stub, "k", time.Minute,
		func() (string, error) {
			calls++

			return "from source", nil
		})
	if err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}

	if value != "cached" {
		t.Errorf("value = %q, want the cached one", value)
	}
	if calls != 0 {
		t.Error("the fallback ran despite a cache hit")
	}
}

func TestGetWithFallbackStoresWhatItFetched(t *testing.T) {
	stub := &stubCache{values: map[string][]byte{}, enabled: true}

	if _, err := GetWithFallback(context.Background(), stub, "k", time.Minute,
		func() (string, error) { return "fetched", nil }); err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}

	if stub.sets != 1 {
		t.Errorf("the fetched value was stored %d times, want once", stub.sets)
	}

	// The second call is served from the cache.
	calls := 0
	value, err := GetWithFallback(context.Background(), stub, "k", time.Minute,
		func() (string, error) {
			calls++

			return "second", nil
		})
	if err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}
	if value != "fetched" || calls != 0 {
		t.Errorf("value = %q after %d fallback calls; the store did not take", value, calls)
	}
}

func TestGetWithFallbackTreatsAReadFailureAsAMiss(t *testing.T) {
	// A broken cache must degrade to the source rather than fail the request.
	stub := &stubCache{values: map[string][]byte{}, enabled: true, getErr: errors.New("connection reset")}

	value, err := GetWithFallback(context.Background(), stub, "k", time.Minute,
		func() (string, error) { return "from source", nil })
	if err != nil {
		t.Fatalf("a cache read failure surfaced to the caller: %v", err)
	}
	if value != "from source" {
		t.Errorf("value = %q, want the fallback's", value)
	}
}

func TestInvalidatePatternIsANoOpWithoutRedis(t *testing.T) {
	// Only RedisCache can scan, and the helper says so; a no-op cache must not error.
	if err := InvalidatePattern(context.Background(), NewRedisCache(nil), "calendar:*"); err != nil {
		t.Errorf("InvalidatePattern on a no-op cache returned %v", err)
	}
}

func TestGetOrSetFetchesOnAMiss(t *testing.T) {
	calls := 0
	var dest interface{}

	err := GetOrSet(context.Background(), NewRedisCache(nil), "k", &dest, time.Minute,
		func() (interface{}, error) {
			calls++

			return "value", nil
		})
	if err != nil {
		t.Fatalf("GetOrSet: %v", err)
	}
	if calls != 1 {
		t.Errorf("the fetch ran %d times, want once", calls)
	}
	if dest != "value" {
		t.Errorf("dest = %v, want the fetched value", dest)
	}
}

func TestGetOrSetPropagatesAFetchFailure(t *testing.T) {
	wantErr := errors.New("source unavailable")
	var dest interface{}

	err := GetOrSet(context.Background(), NewRedisCache(nil), "k", &dest, time.Minute,
		func() (interface{}, error) { return nil, wantErr })

	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want the fetch failure", err)
	}
}

func TestTTLsAreOrdered(t *testing.T) {
	// The summaries change whenever an availability does, so they must not outlive the
	// longer-lived entries they are derived from.
	if TTLDateSummary > TTLCalendar {
		t.Errorf("the date summary TTL (%v) exceeds the calendar TTL (%v)", TTLDateSummary, TTLCalendar)
	}
	if TTLRangeSummary > TTLCalendar {
		t.Errorf("the range summary TTL (%v) exceeds the calendar TTL (%v)", TTLRangeSummary, TTLCalendar)
	}
	for name, ttl := range map[string]time.Duration{
		"TTLCalendar":     TTLCalendar,
		"TTLDateSummary":  TTLDateSummary,
		"TTLRangeSummary": TTLRangeSummary,
	} {
		if ttl <= 0 {
			t.Errorf("%s = %v, which would cache nothing", name, ttl)
		}
	}
}
