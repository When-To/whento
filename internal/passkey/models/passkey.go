// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package models

import (
	"time"

	"github.com/google/uuid"
)

// Passkey represents a WebAuthn credential for passwordless authentication
type Passkey struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Name            string
	CredentialID    []byte
	PublicKey       []byte
	AAGUID          uuid.UUID
	SignCount       int64
	Transports      []string
	BackupEligible  bool // Indicates if credential can be backed up (e.g., cloud passkey)
	BackupState     bool // Indicates if credential is currently backed up
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}

// PasskeyResponse is the API response for a passkey
type PasskeyResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// ToResponse converts a Passkey to PasskeyResponse
func (p *Passkey) ToResponse() *PasskeyResponse {
	return &PasskeyResponse{
		ID:         p.ID.String(),
		Name:       p.Name,
		CreatedAt:  p.CreatedAt,
		LastUsedAt: p.LastUsedAt,
	}
}
