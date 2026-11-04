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

	"github.com/whento/pkg/email"
	// Aliased: the constructor below takes a *slog.Logger named `logger`, which
	// would otherwise shadow the package.
	pkglog "github.com/whento/pkg/logger"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/config"
)

//go:embed templates/participant_email_verification.html
var participantEmailVerificationTemplate string

//go:embed templates/locales/participant_email_verification.json
var participantEmailVerificationTranslations string

// ParticipantEmailStore is the slice of the participant repository this service
// needs: the three calls that move a participant through email verification.
//
// Declared here rather than taking *repository.ParticipantRepository so the token
// lifecycle can be exercised without a database, and returning the calendar *models*
// participant so a fake needs no repository import.
type ParticipantEmailStore interface {
	SetEmailVerificationToken(ctx context.Context, participantID uuid.UUID, emailAddress, token string, expiresAt time.Time) error
	GetByVerificationToken(ctx context.Context, token string) (*calendarModels.Participant, error)
	GetByID(ctx context.Context, id uuid.UUID) (*calendarModels.Participant, error)
	VerifyEmail(ctx context.Context, participantID uuid.UUID) error
}

// ParticipantEmailService handles email verification for participants
type ParticipantEmailService struct {
	participantRepo ParticipantEmailStore
	emailService    Mailer
	cfg             *config.Config
	logger          *slog.Logger
	template        *template.Template
	translations    map[string]map[string]string
}

// NewParticipantEmailService creates a new participant email service
func NewParticipantEmailService(
	participantRepo ParticipantEmailStore,
	emailService Mailer,
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

// IsConfigured reports whether the mailer this service sends through has any
// SMTP configuration. The participant-email endpoints gate on it: without it a
// verification link is offered for an email that could never be delivered.
func (s *ParticipantEmailService) IsConfigured() bool {
	return s.emailService.IsConfigured()
}

// AddEmail adds email to participant and sends verification
func (s *ParticipantEmailService) AddEmail(
	ctx context.Context,
	participantID uuid.UUID,
	emailAddress string,
	participantName string,
	locale string,
) error {
	if err := s.issueVerificationToken(
		ctx, participantID, emailAddress, participantName, locale, "failed to save email verification token",
	); err != nil {
		return err
	}

	// A participant id is half of what authorises access to a calendar — the
	// public token is the other half — so it is fingerprinted like the address.
	s.logger.Info("Participant email verification initiated",
		"participant_ref", pkglog.Fingerprint(participantID.String()),
		"recipient_ref", pkglog.Fingerprint(emailAddress))

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
		s.logger.Info("Email already verified", "participant_ref", pkglog.Fingerprint(participant.ID.String()))
		return nil
	}

	// Verify email
	if err := s.participantRepo.VerifyEmail(ctx, participant.ID); err != nil {
		return fmt.Errorf("failed to verify email: %w", err)
	}

	s.logger.Info("Participant email verified successfully",
		"participant_ref", pkglog.Fingerprint(participant.ID.String()),
		"recipient_ref", pkglog.Fingerprint(*participant.Email))

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

	if err := s.issueVerificationToken(
		ctx, participantID, *participant.Email, participant.Name, locale, "failed to update verification token",
	); err != nil {
		return err
	}

	s.logger.Info("Participant email verification resent",
		"participant_ref", pkglog.Fingerprint(participantID.String()),
		"recipient_ref", pkglog.Fingerprint(*participant.Email))

	return nil
}

// issueVerificationToken stores a fresh token for emailAddress, expiring after the
// configured verification expiry, and mails the link in the background. storeFailure prefixes a storage error: the resend
// endpoint hands its error to the client, so each caller keeps its own wording.
func (s *ParticipantEmailService) issueVerificationToken(
	ctx context.Context,
	participantID uuid.UUID,
	emailAddress, participantName, locale, storeFailure string,
) error {
	// Generate 64-char token (32 bytes = 64 hex chars)
	tokenBytes := make([]byte, 32)
	// Since Go 1.24 rand.Read never returns an error: it crashes the program instead.
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	expiresAt := time.Now().Add(s.cfg.Email.VerificationExpiry)
	if err := s.participantRepo.SetEmailVerificationToken(
		ctx, participantID, emailAddress, token, expiresAt,
	); err != nil {
		return fmt.Errorf("%s: %w", storeFailure, err)
	}

	// Send verification email (fire-and-forget)
	go func() {
		if err := s.sendVerificationEmail(emailAddress, participantName, locale, token); err != nil {
			s.logger.Error("Failed to send participant verification email",
				"participant_ref", pkglog.Fingerprint(participantID.String()),
				"error", err)
		}
	}()

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
		"Greeting":        email.ReplaceVar(trans["greeting"], "ParticipantName", name),
		"Intro":           trans["intro"],
		"CTAInstruction":  trans["cta_instruction"],
		"CTAButton":       trans["cta_button"],
		"OrCopy":          trans["or_copy"],
		"ExpiryNotice":    email.ReplaceVar(trans["expiry_notice"], "ExpiryDuration", expiryDuration),
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

	s.logger.Info("Participant verification email sent",
		"recipient_ref", pkglog.Fingerprint(to), "locale", locale)
	return nil
}
