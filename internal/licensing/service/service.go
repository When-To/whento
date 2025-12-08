// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/whento/whento/internal/licensing/models"
	"github.com/whento/whento/internal/licensing/repository"
)

// Service handles licensing business logic
// License is loaded from DB at startup and kept in RAM for performance
// Signature is verified on every load - DB columns are only for indexing
type Service struct {
	repo      *repository.LicenseRepository
	publicKey ed25519.PublicKey
	log       *slog.Logger

	// In-memory license cache
	activeLicense *models.LicensePayload
	mu            sync.RWMutex // Protects activeLicense
}

// Hardcoded public key for license verification
const LicensePublicKeyBase64 = "Qb7v1/Iy0BIehwam7ALcBHo0X6g8un7WpQke79IPz9I="

// Config holds the configuration for the licensing service
type Config struct {
	// No configuration needed - public key is hardcoded for security
}

// New creates a new licensing service
func New(repo *repository.LicenseRepository, cfg Config, log *slog.Logger) (*Service, error) {
	// Decode hardcoded public key
	publicKeyBytes, err := base64.StdEncoding.DecodeString(LicensePublicKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hardcoded public key: %w", err)
	}

	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid hardcoded public key size: expected %d, got %d", ed25519.PublicKeySize, len(publicKeyBytes))
	}

	publicKey := ed25519.PublicKey(publicKeyBytes)
	log.Info("License service initialized with hardcoded public key")

	return &Service{
		repo:      repo,
		publicKey: publicKey,
		log:       log,
	}, nil
}

// LoadLicenseFromDB loads the license from the database into RAM
// This should be called at application startup
func (s *Service) LoadLicenseFromDB(ctx context.Context) error {
	license, err := s.repo.GetActive(ctx)
	if err != nil {
		// No license found - use community tier
		s.log.Info("No active license found, using Community tier")
		return nil
	}

	// Verify signature before loading into RAM
	if !s.verifySignature(&license.LicenseData) {
		s.log.Error("License signature verification failed", "license_id", license.ID)
		return fmt.Errorf("license signature verification failed - possible tampering detected")
	}

	// Load into RAM (self-hosted licenses are perpetual, no expiration check)
	s.mu.Lock()
	s.activeLicense = &license.LicenseData
	s.mu.Unlock()

	s.log.Info("License loaded into RAM",
		"tier", license.LicenseData.Tier,
		"calendar_limit", license.LicenseData.CalendarLimit,
		"issued_to", license.LicenseData.IssuedTo,
	)

	return nil
}

// GetActiveLicense retrieves the currently active license from RAM
// Returns community tier if no license is active
// Self-hosted licenses are perpetual (no expiration check)
func (s *Service) GetActiveLicense() *models.LicensePayload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.activeLicense == nil {
		// Return community tier
		return &models.LicensePayload{
			Tier:          string(models.TierCommunity),
			CalendarLimit: 30,
			IssuedTo:      "Community",
			IssuedAt:      time.Now(),
		}
	}

	return s.activeLicense
}

// GetServerCalendarLimit returns the server-wide calendar limit
func (s *Service) GetServerCalendarLimit() int {
	license := s.GetActiveLicense()
	return license.CalendarLimit
}

// ActivateLicense validates and activates a new license
func (s *Service) ActivateLicense(ctx context.Context, licenseKey string) error {
	// Check if public key is configured
	if s.publicKey == nil {
		return fmt.Errorf("license activation unavailable: no public key configured (set LICENSE_PUBLIC_KEY environment variable)")
	}

	// Parse the license payload
	var payload models.LicensePayload
	if err := json.Unmarshal([]byte(licenseKey), &payload); err != nil {
		return fmt.Errorf("invalid license format: %w", err)
	}

	// Verify signature
	if !s.verifySignature(&payload) {
		return fmt.Errorf("invalid license signature")
	}

	// Self-hosted licenses are perpetual (no expiration check)

	// Check if this license is already activated
	existingLicense, err := s.repo.GetActive(ctx)
	if err == nil {
		// Same server, update license
		s.log.Info("Updating existing license", "license_id", existingLicense.ID)
	}

	// Create new license record
	license := &models.License{
		LicenseData: payload,
		ActivatedAt: time.Now(),
	}

	if err := s.repo.Create(ctx, license); err != nil {
		return fmt.Errorf("failed to save license: %w", err)
	}

	// Load into RAM
	s.mu.Lock()
	s.activeLicense = &payload
	s.mu.Unlock()

	s.log.Info("License activated successfully",
		"tier", payload.Tier,
		"calendar_limit", payload.CalendarLimit,
		"issued_to", payload.IssuedTo,
	)

	return nil
}

// ReloadLicense reloads the license from the database
// Useful for admin operations
func (s *Service) ReloadLicense(ctx context.Context) error {
	return s.LoadLicenseFromDB(ctx)
}

// verifySignature verifies the Ed25519 signature of a license payload
func (s *Service) verifySignature(payload *models.LicensePayload) bool {
	// No public key configured - cannot verify signatures
	if s.publicKey == nil {
		s.log.Error("Cannot verify license signature: no public key configured")
		return false
	}

	// Reconstruct the signed message
	message := s.constructMessage(payload)

	// Decode signature
	signatureBytes, err := base64.StdEncoding.DecodeString(payload.Signature)
	if err != nil {
		s.log.Error("Failed to decode signature", "error", err)
		return false
	}

	// Verify signature
	return ed25519.Verify(s.publicKey, []byte(message), signatureBytes)
}

// constructMessage constructs the canonical message format for signing/verification
// Format: tier|calendar_limit|issued_to|issued_at|support_key|support_expires_at
func (s *Service) constructMessage(payload *models.LicensePayload) string {
	supportExpiresAtStr := "none"
	if payload.SupportExpiresAt != nil {
		supportExpiresAtStr = payload.SupportExpiresAt.Format(time.RFC3339)
	}

	return fmt.Sprintf("%s|%d|%s|%s|%s|%s",
		payload.Tier,
		payload.CalendarLimit,
		payload.IssuedTo,
		payload.IssuedAt.Format(time.RFC3339),
		payload.SupportKey,
		supportExpiresAtStr,
	)
}

// GetLicenseInfo returns detailed information about the current license
func (s *Service) GetLicenseInfo(ctx context.Context) (*models.LicenseResponse, error) {
	license := s.GetActiveLicense()

	tierConfig := models.GetTierConfig(license.GetTier())

	// Remove signature from response for security
	licenseWithoutSig := *license
	licenseWithoutSig.Signature = ""

	return &models.LicenseResponse{
		License:       &licenseWithoutSig,
		TierConfig:    tierConfig,
		IsActive:      true, // Self-hosted licenses are perpetual
		SupportActive: license.IsSupportActive(),
	}, nil
}

// RemoveLicense removes the active license (reverts to community tier)
func (s *Service) RemoveLicense(ctx context.Context) error {
	license, err := s.repo.GetActive(ctx)
	if err != nil {
		return fmt.Errorf("no active license found")
	}

	if err := s.repo.Delete(ctx, license.ID); err != nil {
		return fmt.Errorf("failed to remove license: %w", err)
	}

	// Clear from RAM
	s.mu.Lock()
	s.activeLicense = nil
	s.mu.Unlock()

	s.log.Info("License removed, reverted to Community tier", "license_id", license.ID)

	return nil
}
