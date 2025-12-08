// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

// FinishSetupRequest represents the request to enable MFA
type FinishSetupRequest struct {
	Code string `json:"code" validate:"required,len=6,numeric"`
}

// VerifyMFARequest represents the request to verify MFA code during login
type VerifyMFARequest struct {
	TempToken string `json:"temp_token" validate:"required"`
	Code      string `json:"code" validate:"required,min=6,max=8"` // 6 for TOTP, 8 for backup codes
}

// DisableMFARequest represents the request to disable MFA
// No fields needed - user is already authenticated via JWT
type DisableMFARequest struct {
}
