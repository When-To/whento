// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
	"github.com/whento/pkg/validator"
	authService "github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/passkey/models"
	"github.com/whento/whento/internal/passkey/service"
)

// PasskeyHandler handles passkey HTTP requests
type PasskeyHandler struct {
	service     *service.PasskeyService
	authService *authService.AuthService
	logger      *slog.Logger
}

// NewPasskeyHandler creates a new passkey handler
func NewPasskeyHandler(service *service.PasskeyService, authSvc *authService.AuthService, logger *slog.Logger) *PasskeyHandler {
	return &PasskeyHandler{
		service:     service,
		authService: authSvc,
		logger:      logger,
	}
}

// @Summary		Begin passkey registration
// @Description	Initiates passkey registration by generating WebAuthn credential creation options. Returns challenge and options for the authenticator.
// @Tags			Passkey
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	models.RegistrationOptionsResponse	"WebAuthn credential creation options"
// @Failure		401	{object}	httputil.ErrorResponse				"Unauthorized"
// @Failure		404	{object}	httputil.ErrorResponse				"User not found"
// @Failure		500	{object}	httputil.ErrorResponse				"Internal server error"
// @Router			/api/v1/passkey/register/begin [post]
func (h *PasskeyHandler) BeginRegistration(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid user ID")
		return
	}

	options, err := h.service.BeginRegistration(r.Context(), userUUID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
			return
		}
		h.logger.Error("Failed to begin registration", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to begin registration")
		return
	}

	// Return options directly - it already marshals to {publicKey: {...}}
	httputil.JSON(w, http.StatusOK, options)
}

// @Summary		Finish passkey registration
// @Description	Completes passkey registration by verifying the credential created by the authenticator. Stores the passkey for future passwordless authentication.
// @Tags			Passkey
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			credential	body		object						true	"WebAuthn credential response from authenticator"
// @Success		201			{object}	models.PasskeyResponse		"Passkey registered successfully"
// @Failure		400			{object}	httputil.ErrorResponse		"Invalid credential or challenge"
// @Failure		401			{object}	httputil.ErrorResponse		"Unauthorized"
// @Failure		404			{object}	httputil.ErrorResponse		"User not found"
// @Failure		500			{object}	httputil.ErrorResponse		"Internal server error"
// @Router			/api/v1/passkey/register/finish [post]
func (h *PasskeyHandler) FinishRegistration(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid user ID")
		return
	}

	passkey, err := h.service.FinishRegistration(r.Context(), userUUID, r)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
			return
		}
		if errors.Is(err, service.ErrInvalidCredential) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid credential")
			return
		}
		if errors.Is(err, service.ErrInvalidChallenge) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid or expired challenge")
			return
		}
		h.logger.Error("Failed to finish registration", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to complete registration")
		return
	}

	httputil.JSON(w, http.StatusCreated, passkey.ToResponse())
}

// @Summary		List passkeys
// @Description	Returns all passkeys registered for the authenticated user.
// @Tags			Passkey
// @Produce		json
// @Security		BearerAuth
// @Success		200	{array}		models.PasskeyResponse	"List of user's passkeys"
// @Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
// @Failure		500	{object}	httputil.ErrorResponse	"Internal server error"
// @Router			/api/v1/passkey/list [get]
func (h *PasskeyHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid user ID")
		return
	}

	passkeys, err := h.service.List(r.Context(), userUUID)
	if err != nil {
		h.logger.Error("Failed to list passkeys", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to list passkeys")
		return
	}

	responses := make([]*models.PasskeyResponse, len(passkeys))
	for i, pk := range passkeys {
		responses[i] = pk.ToResponse()
	}

	httputil.JSON(w, http.StatusOK, responses)
}

