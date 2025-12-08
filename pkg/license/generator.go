// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

// Generate creates and signs a new license with the given configuration
func Generate(cfg GenerateConfig, privateKey ed25519.PrivateKey) (*License, error) {
	// Validate tier
	if cfg.Tier != TierPro && cfg.Tier != TierEnterprise {
		return nil, fmt.Errorf("invalid tier: %s (must be %s or %s)", cfg.Tier, TierPro, TierEnterprise)
	}

	// Set default calendar limit if not specified
	if cfg.CalendarLimit == 0 {
		switch cfg.Tier {
		case TierPro:
			cfg.CalendarLimit = DefaultProLimit
		case TierEnterprise:
			cfg.CalendarLimit = DefaultEnterpriseLimit
		}
	}

	// Set default issuance time if not specified
	if cfg.IssuedAt.IsZero() {
		cfg.IssuedAt = time.Now()
	}

	// Generate support key
	supportKey := GenerateSupportKey()

	// Calculate support expiry based on tier and config
	var supportExpiresAt *time.Time
	supportYears := cfg.SupportYears
	if supportYears == 0 {
		// Use default support years based on tier
		switch cfg.Tier {
		case TierPro:
			supportYears = DefaultProSupportYears
		case TierEnterprise:
			supportYears = DefaultEnterpriseSupportYears
		}
	}

	if supportYears > 0 {
		supportExpiry := cfg.IssuedAt.AddDate(supportYears, 0, 0)
		supportExpiresAt = &supportExpiry
	}

	// Create license
	license := &License{
		Tier:             cfg.Tier,
		CalendarLimit:    cfg.CalendarLimit,
		IssuedTo:         cfg.IssuedTo,
		IssuedAt:         cfg.IssuedAt,
		SupportKey:       supportKey,
		SupportExpiresAt: supportExpiresAt,
	}

	// Sign the license
	signature, err := Sign(license, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign license: %w", err)
	}

	license.Signature = signature

	return license, nil
}

// Sign signs a license and returns the base64-encoded signature
func Sign(license *License, privateKey ed25519.PrivateKey) (string, error) {
	// Construct message to sign
	message := constructMessage(license)

	// Sign the message
	signature := ed25519.Sign(privateKey, []byte(message))

	// Return base64-encoded signature
	return base64.StdEncoding.EncodeToString(signature), nil
}

// constructMessage creates the canonical message string for signing
func constructMessage(license *License) string {
	supportExpiresAtStr := "none"
	if license.SupportExpiresAt != nil {
		supportExpiresAtStr = license.SupportExpiresAt.Format(time.RFC3339)
	}

	return fmt.Sprintf("%s|%d|%s|%s|%s|%s",
		license.Tier,
		license.CalendarLimit,
		license.IssuedTo,
		license.IssuedAt.Format(time.RFC3339),
		license.SupportKey,
		supportExpiresAtStr,
	)
}

// GenerateKeyPair generates a new Ed25519 key pair for license signing
// Returns the public key, private key, and any error
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate key pair: %w", err)
	}
	return publicKey, privateKey, nil
}

// EncodeKeyPair encodes the key pair to base64 strings for storage
func EncodeKeyPair(publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) (publicKeyB64, privateKeyB64 string) {
	publicKeyB64 = base64.StdEncoding.EncodeToString(publicKey)
	privateKeyB64 = base64.StdEncoding.EncodeToString(privateKey)
	return
}

// DecodePrivateKey decodes a base64-encoded private key
func DecodePrivateKey(privateKeyB64 string) (ed25519.PrivateKey, error) {
	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %w", err)
	}

	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key size: expected %d, got %d", ed25519.PrivateKeySize, len(privateKeyBytes))
	}

	return ed25519.PrivateKey(privateKeyBytes), nil
}

// GenerateSupportKey generates a random support key in format SUPP-XXXX-XXXX-XXXX
func GenerateSupportKey() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Removed ambiguous chars (0, O, I, 1)
	const keyLength = 12                               // 3 groups of 4 characters

	key := make([]byte, keyLength)
	for i := range key {
		randomByte := make([]byte, 1)
		rand.Read(randomByte)
		key[i] = charset[int(randomByte[0])%len(charset)]
	}

	// Format as SUPP-XXXX-XXXX-XXXX
	return fmt.Sprintf("SUPP-%s-%s-%s",
		string(key[0:4]),
		string(key[4:8]),
		string(key[8:12]),
	)
}

// Renew creates a renewed license with extended support period
func Renew(existing *License, supportYears int, privateKey ed25519.PrivateKey) (*License, error) {
	// Validate tier
	if existing.Tier != TierPro && existing.Tier != TierEnterprise {
		return nil, fmt.Errorf("cannot renew license for tier: %s", existing.Tier)
	}

	// Set default support years if not specified
	if supportYears == 0 {
		switch existing.Tier {
		case TierPro:
			supportYears = DefaultProSupportYears
		case TierEnterprise:
			supportYears = DefaultEnterpriseSupportYears
		}
	}

	// Validate support years
	if supportYears < 0 || supportYears > 2 {
		return nil, fmt.Errorf("invalid support years: %d (must be 0, 1, or 2)", supportYears)
	}

	// Generate new support key
	newSupportKey := GenerateSupportKey()

	// Calculate new support expiry
	now := time.Now()
	supportExpiry := now.AddDate(supportYears, 0, 0)

	// Create renewed license
	renewed := &License{
		Tier:             existing.Tier,
		CalendarLimit:    existing.CalendarLimit,
		IssuedTo:         existing.IssuedTo,
		IssuedAt:         now, // New issuance date
		SupportKey:       newSupportKey,
		SupportExpiresAt: &supportExpiry,
	}

	// Sign the renewed license
	signature, err := Sign(renewed, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign renewed license: %w", err)
	}

	renewed.Signature = signature

	return renewed, nil
}
