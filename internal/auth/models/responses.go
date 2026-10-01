// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import "time"

// AuthResponse represents an authentication response
type AuthResponse struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"-"`
	// RefreshExpiresAt is when the refresh token stops being valid. It comes from
	// the token generator itself, so the cookie carrying the token can be given
	// the exact same lifetime instead of a second copy of a configured duration.
	// Like RefreshToken, it is deliberately `json:"-"`: the client must never see
	// it, and the browser honestly does not need to — the cookie is complete.
	RefreshExpiresAt time.Time `json:"-"`
	ExpiresIn        int64     `json:"expires_in,omitempty"`
	// SessionID is the server-issued family of the refresh cookie this response
	// belongs to. It is stable across rotation and new on every login, so clients
	// can converge on the cookie's family instead of a locally guessed nonce.
	SessionID  string `json:"session_id,omitempty"`
	User       *User  `json:"user"`
	RequireMFA bool   `json:"require_mfa,omitempty"` // True if 2FA verification is required
	TempToken  string `json:"temp_token,omitempty"`  // Temporary token for 2FA flow (5min expiry)
}

// UserResponse represents a user response (public data)
type UserResponse struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	DisplayName   string     `json:"display_name"`
	Role          string     `json:"role"`
	Locale        string     `json:"locale"`
	Timezone      string     `json:"timezone"`
	EmailVerified bool       `json:"email_verified"`
	CreatedAt     string     `json:"created_at"`
	MFAStatus     *MFAStatus `json:"mfa_status,omitempty"` // MFA/auth status
}

// ToResponse converts a User to UserResponse
func (u *User) ToResponse() *UserResponse {
	return &UserResponse{
		ID:            u.ID.String(),
		Email:         u.Email,
		DisplayName:   u.DisplayName,
		Role:          u.Role,
		Locale:        u.Locale,
		Timezone:      u.Timezone,
		EmailVerified: u.EmailVerified,
		CreatedAt:     u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		MFAStatus:     nil, // Not included by default
	}
}

// UsersListResponse represents a list of users
// Note: Uses custom field name "users" instead of generic "items" for frontend compatibility
type UsersListResponse struct {
	Users []*UserResponse `json:"users"`
	Total int             `json:"total"`
}

// MagicLinkResponse represents a magic link request response
type MagicLinkResponse struct {
	Message string `json:"message"`
}

// MagicLinkAvailableResponse represents availability check response
type MagicLinkAvailableResponse struct {
	Available bool `json:"available"`
}

// BootstrapStatusResponse is the public, pre-authentication read of the auth
// domain's capability state. Clients gate their UI on it: a frontend hides the
// registration button when RegistrationEnabled is false, and an operator — or
// Ansible — checks NeedsBootstrap before attempting POST /api/v1/auth/bootstrap.
type BootstrapStatusResponse struct {
	// NeedsBootstrap is true while the instance has no users yet, which is the
	// only time the bootstrap endpoint accepts a request.
	NeedsBootstrap bool `json:"needs_bootstrap"`
	// RegistrationEnabled mirrors the server's ALLOWED_REGISTER setting. Open
	// registration and the bootstrap flow are both valid ways to create the
	// first account; this tells the client which one the UI may offer.
	RegistrationEnabled bool `json:"registration_enabled"`
}
