// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/auth/sessioncookie"
)

// PasswordResetService is the handler's domain boundary.
type PasswordResetService interface {
	RequestPasswordReset(context.Context, *models.ForgotPasswordRequest) error
	ResetPassword(context.Context, *models.ResetPasswordRequest) (*models.ResetPasswordResponse, error)
}

// PasswordResetHandler handles password reset HTTP requests
type PasswordResetHandler struct {
	passwordResetService PasswordResetService
	logger               *slog.Logger
}

// NewPasswordResetHandler creates a new password reset handler
func NewPasswordResetHandler(passwordResetService PasswordResetService, logger *slog.Logger) *PasswordResetHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PasswordResetHandler{
		passwordResetService: passwordResetService,
		logger:               logger,
	}
}

// ForgotPassword initiates password reset process
//
//	@Summary		Request password reset
//	@Description	Sends a password reset email if the account exists. Always returns success to prevent email enumeration.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.ForgotPasswordRequest	true	"Email address"
//	@Success		200		{object}	models.ForgotPasswordResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		429		{object}	httputil.ErrorResponse	"Rate limit exceeded"
//	@Router			/api/v1/auth/forgot-password [post]
func (h *PasswordResetHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req models.ForgotPasswordRequest
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

	// Always returns nil (fire-and-forget pattern)
	_ = h.passwordResetService.RequestPasswordReset(r.Context(), &req)

	// Always return success to prevent email enumeration
	httputil.JSON(w, http.StatusOK, models.ForgotPasswordResponse{
		Message: "If an account exists with that email, a password reset link has been sent. Please check your inbox.",
	})
}

// ResetPassword validates token and updates password, then signs in or asks for MFA
//
//	@Summary		Reset password
//	@Description	Validates the reset token and updates the password. Without MFA, signs the user in: the access token is returned and the refresh token is set as an httpOnly cookie. With MFA enabled no session is issued: require_mfa and temp_token are returned, and the login completes through the MFA flow.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.ResetPasswordRequest	true	"Reset token and new password"
//	@Success		200		{object}	models.ResetPasswordResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request, validation error, or invalid/expired token"
//	@Failure		500		{object}	httputil.ErrorResponse	"Internal server error"
//	@Router			/api/v1/auth/reset-password [post]
func (h *PasswordResetHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req models.ResetPasswordRequest
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

	resp, err := h.passwordResetService.ResetPassword(r.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidToken) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid or expired reset token")
		} else {
			h.logger.Error("Password reset failed", "error", err)
			httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Unable to reset password. Please try again.")
		}
		return
	}

	// An MFA-protected account gets no session from the reset: the second factor
	// must complete through the MFA flow first, so no refresh cookie is set and
	// no refresh token travels in the body.
	if resp.RequireMFA {
		httputil.JSON(w, http.StatusOK, resp)
		return
	}

	// Set refresh token cookie, with the same lifetime as the JWT it carries.
	if resp.RefreshToken != "" {
		sessioncookie.SetRefreshToken(w, r, resp.RefreshToken, resp.RefreshExpiresAt)
		resp.RefreshToken = ""
	}

	httputil.JSON(w, http.StatusOK, resp)
}
