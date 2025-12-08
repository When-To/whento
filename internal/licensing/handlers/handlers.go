// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

//go:build selfhosted

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/licensing/models"
	"github.com/whento/whento/internal/licensing/service"
	"github.com/whento/whento/internal/quota"
)

// Handler handles licensing-related HTTP requests
type Handler struct {
	service      *service.Service
	quotaService quota.QuotaService
	log          *slog.Logger
}

// New creates a new licensing handler
func New(service *service.Service, quotaService quota.QuotaService, log *slog.Logger) *Handler {
	return &Handler{
		service:      service,
		quotaService: quotaService,
		log:          log,
	}
}

// HandleActivateLicense activates a license key
// @Summary Activate license (Self-hosted only, Admin)
// @Description Activates a license key with Ed25519 signature verification. Admin-only endpoint. Self-hosted specific.
// @Tags Licensing
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object{license_key=string} true "License activation request with license key JSON"
// @Success 200 {object} object{message=string,license=object} "License activated successfully"
// @Failure 400 {object} httputil.ErrorResponse "Invalid license key or signature verification failed"
// @Failure 401 {object} httputil.ErrorResponse "User not authenticated"
// @Failure 403 {object} httputil.ErrorResponse "Admin role required"
// @Failure 500 {object} httputil.ErrorResponse "Failed to get license info"
// @Router /api/v1/license/activate [post]
func (h *Handler) HandleActivateLicense(w http.ResponseWriter, r *http.Request) {
	var req models.ActivateLicenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid request body")
		return
	}

	// Check if user is admin (only admins can activate licenses)
	userRole := middleware.GetUserRole(r.Context())
	if userRole != "admin" {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "Only administrators can activate licenses")
		return
	}

	// Activate license
	if err := h.service.ActivateLicense(r.Context(), req.LicenseKey); err != nil {
		h.log.Error("Failed to activate license", "error", err)
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	h.log.Info("License activated successfully")

	// Return updated license info
	info, err := h.service.GetLicenseInfo(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get license info")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"message": "License activated successfully",
		"license": info,
	})
}

// HandleGetLicenseInfo returns the current license information
// @Summary Get license information (Self-hosted only)
// @Description Returns current license details including tier, limits, usage, and support status. Self-hosted specific.
// @Tags Licensing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{license=object,tier_config=object,usage=int,can_create=bool,is_active=bool,support_active=bool} "License information retrieved successfully"
// @Failure 401 {object} httputil.ErrorResponse "User not authenticated"
// @Failure 500 {object} httputil.ErrorResponse "Failed to get license info"
// @Router /api/v1/license/info [get]
func (h *Handler) HandleGetLicenseInfo(w http.ResponseWriter, r *http.Request) {
	// Get license info
	info, err := h.service.GetLicenseInfo(r.Context())
	if err != nil {
		h.log.Error("Failed to get license info", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get license info")
		return
	}

	// Get user ID from context (for per-user quota check, but self-hosted uses server-wide)
	userIDStr := middleware.GetUserID(r.Context())
	var userID uuid.UUID
	if userIDStr != "" {
		userID, _ = uuid.Parse(userIDStr)
	}

	// Get server-wide usage (self-hosted mode)
	usage, err := h.quotaService.GetServerUsage(r.Context())
	if err != nil {
		h.log.Error("Failed to get server usage", "error", err)
		// Don't fail, just set to 0
		usage = 0
	}

	// Check if server can create more calendars
	canCreate := false
	if userID != uuid.Nil {
		canCreate, _ = h.quotaService.CanCreateCalendar(r.Context(), userID)
	}

	// Add usage and can_create to response
	info.Usage = usage
	info.CanCreate = canCreate

	httputil.JSON(w, http.StatusOK, info)
}

// HandleRemoveLicense removes the current license (admin only)
// @Summary Remove license (Self-hosted only, Admin)
// @Description Removes the current license and reverts to Community tier. Admin-only endpoint. Self-hosted specific.
// @Tags Licensing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{message=string} "License removed successfully"
// @Failure 401 {object} httputil.ErrorResponse "User not authenticated"
// @Failure 403 {object} httputil.ErrorResponse "Admin role required"
// @Failure 500 {object} httputil.ErrorResponse "Failed to remove license"
// @Router /api/v1/license [delete]
func (h *Handler) HandleRemoveLicense(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	userRole := middleware.GetUserRole(r.Context())
	if userRole != "admin" {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "Only administrators can remove licenses")
		return
	}

	if err := h.service.RemoveLicense(r.Context()); err != nil {
		h.log.Error("Failed to remove license", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, err.Error())
		return
	}

	h.log.Info("License removed successfully")

	httputil.JSON(w, http.StatusOK, map[string]string{
		"message": "License removed successfully, reverted to Community tier",
	})
}

// HandleReloadLicense reloads the license from database into RAM (admin only)
// @Summary Reload license from database (Self-hosted only, Admin)
// @Description Reloads the license from database into RAM. Used after manual database updates. Admin-only endpoint. Self-hosted specific.
// @Tags Licensing
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{message=string,license=object} "License reloaded successfully"
// @Failure 401 {object} httputil.ErrorResponse "User not authenticated"
// @Failure 403 {object} httputil.ErrorResponse "Admin role required"
// @Failure 500 {object} httputil.ErrorResponse "Failed to reload license"
// @Router /api/v1/license/reload [post]
func (h *Handler) HandleReloadLicense(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	userRole := middleware.GetUserRole(r.Context())
	if userRole != "admin" {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "Only administrators can reload licenses")
		return
	}

	if err := h.service.ReloadLicense(r.Context()); err != nil {
		h.log.Error("Failed to reload license", "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, err.Error())
		return
	}

	h.log.Info("License reloaded successfully")

	// Return updated license info
	info, err := h.service.GetLicenseInfo(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to get license info")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]interface{}{
		"message": "License reloaded successfully",
		"license": info,
	})
}

