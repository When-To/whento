// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package quota

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
)

// Handler handles quota-related HTTP requests
type Handler struct {
	service QuotaService
	log     *slog.Logger
}

// NewHandler creates a new quota handler
func NewHandler(service QuotaService, log *slog.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

// HandleGetLimits returns quota limits and current usage
//
//	@Summary		Get quota limits and usage
//	@Description	Returns current quota limits (per-user for Cloud, per-server for Self-hosted) and usage statistics. Includes whether user can create more calendars and upgrade URL.
//	@Tags			Quota
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	LimitInfo				"Quota limits and usage information"
//	@Failure		401	{object}	httputil.ErrorResponse	"User not authenticated"
//	@Failure		500	{object}	httputil.ErrorResponse	"Failed to get quota limits"
//	@Router			/api/v1/quota/limits [get]
func (h *Handler) HandleGetLimits(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userIDStr := middleware.GetUserID(r.Context())
	if userIDStr == "" {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "User not authenticated")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, httputil.ErrCodeUnauthorized, "Invalid user ID")
		return
	}

	// Get user limit
	userLimit, err := h.service.GetUserLimit(r.Context(), userID)
	if err != nil {
		h.log.Error("Failed to get user limit", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get quota limits")
		return
	}

	// Get server limit (will be -1 for cloud, actual limit for selfhosted)
	serverLimit, _ := h.service.GetServerLimit(r.Context())

	// Get current usage
	userUsage, err := h.service.GetCurrentUsage(r.Context(), userID)
	if err != nil {
		h.log.Error("Failed to get user usage", "error", err, "user_id", userID)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get usage")
		return
	}

	serverUsage, _ := h.service.GetServerUsage(r.Context())

	// Check if user can create more calendars
	canCreate, _ := h.service.CanCreateCalendar(r.Context(), userID)

	// Determine limitation type
	limitationType := "per_user"
	upgradeURL := "/billing/upgrade" // Default for cloud

	if serverLimit > 0 {
		// Self-hosted
		limitationType = "per_server"
		upgradeURL = "https://whento.be/pricing"
	} else if userLimit == 0 {
		limitationType = "none"
	}

	response := LimitInfo{
		UserLimit:      userLimit,
		ServerLimit:    serverLimit,
		UserUsage:      userUsage,
		ServerUsage:    serverUsage,
		CanCreate:      canCreate,
		LimitationType: limitationType,
		UpgradeURL:     upgradeURL,
	}

	httputil.JSON(w, http.StatusOK, response)
}
