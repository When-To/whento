// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"net/http"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
)

// PasswordResetHandler handles password reset HTTP requests
type PasswordResetHandler struct {
	passwordResetService *service.PasswordResetService
}

// NewPasswordResetHandler creates a new password reset handler
func NewPasswordResetHandler(passwordResetService *service.PasswordResetService) *PasswordResetHandler {
	return &PasswordResetHandler{
		passwordResetService: passwordResetService,
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

// ResetPassword validates token and updates password with auto-login
//
//	@Summary		Reset password
//	@Description	Validates the reset token and updates the password. Automatically logs in the user and returns JWT tokens.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.ResetPasswordRequest	true	"Reset token and new password"
//	@Success		200		{object}	models.AuthResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request, validation error, or invalid/expired token"
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
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	// Set refresh token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    resp.RefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	})

	httputil.JSON(w, http.StatusOK, resp)
}
