// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"

	authRepo "github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/mfa/models"
	"github.com/whento/whento/internal/mfa/repository"
)

var (
	ErrMFANotFound        = errors.New("MFA not configured")
	ErrMFAAlreadyEnabled  = errors.New("MFA is already enabled")
	ErrMFANotEnabled      = errors.New("MFA is not enabled")
	ErrInvalidCode        = errors.New("invalid verification code")
	ErrUserNotFound       = errors.New("user not found")
	ErrAllBackupCodesUsed = errors.New("all backup codes have been used")
)

// MFAService handles MFA business logic
type MFAService struct {
	repo       *repository.MFARepository
	userRepo   *authRepo.UserRepository
	issuer     string
	period     uint
	digits     otp.Digits
	bcryptCost int
	logger     *slog.Logger
}

// NewMFAService creates a new MFA service
func NewMFAService(
	repo *repository.MFARepository,
	userRepo *authRepo.UserRepository,
	cfg *config.Config,
	logger *slog.Logger,
) *MFAService {
	return &MFAService{
		repo:       repo,
		userRepo:   userRepo,
		issuer:     cfg.TOTPIssuer,
		period:     cfg.TOTPPeriod,
		digits:     otp.Digits(cfg.TOTPDigits),
		bcryptCost: cfg.BcryptCost,
		logger:     logger,
	}
}

// BeginSetup generates TOTP secret and QR code
func (s *MFAService) BeginSetup(ctx context.Context, userID uuid.UUID) (*models.TOTPSetupResponse, error) {
	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Check if MFA is already enabled
	mfa, err := s.repo.GetByUserID(ctx, userID)
	if err != nil && !errors.Is(err, repository.ErrMFANotFound) {
		return nil, fmt.Errorf("failed to check MFA status: %w", err)
	}

	if mfa != nil && mfa.Enabled {
		return nil, ErrMFAAlreadyEnabled
	}

	// Generate TOTP secret
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.issuer,
		AccountName: user.Email,
		Period:      s.period,
		Digits:      s.digits,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// Generate QR code
	// Encode QR code as PNG
	png, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to encode QR code: %w", err)
	}

	// Convert to base64 data URL
	qrCodeDataURL := fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(png))

	// Generate backup codes
	backupCodes, err := s.generateBackupCodes(10)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// Hash backup codes before storing
	hashedBackupCodes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), s.bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash backup code: %w", err)
		}
		hashedBackupCodes[i] = string(hash)
	}

	// Create or update MFA configuration (not enabled yet)
	now := time.Now()
	userMFA := &models.UserMFA{
		UserID:          userID,
		Enabled:         false,
		Secret:          key.Secret(),
		BackupCodes:     hashedBackupCodes,
		BackupCodesUsed: []string{},
		CreatedAt:       now,
		EnabledAt:       nil,
	}

	if mfa == nil {
		// Create new MFA config
		if err := s.repo.Create(ctx, userMFA); err != nil {
			return nil, fmt.Errorf("failed to create MFA config: %w", err)
		}
	} else {
		// Update existing MFA config
		if err := s.repo.Update(ctx, userMFA); err != nil {
			return nil, fmt.Errorf("failed to update MFA config: %w", err)
		}
	}

	return &models.TOTPSetupResponse{
		Secret:      key.Secret(),
		QRCodeURL:   qrCodeDataURL,
		BackupCodes: backupCodes,
	}, nil
}

// FinishSetup verifies TOTP code and enables MFA
func (s *MFAService) FinishSetup(ctx context.Context, userID uuid.UUID, code string) error {
	// Get MFA configuration
	mfa, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrMFANotFound) {
			return errors.New("MFA setup not started")
		}
		return fmt.Errorf("failed to get MFA config: %w", err)
	}

	// Verify TOTP code
	valid := totp.Validate(code, mfa.Secret)
	if !valid {
		return ErrInvalidCode
	}

	// Enable MFA
	now := time.Now()
	mfa.Enabled = true
	mfa.EnabledAt = &now

	if err := s.repo.Update(ctx, mfa); err != nil {
		return fmt.Errorf("failed to enable MFA: %w", err)
	}

	return nil
}

// VerifyCode verifies TOTP code or backup code
func (s *MFAService) VerifyCode(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	// Get MFA configuration
	mfa, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrMFANotFound) {
			return false, ErrMFANotFound
		}
		return false, fmt.Errorf("failed to get MFA config: %w", err)
	}

	if !mfa.Enabled {
		return false, ErrMFANotEnabled
	}

	// Try TOTP code first (6 digits)
	if len(code) == 6 {
		valid := totp.Validate(code, mfa.Secret)
		return valid, nil
	}

	// Try backup code (8 characters)
	if len(code) == 8 {
		return s.verifyBackupCode(ctx, mfa, code)
	}

	return false, nil
}

