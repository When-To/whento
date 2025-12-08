// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package license

import "time"

// License represents a self-hosted license
type License struct {
	Tier             string     `json:"tier"`
	CalendarLimit    int        `json:"calendar_limit"`
	IssuedTo         string     `json:"issued_to"`
	IssuedAt         time.Time  `json:"issued_at"`
	SupportKey       string     `json:"support_key"`
	SupportExpiresAt *time.Time `json:"support_expires_at,omitempty"`
	Signature        string     `json:"signature"`
}

// GenerateConfig holds configuration for license generation
type GenerateConfig struct {
	Tier          string    // "pro" or "enterprise"
	CalendarLimit int       // Calendar limit (0 = unlimited)
	IssuedTo      string    // Company or person name
	IssuedAt      time.Time // Issuance timestamp
	SupportYears  int       // Support years (1 or 2, 0 = default based on tier)
}

// Tier constants
const (
	TierPro        = "pro"
	TierEnterprise = "enterprise"
)

// Default calendar limits
const (
	DefaultProLimit        = 300 // Pro tier: 300 calendars
	DefaultEnterpriseLimit = 0   // Enterprise tier: unlimited
)

// Default support years
const (
	DefaultProSupportYears        = 1 // Pro: 1 year support
	DefaultEnterpriseSupportYears = 2 // Enterprise: 2 years support
)
