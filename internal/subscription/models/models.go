// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/whento/pkg/models"
)

// Use shared enum types
type SubscriptionPlan = models.SubscriptionPlan
type SubscriptionStatus = models.SubscriptionStatus

// Re-export shared constants for backward compatibility
const (
	PlanFree  = models.PlanFree
	PlanPro   = models.PlanPro
	PlanPower = models.PlanPower
)

const (
	StatusActive     = models.StatusActive
	StatusCanceled   = models.StatusCanceled
	StatusPastDue    = models.StatusPastDue
	StatusIncomplete = models.StatusIncomplete
	StatusTrialing   = models.StatusTrialing
)

// Subscription represents a user's subscription in the cloud version
type Subscription struct {
	models.TimestampedEntity
	UserID               uuid.UUID          `json:"user_id" db:"user_id"`
	Plan                 SubscriptionPlan   `json:"plan" db:"plan" swaggertype:"string" enums:"free,pro,power"`
	Status               SubscriptionStatus `json:"status" db:"status" swaggertype:"string" enums:"active,canceled,past_due,incomplete,trialing"`
	StripeCustomerID     string             `json:"stripe_customer_id,omitempty" db:"stripe_customer_id"`
	StripeSubscriptionID string             `json:"stripe_subscription_id,omitempty" db:"stripe_subscription_id"`
	CalendarLimit        int                `json:"calendar_limit" db:"calendar_limit"`
	CurrentPeriodStart   time.Time          `json:"current_period_start" db:"current_period_start"`
	CurrentPeriodEnd     time.Time          `json:"current_period_end" db:"current_period_end"`
	CancelAtPeriodEnd    bool               `json:"cancel_at_period_end" db:"cancel_at_period_end"`
}

// PlanConfig is an alias for TieredConfig for backward compatibility
type PlanConfig = models.TieredConfig

// GetPlanConfig returns the configuration for each plan
func GetPlanConfig(plan SubscriptionPlan) PlanConfig {
	configs := map[SubscriptionPlan]PlanConfig{
		PlanFree: models.NewSubscriptionPlanConfig(
			PlanFree,
			3,    // CalendarLimit
			0,    // Price
			"",   // StripePriceID (none for free plan)
			[]string{"3 calendars", "Unlimited participants", "iCal subscriptions"},
		),
		PlanPro: models.NewSubscriptionPlanConfig(
			PlanPro,
			30,                        // CalendarLimit
			2500,                      // Price: 25€/year + VAT
			"",                        // StripePriceID (set via env var)
			[]string{"30 calendars", "Unlimited participants", "iCal subscriptions", "Email support", "Annual billing"},
		),
		PlanPower: models.NewSubscriptionPlanConfig(
			PlanPower,
			0,                         // CalendarLimit: unlimited
			10000,                     // Price: 100€/year + VAT
			"",                        // StripePriceID (set via env var)
			[]string{"Unlimited calendars", "Unlimited participants", "iCal subscriptions", "Priority support", "Annual billing"},
		),
	}
	return configs[plan]
}

// CreateCheckoutRequest represents a request to create a Stripe checkout session
type CreateCheckoutRequest struct {
	Plan       SubscriptionPlan `json:"plan" validate:"required,oneof=pro power" swaggertype:"string" enums:"pro,power"`
	SuccessURL string           `json:"success_url" validate:"required,url"`
	CancelURL  string           `json:"cancel_url" validate:"required,url"`
	// Billing information for VAT calculation
	Name       string `json:"name" validate:"required"`
	Email      string `json:"email" validate:"required,email"`
	Company    string `json:"company"`
	VATNumber  string `json:"vat_number"`
	Address    string `json:"address"`
	PostalCode string `json:"postal_code"` // For VAT regional exceptions (e.g., French DOM-TOM)
	Country    string `json:"country" validate:"required,len=2"`
}

// CreateCheckoutResponse contains the checkout session URL
type CreateCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
	SessionID   string `json:"session_id"`
}

// CreatePortalRequest represents a request to create a Stripe customer portal session
type CreatePortalRequest struct {
	ReturnURL string `json:"return_url" validate:"required,url"`
}

// CreatePortalResponse contains the customer portal URL
type CreatePortalResponse struct {
	PortalURL string `json:"portal_url"`
}

// SubscriptionResponse is the API response for subscription information
type SubscriptionResponse struct {
	Subscription *Subscription `json:"subscription"`
	PlanConfig   PlanConfig    `json:"plan_config"`
}

// AccountingRequest represents request filters for accounting data
type AccountingRequest struct {
	Year  int `json:"year" validate:"required,min=2020,max=2100"`
	Month int `json:"month" validate:"omitempty,min=1,max=12"` // 0 = whole year
}

// AccountingCountryRow represents revenue data for a single country
type AccountingCountryRow struct {
	Country      string  `json:"country"`       // ISO 3166-1 alpha-2 country code
	CountryName  string  `json:"country_name"`  // Human-readable country name
	RevenueHT    float64 `json:"revenue_ht"`    // Revenue excluding VAT (in EUR)
	VAT          float64 `json:"vat"`           // VAT amount (in EUR)
	RevenueTTC   float64 `json:"revenue_ttc"`   // Revenue including VAT (in EUR)
	InvoiceCount int     `json:"invoice_count"` // Number of invoices
}

// AccountingResponse contains accounting data grouped by country
type AccountingResponse struct {
	Year     int                    `json:"year"`
	Month    int                    `json:"month"` // 0 = whole year
	Rows     []AccountingCountryRow `json:"rows"`
	TotalHT  float64                `json:"total_ht"`
	TotalVAT float64                `json:"total_vat"`
	TotalTTC float64                `json:"total_ttc"`
}
