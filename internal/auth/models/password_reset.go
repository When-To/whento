// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

// ForgotPasswordRequest represents a password reset request
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email,max=255"`
}

// ResetPasswordRequest represents a password reset with token
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required,len=64"`
	NewPassword string `json:"new_password" validate:"required,strongpassword,max=72"`
}

// ForgotPasswordResponse is returned for password reset requests
// Always returns the same message to prevent email enumeration
type ForgotPasswordResponse struct {
	Message string `json:"message"`
}

// ResetPasswordResponse includes tokens for auto-login after password reset
type ResetPasswordResponse struct {
	Message      string        `json:"message"`
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	User         *UserResponse `json:"user"`
}
