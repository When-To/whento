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
	"github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/config"
)

//go:embed templates/participant_email_verification.html
var participantEmailVerificationTemplate string

//go:embed templates/locales/participant_email_verification.json
var participantEmailVerificationTranslations string

// ParticipantEmailService handles email verification for participants
type ParticipantEmailService struct {
	participantRepo *repository.ParticipantRepository
	emailService    *email.Service
	cfg             *config.Config
	logger          *slog.Logger
	template        *template.Template
	translations    map[string]map[string]string
}

// NewParticipantEmailService creates a new participant email service
func NewParticipantEmailService(
	participantRepo *repository.ParticipantRepository,
	emailService *email.Service,
	cfg *config.Config,
	logger *slog.Logger,
) *ParticipantEmailService {
	// Parse email verification template
	tmpl, err := template.New("participant_email_verification").Parse(participantEmailVerificationTemplate)
	if err != nil {
		logger.Error("Failed to parse participant email verification template", "error", err)
	}

	// Load email verification translations
	var trans map[string]map[string]string
	if err := json.Unmarshal([]byte(participantEmailVerificationTranslations), &trans); err != nil {
		logger.Error("Failed to load participant email verification translations", "error", err)
	}

	return &ParticipantEmailService{
		participantRepo: participantRepo,
		emailService:    emailService,
		cfg:             cfg,
		logger:          logger,
		template:        tmpl,
		translations:    trans,
	}
}

// AddEmail adds email to participant and sends verification
func (s *ParticipantEmailService) AddEmail(
	ctx context.Context,
	participantID uuid.UUID,
	emailAddress string,
	participantName string,
	locale string,
) error {
	// Generate 64-char token (32 bytes = 64 hex chars)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("failed to generate verification token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Set expiry (24 hours from config)
	expiresAt := time.Now().Add(s.cfg.Email.VerificationExpiry)

	// Save to database
	if err := s.participantRepo.SetEmailVerificationToken(
		ctx, participantID, emailAddress, token, expiresAt,
	); err != nil {
		return fmt.Errorf("failed to save email verification token: %w", err)
	}

	// Send verification email (fire-and-forget)
	go func() {
		if err := s.sendVerificationEmail(emailAddress, participantName, locale, token); err != nil {
			s.logger.Error("Failed to send participant verification email",
				"participant_id", participantID,
				"error", err)
		}
	}()

	s.logger.Info("Participant email verification initiated",
		"participant_id", participantID,
		"email", emailAddress)

	return nil
}

// VerifyEmail verifies participant email with token
func (s *ParticipantEmailService) VerifyEmail(
	ctx context.Context,
	token string,
) error {
	// Get participant by token (includes expiry check in SQL)
	participant, err := s.participantRepo.GetByVerificationToken(ctx, token)
	if err != nil {
		return fmt.Errorf("invalid or expired verification token")
	}

	// Check if email already verified
	if participant.EmailVerified {
		s.logger.Info("Email already verified", "participant_id", participant.ID)
		return nil
	}

	// Verify email
	if err := s.participantRepo.VerifyEmail(ctx, participant.ID); err != nil {
		return fmt.Errorf("failed to verify email: %w", err)
	}

	s.logger.Info("Participant email verified successfully",
		"participant_id", participant.ID,
		"email", *participant.Email)

	return nil
}

// ResendVerification resends the verification email
func (s *ParticipantEmailService) ResendVerification(
	ctx context.Context,
	participantID uuid.UUID,
	locale string,
) error {
	// Get participant
	participant, err := s.participantRepo.GetByID(ctx, participantID)
	if err != nil {
		return fmt.Errorf("participant not found")
	}

	// Check if already verified
	if participant.EmailVerified {
		return fmt.Errorf("email already verified")
	}

	// Check if email is set
	if participant.Email == nil || *participant.Email == "" {
		return fmt.Errorf("no email address set for this participant")
	}

	// Generate new token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("failed to generate verification token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Set new expiry
	expiresAt := time.Now().Add(s.cfg.Email.VerificationExpiry)

	// Update token in database
	if err := s.participantRepo.SetEmailVerificationToken(
		ctx, participantID, *participant.Email, token, expiresAt,
	); err != nil {
		return fmt.Errorf("failed to update verification token: %w", err)
	}

	// Send verification email (fire-and-forget)
	go func() {
		if err := s.sendVerificationEmail(*participant.Email, participant.Name, locale, token); err != nil {
			s.logger.Error("Failed to resend participant verification email",
				"participant_id", participantID,
				"error", err)
		}
	}()

	s.logger.Info("Participant email verification resent",
		"participant_id", participantID,
		"email", *participant.Email)

	return nil
}

// sendVerificationEmail sends the actual verification email
func (s *ParticipantEmailService) sendVerificationEmail(
	to, name, locale, token string,
) error {
	if !s.emailService.IsConfigured() {
		s.logger.Warn("Email service not configured, skipping participant verification email")
		return nil
	}

	verificationURL := fmt.Sprintf("%s/c/verify-email/%s", s.cfg.AppURL, token)

	// Get translations for locale (fallback to english)
	trans, ok := s.translations[locale]
	if !ok {
		trans = s.translations["en"]
	}

	// Prepare template data
	expiryDuration := s.cfg.Email.VerificationExpiry.String()
	data := map[string]string{
		"Subject":         trans["subject"],
		"Greeting":        replaceVar(trans["greeting"], "ParticipantName", name),
		"Intro":           trans["intro"],
		"CTAInstruction":  trans["cta_instruction"],
		"CTAButton":       trans["cta_button"],
		"OrCopy":          trans["or_copy"],
		"ExpiryNotice":    replaceVar(trans["expiry_notice"], "ExpiryDuration", expiryDuration),
		"SecurityNotice":  trans["security_notice"],
		"Signature":       trans["signature"],
		"VerificationURL": verificationURL,
	}

	// Execute template
	var htmlBody bytes.Buffer
	if err := s.template.Execute(&htmlBody, data); err != nil {
		s.logger.Error("Failed to execute participant verification template", "error", err)
		return err
	}

	// Send email
	if err := s.emailService.Send(email.Email{
		To:      []string{to},
		Subject: trans["subject"],
		Body:    htmlBody.String(),
		HTML:    true,
	}); err != nil {
		return err
	}

	s.logger.Info("Participant verification email sent", "to", to, "locale", locale)
	return nil
}

// replaceVar replaces {{.VarName}} with value in a string
func replaceVar(str, varName, value string) string {
	placeholder := "{{." + varName + "}}"
	return strings.ReplaceAll(str, placeholder, value)
}
