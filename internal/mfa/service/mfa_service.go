// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"

	authModels "github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/mfa/models"
	"github.com/whento/whento/internal/mfa/repository"
)

var (
	ErrMFANotFound       = errors.New("MFA not configured")
	ErrMFAAlreadyEnabled = errors.New("MFA is already enabled")
	ErrMFANotEnabled     = errors.New("MFA is not enabled")
	ErrInvalidCode       = errors.New("invalid verification code")
	ErrUserNotFound      = errors.New("user not found")
)

// MFAStore is the slice of the MFA repository this service needs.
//
// Declared here rather than taking *repository.MFARepository directly so the service
// can be exercised without a database — it was at 0% for exactly that reason. Go
// interfaces are structural, so the concrete repository satisfies it and no call site
// changes.
type MFAStore interface {
	Create(ctx context.Context, mfa *models.UserMFA) error
	GetByUserID(ctx context.Context, userID uuid.UUID) (*models.UserMFA, error)
	Update(ctx context.Context, mfa *models.UserMFA) error
	ConsumeBackupCode(ctx context.Context, userID uuid.UUID, hash string) (bool, error)
	ConsumeTOTPStep(ctx context.Context, userID uuid.UUID, secret string, step int64) (bool, error)
	EnableAndRevokeSessions(ctx context.Context, userID uuid.UUID, secret string, enabledAt time.Time) error
	Delete(ctx context.Context, userID uuid.UUID) error
}

// UserLookup is the one thing this service asks of the user repository.
type UserLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*authModels.User, error)
}

// MFAService handles MFA business logic
type MFAService struct {
	repo       MFAStore
	userRepo   UserLookup
	issuer     string
	bcryptCost int
	logger     *slog.Logger
}

// NewMFAService creates a new MFA service
func NewMFAService(
	repo MFAStore,
	userRepo UserLookup,
	cfg *config.Config,
	logger *slog.Logger,
) *MFAService {
	return &MFAService{
		repo:       repo,
		userRepo:   userRepo,
		issuer:     cfg.TOTPIssuer,
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

	// Generate TOTP secret (totpPeriod seconds, six digits: the values
	// FinishSetup and VerifyCode check against)
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.issuer,
		AccountName: user.Email,
		Period:      totpPeriod,
		Digits:      otp.DigitsSix,
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
	backupCodes, hashedBackupCodes, err := s.generateBackupCodes(10)
	if err != nil {
		return nil, err
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

	if mfa.Enabled {
		return ErrMFAAlreadyEnabled
	}
	// Enable only the secret whose code was verified, together with the session fence.
	if err := s.repo.EnableAndRevokeSessions(ctx, userID, mfa.Secret, time.Now()); err != nil {
		return fmt.Errorf("failed to enable MFA and revoke sessions: %w", err)
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
		step, ok := matchTOTPStep(code, mfa.Secret, time.Now())
		if !ok {
			return false, nil
		}
		// Each step is accepted once, and never at or behind one already accepted,
		// so an observed code cannot be replayed inside its validity window.
		return s.repo.ConsumeTOTPStep(ctx, userID, mfa.Secret, step)
	}

	// Try backup code (8 characters)
	if len(code) == 8 {
		return s.verifyBackupCode(ctx, userID, mfa, code)
	}

	return false, nil
}

// totpPeriod and totpSkew are the defaults totp.Validate applies, unrolled so
// the step a code matched is known.
const (
	totpPeriod = 30
	totpSkew   = 1
)

// matchTOTPStep returns the time step code is valid for, within totpSkew steps
// of now.
func matchTOTPStep(code, secret string, now time.Time) (int64, bool) {
	current := now.Unix() / totpPeriod
	for step := current - totpSkew; step <= current+totpSkew; step++ {
		ok, _ := totp.ValidateCustom(code, secret, time.Unix(step*totpPeriod, 0), totp.ValidateOpts{
			Period:    totpPeriod,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		})
		if ok {
			return step, true
		}
	}
	return 0, false
}

// verifyBackupCode verifies and marks a backup code as used
func (s *MFAService) verifyBackupCode(ctx context.Context, userID uuid.UUID, mfa *models.UserMFA, code string) (bool, error) {
	// Check each backup code
	for _, hashedCode := range mfa.BackupCodes {
		// Skip if already used
		if slices.Contains(mfa.BackupCodesUsed, hashedCode) {
			continue
		}

		// Verify backup code
		err := bcrypt.CompareHashAndPassword([]byte(hashedCode), []byte(code))
		if err == nil {
			// Spent atomically in the store: a concurrent request with the same
			// code that got there first, or a failed write, is a rejection.
			return s.repo.ConsumeBackupCode(ctx, userID, hashedCode)
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
	backupCodes, hashedBackupCodes, err := s.generateBackupCodes(10)
	if err != nil {
		return nil, err
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

// generateBackupCodes generates random alphanumeric backup codes, returning
// them in plain text (shown once to the user) and bcrypt-hashed (stored).
func (s *MFAService) generateBackupCodes(count int) (plain, hashed []string, err error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Removed ambiguous characters
	const codeLength = 8

	plain = make([]string, count)
	hashed = make([]string, count)
	for i := 0; i < count; i++ {
		code := make([]byte, codeLength)
		for j := 0; j < codeLength; j++ {
			num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to generate backup codes: %w", err)
			}
			code[j] = charset[num.Int64()]
		}
		plain[i] = string(code)
		hash, err := bcrypt.GenerateFromPassword(code, s.bcryptCost)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to hash backup code: %w", err)
		}
		hashed[i] = string(hash)
	}

	return plain, hashed, nil
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
