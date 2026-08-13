// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/email"
	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/middleware"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/config"
)

//go:embed templates/email_verification.html
var emailVerificationTemplate string

//go:embed templates/locales/email_verification.json
var emailVerificationTranslationsJSON string

// AuthHandler handles authentication HTTP requests
type AuthHandler struct {
	authService              *service.AuthService
	userRepo                 UserStore
	emailService             EmailSender
	cfg                      *config.Config
	logger                   *slog.Logger
	verificationTemplate     *template.Template
	verificationTranslations map[string]map[string]string
	mfaRepo                  MFARepository
	passkeyRepo              PasskeyRepository
}

// UserStore is the slice of the user repository this handler needs.
//
// Declared here rather than taking *repository.UserRepository directly so the package
// can be exercised without a database — it was at 0% for exactly that reason. Go
// interfaces are structural, so the concrete repository satisfies it and no call site
// changes.
type UserStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByVerificationToken(ctx context.Context, token string) (*models.User, error)
	SetVerificationToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error
	VerifyEmail(ctx context.Context, userID uuid.UUID) error
}

// EmailSender is the slice of pkg/email this handler needs. Sending a real message is
// out of reach in a test; deciding whether to send one is not.
type EmailSender interface {
	Send(message email.Email) error
	IsConfigured() bool
}

// MFARepository interface for MFA status checking
type MFARepository interface {
	IsEnabled(ctx context.Context, userID uuid.UUID) (bool, error)
}

// PasskeyRepository interface for passkey counting
type PasskeyRepository interface {
	CountByUserID(ctx context.Context, userID uuid.UUID) (int, error)
}

// setRefreshTokenCookie sets the refresh token as an httpOnly secure cookie.
func setRefreshTokenCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	})
}

// clearRefreshTokenCookie removes the refresh token cookie.
func clearRefreshTokenCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(
	authService *service.AuthService,
	userRepo UserStore,
	emailService EmailSender,
	cfg *config.Config,
	logger *slog.Logger,
	mfaRepo MFARepository,
	passkeyRepo PasskeyRepository,
) *AuthHandler {
	// Parse email verification template
	verificationTmpl, err := template.New("email_verification").Parse(emailVerificationTemplate)
	if err != nil {
		logger.Error("Failed to parse email verification template", "error", err)
	}

	// Load email verification translations
	var verificationTrans map[string]map[string]string
	if err := json.Unmarshal([]byte(emailVerificationTranslationsJSON), &verificationTrans); err != nil {
		logger.Error("Failed to load email verification translations", "error", err)
	}

	return &AuthHandler{
		authService:              authService,
		userRepo:                 userRepo,
		emailService:             emailService,
		cfg:                      cfg,
		logger:                   logger,
		verificationTemplate:     verificationTmpl,
		verificationTranslations: verificationTrans,
		mfaRepo:                  mfaRepo,
		passkeyRepo:              passkeyRepo,
	}
}

// Register handles user registration
//
//	@Summary		Register a new user
//	@Description	Creates a new user account. First registered user automatically becomes admin.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.RegisterRequest	true	"Registration details"
//	@Success		201		{object}	models.AuthResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		403		{object}	httputil.ErrorResponse	"Registration disabled or email not allowed"
//	@Failure		409		{object}	httputil.ErrorResponse	"Email already exists"
//	@Failure		429		{object}	httputil.ErrorResponse	"Rate limit exceeded"
//	@Router			/api/v1/auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	resp, err := h.authService.Register(r.Context(), &req)
	if err != nil {
		// Use identical error messages and status codes to prevent account enumeration.
		// All user-facing registration failures return the same generic response
		// so attackers cannot distinguish "email exists" from "email not allowed".
		if errors.Is(err, service.ErrUserAlreadyExists) ||
			errors.Is(err, service.ErrEmailNotAllowed) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Registration failed")
			return
		}
		if errors.Is(err, service.ErrRegistrationDisabled) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "Registration is not available")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to register user")
		return
	}

	// Send verification email if email verification is enabled
	if h.cfg.Email.VerificationEnabled && h.emailService.IsConfigured() {
		// Generate verification token
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			h.logger.Error("Failed to generate verification token", "error", err)
		} else {
			token := hex.EncodeToString(tokenBytes)
			expiresAt := time.Now().Add(h.cfg.Email.VerificationExpiry)

			// Save token to database
			if err := h.userRepo.SetVerificationToken(r.Context(), resp.User.ID, token, expiresAt); err != nil {
				h.logger.Error("Failed to save verification token", "error", err)
			} else {
				// Send verification email (async, don't block registration)
				go h.sendVerificationEmail(resp.User.Email, resp.User.DisplayName, resp.User.Locale, token)
			}
		}
	}

	httputil.JSON(w, http.StatusCreated, resp)
}

