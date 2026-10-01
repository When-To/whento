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
	"html/template"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/whento/pkg/email"

	// Aliased: the constructor below takes a *slog.Logger named `logger`, which
	// would otherwise shadow the package.
	pkglog "github.com/whento/pkg/logger"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/config"
)

//go:embed templates/password_reset.html
var passwordResetTemplate string

//go:embed templates/locales/password_reset.json
var passwordResetTranslationsJSON string

const (
	resetTokenLength = 32 // bytes (64 hex chars)
)

// The seams below are what the password-reset flow needs of its collaborators.
// Declared here rather than taking concrete repositories so the service can be
// exercised without a database or an SMTP server. The repository types satisfy
// them structurally, so no call site changes.

// PasswordResetUserStore is the slice of the user repository this service uses.
type PasswordResetUserStore interface {
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByPasswordResetToken(ctx context.Context, token string) (*models.User, error)
	SetPasswordResetToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error
	ClearPasswordResetToken(ctx context.Context, userID uuid.UUID) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error
}

// PasswordResetTokenStore revokes the user's other sessions on a reset, and
// stores the fresh auto-login pair.
type PasswordResetTokenStore interface {
	Create(ctx context.Context, token *models.RefreshToken, securityGeneration int64) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
}

// PasswordResetMailer sends the reset email and reports whether one can be sent.
type PasswordResetMailer interface {
	IsConfigured() bool
	Send(msg email.Email) error
}

// PasswordResetTokenIssuer mints the auto-login token pair after a reset.
type PasswordResetTokenIssuer interface {
	GenerateAccessToken(userID, email, role string) (string, error)
	GenerateRefreshToken(userID string) (string, time.Time, error)
	IssueRefreshToken(userID, familyID string) (string, time.Time, string, error)
}

// PasswordResetService handles password reset business logic
type PasswordResetService struct {
	userRepo          PasswordResetUserStore
	tokenRepo         PasswordResetTokenStore
	emailService      PasswordResetMailer
	jwtManager        PasswordResetTokenIssuer
	cfg               *config.Config
	logger            *slog.Logger
	bcryptCost        int
	resetTemplate     *template.Template
	resetTranslations map[string]map[string]string
}

// NewPasswordResetService creates a new password reset service
func NewPasswordResetService(
	userRepo PasswordResetUserStore,
	tokenRepo PasswordResetTokenStore,
	emailService PasswordResetMailer,
	jwtManager PasswordResetTokenIssuer,
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

// processPasswordReset handles the actual password reset logic in background.
//
// It deliberately does not inherit the request context. RequestPasswordReset returns
// as soon as this goroutine is started, so the request context is cancelled almost
// immediately — inheriting it would abort the lookup, the token write and the email.
// The work is bounded by its own timeout instead.
//
//nolint:contextcheck // detached on purpose, see above.
func (s *PasswordResetService) processPasswordReset(email string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Look up user
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// User not found - silently log and exit (no error to caller)
		// There is no user id to name here, so a fingerprint is all that is left
		// to tell "one address, tried repeatedly" from "many addresses, once each".
		s.logger.Debug("password reset requested for non-existent email",
			"account_ref", pkglog.Fingerprint(email),
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

	// Store token with expiry, configured via PASSWORD_RESET_EXPIRY. The expiry
	// has to be *the* expiry in both places that mention it: stored here for the
	// validity check, and in the email text the user reads.
	expiresAt := time.Now().Add(s.cfg.Email.PasswordResetExpiry)
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
				"error", err.Error())
		} else {
			s.logger.Info("password reset email sent",
				"user_id", user.ID)
		}
	} else {
		// SMTP not configured: the log *is* the delivery channel here, so this one
		// line deliberately carries the reset token. It is the only way an operator
		// running without SMTP can complete a reset, and it only ever happens on an
		// instance that has no mail configured at all. The address is still not
		// written — an operator who can read this log can map the user id in the
		// database, which is the same database the account lives in.
		//
		// See docs/logging-and-privacy.md; this is the single documented exception.
		s.logger.Warn("SMTP not configured - password reset link (copy this URL):",
			"user_id", user.ID,
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

	// Invalidate every other session and advance the security generation before
	// this reset publishes its own. A login that already accepted the old
	// password presents the previous generation and is refused.
	generation, err := s.tokenRepo.DeleteByUserID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke sessions after password reset: %w", err)
	}
	user.SecurityGeneration = generation

	// Generate new tokens for auto-login
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID.String(), user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, expiresAt, familyID, err := s.jwtManager.IssueRefreshToken(user.ID.String(), "")
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token hash
	storedToken := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(refreshToken),
		ExpiresAt: expiresAt,
		FamilyID:  familyID,
	}
	storedToken.ID = uuid.New()

	if err := s.tokenRepo.Create(ctx, storedToken, user.SecurityGeneration); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	s.logger.Info("password reset successful with auto-login",
		"user_id", user.ID)

	return &models.ResetPasswordResponse{
		Message:          "Password reset successful",
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: expiresAt,
		SessionID:        familyID,
		User:             toUserResponse(user),
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
	expiryDuration := s.cfg.Email.PasswordResetExpiry.String()
	data := map[string]any{
		"Subject":        trans["subject"],
		"Greeting":       email.ReplaceVar(trans["greeting"], "DisplayName", user.DisplayName),
		"Intro":          trans["intro"],
		"CTAInstruction": trans["cta_instruction"],
		"CTAButton":      trans["cta_button"],
		"OrCopy":         trans["or_copy"],
		"ExpiryNotice":   email.ReplaceVar(trans["expiry_notice"], "ExpiryDuration", expiryDuration),
		"SecurityNotice": trans["security_notice"],
		// The one locale string holding deliberate markup: the signature ends with a
		// <br> between the sign-off and the team name. Everything else in this map is a
		// plain string, so html/template escapes it — which is exactly what has to
		// happen to a display name.
		"Signature": template.HTML(trans["signature"]),
		"ResetURL":  resetURL,
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

	s.logger.Info("Password reset email sent", "user_id", user.ID, "locale", user.Locale)
	return nil
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
