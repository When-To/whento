// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build !cloud

package handlers

import (
	"net/http"

	"github.com/whento/pkg/httputil"
	"github.com/whento/whento/internal/auth/models"
)

// ListUsers returns all users without subscription info (admin only, selfhosted build)
//
//	@Summary		List all users
//	@Description	Returns all users with MFA status. Admin only. Self-hosted version (no subscription info).
//	@Tags			Admin
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	models.UsersListResponse
//	@Failure		401	{object}	httputil.ErrorResponse	"Unauthorized"
//	@Failure		403	{object}	httputil.ErrorResponse	"Forbidden (requires admin role)"
//	@Router			/api/v1/auth/admin/users [get]
func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.authService.ListUsers(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to list users")
		return
	}

	var responses []*models.UserResponse
	for _, user := range users {
		resp := user.ToResponse()

		// Enrich with MFA status (TOTP + passkey count)
		totpEnabled, _ := h.mfaRepo.IsEnabled(r.Context(), user.ID)
		passkeyCount, _ := h.passkeyRepo.CountByUserID(r.Context(), user.ID)
		resp.MFAStatus = &models.MFAStatus{
			TOTPEnabled:  totpEnabled,
			PasskeyCount: passkeyCount,
		}

		responses = append(responses, resp)
	}

	httputil.JSON(w, http.StatusOK, models.UsersListResponse{
		Users: responses,
		Total: len(responses),
	})
}
