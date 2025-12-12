// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/email"
	"github.com/whento/pkg/jwt"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/config"
)

//go:embed templates/password_reset.html
var passwordResetTemplate string

//go:embed templates/locales/password_reset.json
var passwordResetTranslationsJSON string

const (
	passwordResetTokenExpiry = 1 * time.Hour
	resetTokenLength         = 32 // bytes (64 hex chars)
)

// PasswordResetService handles password reset business logic
type PasswordResetService struct {
	userRepo          *repository.UserRepository
	tokenRepo         *repository.TokenRepository
	emailService      *email.Service
	jwtManager        *jwt.Manager
	cfg               *config.Config
	logger            *slog.Logger
	bcryptCost        int
	resetTemplate     *template.Template
	resetTranslations map[string]map[string]string
}

// NewPasswordResetService creates a new password reset service
func NewPasswordResetService(
	userRepo *repository.UserRepository,
	tokenRepo *repository.TokenRepository,
	emailService *email.Service,
	jwtManager *jwt.Manager,
	cfg *config.Config,
	logger *slog.Logger,
	bcryptCost int,
) *PasswordResetService {
	// Parse password reset template
	resetTmpl, err := template.New("password_reset").Parse(passwordResetTemplate)
	if err != nil {
		logger.Error("Failed to parse password reset template", "error", err)
	}

	// Load password reset translations
	var resetTrans map[string]map[string]string
	if err := json.Unmarshal([]byte(passwordResetTranslationsJSON), &resetTrans); err != nil {
		logger.Error("Failed to load password reset translations", "error", err)
	}

	return &PasswordResetService{
		userRepo:          userRepo,
		tokenRepo:         tokenRepo,
		emailService:      emailService,
		jwtManager:        jwtManager,
		cfg:               cfg,
		logger:            logger,
		bcryptCost:        bcryptCost,
		resetTemplate:     resetTmpl,
		resetTranslations: resetTrans,
	}
}

// RequestPasswordReset initiates password reset process
// Always returns success to prevent email enumeration
func (s *PasswordResetService) RequestPasswordReset(ctx context.Context, req *models.ForgotPasswordRequest) error {
	// Fire-and-forget goroutine to prevent timing attacks
	go s.processPasswordReset(req.Email)

	// Always return success immediately
	return nil
}

// processPasswordReset handles the actual password reset logic in background
func (s *PasswordResetService) processPasswordReset(email string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Look up user
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// User not found - silently log and exit (no error to caller)
		s.logger.Debug("password reset requested for non-existent email",
			"email", email,
			"error", err.Error())
		return
	}

	// Generate cryptographically secure token
	token, err := s.generateResetToken()
	if err != nil {
		s.logger.Error("failed to generate reset token",
			"user_id", user.ID,
			"error", err.Error())
		return
	}

	// Store token with expiry
	expiresAt := time.Now().Add(passwordResetTokenExpiry)
	if err := s.userRepo.SetPasswordResetToken(ctx, user.ID, token, expiresAt); err != nil {
		s.logger.Error("failed to store reset token",
			"user_id", user.ID,
			"error", err.Error())
		return
	}

	// Build reset URL
	resetURL := fmt.Sprintf("%s/reset-password/%s", s.cfg.AppURL, token)

	// Send email or log
	if s.emailService.IsConfigured() {
		if err := s.sendPasswordResetEmail(user, resetURL); err != nil {
			s.logger.Error("failed to send password reset email",
				"user_id", user.ID,
				"email", user.Email,
				"error", err.Error())
		} else {
			s.logger.Info("password reset email sent",
				"user_id", user.ID,
				"email", user.Email)
		}
	} else {
		// SMTP not configured - log the reset link
		s.logger.Warn("SMTP not configured - password reset link (copy this URL):",
			"user", user.Email,
			"display_name", user.DisplayName,
			"reset_url", resetURL,
			"expires_at", expiresAt.Format(time.RFC3339))
	}
}

