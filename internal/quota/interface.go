// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package quota

import (
	"context"

	"github.com/google/uuid"
)

// QuotaService defines the interface for calendar quota management
// Implementations differ based on build type (cloud vs selfhosted)
type QuotaService interface {
	// CanCreateCalendar checks if a user can create a new calendar
	CanCreateCalendar(ctx context.Context, userID uuid.UUID) (bool, error)

	// GetUserLimit returns the calendar limit for a specific user
	// Returns 0 for unlimited
	GetUserLimit(ctx context.Context, userID uuid.UUID) (int, error)

	// GetServerLimit returns the server-wide calendar limit (self-hosted only)
	// Returns 0 for unlimited, -1 if not applicable (cloud)
	GetServerLimit(ctx context.Context) (int, error)

	// GetCurrentUsage returns the current calendar count for a user
	GetCurrentUsage(ctx context.Context, userID uuid.UUID) (int, error)

	// GetServerUsage returns the total calendar count across all users
	GetServerUsage(ctx context.Context) (int, error)

	// IsOverQuota checks if a user has exceeded their allowed calendar limit
	// This happens when subscription/license expires but user still has more calendars than allowed
	// When over quota, users should be blocked from creating calendars and accessing ICS feeds
	IsOverQuota(ctx context.Context, userID uuid.UUID) (bool, error)
}

// LimitInfo contains detailed information about limits and usage
type LimitInfo struct {
	UserLimit      int    `json:"user_limit"`   // 0 = unlimited
	ServerLimit    int    `json:"server_limit"` // 0 = unlimited, -1 = N/A
	UserUsage      int    `json:"user_usage"`
	ServerUsage    int    `json:"server_usage"`
	CanCreate      bool   `json:"can_create"`
	LimitationType string `json:"limitation_type"` // "per_user", "per_server", "none"
	UpgradeURL     string `json:"upgrade_url"`
}
