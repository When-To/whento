// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

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

	"github.com/whento/pkg/email"
	"github.com/whento/pkg/jwt"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/config"
)

//go:embed templates/magic_link.html
var magicLinkTemplate string

//go:embed templates/locales/email_magic_link.json
var magicLinkTranslationsJSON string

type MagicLinkService struct {
	userRepo     *repository.UserRepository
	tokenRepo    *repository.TokenRepository
	emailService *email.Service
	jwtManager   *jwt.Manager
	cfg          *config.Config
	logger       *slog.Logger
	template     *template.Template
	translations map[string]map[string]string
}

func NewMagicLinkService(
	userRepo *repository.UserRepository,
	tokenRepo *repository.TokenRepository,
	emailService *email.Service,
	jwtManager *jwt.Manager,
	cfg *config.Config,
	logger *slog.Logger,
) *MagicLinkService {
	// Parse template
	tmpl, err := template.New("magic_link").Parse(magicLinkTemplate)
	if err != nil {
		logger.Error("Failed to parse magic link template", "error", err)
	}

	// Load translations
	var translations map[string]map[string]string
	if err := json.Unmarshal([]byte(magicLinkTranslationsJSON), &translations); err != nil {
		logger.Error("Failed to load magic link translations", "error", err)
	}

	return &MagicLinkService{
		userRepo:     userRepo,
		tokenRepo:    tokenRepo,
		emailService: emailService,
		jwtManager:   jwtManager,
		cfg:          cfg,
		logger:       logger,
		template:     tmpl,
		translations: translations,
	}
}

// RequestMagicLink generates and sends a magic link (always returns nil for anti-enumeration)
func (s *MagicLinkService) RequestMagicLink(ctx context.Context, email string) error {
	// Get verified user (returns ErrUserNotFound if not verified or doesn't exist)
	user, err := s.userRepo.GetByEmailVerified(ctx, email)
	if err != nil {
		// CRITICAL: Return nil even if user not found (anti-enumeration)
		s.logger.Info("Magic link requested for non-existent or unverified email", "email", email)
		return nil
	}

	// Generate secure random token (32 bytes = 64 hex chars)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		s.logger.Error("Failed to generate magic link token", "error", err)
		return nil // Return nil to avoid revealing internal errors
	}
	token := hex.EncodeToString(tokenBytes)

	// Set expiry (1 hour default)
	expiresAt := time.Now().Add(s.cfg.Email.MagicLinkExpiry)

	// Save token to database (uses magic_link_token columns)
	if err := s.userRepo.SetMagicLinkToken(ctx, user.ID, token, expiresAt); err != nil {
		s.logger.Error("Failed to save magic link token", "error", err)
		return nil // Return nil to avoid revealing internal errors
	}

	// Send email asynchronously (fire and forget)
	go s.sendMagicLinkEmail(user.Email, user.DisplayName, user.Locale, token)

	return nil // Always return nil (anti-enumeration)
}

// VerifyMagicLink verifies token and generates JWT auth response
func (s *MagicLinkService) VerifyMagicLink(ctx context.Context, token string) (*models.AuthResponse, error) {
	// Get user by token (validates expiry in SQL query)
	user, err := s.userRepo.GetByMagicLinkToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired magic link")
	}

	// Clear token immediately (single-use enforcement)
	if err := s.userRepo.ClearMagicLinkToken(ctx, user.ID); err != nil {
		s.logger.Error("Failed to clear magic link token", "error", err)
		// Continue anyway - user can still be authenticated
	}

	// Generate JWT tokens
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID.String(), user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, refreshExpiresAt, err := s.jwtManager.GenerateRefreshToken(user.ID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token in database
	storedToken := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: repository.HashToken(refreshToken),
		ExpiresAt: refreshExpiresAt,
	}
	storedToken.ID = uuid.New()

	if err := s.tokenRepo.Create(ctx, storedToken); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	// Build auth response
	return &models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.cfg.JWTAccessExpiry.Seconds()),
		User:         user,
	}, nil
}

// sendMagicLinkEmail sends the magic link email (async, fire-and-forget)
func (s *MagicLinkService) sendMagicLinkEmail(to, displayName, locale, token string) {
	if !s.emailService.IsConfigured() {
		s.logger.Warn("Email service not configured, cannot send magic link")
		return
	}

	// Build magic link URL
	magicLinkURL := fmt.Sprintf("%s/auth/magic-link/verify/%s", s.cfg.AppURL, token)

	// Get translations for locale (fallback to english)
	trans, ok := s.translations[locale]
	if !ok {
		trans = s.translations["en"]
	}

	// Prepare template data
	data := map[string]string{
		"Subject":        trans["subject"],
		"Greeting":       replaceVar(trans["greeting"], "DisplayName", displayName),
		"Intro":          trans["intro"],
		"CTAInstruction": trans["cta_instruction"],
		"CTAButton":      trans["cta_button"],
		"OrCopy":         trans["or_copy"],
		"ExpiryNotice":   trans["expiry_notice"],
		"SecurityNotice": trans["security_notice"],
		"Signature":      trans["signature"],
		"MagicLinkURL":   magicLinkURL,
	}

	// Execute template
	var htmlBody bytes.Buffer
	if err := s.template.Execute(&htmlBody, data); err != nil {
		s.logger.Error("Failed to execute magic link template", "error", err)
		return
	}

	// Send email
	err := s.emailService.Send(email.Email{
		To:      []string{to},
		Subject: trans["subject"],
		Body:    htmlBody.String(),
		HTML:    true,
	})

	if err != nil {
		s.logger.Error("Failed to send magic link email", "error", err, "to", to)
	} else {
		s.logger.Info("Magic link email sent", "to", to, "locale", locale)
	}
}

// replaceVar replaces {{.VarName}} with value in a string
func replaceVar(str, varName, value string) string {
	placeholder := "{{." + varName + "}}"
	return strings.ReplaceAll(str, placeholder, value)
}