// ResetPassword validates token and updates password, then auto-logs in the user
func (s *PasswordResetService) ResetPassword(ctx context.Context, req *models.ResetPasswordRequest) (*models.ResetPasswordResponse, error) {
	// Validate token and get user
	user, err := s.userRepo.GetByPasswordResetToken(ctx, req.Token)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired reset token")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password
	if err := s.userRepo.UpdatePassword(ctx, user.ID, string(hashedPassword)); err != nil {
		return nil, fmt.Errorf("failed to update password: %w", err)
	}

	// Clear reset token
	if err := s.userRepo.ClearPasswordResetToken(ctx, user.ID); err != nil {
		s.logger.Error("failed to clear reset token after password update",
			"user_id", user.ID,
			"error", err.Error())
		// Non-fatal - continue with auto-login
	}

	// Invalidate all existing refresh tokens (force re-login on other devices)
	if err := s.tokenRepo.DeleteByUserID(ctx, user.ID); err != nil {
		s.logger.Error("failed to revoke existing tokens after password reset",
			"user_id", user.ID,
			"error", err.Error())
		// Non-fatal - continue
	}

	// Generate new tokens for auto-login
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID.String(), user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, expiresAt, err := s.jwtManager.GenerateRefreshToken(user.ID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token hash
	storedToken := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(refreshToken),
		ExpiresAt: expiresAt,
	}
	storedToken.ID = uuid.New()

	if err := s.tokenRepo.Create(ctx, storedToken); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	s.logger.Info("password reset successful with auto-login",
		"user_id", user.ID,
		"email", user.Email)

	return &models.ResetPasswordResponse{
		Message:      "Password reset successful",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         toUserResponse(user),
	}, nil
}

// generateResetToken creates a cryptographically secure random token
func (s *PasswordResetService) generateResetToken() (string, error) {
	bytes := make([]byte, resetTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// sendPasswordResetEmail sends the reset email
func (s *PasswordResetService) sendPasswordResetEmail(user *models.User, resetURL string) error {
	// Get translations for locale (fallback to english)
	trans, ok := s.resetTranslations[user.Locale]
	if !ok {
		trans = s.resetTranslations["en"]
	}

	// Prepare template data
	expiryDuration := passwordResetTokenExpiry.String()
	data := map[string]string{
		"Subject":        trans["subject"],
		"Greeting":       replaceVarPR(trans["greeting"], "DisplayName", user.DisplayName),
		"Intro":          trans["intro"],
		"CTAInstruction": trans["cta_instruction"],
		"CTAButton":      trans["cta_button"],
		"OrCopy":         trans["or_copy"],
		"ExpiryNotice":   replaceVarPR(trans["expiry_notice"], "ExpiryDuration", expiryDuration),
		"SecurityNotice": trans["security_notice"],
		"Signature":      trans["signature"],
		"ResetURL":       resetURL,
	}

	// Execute template
	var htmlBody bytes.Buffer
	if err := s.resetTemplate.Execute(&htmlBody, data); err != nil {
		s.logger.Error("Failed to execute password reset template", "error", err)
		return err
	}

	// Send email
	if err := s.emailService.Send(email.Email{
		To:      []string{user.Email},
		Subject: trans["subject"],
		Body:    htmlBody.String(),
		HTML:    true,
	}); err != nil {
		return err
	}

	s.logger.Info("Password reset email sent", "to", user.Email, "locale", user.Locale)
	return nil
}

// replaceVarPR replaces {{.VarName}} with value in a string
func replaceVarPR(str, varName, value string) string {
	placeholder := "{{." + varName + "}}"
	return strings.ReplaceAll(str, placeholder, value)
}

// toUserResponse converts User to UserResponse
func toUserResponse(user *models.User) *models.UserResponse {
	return &models.UserResponse{
		ID:            user.ID.String(),
		Email:         user.Email,
		DisplayName:   user.DisplayName,
		Role:          user.Role,
		Locale:        user.Locale,
		Timezone:      user.Timezone,
		EmailVerified: user.EmailVerified,
		CreatedAt:     user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
