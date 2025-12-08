// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/mfa/service"
)

// AdminMFAHandler handles admin operations on user MFA
type AdminMFAHandler struct {
	mfaService *service.MFAService
	logger     *slog.Logger
}

// NewAdminMFAHandler creates a new admin MFA handler
func NewAdminMFAHandler(mfaService *service.MFAService, logger *slog.Logger) *AdminMFAHandler {
	return &AdminMFAHandler{
		mfaService: mfaService,
		logger:     logger,
	}
}

// AdminDisable2FA disables TOTP for a user (admin only)
//
//	@Summary		Disable user 2FA (admin)
//	@Description	Disables TOTP two-factor authentication for a specific user. Admin only. Cannot disable own 2FA.
//	@Tags			Admin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"User ID"
//	@Success		200	{object}	object{message=string,disabled_for_user=string,disabled_by_admin=string}
//	@Failure		400	{object}	httputil.ErrorResponse	"Invalid user ID, cannot disable own 2FA, or user does not have TOTP enabled"
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	httputil.ErrorResponse	"Forbidden (requires admin role)"
//	@Failure		404	{object}	httputil.ErrorResponse	"User not found"
//	@Failure		500	{object}	httputil.ErrorResponse	"Failed to disable 2FA"
//	@Router			/api/v1/auth/admin/users/{id}/mfa/totp [delete]
func (h *AdminMFAHandler) AdminDisable2FA(w http.ResponseWriter, r *http.Request) {
	// Get admin user ID from context
	adminUserIDStr := middleware.GetUserID(r.Context())
	adminUserID, err := uuid.Parse(adminUserIDStr)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid admin user ID")
		return
	}

	// Get target user ID from URL
	userIDStr := chi.URLParam(r, "id")
	targetUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid user ID")
		return
	}

	// Prevent admin from disabling own 2FA
	if targetUserID == adminUserID {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Cannot disable your own 2FA")
		return
	}

	// Disable 2FA
	result, err := h.mfaService.AdminDisable2FA(r.Context(), targetUserID, adminUserID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "User not found")
			return
		}
		if errors.Is(err, service.ErrMFANotEnabled) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "User does not have TOTP 2FA enabled")
			return
		}
		h.logger.Error("failed to disable user 2FA",
			"error", err,
			"admin_id", adminUserID.String(),
			"user_id", targetUserID.String(),
		)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to disable 2FA")
		return
	}

	httputil.JSON(w, http.StatusOK, result)
}
