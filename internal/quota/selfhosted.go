// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package quota

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/licensing/service"
)

// SelfHostedQuotaService implements QuotaService for the self-hosted version
type SelfHostedQuotaService struct {
	licensingService *service.Service
	calendarRepo     CalendarCounter
}

// CalendarCounter is an interface for counting calendars
type CalendarCounter interface {
	CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
	CountAll(ctx context.Context) (int, error)
}

// NewSelfHostedService creates a new self-hosted quota service
func NewSelfHostedService(licensingService *service.Service, calendarRepo CalendarCounter) *SelfHostedQuotaService {
	return &SelfHostedQuotaService{
		licensingService: licensingService,
		calendarRepo:     calendarRepo,
	}
}

// CanCreateCalendar checks if a new calendar can be created based on server-wide license limits
func (s *SelfHostedQuotaService) CanCreateCalendar(ctx context.Context, userID uuid.UUID) (bool, error) {
	serverLimit, err := s.GetServerLimit(ctx)
	if err != nil {
		return false, err
	}

	// 0 means unlimited
	if serverLimit == 0 {
		return true, nil
	}

	totalCalendars, err := s.GetServerUsage(ctx)
	if err != nil {
		return false, err
	}

	// Return false without error if limit reached (not an internal error)
	return totalCalendars < serverLimit, nil
}

// GetUserLimit returns the server limit (self-hosted has server-wide limits, not per-user)
func (s *SelfHostedQuotaService) GetUserLimit(ctx context.Context, userID uuid.UUID) (int, error) {
	// In self-hosted, there's no per-user limit, only server-wide
	return s.GetServerLimit(ctx)
}

// GetServerLimit returns the server-wide calendar limit based on the license
func (s *SelfHostedQuotaService) GetServerLimit(ctx context.Context) (int, error) {
	// License is loaded in RAM, no need for context
	limit := s.licensingService.GetServerCalendarLimit()
	return limit, nil
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

// IsOverQuota checks if the server has exceeded its calendar limit
// For self-hosted, this is a server-wide check (not per-user)
// When license expires/downgrades, the server may have more calendars than allowed
func (s *SelfHostedQuotaService) IsOverQuota(ctx context.Context, userID uuid.UUID) (bool, error) {
	serverLimit, err := s.GetServerLimit(ctx)
	if err != nil {
		return false, err
	}

	// Unlimited = can never be over quota
	if serverLimit == 0 {
		return false, nil
	}

	serverUsage, err := s.GetServerUsage(ctx)
	if err != nil {
		return false, err
	}

	// Over quota if server usage exceeds limit
	return serverUsage > serverLimit, nil
}

// GetLimitInfo returns detailed information about limits and usage
func (s *SelfHostedQuotaService) GetLimitInfo(ctx context.Context, userID uuid.UUID) (*LimitInfo, error) {
	serverLimit, err := s.GetServerLimit(ctx)
	if err != nil {
		return nil, err
	}

	userUsage, err := s.GetCurrentUsage(ctx, userID)
	if err != nil {
		return nil, err
	}

	serverUsage, err := s.GetServerUsage(ctx)
	if err != nil {
		return nil, err
	}

	canCreate, _ := s.CanCreateCalendar(ctx, userID)

	limitationType := "per_server"
	if serverLimit == 0 {
		limitationType = "none"
	}

	return &LimitInfo{
		UserLimit:      serverLimit, // Same as server limit
		ServerLimit:    serverLimit,
		UserUsage:      userUsage,
		ServerUsage:    serverUsage,
		CanCreate:      canCreate,
		LimitationType: limitationType,
		UpgradeURL:     "https://whento.be/pricing",
	}, nil
}
