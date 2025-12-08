// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

// PricingConfig represents a generic pricing tier configuration.
type PricingConfig struct {
	// Limit represents the resource limit (e.g., number of calendars)
	Limit int `json:"limit"`

	// Price represents the cost in cents
	Price int `json:"price"`

	// Features is a list of features included in this tier
	Features []string `json:"features"`

	// Description provides additional details about the tier
	Description string `json:"description,omitempty"`
}

// QuotaStatus represents the current quota usage status.
type QuotaStatus struct {
	// Limit is the maximum allowed resources
	Limit int `json:"limit"`

	// Usage is the current resource consumption
	Usage int `json:"usage"`

	// Available is the remaining resources (Limit - Usage)
	Available int `json:"available"`

	// CanCreate indicates if new resources can be created
	CanCreate bool `json:"can_create"`

	// IsOverQuota indicates if current usage exceeds the limit
	IsOverQuota bool `json:"is_over_quota"`
}

// NewQuotaStatus creates a QuotaStatus from limit and usage values.
func NewQuotaStatus(limit, usage int) QuotaStatus {
	available := limit - usage
	if available < 0 {
		available = 0
	}

	return QuotaStatus{
		Limit:       limit,
		Usage:       usage,
		Available:   available,
		CanCreate:   usage < limit,
		IsOverQuota: usage > limit,
	}
}
