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

// BootstrapFlow is what the handler needs of the bootstrap domain: the public
// capability read and the one-shot first-user creation.
//
// Declared here rather than taking the concrete *service.BootstrapService so
// the handler can be exercised without a database. The concrete service
// satisfies it, and no call site changes.
type BootstrapFlow interface {
	Status(ctx context.Context) (*models.BootstrapStatusResponse, error)
	CreateFirstUser(ctx context.Context, req *models.BootstrapRequest) (*models.AuthResponse, error)
}

// BootstrapHandler owns the two bootstrap endpoints: GET /api/v1/auth/status
// (the pre-auth capability read the whole UI gates on) and POST
// /api/v1/auth/bootstrap (the one-time first-user creation).
type BootstrapHandler struct {
	service BootstrapFlow
	logger  *slog.Logger
}

func NewBootstrapHandler(service BootstrapFlow, logger *slog.Logger) *BootstrapHandler {
	return &BootstrapHandler{
		service: service,
		logger:  logger,
	}
}

// Status serves the public, unauthenticated capability state.
//
//	@Summary		Bootstrap and registration status
//	@Description	Reports whether the instance still needs its first account created (bootstrap) and whether open registration is enabled. Public: the frontend gates its register button and /bootstrap route on it, and automation polls it before bootstrapping.
//	@Tags			Authentication
//	@Produce		json
//	@Success		200	{object}	models.BootstrapStatusResponse
//	@Failure		429	{object}	httputil.ErrorResponse	"Rate limit exceeded"
//	@Failure		503	{object}	httputil.ErrorResponse	"Bootstrap state temporarily unavailable"
//	@Router			/api/v1/auth/status [get]
func (h *BootstrapHandler) Status(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.Status(r.Context())
	if err != nil {
		h.logger.Error("Failed to read bootstrap status", "error", err)
		httputil.Error(w, http.StatusServiceUnavailable, httputil.ErrCodeUnavailable, "Bootstrap state temporarily unavailable")
		return
	}

	httputil.JSON(w, http.StatusOK, status)
}

// Create is the one-time first-user bootstrap. It is available only while the
// instance has no users, and only to whoever holds the boot key; the account it
// creates is the administrator, and the response is a fully usable session, so
// Ansible can take the access token and keep driving the authenticated API.
//
//	@Summary		Bootstrap the first user
//	@Description	Creates the first (administrator) account of an unconfigured instance. Requires the boot key, which is either set via BOOTSTRAP_KEY or printed to the server logs at startup. The endpoint closes forever once a user exists.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.BootstrapRequest	true	"Boot key and first-user credentials"
//	@Success		201		{object}	models.AuthResponse
//	@Failure		400		{object}	httputil.ErrorResponse	"Invalid request body or validation error"
//	@Failure		401		{object}	httputil.ErrorResponse	"Invalid bootstrap key"
//	@Failure		409		{object}	httputil.ErrorResponse	"Instance already configured"
//	@Failure		429		{object}	httputil.ErrorResponse	"Rate limit exceeded"
//	@Failure		500		{object}	httputil.ErrorResponse	"Bootstrap or session creation failed"
//	@Router			/api/v1/auth/bootstrap [post]
func (h *BootstrapHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.BootstrapRequest
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

	resp, err := h.service.CreateFirstUser(r.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrBootstrapKeyInvalid) {
			httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid bootstrap key")
			return
		}
		if errors.Is(err, service.ErrBootstrapUnavailable) {
			httputil.Error(w, http.StatusConflict, httputil.ErrCodeConflict, "Instance is already configured")
			return
		}
		h.logger.Error("Failed to bootstrap the first user", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to bootstrap user")
		return
	}

	// Same as Login and Register: the refresh token is set as an httpOnly cookie,
	// so the freshly created account walks straight into a working session.
	if resp.RefreshToken != "" {
		sessioncookie.SetRefreshToken(w, r, resp.RefreshToken, resp.RefreshExpiresAt)
		resp.RefreshToken = ""
	}

	httputil.JSON(w, http.StatusCreated, resp)
}
