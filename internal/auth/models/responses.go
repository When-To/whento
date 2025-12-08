// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

// AuthResponse represents an authentication response
type AuthResponse struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	User         *User  `json:"user"`
	RequireMFA   bool   `json:"require_mfa,omitempty"` // True if 2FA verification is required
	TempToken    string `json:"temp_token,omitempty"`  // Temporary token for 2FA flow (5min expiry)
}

// UserResponse represents a user response (public data)
type UserResponse struct {
	ID            string            `json:"id"`
	Email         string            `json:"email"`
	DisplayName   string            `json:"display_name"`
	Role          string            `json:"role"`
	Locale        string            `json:"locale"`
	Timezone      string            `json:"timezone"`
	EmailVerified bool              `json:"email_verified"`
	CreatedAt     string            `json:"created_at"`
	Subscription  *SubscriptionInfo `json:"subscription,omitempty"` // Cloud only
	MFAStatus     *MFAStatus        `json:"mfa_status,omitempty"`   // MFA/auth status
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
		Subscription:  nil, // Not included by default
		MFAStatus:     nil, // Not included by default
	}
}

// ToResponseWithSubscription converts UserWithSubscription to UserResponse (cloud only)
func (u *UserWithSubscription) ToResponseWithSubscription() *UserResponse {
	resp := u.User.ToResponse()
	resp.Subscription = u.Subscription
	return resp
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