// sendVerificationEmail sends the verification email (helper for Register)
func (h *AuthHandler) sendVerificationEmail(to, displayName, locale, token string) {
	if !h.emailService.IsConfigured() {
		h.logger.Warn("Email service not configured, cannot send verification email")
		return
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
		"Greeting":        replaceVar(trans["greeting"], "DisplayName", displayName),
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
	if err := h.verificationTemplate.Execute(&htmlBody, data); err != nil {
		h.logger.Error("Failed to execute verification template", "error", err)
		return
	}

	// Send email
	err := h.emailService.Send(email.Email{
		To:      []string{to},
		Subject: trans["subject"],
		Body:    htmlBody.String(),
		HTML:    true,
	})

	if err != nil {
		h.logger.Error("Failed to send verification email", "error", err, "recipient_ref", logger.Fingerprint(to))
	} else {
		h.logger.Info("Verification email sent", "recipient_ref", logger.Fingerprint(to), "locale", locale)
	}
}

// replaceVar replaces {{.VarName}} with HTML-escaped value in a string
func replaceVar(str, varName, value string) string {
	placeholder := "{{." + varName + "}}"
	return strings.ReplaceAll(str, placeholder, html.EscapeString(value))
}

// Login handles user login
//
//	@Summary		User login
//	@Description	Authenticates a user with email and password. Returns JWT tokens. If 2FA is enabled, returns mfa_required=true with a temporary token.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.LoginRequest	true	"Login credentials"
//	@Success		200		{object}	models.AuthResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		401		{object}	httputil.ErrorResponse	"Invalid credentials"
//	@Failure		429		{object}	httputil.ErrorResponse	"Rate limit exceeded"
//	@Router			/api/v1/auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		logger.FromContext(r.Context()).Error("Failed to decode login request", "error", err)
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	resp, err := h.authService.Login(r.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrAccountLocked) {
			// The address itself is not written: a lockout line is one an operator
			// reads while an incident is running, and what they need from it is
			// whether the attempts are all against one account or spread over many.
			// The fingerprint answers that; the address would only add a list of
			// who has an account here to whatever reads the log stream.
			logger.FromContext(r.Context()).Warn("Login attempt on locked account",
				"account_ref", logger.Fingerprint(req.Email))
			httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid email or password")
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid email or password")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to login")
		return
	}

	// Set refresh token as httpOnly cookie
	if resp.RefreshToken != "" {
		setRefreshTokenCookie(w, r, resp.RefreshToken)
		resp.RefreshToken = ""
	}

	httputil.JSON(w, http.StatusOK, resp)
}

// Refresh handles token refresh
//
//	@Summary		Refresh access token
//	@Description	Generates a new access token using a valid refresh token
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.RefreshRequest	true	"Refresh token"
//	@Success		200		{object}	models.AuthResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body"
//	@Failure		401		{object}	httputil.ErrorResponse	"Invalid or expired refresh token"
//	@Router			/api/v1/auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// Read refresh token from httpOnly cookie
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid or expired refresh token")
		return
	}

	resp, err := h.authService.RefreshToken(r.Context(), cookie.Value)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid or expired refresh token")
		return
	}

	// Rotate refresh token cookie
	if resp.RefreshToken != "" {
		setRefreshTokenCookie(w, r, resp.RefreshToken)
		resp.RefreshToken = ""
	}

	httputil.JSON(w, http.StatusOK, resp)
}

