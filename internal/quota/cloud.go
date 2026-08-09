// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package quota

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/google/uuid"
)

// CloudCalendarLimit is how many calendars a hosted account may own.
//
// It used to be read from the user's Stripe subscription, which is why this package
// depended on internal/subscription. The hosted service now gives everyone the same
// allowance, so the number lives here and the dependency is gone.
const CloudCalendarLimit = 3

// CloudQuotaService implements QuotaService for the cloud version.
// Limits are per user; there is no server-wide cap.
type CloudQuotaService struct {
	calendarRepo CalendarCounter
}

// CalendarCounter is an interface for counting calendars
type CalendarCounter interface {
	CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
	CountAll(ctx context.Context) (int, error)
}

// NewCloudService creates a new cloud quota service
func NewCloudService(calendarRepo CalendarCounter) *CloudQuotaService {
	return &CloudQuotaService{calendarRepo: calendarRepo}
}

// CanCreateCalendar reports whether the user is below their calendar allowance
func (s *CloudQuotaService) CanCreateCalendar(ctx context.Context, userID uuid.UUID) (bool, error) {
	current, err := s.GetCurrentUsage(ctx, userID)
	if err != nil {
		return false, err
	}

	return current < CloudCalendarLimit, nil
}

// GetUserLimit returns the calendar limit for a specific user
func (s *CloudQuotaService) GetUserLimit(_ context.Context, _ uuid.UUID) (int, error) {
	return CloudCalendarLimit, nil
}

// GetServerLimit returns -1 as cloud version doesn't have server-wide limits
func (s *CloudQuotaService) GetServerLimit(_ context.Context) (int, error) {
	return -1, nil // Not applicable for cloud
}

// GetCurrentUsage returns the current calendar count for a user
func (s *CloudQuotaService) GetCurrentUsage(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := s.calendarRepo.CountByUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to count calendars: %w", err)
	}

	return count, nil
}

// GetServerUsage returns the total calendar count across all users
func (s *CloudQuotaService) GetServerUsage(ctx context.Context) (int, error) {
	count, err := s.calendarRepo.CountAll(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count all calendars: %w", err)
	}

	return count, nil
}

// QuotaLockKey returns a per-user advisory lock key derived from the user's UUID
func (s *CloudQuotaService) QuotaLockKey(userID uuid.UUID) int64 {
	return int64(binary.BigEndian.Uint64(userID[:8]))
}

// IsOverQuota reports whether the user owns more calendars than the allowance.
//
// Reachable when the allowance is lowered, or when calendars were created before it
// applied. Such a user keeps their calendars but cannot create more, and their ICS
// feeds stop rendering.
func (s *CloudQuotaService) IsOverQuota(ctx context.Context, userID uuid.UUID) (bool, error) {
	current, err := s.GetCurrentUsage(ctx, userID)
	if err != nil {
		return false, err
	}

	return current > CloudCalendarLimit, nil
}
