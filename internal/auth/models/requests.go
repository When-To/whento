// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Email       string `json:"email" validate:"required,email,max=255"`
	Password    string `json:"password" validate:"required,strongpassword,max=72"`
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

// UpdateProfileRequest represents a profile update request
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name,omitempty" validate:"omitempty,min=2,max=100"`
	Locale      *string `json:"locale,omitempty" validate:"omitempty,oneof=fr en"`
	Timezone    *string `json:"timezone,omitempty" validate:"omitempty,timezone"`
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,strongpassword,max=72"`
}

// UpdateRoleRequest represents a role update request (admin only)
type UpdateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}

// MagicLinkRequest represents a magic link request
type MagicLinkRequest struct {
	Email string `json:"email" validate:"required,email"`
}
