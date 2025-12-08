// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package handlers

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/email"
	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/validator"
	"github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/auth/service"
)

type MagicLinkHandler struct {
	magicLinkService *service.MagicLinkService
	emailService     *email.Service
	logger           *slog.Logger
}

func NewMagicLinkHandler(
	magicLinkService *service.MagicLinkService,
	emailService *email.Service,
	logger *slog.Logger,
) *MagicLinkHandler {
	return &MagicLinkHandler{
		magicLinkService: magicLinkService,
		emailService:     emailService,
		logger:           logger,
	}
}

// RequestMagicLink handles magic link request (always returns 200 OK)
//
//	@Summary		Request magic link login
//	@Description	Sends a magic link login email if the account exists and is verified. Always returns success to prevent email enumeration.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.MagicLinkRequest	true	"Email address"
//	@Success		200		{object}	models.MagicLinkResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		429		{object}	httputil.ErrorResponse	"Rate limit exceeded"
//	@Router			/api/v1/auth/magic-link/request [post]
func (h *MagicLinkHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req models.MagicLinkRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if err := validator.Validate(&req); err != nil {
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			httputil.ValidationError(w, validationErrs)
			return
		}
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, err.Error())
		return
	}

	// Request magic link (always returns nil for anti-enumeration)
	_ = h.magicLinkService.RequestMagicLink(ctx, req.Email)

	// ALWAYS return 200 OK with generic message (anti-enumeration)
	httputil.JSON(w, http.StatusOK, models.MagicLinkResponse{
		Message: "If an account with this email exists and is verified, a magic link has been sent. Please check your inbox.",
	})
}

// VerifyMagicLink handles magic link verification
//
//	@Summary		Verify magic link
//	@Description	Verifies a magic link token and logs in the user. Returns JWT tokens on success.
//	@Tags			Authentication
//	@Produce		json
//	@Param			token	path		string	true	"Magic link token"
//	@Success		200		{object}	models.AuthResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid or expired magic link token"
//	@Router			/api/v1/auth/magic-link/verify/{token} [get]
func (h *MagicLinkHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := chi.URLParam(r, "token")

	// Validate token format (64 hex chars)
	if !isValidToken(token) {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid magic link token")
		return
	}

	// Verify magic link and generate auth tokens
	authResponse, err := h.magicLinkService.VerifyMagicLink(ctx, token)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid or expired magic link")
		return
	}

	httputil.JSON(w, http.StatusOK, authResponse)
}

// CheckAvailable checks if magic link feature is available (SMTP configured)
//
//	@Summary		Check magic link availability
//	@Description	Returns whether magic link login is available (requires SMTP configuration)
//	@Tags			Authentication
//	@Produce		json
//	@Success		200	{object}	models.MagicLinkAvailableResponse
//	@Router			/api/v1/auth/magic-link/available [get]
func (h *MagicLinkHandler) CheckAvailable(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, models.MagicLinkAvailableResponse{
		Available: h.emailService.IsConfigured(),
	})
}

func isValidToken(token string) bool {
	// Token must be 64 hex characters
	matched, _ := regexp.MatchString("^[a-f0-9]{64}$", token)
	return matched
}
