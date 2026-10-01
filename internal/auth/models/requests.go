// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

// RegisterRequest represents a registration request
//
// Password carries both `max=72` (rune ceiling, read by the swagger generator
// into maxLength) and `maxbytes=72` (the real 72-byte bcrypt ceiling, in UTF-8
// bytes). The two are redundant at runtime — bytes >= runes, so maxbytes is
// always the binding one — but `max=72` is what keeps the API contract showing
// the upper bound through a clean swagger regeneration.
type RegisterRequest struct {
	Email       string `json:"email" validate:"required,email,max=255"`
	Password    string `json:"password" validate:"required,strongpassword,max=72,maxbytes=72"`
	DisplayName string `json:"display_name" validate:"required,min=2,max=100"`
	Locale      string `json:"locale" validate:"omitempty,oneof=fr en"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// RefreshRequest represents a token refresh request
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// BootstrapRequest creates the first (administrator) account of an instance
// that has no users yet. The boot key is the proof that whoever is submitting
// it has access to the server's logs (or configured BOOTSTRAP_KEY); the account
// it creates is always the admin, because "first user is the administrator" is
// how a closed instance stops needing open registration.
type BootstrapRequest struct {
	BootKey  string `json:"boot_key" validate:"required,min=16,max=256"`
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,strongpassword,max=72,maxbytes=72"`
	// DisplayName and Locale match RegisterRequest; see its comment about the
	// paired `max`/`maxbytes` password tags.
	DisplayName string `json:"display_name" validate:"required,min=2,max=100"`
	Locale      string `json:"locale" validate:"omitempty,oneof=fr en"`
}

// UpdateProfileRequest represents a profile update request
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name,omitempty" validate:"omitempty,min=2,max=100"`
	Locale      *string `json:"locale,omitempty" validate:"omitempty,oneof=fr en"`
	Timezone    *string `json:"timezone,omitempty" validate:"omitempty,timezone"`
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,strongpassword,max=72,maxbytes=72"`
}

// UpdateRoleRequest represents a role update request (admin only)
type UpdateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}

// MagicLinkRequest represents a magic link request
type MagicLinkRequest struct {
	Email string `json:"email" validate:"required,email"`
}
