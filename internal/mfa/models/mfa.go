// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

import (
	"time"

	"github.com/google/uuid"
)

// UserMFA represents a user's MFA configuration
type UserMFA struct {
	UserID          uuid.UUID
	Enabled         bool
	Secret          string
	BackupCodes     []string
	BackupCodesUsed []string
	CreatedAt       time.Time
	EnabledAt       *time.Time
}

// MFAStatusResponse is the API response for MFA status
type MFAStatusResponse struct {
	Enabled bool `json:"enabled"`
}

// TOTPSetupResponse contains the data needed to set up TOTP
type TOTPSetupResponse struct {
	Secret      string   `json:"secret"`
	QRCodeURL   string   `json:"qr_code_url"`
	BackupCodes []string `json:"backup_codes"`
}

// BackupCodesResponse contains regenerated backup codes
type BackupCodesResponse struct {
	BackupCodes []string `json:"backup_codes"`
}
