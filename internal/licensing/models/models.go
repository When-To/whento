// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/whento/pkg/models"
)

// Use shared enum types
type LicenseTier = models.LicenseTier

// Re-export shared constants for backward compatibility
const (
	TierCommunity  = models.TierCommunity
	TierPro        = models.TierPro
	TierEnterprise = models.TierEnterprise
)

// License represents a self-hosted license stored in the database
type License struct {
	models.Entity
	LicenseData LicensePayload `json:"license_data" db:"license_data"` // JSONB column with full signed payload
	ActivatedAt time.Time      `json:"activated_at" db:"activated_at"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
}

// LicensePayload is the signed data structure for license keys
// This is the source of truth stored in JSONB and verified via Ed25519 signature
// Self-hosted licenses are perpetual (no expiration), only support has a time limit
type LicensePayload struct {
	Tier             string     `json:"tier"` // Stored as string for JSON compatibility
	CalendarLimit    int        `json:"calendar_limit"`
	IssuedTo         string     `json:"issued_to"`
	IssuedAt         time.Time  `json:"issued_at"`
	SupportKey       string     `json:"support_key"`                  // Unique key for support requests (e.g., SUPP-XXXX-XXXX-XXXX)
	SupportExpiresAt *time.Time `json:"support_expires_at,omitempty"` // When support period ends
	Signature        string     `json:"signature"`                    // Ed25519 signature in base64
}

// GetTier returns the LicenseTier enum from the string
func (lp *LicensePayload) GetTier() LicenseTier {
	return LicenseTier(lp.Tier)
}

// IsSupportActive checks if the support period is still active
func (lp *LicensePayload) IsSupportActive() bool {
	if lp.SupportExpiresAt == nil {
		return false // No support or expired
	}
	return time.Now().Before(*lp.SupportExpiresAt)
}

// TierConfig is an alias for TieredConfig for backward compatibility
type TierConfig = models.TieredConfig

// GetTierConfig returns the configuration for each tier
func GetTierConfig(tier LicenseTier) TierConfig {
	configs := map[LicenseTier]TierConfig{
		TierCommunity: models.NewLicenseTierConfig(
			TierCommunity,
			30,                        // CalendarLimit
			0,                         // Price (free)
			"Community (GitHub Issues)", // SupportLevel
			[]string{"30 calendars (server total)", "Unlimited participants", "iCal subscriptions"},
		),
		TierPro: models.NewLicenseTierConfig(
			TierPro,
			300,            // CalendarLimit
			10000,          // Price: 100€ + VAT lifetime
			"Email support", // SupportLevel
			[]string{"300 calendars", "Unlimited participants", "iCal subscriptions", "1 year support", "60€/year renewal"},
		),
		TierEnterprise: models.NewLicenseTierConfig(
			TierEnterprise,
			0,                 // CalendarLimit: unlimited
			25000,             // Price: 250€ + VAT lifetime
			"Priority support", // SupportLevel
			[]string{"Unlimited calendars", "Unlimited participants", "iCal subscriptions", "2 years support", "60€/year renewal"},
		),
	}
	return configs[tier]
}

// ActivateLicenseRequest represents a request to activate a license
type ActivateLicenseRequest struct {
	LicenseKey string `json:"license_key" validate:"required"`
}

// LicenseResponse is the API response for license information
type LicenseResponse struct {
	License       *LicensePayload `json:"license"` // Return the payload (without signature for security)
	TierConfig    TierConfig      `json:"tier_config"`
	Usage         int             `json:"usage"`
	CanCreate     bool            `json:"can_create"`
	IsActive      bool            `json:"is_active"`      // Always true for self-hosted (perpetual licenses)
	SupportActive bool            `json:"support_active"` // Whether support is still active
}

