// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
	"github.com/whento/pkg/validator"
	calendarRepo "github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/notify/models"
)

// NotifyConfigHandler handles notification configuration HTTP requests
type NotifyConfigHandler struct {
	calendarRepo *calendarRepo.CalendarRepository
	logger       *slog.Logger
}

// NewNotifyConfigHandler creates a new notification config handler
func NewNotifyConfigHandler(
	calendarRepo *calendarRepo.CalendarRepository,
	logger *slog.Logger,
) *NotifyConfigHandler {
	return &NotifyConfigHandler{
		calendarRepo: calendarRepo,
		logger:       logger,
	}
}

// GetConfig retrieves notification configuration
//
//	@Summary		Get notification configuration
//	@Description	Retrieves the notification configuration for a calendar (owner only)
//	@Tags			Notifications
//	@Security		BearerAuth
//	@Produce		json
//	@Param			id	path		string	true	"Calendar ID"
//	@Success		200	{object}	models.NotifyConfigResponse
//	@Failure		401	{object}	httputil.ErrorResponse
//	@Failure		403	{object}	httputil.ErrorResponse
//	@Failure		404	{object}	httputil.ErrorResponse
//	@Failure		500	{object}	httputil.ErrorResponse
//	@Router			/api/v1/calendars/{id}/notify-config [get]
func (h *NotifyConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	calendarID := chi.URLParam(r, "id")
	userIDStr := middleware.GetUserID(ctx)

	// Parse calendar ID
	cid, err := uuid.Parse(calendarID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid calendar ID")
		return
	}

	// Get calendar
	calendar, err := h.calendarRepo.GetByID(ctx, cid)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
		return
	}

	// Check ownership (middleware should handle this, but double-check)
	userID, _ := uuid.Parse(userIDStr)
	if calendar.OwnerID != userID {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't own this calendar")
		return
	}

	// Parse notify_config or return default
	var config models.NotifyConfig
	if calendar.NotifyConfig != nil && *calendar.NotifyConfig != "" {
		if err := json.Unmarshal([]byte(*calendar.NotifyConfig), &config); err != nil {
			h.logger.Error("Failed to parse notify config", "calendar_id", cid, "error", err)
			httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to parse notification config")
			return
		}
	} else {
		// Return default config
		config = models.NotifyConfig{
			Enabled:            false,
			NotifyOwner:        true,
			NotifyParticipants: false,
			Channels: models.ChannelConfig{
				Email:    models.EmailChannelConfig{Enabled: true},
				Discord:  models.DiscordChannelConfig{Enabled: false},
				Slack:    models.SlackChannelConfig{Enabled: false},
				Telegram: models.TelegramChannelConfig{Enabled: false},
			},
			Reminders: models.ReminderConfig{
				Enabled:     false,
				HoursBefore: 24,
			},
		}
	}

	httputil.JSON(w, http.StatusOK, models.NotifyConfigResponse{Config: config})
}

// UpdateConfig updates notification configuration
//
//	@Summary		Update notification configuration
//	@Description	Updates the notification configuration for a calendar (owner only)
//	@Tags			Notifications
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string								true	"Calendar ID"
//	@Param			request	body		models.UpdateNotifyConfigRequest	true	"Notification configuration"
//	@Success		200		{object}	models.NotifyConfigResponse
//	@Failure		400		{object}	httputil.ErrorResponse
//	@Failure		401		{object}	httputil.ErrorResponse
//	@Failure		403		{object}	httputil.ErrorResponse
//	@Failure		404		{object}	httputil.ErrorResponse
//	@Failure		500		{object}	httputil.ErrorResponse
//	@Router			/api/v1/calendars/{id}/notify-config [patch]
func (h *NotifyConfigHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	calendarID := chi.URLParam(r, "id")
	userIDStr := middleware.GetUserID(ctx)

	// Parse calendar ID
	cid, err := uuid.Parse(calendarID)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid calendar ID")
		return
	}

	// Parse request
	var req models.UpdateNotifyConfigRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	// Validate request
	if err := validator.Validate(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, err.Error())
		return
	}

	// Get calendar
	calendar, err := h.calendarRepo.GetByID(ctx, cid)
	if err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
		return
	}

	// Check ownership
	userID, _ := uuid.Parse(userIDStr)
	if calendar.OwnerID != userID {
		httputil.Error(w, http.StatusForbidden, httputil.ErrCodeForbidden, "You don't own this calendar")
		return
	}

	// Serialize to JSON
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		h.logger.Error("Failed to marshal notify config", "calendar_id", cid, "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to save notification config")
		return
	}

	// Update calendar notify_config field and notify_on_threshold flag
	configStr := string(configJSON)
	if err := h.calendarRepo.UpdateNotifyConfig(ctx, cid, configStr, req.Config.Enabled); err != nil {
		h.logger.Error("Failed to update notify config", "calendar_id", cid, "error", err)
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Failed to update notification config")
		return
	}

	h.logger.Info("Notification config updated", "calendar_id", cid, "enabled", req.Config.Enabled, "notify_on_threshold", req.Config.Enabled)

	httputil.JSON(w, http.StatusOK, models.NotifyConfigResponse{Config: req.Config})
}