// @Summary		Rename passkey
// @Description	Updates the friendly name of a passkey for easier identification.
// @Tags			Passkey
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		string						true	"Passkey ID (UUID)"
// @Param			request	body		models.RenamePasskeyRequest	true	"New passkey name"
// @Success		200		{object}	map[string]string			"Passkey renamed successfully"
// @Failure		400		{object}	httputil.ErrorResponse		"Invalid request or passkey ID"
// @Failure		401		{object}	httputil.ErrorResponse		"Unauthorized"
// @Failure		403		{object}	httputil.ErrorResponse		"Forbidden - passkey belongs to another user"
// @Failure		404		{object}	httputil.ErrorResponse		"Passkey not found"
// @Failure		500		{object}	httputil.ErrorResponse		"Internal server error"
// @Router			/api/v1/passkey/{id}/name [patch]
func (h *PasskeyHandler) Rename(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid user ID")
		return
	}

	passkeyID := chi.URLParam(r, "id")
	passkeyUUID, err := uuid.Parse(passkeyID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid passkey ID")
		return
	}

	var req models.RenamePasskeyRequest
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

	if err := h.service.Rename(r.Context(), passkeyUUID, userUUID, req.Name); err != nil {
		if errors.Is(err, service.ErrPasskeyNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Passkey not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to modify this passkey")
			return
		}
		h.logger.Error("Failed to rename passkey", "error", err, "passkey_id", passkeyID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to rename passkey")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "Passkey renamed successfully"})
}

// @Summary		Delete passkey
// @Description	Deletes a passkey from the user's account. The passkey can no longer be used for authentication.
// @Tags			Passkey
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		string					true	"Passkey ID (UUID)"
// @Success		200	{object}	map[string]string		"Passkey deleted successfully"
// @Failure		400	{object}	httputil.ErrorResponse	"Invalid passkey ID"
// @Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
// @Failure		403	{object}	httputil.ErrorResponse	"Forbidden - passkey belongs to another user"
// @Failure		404	{object}	httputil.ErrorResponse	"Passkey not found"
// @Failure		500	{object}	httputil.ErrorResponse	"Internal server error"
// @Router			/api/v1/passkey/{id} [delete]
func (h *PasskeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Unauthorized")
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid user ID")
		return
	}

	passkeyID := chi.URLParam(r, "id")
	passkeyUUID, err := uuid.Parse(passkeyID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid passkey ID")
		return
	}

	if err := h.service.Delete(r.Context(), passkeyUUID, userUUID); err != nil {
		if errors.Is(err, service.ErrPasskeyNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Passkey not found")
			return
		}
		if errors.Is(err, service.ErrUnauthorized) {
			httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't have permission to delete this passkey")
			return
		}
		h.logger.Error("Failed to delete passkey", "error", err, "passkey_id", passkeyID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to delete passkey")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "Passkey deleted successfully"})
}

// @Summary		Begin passkey authentication
// @Description	Initiates passwordless authentication with passkeys. Supports discoverable credentials (usernameless login).
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Success		200	{object}	models.DiscoverableAuthenticationOptionsResponse	"WebAuthn authentication options with challenge ID"
// @Failure		500	{object}	httputil.ErrorResponse								"Internal server error"
// @Router			/api/v1/auth/passkey/login/begin [post]
func (h *PasskeyHandler) BeginDiscoverableAuthentication(w http.ResponseWriter, r *http.Request) {
	options, challengeID, err := h.service.BeginDiscoverableAuthentication(r.Context())
	if err != nil {
		h.logger.Error("Failed to begin discoverable authentication", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to begin authentication")
		return
	}

	// Return options with challengeId at the same level as publicKey
	// options.Response contains the PublicKeyCredentialRequestOptions without the wrapper
	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"publicKey":   options.Response,
		"challengeId": challengeID,
	})
}

// @Summary		Finish passkey authentication
// @Description	Completes passwordless authentication by verifying the passkey credential. Returns JWT tokens. If 2FA is enabled, returns mfa_required=true with a temporary token.
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Param			X-Challenge-ID	header		string										true	"Challenge ID from begin authentication"
// @Param			credential		body		object										true	"WebAuthn credential assertion from authenticator"
// @Success		200				{object}	object{access_token=string,token_type=string,expires_in=int,refresh_token=string,user=object}	"Authentication successful with access and refresh tokens"
// @Failure		400				{object}	httputil.ErrorResponse						"Invalid request, credential, or challenge"
// @Failure		401				{object}	httputil.ErrorResponse						"Invalid credentials"
// @Failure		500				{object}	httputil.ErrorResponse						"Internal server error"
// @Router			/api/v1/auth/passkey/login/finish [post]
func (h *PasskeyHandler) FinishAuthentication(w http.ResponseWriter, r *http.Request) {
	// Extract challengeId from header (sent by frontend to avoid polluting WebAuthn body)
	challengeID := r.Header.Get("X-Challenge-ID")
	if challengeID == "" {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "X-Challenge-ID header is required")
		return
	}

	h.logger.Info("Received passkey authentication request", "challenge_id", challengeID)

	user, err := h.service.FinishAuthentication(r.Context(), challengeID, r)
	if err != nil {
		if errors.Is(err, service.ErrPasskeyNotFound) {
			httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid credentials")
			return
		}
		if errors.Is(err, service.ErrInvalidCredential) {
			httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid credentials")
			return
		}
		if errors.Is(err, service.ErrInvalidChallenge) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid or expired challenge")
			return
		}
		h.logger.Error("Failed to finish authentication", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to complete authentication")
		return
	}

	// Complete login with auth service (checks for 2FA and generates tokens)
	authResponse, err := h.authService.PasskeyLogin(r.Context(), user)
	if err != nil {
		h.logger.Error("Failed to complete passkey login", "error", err, "user_id", user.ID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to complete login")
		return
	}

	httputil.JSON(w, http.StatusOK, authResponse)
}