// verifyBackupCode verifies and marks a backup code as used
func (s *MFAService) verifyBackupCode(ctx context.Context, mfa *models.UserMFA, code string) (bool, error) {
	// Check each backup code
	for _, hashedCode := range mfa.BackupCodes {
		// Skip if already used
		alreadyUsed := false
		for _, usedCode := range mfa.BackupCodesUsed {
			if usedCode == hashedCode {
				alreadyUsed = true
				break
			}
		}
		if alreadyUsed {
			continue
		}

		// Verify backup code
		err := bcrypt.CompareHashAndPassword([]byte(hashedCode), []byte(code))
		if err == nil {
			// Mark as used
			mfa.BackupCodesUsed = append(mfa.BackupCodesUsed, hashedCode)

			// Update in database
			if err := s.repo.Update(ctx, mfa); err != nil {
				s.logger.Error("Failed to update backup code usage", "error", err)
				// Don't fail verification if update fails
			}

			return true, nil
		}
	}

	return false, nil
}

// Disable2FA disables MFA for the authenticated user
func (s *MFAService) Disable2FA(ctx context.Context, userID uuid.UUID) error {
	// Check if MFA exists
	mfa, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrMFANotFound) {
			return ErrMFANotEnabled
		}
		return fmt.Errorf("failed to get MFA config: %w", err)
	}

	if !mfa.Enabled {
		return ErrMFANotEnabled
	}

	// Delete MFA configuration
	if err := s.repo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("failed to disable MFA: %w", err)
	}

	return nil
}

// RegenerateBackupCodes generates new backup codes
func (s *MFAService) RegenerateBackupCodes(ctx context.Context, userID uuid.UUID) ([]string, error) {
	// Get MFA configuration
	mfa, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrMFANotFound) {
			return nil, ErrMFANotFound
		}
		return nil, fmt.Errorf("failed to get MFA config: %w", err)
	}

	if !mfa.Enabled {
		return nil, ErrMFANotEnabled
	}

	// Generate new backup codes
	backupCodes, err := s.generateBackupCodes(10)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// Hash backup codes
	hashedBackupCodes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), s.bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash backup code: %w", err)
		}
		hashedBackupCodes[i] = string(hash)
	}

	// Update MFA configuration
	mfa.BackupCodes = hashedBackupCodes
	mfa.BackupCodesUsed = []string{}

	if err := s.repo.Update(ctx, mfa); err != nil {
		return nil, fmt.Errorf("failed to update backup codes: %w", err)
	}

	return backupCodes, nil
}

// GetStatus returns MFA status for a user
func (s *MFAService) GetStatus(ctx context.Context, userID uuid.UUID) (*models.MFAStatusResponse, error) {
	mfa, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrMFANotFound) {
			return &models.MFAStatusResponse{Enabled: false}, nil
		}
		return nil, fmt.Errorf("failed to get MFA status: %w", err)
	}

	return &models.MFAStatusResponse{Enabled: mfa.Enabled}, nil
}

// generateBackupCodes generates random alphanumeric backup codes
func (s *MFAService) generateBackupCodes(count int) ([]string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Removed ambiguous characters
	const codeLength = 8

	codes := make([]string, count)
	for i := 0; i < count; i++ {
		code := make([]byte, codeLength)
		for j := 0; j < codeLength; j++ {
			num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return nil, fmt.Errorf("failed to generate random number: %w", err)
			}
			code[j] = charset[num.Int64()]
		}
		codes[i] = string(code)
	}

	return codes, nil
}

// AdminDisable2FA disables TOTP for a user (admin action, no password required)
func (s *MFAService) AdminDisable2FA(ctx context.Context, targetUserID, adminUserID uuid.UUID) (*models.AdminDisable2FAResponse, error) {
	// Verify target user exists
	_, err := s.userRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Get MFA configuration
	mfa, err := s.repo.GetByUserID(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, repository.ErrMFANotFound) {
			return nil, ErrMFANotEnabled
		}
		return nil, fmt.Errorf("failed to get MFA config: %w", err)
	}

	if !mfa.Enabled {
		return nil, ErrMFANotEnabled
	}

	// Count backup codes before deletion
	backupCodesCount := len(mfa.BackupCodes)

	// Delete MFA configuration
	if err := s.repo.Delete(ctx, targetUserID); err != nil {
		return nil, fmt.Errorf("failed to disable MFA: %w", err)
	}

	// Log admin action
	s.logger.Info("admin disabled user 2FA",
		"admin_id", adminUserID.String(),
		"user_id", targetUserID.String(),
		"backup_codes_removed", backupCodesCount,
	)

	return &models.AdminDisable2FAResponse{
		TOTPDisabled:       true,
		BackupCodesRemoved: backupCodesCount,
	}, nil
}
