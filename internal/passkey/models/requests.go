// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

// RenamePasskeyRequest represents the request to rename a passkey
type RenamePasskeyRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}
