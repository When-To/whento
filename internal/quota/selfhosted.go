// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package quota

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// SelfHostedCalendarLimit is the server-wide calendar allowance. Zero means unlimited.
//
// It used to come from a signed licence file, which is why this package depended on
// internal/licensing. Self-hosting is unrestricted now, so the number lives here and
// the dependency is gone.
const SelfHostedCalendarLimit = 0

// SelfHostedQuotaService implements QuotaService for the self-hosted version.
// Limits are server-wide rather than per user, and there are none.
type SelfHostedQuotaService struct {
	calendarRepo CalendarCounter
}

// CalendarCounter is an interface for counting calendars
type CalendarCounter interface {
	CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
	CountAll(ctx context.Context) (int, error)
}

// NewSelfHostedService creates a new self-hosted quota service
func NewSelfHostedService(calendarRepo CalendarCounter) *SelfHostedQuotaService {
	return &SelfHostedQuotaService{calendarRepo: calendarRepo}
}

// CanCreateCalendar always allows creation: self-hosting is unrestricted
func (s *SelfHostedQuotaService) CanCreateCalendar(_ context.Context, _ uuid.UUID) (bool, error) {
	return true, nil
}

// GetUserLimit returns the server limit (self-hosted has server-wide limits, not per-user)
func (s *SelfHostedQuotaService) GetUserLimit(ctx context.Context, _ uuid.UUID) (int, error) {
	return s.GetServerLimit(ctx)
}

// GetServerLimit returns 0, meaning unlimited
func (s *SelfHostedQuotaService) GetServerLimit(_ context.Context) (int, error) {
	return SelfHostedCalendarLimit, nil
}

// GetCurrentUsage returns the current calendar count for a user
func (s *SelfHostedQuotaService) GetCurrentUsage(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := s.calendarRepo.CountByUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count calendars: %w", err)
	}

	return count, nil
}

// GetServerUsage returns the total calendar count across all users
func (s *SelfHostedQuotaService) GetServerUsage(ctx context.Context) (int, error) {
	count, err := s.calendarRepo.CountAll(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count all calendars: %w", err)
	}

	return count, nil
}

// QuotaLockKey returns a fixed server-wide advisory lock key for self-hosted quota enforcement
func (s *SelfHostedQuotaService) QuotaLockKey(_ uuid.UUID) int64 {
	return 0x57484E54_4F51 // "WHNTOQ" — fixed key for server-wide lock
}

// IsOverQuota always reports false: there is no limit to exceed
func (s *SelfHostedQuotaService) IsOverQuota(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}
