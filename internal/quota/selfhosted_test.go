// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package quota

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var errCounting = errors.New("counting failed")

// stubCounter is a hand-written CalendarCounter. It records what it was asked, which is
// how the tests below prove the self-hosted service never gates on a count at all.
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

func TestSelfHostedCalendarLimitIsUnlimited(t *testing.T) {
	// Zero is the sentinel for unlimited, not for "none allowed". Reading it as the
	// latter would lock every self-hosted install out of creating anything.
	if SelfHostedCalendarLimit != 0 {
		t.Errorf("SelfHostedCalendarLimit = %d, want 0 (unlimited)", SelfHostedCalendarLimit)
	}
}

// TestSelfHostedNeverRefuses is the whole point of the self-hosted variant: there is no
// licence to run out of, so no count can produce a refusal.
func TestSelfHostedNeverRefuses(t *testing.T) {
	tests := []struct {
		name  string
		owned int
		all   int
		err   error
	}{
		{name: "no calendars", owned: 0, all: 0},
		{name: "a handful", owned: 3, all: 12},
		{name: "an absurd number", owned: 100000, all: 999999},
		{name: "even when counting fails", owned: 0, all: 0, err: errCounting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := &stubCounter{byUser: tt.owned, all: tt.all, byUserErr: tt.err, allErr: tt.err}
			service := NewSelfHostedService(counter)
			ctx := context.Background()

			canCreate, err := service.CanCreateCalendar(ctx, uuid.New())
			if err != nil {
				t.Fatalf("CanCreateCalendar returned an error: %v", err)
			}
			if !canCreate {
				t.Error("CanCreateCalendar = false; self-hosting is unrestricted")
			}

			over, err := service.IsOverQuota(ctx, uuid.New())
			if err != nil {
				t.Fatalf("IsOverQuota returned an error: %v", err)
			}
			if over {
				t.Error("IsOverQuota = true; there is no limit to exceed")
			}

			// Neither decision may depend on a count — a repository failure must not
			// be able to lock an operator out of their own install.
			if counter.byUserCalls != 0 || counter.allCalls != 0 {
				t.Errorf(
					"the decision consulted the repository (byUser=%d, all=%d)",
					counter.byUserCalls, counter.allCalls,
				)
			}
		})
	}
}

func TestSelfHostedLimits(t *testing.T) {
	service := NewSelfHostedService(&stubCounter{})
	ctx := context.Background()

	serverLimit, err := service.GetServerLimit(ctx)
	if err != nil {
		t.Fatalf("GetServerLimit: %v", err)
	}
	if serverLimit != 0 {
		t.Errorf("GetServerLimit = %d, want 0 (unlimited)", serverLimit)
	}

	// Self-hosted limits are server-wide, so the per-user question resolves to the
	// same answer rather than to a different number.
	userLimit, err := service.GetUserLimit(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetUserLimit: %v", err)
	}
	if userLimit != serverLimit {
		t.Errorf("GetUserLimit = %d, want the server limit %d", userLimit, serverLimit)
	}
}

func TestSelfHostedUsage(t *testing.T) {
	counter := &stubCounter{byUser: 2, all: 41}
	service := NewSelfHostedService(counter)
	ctx := context.Background()

	// Usage is still reported even though nothing is enforced — the UI shows it.
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

	failing := NewSelfHostedService(&stubCounter{byUserErr: errCounting, allErr: errCounting})
	if _, err := failing.GetCurrentUsage(ctx, uuid.New()); !errors.Is(err, errCounting) {
		t.Errorf("GetCurrentUsage did not wrap the cause: %v", err)
	}
	if _, err := failing.GetServerUsage(ctx); !errors.Is(err, errCounting) {
		t.Errorf("GetServerUsage did not wrap the cause: %v", err)
	}
}

// TestSelfHostedQuotaLockKeyIsServerWide mirrors the cloud test and asserts the
// opposite property: one fixed key for everyone, because the limit is server-wide.
func TestSelfHostedQuotaLockKeyIsServerWide(t *testing.T) {
	service := NewSelfHostedService(&stubCounter{})

	alice := uuid.MustParse("7fbd281f-d193-42a3-8b75-520421a5ff3b")
	bob := uuid.MustParse("1c0f7a52-9b3e-4d61-8a77-2f5c9e0d1b84")

	if service.QuotaLockKey(alice) != service.QuotaLockKey(bob) {
		t.Error("two users produced different lock keys; the self-hosted lock is server-wide")
	}
	if got, want := service.QuotaLockKey(alice), int64(0x57484E544F51); got != want {
		t.Errorf("QuotaLockKey = %#x, want %#x", got, want)
	}
}

// TestSelfHostedServiceSatisfiesTheInterface is the build-tag tripwire: if the two
// implementations drift apart in shape, one variant stops compiling here rather than
// in cmd/.
func TestSelfHostedServiceSatisfiesTheInterface(t *testing.T) {
	var _ QuotaService = NewSelfHostedService(&stubCounter{})
}
