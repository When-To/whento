// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import "time"

// ForgotPasswordRequest represents a password reset request
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
}

// ResetPasswordRequest represents a password reset with token
//
// NewPassword carries both `max=72` (rune ceiling, read by the swagger
// generator into maxLength) and `maxbytes=72` (the real 72-byte bcrypt ceiling,
// in UTF-8 bytes). They are redundant at runtime — bytes >= runes, so maxbytes
// is always the binding one — but `max=72` is what keeps the API contract
// showing the upper bound through a clean swagger regeneration.
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required,len=64"`
	NewPassword string `json:"new_password" validate:"required,strongpassword,max=72,maxbytes=72"`
}

// ForgotPasswordResponse is returned for password reset requests
// Always returns the same message to prevent email enumeration
type ForgotPasswordResponse struct {
	Message string `json:"message"`
}

// ResetPasswordResponse includes tokens for auto-login after password reset.
//
// RefreshToken leaves over the httpOnly cookie the handler sets, never over JSON —
// `json:"-"` here is what keeps a seven-day credential out of reach of page scripts,
// matching AuthResponse. Serialising it made reset-password the one endpoint that
// handed its refresh token to anything that could read a response body.
//
// RefreshExpiresAt carries the token's real lifetime to the cookie writer; it is
// also `json:"-"`, for the same reason.
type ResetPasswordResponse struct {
	Message          string    `json:"message"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"-"`
	RefreshExpiresAt time.Time `json:"-"`
	// SessionID is the server family of the auto-login cookie, same contract as AuthResponse.
	SessionID string        `json:"session_id,omitempty"`
	User      *UserResponse `json:"user"`
}
