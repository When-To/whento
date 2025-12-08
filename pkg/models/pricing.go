// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

// TieredConfig represents a generic pricing tier configuration
// Used for both cloud subscriptions and self-hosted licenses
type TieredConfig struct {
	// Name is the tier/plan identifier (e.g., "free", "pro", "enterprise")
	Name string `json:"name"`

	// CalendarLimit is the maximum number of calendars (0 = unlimited)
	CalendarLimit int `json:"calendar_limit"`

	// Price is the cost in cents (for subscriptions: monthly/annual, for licenses: one-time)
	Price int `json:"price"`

	// Features is a list of features included in this tier
	Features []string `json:"features"`

	// Description provides additional details about the tier
	Description string `json:"description,omitempty"`

	// StripePriceID is the Stripe price ID (cloud mode only)
	StripePriceID string `json:"stripe_price_id,omitempty"`

	// SupportLevel describes the support tier (self-hosted mode only)
	SupportLevel string `json:"support_level,omitempty"`
}

// NewSubscriptionPlanConfig creates a TieredConfig for cloud subscription plans
func NewSubscriptionPlanConfig(plan SubscriptionPlan, limit, price int, stripePriceID string, features []string) TieredConfig {
	return TieredConfig{
		Name:          string(plan),
		CalendarLimit: limit,
		Price:         price,
		StripePriceID: stripePriceID,
		Features:      features,
	}
}

// NewLicenseTierConfig creates a TieredConfig for self-hosted license tiers
func NewLicenseTierConfig(tier LicenseTier, limit, price int, supportLevel string, features []string) TieredConfig {
	return TieredConfig{
		Name:          string(tier),
		CalendarLimit: limit,
		Price:         price,
		SupportLevel:  supportLevel,
		Features:      features,
	}
}
