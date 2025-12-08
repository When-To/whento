// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/email"
	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/auth/repository"
	"github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/config"
)

//go:embed templates/email_verification.html
var emailVerificationTemplateEV string

//go:embed templates/locales/email_verification.json
var emailVerificationTranslationsEV string

// EmailVerificationHandler handles email verification HTTP requests
type EmailVerificationHandler struct {
	authService              *service.AuthService
	userRepo                 *repository.UserRepository
	emailService             *email.Service
	cfg                      *config.Config
	logger                   *slog.Logger
	verificationTemplate     *template.Template
	verificationTranslations map[string]map[string]string
}

// NewEmailVerificationHandler creates a new email verification handler
func NewEmailVerificationHandler(
	authService *service.AuthService,
	userRepo *repository.UserRepository,
	emailService *email.Service,
	cfg *config.Config,
	logger *slog.Logger,
) *EmailVerificationHandler {
	// Parse email verification template
	verificationTmpl, err := template.New("email_verification").Parse(emailVerificationTemplateEV)
	if err != nil {
		logger.Error("Failed to parse email verification template", "error", err)
	}

	// Load email verification translations
	var verificationTrans map[string]map[string]string
	if err := json.Unmarshal([]byte(emailVerificationTranslationsEV), &verificationTrans); err != nil {
		logger.Error("Failed to load email verification translations", "error", err)
	}

	return &EmailVerificationHandler{
		authService:              authService,
		userRepo:                 userRepo,
		emailService:             emailService,
		cfg:                      cfg,
		logger:                   logger,
		verificationTemplate:     verificationTmpl,
		verificationTranslations: verificationTrans,
	}
}

// SendVerificationEmail sends a verification email to the authenticated user
//
//	@Summary		Send verification email
//	@Description	Generates and sends a verification email to the authenticated user's email address
//	@Tags			Authentication
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	map[string]string	"Verification email sent or email already verified"
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		404	{object}	httputil.ErrorResponse	"User not found"
//	@Failure		500	{object}	httputil.ErrorResponse	"Failed to generate or send verification email"
//	@Router			/api/v1/auth/send-verification [post]
func (h *EmailVerificationHandler) SendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userIDStr := middleware.GetUserID(ctx)
	if userIDStr == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "User not authenticated")
		return
	}

	// Parse user ID
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid user ID")
		return
	}

	// Get user
	user, err := h.userRepo.GetByID(ctx, userID)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
		return
	}

	// Check if email already verified
	if user.EmailVerified {
		httputil.JSON(w, http.StatusOK, map[string]string{
			"message": "Email already verified",
		})
		return
	}

	// Generate verification token (32 bytes = 64 hex chars)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to generate verification token")
		return
	}
	token := hex.EncodeToString(tokenBytes)

	// Set token expiry (from config)
	expiresAt := time.Now().Add(h.cfg.Email.VerificationExpiry)

	// Save token to database
	if err := h.userRepo.SetVerificationToken(ctx, user.ID, token, expiresAt); err != nil {
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to save verification token")
		return
	}

	// Send verification email
	if err := h.sendVerificationEmail(user.Email, user.DisplayName, user.Locale, token); err != nil {
		h.logger.Error("Failed to send verification email", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to send verification email")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{
		"message": "Verification email sent successfully",
	})
}

// VerifyEmail verifies a user's email with the provided token
//
//	@Summary		Verify email address
//	@Description	Verifies a user's email address using the verification token from the email
//	@Tags			Authentication
//	@Produce		json
//	@Param			token	path		string	true	"Verification token"
//	@Success		200		{object}	map[string]string	"Email verified successfully or already verified"
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid or expired verification token"
//	@Failure		500		{object}	httputil.ErrorResponse	"Failed to verify email"
//	@Router			/api/v1/auth/verify-email/{token} [get]
func (h *EmailVerificationHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := chi.URLParam(r, "token")

	if token == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Verification token is required")
		return
	}

	// Get user by token
	user, err := h.userRepo.GetByVerificationToken(ctx, token)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid verification token")
		return
	}

	// Check if email already verified
	if user.EmailVerified {
		httputil.JSON(w, http.StatusOK, map[string]string{
			"message": "Email already verified",
		})
		return
	}

	// Check if token expired
	if user.VerificationTokenExpiresAt != nil && time.Now().After(*user.VerificationTokenExpiresAt) {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Verification token has expired")
		return
	}

	// Verify email
	if err := h.userRepo.VerifyEmail(ctx, user.ID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to verify email")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{
		"message": "Email verified successfully",
	})
}

// sendVerificationEmail sends the actual verification email
func (h *EmailVerificationHandler) sendVerificationEmail(to, displayName, locale, token string) error {
	if !h.emailService.IsConfigured() {
		h.logger.Warn("Email service not configured, skipping verification email")
		return nil
	}

	verificationURL := fmt.Sprintf("%s/verify-email/%s", h.cfg.AppURL, token)

	// Get translations for locale (fallback to english)
	trans, ok := h.verificationTranslations[locale]
	if !ok {
		trans = h.verificationTranslations["en"]
	}

	// Prepare template data
	expiryDuration := h.cfg.Email.VerificationExpiry.String()
	data := map[string]string{
		"Subject":         trans["subject"],
		"Greeting":        replaceVarEV(trans["greeting"], "DisplayName", displayName),
		"Intro":           trans["intro"],
		"CTAInstruction":  trans["cta_instruction"],
		"CTAButton":       trans["cta_button"],
		"OrCopy":          trans["or_copy"],
		"ExpiryNotice":    replaceVarEV(trans["expiry_notice"], "ExpiryDuration", expiryDuration),
		"SecurityNotice":  trans["security_notice"],
		"Signature":       trans["signature"],
		"VerificationURL": verificationURL,
	}

	// Execute template
	var htmlBody bytes.Buffer
	if err := h.verificationTemplate.Execute(&htmlBody, data); err != nil {
		h.logger.Error("Failed to execute verification template", "error", err)
		return err
	}

	// Send email
	if err := h.emailService.Send(email.Email{
		To:      []string{to},
		Subject: trans["subject"],
		Body:    htmlBody.String(),
		HTML:    true,
	}); err != nil {
		return err
	}

	h.logger.Info("Verification email sent", "to", to, "locale", locale)
	return nil
}

// replaceVarEV replaces {{.VarName}} with value in a string
func replaceVarEV(str, varName, value string) string {
	placeholder := "{{." + varName + "}}"
	return strings.ReplaceAll(str, placeholder, value)
}
