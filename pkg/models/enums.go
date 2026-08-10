// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

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
