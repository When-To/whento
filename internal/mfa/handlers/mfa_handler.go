// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/jwt"
	"github.com/whento/pkg/middleware"
	"github.com/whento/pkg/validator"
	authService "github.com/whento/whento/internal/auth/service"
	"github.com/whento/whento/internal/mfa/models"
	"github.com/whento/whento/internal/mfa/service"
)

// MFAHandler handles MFA HTTP requests
type MFAHandler struct {
	service     *service.MFAService
	authService *authService.AuthService
	jwtManager  *jwt.Manager
	logger      *slog.Logger
}

// NewMFAHandler creates a new MFA handler
func NewMFAHandler(service *service.MFAService, authSvc *authService.AuthService, jwtManager *jwt.Manager, logger *slog.Logger) *MFAHandler {
	return &MFAHandler{
		service:     service,
		authService: authSvc,
		jwtManager:  jwtManager,
		logger:      logger,
	}
}

// @Summary		Begin MFA setup
// @Description	Initiates MFA setup by generating a TOTP secret and backup codes. Returns QR code URL and secret for authenticator app configuration.
// @Tags			MFA
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	models.TOTPSetupResponse	"TOTP secret, QR code URL, and backup codes"
// @Failure		401	{object}	httputil.ErrorResponse		"Unauthorized"
// @Failure		404	{object}	httputil.ErrorResponse		"User not found"
// @Failure		409	{object}	httputil.ErrorResponse		"MFA is already enabled"
// @Failure		500	{object}	httputil.ErrorResponse		"Internal server error"
// @Router			/api/v1/mfa/setup/begin [post]
func (h *MFAHandler) BeginSetup(w http.ResponseWriter, r *http.Request) {
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

	response, err := h.service.BeginSetup(r.Context(), userUUID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
			return
		}
		if errors.Is(err, service.ErrMFAAlreadyEnabled) {
			httputil.Error(w, http.StatusConflict, httputil.ErrCodeConflict, "MFA is already enabled")
			return
		}
		h.logger.Error("Failed to begin MFA setup", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to begin MFA setup")
		return
	}

	httputil.JSON(w, http.StatusOK, response)
}

// @Summary		Finish MFA setup
// @Description	Completes MFA setup by verifying the TOTP code from the authenticator app. Enables MFA for the user.
// @Tags			MFA
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		models.FinishSetupRequest	true	"TOTP code from authenticator app"
// @Success		200		{object}	map[string]string			"MFA enabled successfully"
// @Failure		400		{object}	httputil.ErrorResponse		"Invalid request or verification code"
// @Failure		401		{object}	httputil.ErrorResponse		"Unauthorized"
// @Failure		500		{object}	httputil.ErrorResponse		"Internal server error"
// @Router			/api/v1/mfa/setup/finish [post]
func (h *MFAHandler) FinishSetup(w http.ResponseWriter, r *http.Request) {
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

	var req models.FinishSetupRequest
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

	if err := h.service.FinishSetup(r.Context(), userUUID, req.Code); err != nil {
		if errors.Is(err, service.ErrInvalidCode) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid verification code")
			return
		}
		h.logger.Error("Failed to finish MFA setup", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to enable MFA")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "MFA enabled successfully"})
}

// @Summary		Disable MFA
// @Description	Disables MFA for the authenticated user. Removes TOTP secret and backup codes.
// @Tags			MFA
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]string		"MFA disabled successfully"
// @Failure		400	{object}	httputil.ErrorResponse	"MFA is not enabled"
// @Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
// @Failure		500	{object}	httputil.ErrorResponse	"Internal server error"
// @Router			/api/v1/mfa/disable [post]
func (h *MFAHandler) Disable(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.Disable2FA(r.Context(), userUUID); err != nil {
		if errors.Is(err, service.ErrMFANotEnabled) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "MFA is not enabled")
			return
		}
		h.logger.Error("Failed to disable MFA", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to disable MFA")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"message": "MFA disabled successfully"})
}

