// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/whento/pkg/models"
)

// Re-export shared enums for backward compatibility
const (
	RoleUser  = string(models.RoleUser)
	RoleAdmin = string(models.RoleAdmin)
)

const (
	LocaleFR = string(models.LocaleFR)
	LocaleEN = string(models.LocaleEN)
)

// User represents a user in the system
type User struct {
	models.TimestampedEntity
	Email                         string     `json:"email"`
	PasswordHash                  string     `json:"-"`
	DisplayName                   string     `json:"display_name"`
	Role                          string     `json:"role"`
	Locale                        string     `json:"locale"`
	Timezone                      string     `json:"timezone"`
	EmailVerified                 bool       `json:"email_verified"`
	VerificationToken             *string    `json:"-"`
	VerificationTokenExpiresAt    *time.Time `json:"-"`
	PasswordResetToken            *string    `json:"-"`
	PasswordResetTokenExpiresAt   *time.Time `json:"-"`
	MagicLinkToken                *string    `json:"-"`
	MagicLinkTokenExpiresAt       *time.Time `json:"-"`
}

// RefreshToken represents a refresh token
type RefreshToken struct {
	models.Entity
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// IsAdmin checks if user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// SubscriptionInfo contains subscription details (cloud only)
type SubscriptionInfo struct {
	Plan          string `json:"plan"`
	Status        string `json:"status"`
	CalendarLimit int    `json:"calendar_limit"`
}

// UserWithSubscription extends User with subscription info (cloud builds only)
type UserWithSubscription struct {
	User
	Subscription *SubscriptionInfo `json:"subscription,omitempty"`
}

// MFAStatus contains MFA/authentication status for a user
type MFAStatus struct {
	TOTPEnabled   bool `json:"totp_enabled"`
	PasskeyCount  int  `json:"passkey_count"`
}