// Logout handles user logout
//
//	@Summary		User logout
//	@Description	Invalidates the refresh token
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.RefreshRequest	true	"Refresh token to invalidate"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body"
//	@Router			/api/v1/auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Read refresh token from httpOnly cookie
	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie.Value != "" {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}

	clearRefreshTokenCookie(w, r)

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "Logged out successfully"})
}

// GetMe returns the current user's profile
//
//	@Summary		Get current user
//	@Description	Returns the authenticated user's profile information
//	@Tags			Authentication
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	models.UserResponse
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		404	{object}	httputil.ErrorResponse	"User not found"
//	@Router			/api/v1/auth/me [get]
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	user, err := h.authService.GetCurrentUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get user")
		return
	}

	httputil.JSON(w, http.StatusOK, user.ToResponse())
}

// UpdateMe updates the current user's profile
//
//	@Summary		Update current user
//	@Description	Updates the authenticated user's profile (display name, locale, timezone)
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		models.UpdateProfileRequest	true	"Profile updates"
//	@Success		200		{object}	models.UserResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		401		{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		404		{object}	httputil.ErrorResponse	"User not found"
//	@Router			/api/v1/auth/me [patch]
func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	var req models.UpdateProfileRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	user, err := h.authService.UpdateProfile(r.Context(), userID, &req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to update profile")
		return
	}

	httputil.JSON(w, http.StatusOK, user.ToResponse())
}

// ChangePassword changes the current user's password
//
//	@Summary		Change password
//	@Description	Changes the authenticated user's password (requires current password)
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		models.ChangePasswordRequest	true	"Password change details"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request or current password incorrect"
//	@Failure		401		{object}	httputil.ErrorResponse	"Unauthorized"
//	@Router			/api/v1/auth/me/password [patch]
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	var req models.ChangePasswordRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	err := h.authService.ChangePassword(r.Context(), userID, &req)
	if err != nil {
		if errors.Is(err, service.ErrPasswordMismatch) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Current password is incorrect")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to change password")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "Password changed successfully"})
}

// ListUsers is now implemented in list_users_cloud.go and list_users_selfhosted.go
// using build tags for conditional compilation

// UpdateUserRole updates a user's role (admin only)
//
//	@Summary		Update user role
//	@Description	Updates a user's role (admin or user). Admin only. Cannot change own role.
//	@Tags			Admin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string					true	"User ID"
//	@Param			request	body		models.UpdateRoleRequest	true	"New role"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	httputil.ErrorResponse	"Cannot change own role or invalid request"
//	@Failure		401		{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403		{object}	httputil.ErrorResponse	"Forbidden (requires admin role)"
//	@Failure		404		{object}	httputil.ErrorResponse	"User not found"
//	@Router			/api/v1/auth/admin/users/{id}/role [patch]
func (h *AuthHandler) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	currentUserID := middleware.GetUserID(r.Context())
	targetUserID := chi.URLParam(r, "id")

	var req models.UpdateRoleRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	err := h.authService.UpdateUserRole(r.Context(), currentUserID, targetUserID, req.Role)
	if err != nil {
		if errors.Is(err, service.ErrCannotDemoteSelf) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Cannot change your own role")
			return
		}
		if errors.Is(err, service.ErrUserNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to update role")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "Role updated successfully"})
}

// DeleteUser deletes a user (admin only)
//
//	@Summary		Delete user
//	@Description	Deletes a user account. Admin only. Cannot delete own account.
//	@Tags			Admin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"User ID"
//	@Success		200	{object}	map[string]string
//	@Failure		400	{object}	httputil.ErrorResponse	"Cannot delete own account"
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	httputil.ErrorResponse	"Forbidden (requires admin role)"
//	@Failure		404	{object}	httputil.ErrorResponse	"User not found"
//	@Router			/api/v1/auth/admin/users/{id} [delete]
func (h *AuthHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	currentUserID := middleware.GetUserID(r.Context())
	targetUserID := chi.URLParam(r, "id")

	err := h.authService.DeleteUser(r.Context(), currentUserID, targetUserID)
	if err != nil {
		if errors.Is(err, service.ErrCannotDeleteSelf) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Cannot delete your own account")
			return
		}
		if errors.Is(err, service.ErrUserNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to delete user")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
}
