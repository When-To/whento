// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

// RegistrationOptionsResponse wraps WebAuthn registration options
type RegistrationOptionsResponse struct {
	PublicKey interface{} `json:"publicKey"`
}

// AuthenticationOptionsResponse wraps WebAuthn authentication options
type AuthenticationOptionsResponse struct {
	PublicKey interface{} `json:"publicKey"`
}

// DiscoverableAuthenticationOptionsResponse wraps WebAuthn authentication options for discoverable credentials
type DiscoverableAuthenticationOptionsResponse struct {
	PublicKey   interface{} `json:"publicKey"`
	ChallengeID string      `json:"challengeId"`
}