// @Summary		Get MFA status
// @Description	Returns the MFA status for the authenticated user (enabled or disabled).
// @Tags			MFA
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	models.MFAStatusResponse	"MFA status"
// @Failure		401	{object}	httputil.ErrorResponse		"Unauthorized"
// @Failure		500	{object}	httputil.ErrorResponse		"Internal server error"
// @Router			/api/v1/mfa/status [get]
func (h *MFAHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
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

	status, err := h.service.GetStatus(r.Context(), userUUID)
	if err != nil {
		h.logger.Error("Failed to get MFA status", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get MFA status")
		return
	}

	httputil.JSON(w, http.StatusOK, status)
}

// @Summary		Regenerate backup codes
// @Description	Generates new backup codes for the authenticated user. Invalidates all previous backup codes.
// @Tags			MFA
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	models.BackupCodesResponse	"New backup codes"
// @Failure		400	{object}	httputil.ErrorResponse		"MFA is not enabled"
// @Failure		401	{object}	httputil.ErrorResponse		"Unauthorized"
// @Failure		500	{object}	httputil.ErrorResponse		"Internal server error"
// @Router			/api/v1/mfa/backup-codes/regenerate [post]
func (h *MFAHandler) RegenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
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

	backupCodes, err := h.service.RegenerateBackupCodes(r.Context(), userUUID)
	if err != nil {
		if errors.Is(err, service.ErrMFANotFound) || errors.Is(err, service.ErrMFANotEnabled) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "MFA is not enabled")
			return
		}
		h.logger.Error("Failed to regenerate backup codes", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to regenerate backup codes")
		return
	}

	httputil.JSON(w, http.StatusOK, &models.BackupCodesResponse{
		BackupCodes: backupCodes,
	})
}

// @Summary		Verify MFA during login
// @Description	Verifies TOTP or backup code during login for users with MFA enabled. Completes authentication and returns JWT tokens.
// @Tags			Authentication
// @Accept			json
// @Produce		json
// @Param			request	body		models.VerifyMFARequest							true	"Temporary token and MFA code"
// @Success		200		{object}	object{access_token=string,token_type=string,expires_in=int,refresh_token=string,user=object}	"Authentication successful with access and refresh tokens"
// @Failure		400		{object}	httputil.ErrorResponse							"Invalid request or MFA not enabled"
// @Failure		401		{object}	httputil.ErrorResponse							"Invalid temporary token or MFA code"
// @Failure		500		{object}	httputil.ErrorResponse							"Internal server error"
// @Router			/api/v1/auth/mfa/verify [post]
func (h *MFAHandler) VerifyLogin(w http.ResponseWriter, r *http.Request) {
	var req models.VerifyMFARequest
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

	// Validate temp token and extract user ID
	claims, err := h.jwtManager.ValidateCustomToken(req.TempToken)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid or expired temp token")
		return
	}

	// Check mfa_pending claim
	mfaPending, ok := claims["mfa_pending"].(bool)
	if !ok || !mfaPending {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid temp token")
		return
	}

	// Extract user ID
	userIDStr, ok := claims["user_id"].(string)
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid temp token")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid user ID in token")
		return
	}

	// Verify MFA code (TOTP or backup code)
	valid, err := h.service.VerifyCode(r.Context(), userID, req.Code)
	if err != nil {
		if errors.Is(err, service.ErrMFANotFound) || errors.Is(err, service.ErrMFANotEnabled) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "MFA is not enabled for this user")
			return
		}
		h.logger.Error("Failed to verify MFA code", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to verify MFA code")
		return
	}

	if !valid {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid MFA code")
		return
	}

	// Complete login by generating full auth tokens
	authResponse, err := h.authService.VerifyMFAAndLogin(r.Context(), req.TempToken, req.Code)
	if err != nil {
		h.logger.Error("Failed to complete MFA login", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to complete login")
		return
	}

	httputil.JSON(w, http.StatusOK, authResponse)
}
