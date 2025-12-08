// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build cloud

package quota

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/subscription/service"
)

// CloudQuotaService implements QuotaService for the cloud version
type CloudQuotaService struct {
	subscriptionService *service.Service
	calendarRepo        CalendarCounter
}

// CalendarCounter is an interface for counting calendars
type CalendarCounter interface {
	CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
	CountAll(ctx context.Context) (int, error)
}

// NewCloudService creates a new cloud quota service
func NewCloudService(subscriptionService *service.Service, calendarRepo CalendarCounter) *CloudQuotaService {
	return &CloudQuotaService{
		subscriptionService: subscriptionService,
		calendarRepo:        calendarRepo,
	}
}

// CanCreateCalendar checks if a user can create a new calendar based on their subscription
func (s *CloudQuotaService) CanCreateCalendar(ctx context.Context, userID uuid.UUID) (bool, error) {
	limit, err := s.GetUserLimit(ctx, userID)
	if err != nil {
		return false, err
	}

	// 0 means unlimited
	if limit == 0 {
		return true, nil
	}

	current, err := s.GetCurrentUsage(ctx, userID)
	if err != nil {
		return false, err
	}

	return current < limit, nil
}

// GetUserLimit returns the calendar limit for a specific user based on their subscription
func (s *CloudQuotaService) GetUserLimit(ctx context.Context, userID uuid.UUID) (int, error) {
	limit, err := s.subscriptionService.GetCalendarLimit(ctx, userID)
	if err != nil {
		return 3, fmt.Errorf("failed to get calendar limit: %w", err)
	}

	return limit, nil
}

// GetServerLimit returns -1 as cloud version doesn't have server-wide limits
func (s *CloudQuotaService) GetServerLimit(ctx context.Context) (int, error) {
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

// IsOverQuota checks if user has exceeded their calendar limit
// This happens when subscription expires/downgrades but user still has more calendars than allowed
func (s *CloudQuotaService) IsOverQuota(ctx context.Context, userID uuid.UUID) (bool, error) {
	limit, err := s.GetUserLimit(ctx, userID)
	if err != nil {
		return false, err
	}

	// Unlimited = can never be over quota
	if limit == 0 {
		return false, nil
	}

	current, err := s.GetCurrentUsage(ctx, userID)
	if err != nil {
		return false, err
	}

	// Over quota if current usage exceeds limit
	return current > limit, nil
}

// GetLimitInfo returns detailed information about limits and usage
func (s *CloudQuotaService) GetLimitInfo(ctx context.Context, userID uuid.UUID) (*LimitInfo, error) {
	userLimit, err := s.GetUserLimit(ctx, userID)
	if err != nil {
		return nil, err
	}

	userUsage, err := s.GetCurrentUsage(ctx, userID)
	if err != nil {
		return nil, err
	}

	serverUsage, _ := s.GetServerUsage(ctx)

	canCreate, _ := s.CanCreateCalendar(ctx, userID)

	limitationType := "per_user"
	if userLimit == 0 {
		limitationType = "none"
	}

	return &LimitInfo{
		UserLimit:      userLimit,
		ServerLimit:    -1, // N/A for cloud
		UserUsage:      userUsage,
		ServerUsage:    serverUsage,
		CanCreate:      canCreate,
		LimitationType: limitationType,
		UpgradeURL:     "/billing/upgrade",
	}, nil
}
