// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

// AdminDisable2FAResponse represents the response when admin disables user's 2FA
type AdminDisable2FAResponse struct {
	TOTPDisabled       bool `json:"totp_disabled"`
	BackupCodesRemoved int  `json:"backup_codes_removed"`
}
