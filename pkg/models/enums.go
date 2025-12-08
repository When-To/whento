// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

// Role represents user roles in the system
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// IsValid checks if the role is valid
func (r Role) IsValid() bool {
	return r == RoleUser || r == RoleAdmin
}

// String returns the string representation of the role
func (r Role) String() string {
	return string(r)
}

// Locale represents supported locales
type Locale string

const (
	LocaleFR Locale = "fr"
	LocaleEN Locale = "en"
)

// IsValid checks if the locale is valid
func (l Locale) IsValid() bool {
	return l == LocaleFR || l == LocaleEN
}

// String returns the string representation of the locale
func (l Locale) String() string {
	return string(l)
}

// SubscriptionPlan represents available subscription tiers (cloud mode)
type SubscriptionPlan string

const (
	PlanFree  SubscriptionPlan = "free"
	PlanPro   SubscriptionPlan = "pro"
	PlanPower SubscriptionPlan = "power"
)

// IsValid checks if the subscription plan is valid
func (p SubscriptionPlan) IsValid() bool {
	return p == PlanFree || p == PlanPro || p == PlanPower
}

// String returns the string representation of the plan
func (p SubscriptionPlan) String() string {
	return string(p)
}

// SubscriptionStatus represents the current state of a subscription (cloud mode)
type SubscriptionStatus string

const (
	StatusActive     SubscriptionStatus = "active"
	StatusCanceled   SubscriptionStatus = "canceled"
	StatusPastDue    SubscriptionStatus = "past_due"
	StatusIncomplete SubscriptionStatus = "incomplete"
	StatusTrialing   SubscriptionStatus = "trialing"
)

// IsValid checks if the subscription status is valid
func (s SubscriptionStatus) IsValid() bool {
	switch s {
	case StatusActive, StatusCanceled, StatusPastDue, StatusIncomplete, StatusTrialing:
		return true
	default:
		return false
	}
}

// String returns the string representation of the status
func (s SubscriptionStatus) String() string {
	return string(s)
}

// LicenseTier represents available license tiers (self-hosted mode)
type LicenseTier string

const (
	TierCommunity  LicenseTier = "community"
	TierPro        LicenseTier = "pro"
	TierEnterprise LicenseTier = "enterprise"
)

// IsValid checks if the license tier is valid
func (t LicenseTier) IsValid() bool {
	return t == TierCommunity || t == TierPro || t == TierEnterprise
}

// String returns the string representation of the tier
func (t LicenseTier) String() string {
	return string(t)
}

// OrderStatus represents the current state of an order (ecommerce)
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusCompleted OrderStatus = "completed"
	OrderStatusRefunded  OrderStatus = "refunded"
	OrderStatusFailed    OrderStatus = "failed"
)

// IsValid checks if the order status is valid
func (o OrderStatus) IsValid() bool {
	switch o {
	case OrderStatusPending, OrderStatusCompleted, OrderStatusRefunded, OrderStatusFailed:
		return true
	default:
		return false
	}
}

// String returns the string representation of the order status
func (o OrderStatus) String() string {
	return string(o)
}

// HolidaysPolicy represents how holidays are handled in calendars
type HolidaysPolicy string

const (
	HolidaysPolicyIgnore HolidaysPolicy = "ignore"
	HolidaysPolicyAllow  HolidaysPolicy = "allow"
	HolidaysPolicyBlock  HolidaysPolicy = "block"
)

// IsValid checks if the holidays policy is valid
func (h HolidaysPolicy) IsValid() bool {
	return h == HolidaysPolicyIgnore || h == HolidaysPolicyAllow || h == HolidaysPolicyBlock
}

// String returns the string representation of the holidays policy
func (h HolidaysPolicy) String() string {
	return string(h)
}
