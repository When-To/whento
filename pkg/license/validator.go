// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// Validate verifies a license signature using the public key
// Returns nil if the license is valid, error otherwise
func Validate(license *License, publicKey ed25519.PublicKey) error {
	if license == nil {
		return fmt.Errorf("license is nil")
	}

	// Decode the signature
	signatureBytes, err := base64.StdEncoding.DecodeString(license.Signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Construct the message that was signed
	message := constructMessage(license)

	// Verify the signature
	if !ed25519.Verify(publicKey, []byte(message), signatureBytes) {
		return fmt.Errorf("invalid license signature")
	}

	return nil
}

// ValidateSupportKey checks if a support key has the correct format
func ValidateSupportKey(supportKey string) error {
	if len(supportKey) != 17 { // SUPP-XXXX-XXXX-XXXX = 17 characters
		return fmt.Errorf("invalid support key length: expected 17, got %d", len(supportKey))
	}

	if supportKey[0:5] != "SUPP-" {
		return fmt.Errorf("invalid support key prefix: expected SUPP-, got %s", supportKey[0:5])
	}

	if supportKey[9] != '-' || supportKey[14] != '-' {
		return fmt.Errorf("invalid support key format: expected SUPP-XXXX-XXXX-XXXX")
	}

	return nil
}
