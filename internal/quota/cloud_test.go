// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package quota

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var errCounting = errors.New("counting failed")

// stubCounter is a hand-written CalendarCounter. It records what it was asked so the
// tests can assert the service does not, say, consult the server-wide count for a
// per-user decision.
type stubCounter struct {
	byUser    int
	byUserErr error
	all       int
	allErr    error

	byUserCalls int
	allCalls    int
	lastUserID  uuid.UUID
}

var _ CalendarCounter = (*stubCounter)(nil)

func (s *stubCounter) CountByUser(_ context.Context, userID uuid.UUID) (int, error) {
	s.byUserCalls++
	s.lastUserID = userID

	return s.byUser, s.byUserErr
}

func (s *stubCounter) CountAll(_ context.Context) (int, error) {
	s.allCalls++

	return s.all, s.allErr
}

func TestCloudCalendarLimitIsThree(t *testing.T) {
	// The hosted allowance is a product decision with no upgrade path behind it any
	// more. If this number moves, it moves deliberately.
	if CloudCalendarLimit != 3 {
		t.Errorf("CloudCalendarLimit = %d, want 3", CloudCalendarLimit)
	}
}

func TestCloudCanCreateCalendar(t *testing.T) {
	tests := []struct {
		name    string
		owned   int
		err     error
		want    bool
		wantErr bool
	}{
		{name: "no calendars yet", owned: 0, want: true},
		{name: "one below the allowance", owned: 2, want: true},
		{name: "exactly at the allowance", owned: 3, want: false},
		{name: "already over the allowance", owned: 7, want: false},
		{name: "the count failed", owned: 0, err: errCounting, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := &stubCounter{byUser: tt.owned, byUserErr: tt.err}
			service := NewCloudService(counter)

			userID := uuid.New()
			got, err := service.CanCreateCalendar(context.Background(), userID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !errors.Is(err, errCounting) {
					t.Errorf("error does not wrap the cause: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CanCreateCalendar with %d owned = %v, want %v", tt.owned, got, tt.want)
			}
			if counter.lastUserID != userID {
				t.Errorf("counted for %v, want %v", counter.lastUserID, userID)
			}
			if counter.allCalls != 0 {
				t.Errorf("consulted the server-wide count %d times; cloud limits are per user", counter.allCalls)
			}
		})
	}
}

func TestCloudIsOverQuota(t *testing.T) {
	tests := []struct {
		name    string
		owned   int
		err     error
		want    bool
		wantErr bool
	}{
		{name: "below", owned: 1, want: false},
		// At the allowance the user is not over it — they simply cannot add more. The
		// distinction matters: IsOverQuota also gates ICS rendering.
		{name: "exactly at the allowance", owned: 3, want: false},
		{name: "over, as after a lowered allowance", owned: 4, want: true},
		{name: "the count failed", owned: 0, err: errCounting, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewCloudService(&stubCounter{byUser: tt.owned, byUserErr: tt.err})

			got, err := service.IsOverQuota(context.Background(), uuid.New())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("IsOverQuota with %d owned = %v, want %v", tt.owned, got, tt.want)
			}
		})
	}
}

func TestCloudLimits(t *testing.T) {
	service := NewCloudService(&stubCounter{})
	ctx := context.Background()

	userLimit, err := service.GetUserLimit(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetUserLimit: %v", err)
	}
	if userLimit != CloudCalendarLimit {
		t.Errorf("GetUserLimit = %d, want %d", userLimit, CloudCalendarLimit)
	}

	// -1, not 0: zero means unlimited, and the handler reads the two differently when
	// deciding whether the cap is per-user or per-server.
	serverLimit, err := service.GetServerLimit(ctx)
	if err != nil {
		t.Fatalf("GetServerLimit: %v", err)
	}
	if serverLimit != -1 {
		t.Errorf("GetServerLimit = %d, want -1 (not applicable)", serverLimit)
	}
}

func TestCloudUsage(t *testing.T) {
	counter := &stubCounter{byUser: 2, all: 41}
	service := NewCloudService(counter)
	ctx := context.Background()

	userUsage, err := service.GetCurrentUsage(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetCurrentUsage: %v", err)
	}
	if userUsage != 2 {
		t.Errorf("GetCurrentUsage = %d, want 2", userUsage)
	}

	serverUsage, err := service.GetServerUsage(ctx)
	if err != nil {
		t.Fatalf("GetServerUsage: %v", err)
	}
	if serverUsage != 41 {
		t.Errorf("GetServerUsage = %d, want 41", serverUsage)
	}

	failing := NewCloudService(&stubCounter{byUserErr: errCounting, allErr: errCounting})
	if _, err := failing.GetCurrentUsage(ctx, uuid.New()); !errors.Is(err, errCounting) {
		t.Errorf("GetCurrentUsage did not wrap the cause: %v", err)
	}
	if _, err := failing.GetServerUsage(ctx); !errors.Is(err, errCounting) {
		t.Errorf("GetServerUsage did not wrap the cause: %v", err)
	}
}

// TestCloudQuotaLockKeyIsPerUser matters for correctness under concurrency: the key
// serialises calendar creation, so two different users must not collide on it, and the
// same user must always land on the same key.
func TestCloudQuotaLockKeyIsPerUser(t *testing.T) {
	service := NewCloudService(&stubCounter{})

	alice := uuid.MustParse("7fbd281f-d193-42a3-8b75-520421a5ff3b")
	bob := uuid.MustParse("1c0f7a52-9b3e-4d61-8a77-2f5c9e0d1b84")

	// Two separate calls, kept in variables so the assertion is a comparison of two
	// values rather than of two identical expressions.
	first, second := service.QuotaLockKey(alice), service.QuotaLockKey(alice)
	if first != second {
		t.Errorf("the same user produced two different lock keys: %#x and %#x", first, second)
	}
	if service.QuotaLockKey(alice) == service.QuotaLockKey(bob) {
		t.Error("two different users collided on the same lock key")
	}

	// Derived from the first 8 bytes, big-endian.
	if got, want := service.QuotaLockKey(alice), int64(0x7fbd281fd19342a3); got != want {
		t.Errorf("QuotaLockKey = %#x, want %#x", got, want)
	}

	// A UUID whose leading bit is set must not blow up or alias; it simply goes
	// negative once reinterpreted as a signed key.
	high := uuid.MustParse("ffffffff-ffff-ffff-0000-000000000000")
	if got := service.QuotaLockKey(high); got != -1 {
		t.Errorf("QuotaLockKey for a high UUID = %d, want -1", got)
	}
}

// TestCloudServiceSatisfiesTheInterface is the build-tag tripwire: if the cloud and
// self-hosted implementations drift apart in shape, one of the two variants stops
// compiling here rather than in cmd/.
func TestCloudServiceSatisfiesTheInterface(t *testing.T) {
	var _ QuotaService = NewCloudService(&stubCounter{})
}
