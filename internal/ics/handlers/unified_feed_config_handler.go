// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/logger"
	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/ics/service"
)

type UnifiedFeedConfigHandler struct {
	configService *service.UnifiedFeedConfigService
}

func NewUnifiedFeedConfigHandler(configService *service.UnifiedFeedConfigService) *UnifiedFeedConfigHandler {
	return &UnifiedFeedConfigHandler{
		configService: configService,
	}
}

// GetConfig handles GET /api/v1/ics/unified-feed
//
//	@Summary		Get unified feed configuration
//	@Description	Returns the current user's unified ICS feed configuration including token and selected calendars.
//	@Tags			ICS
//	@Security		BearerAuth
//	@Produce		json
//	@Success		200	{object}	service.UnifiedFeedConfig
//	@Failure		401	{string}	string	"Unauthorized"
//	@Router			/api/v1/ics/unified-feed [get]
func (h *UnifiedFeedConfigHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	userID, err := h.getUserID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	config, err := h.configService.GetConfig(r.Context(), userID)
	if err != nil {
		log.Error("Failed to get unified feed config", "error", err)
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	httputil.JSON(w, http.StatusOK, config)
}

// Create handles POST /api/v1/ics/unified-feed
//
//	@Summary		Create unified feed
//	@Description	Creates a new unified ICS feed for the current user.
//	@Tags			ICS
//	@Security		BearerAuth
//	@Produce		json
//	@Success		201	{object}	service.UnifiedFeedConfig
//	@Failure		401	{string}	string	"Unauthorized"
//	@Failure		409	{string}	string	"Feed already exists"
//	@Router			/api/v1/ics/unified-feed [post]
func (h *UnifiedFeedConfigHandler) Create(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	userID, err := h.getUserID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	config, err := h.configService.CreateFeed(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrFeedAlreadyExists) {
			httputil.Error(w, http.StatusConflict, "feed_exists", "Unified feed already exists")
			return
		}
		log.Error("Failed to create unified feed", "error", err)
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	httputil.JSON(w, http.StatusCreated, config)
}

type updateCalendarsRequest struct {
	CalendarIDs []string `json:"calendar_ids"`
}

// UpdateCalendars handles PATCH /api/v1/ics/unified-feed/calendars
//
//	@Summary		Update unified feed calendars
//	@Description	Updates which calendars are included in the unified ICS feed.
//	@Tags			ICS
//	@Security		BearerAuth
//	@Accept			json
//	@Param			body	body	updateCalendarsRequest	true	"Calendar IDs to include"
//	@Success		204
//	@Failure		400	{string}	string	"Invalid request"
//	@Failure		401	{string}	string	"Unauthorized"
//	@Failure		403	{string}	string	"Calendar does not belong to user"
//	@Failure		404	{string}	string	"Feed not found"
//	@Router			/api/v1/ics/unified-feed/calendars [patch]
func (h *UnifiedFeedConfigHandler) UpdateCalendars(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	userID, err := h.getUserID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	var req updateCalendarsRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	err = h.configService.UpdateCalendars(r.Context(), userID, req.CalendarIDs)
	if err != nil {
		if errors.Is(err, service.ErrFeedNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "Unified feed not found")
			return
		}
		if errors.Is(err, service.ErrInvalidCalendarOwner) {
			httputil.Error(w, http.StatusForbidden, "forbidden", "One or more calendars do not belong to you")
			return
		}
		log.Error("Failed to update unified feed calendars", "error", err)
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RegenerateToken handles POST /api/v1/ics/unified-feed/regenerate-token
//
//	@Summary		Regenerate unified feed token
//	@Description	Regenerates the ICS token for the unified feed, invalidating the previous URL.
//	@Tags			ICS
//	@Security		BearerAuth
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Failure		401	{string}	string	"Unauthorized"
//	@Failure		404	{string}	string	"Feed not found"
//	@Router			/api/v1/ics/unified-feed/regenerate-token [post]
func (h *UnifiedFeedConfigHandler) RegenerateToken(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	userID, err := h.getUserID(r)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	token, err := h.configService.RegenerateToken(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrFeedNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "Unified feed not found")
			return
		}
		log.Error("Failed to regenerate unified feed token", "error", err)
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]string{"ics_token": token})
}

func (h *UnifiedFeedConfigHandler) getUserID(r *http.Request) (uuid.UUID, error) {
	userIDStr := middleware.GetUserID(r.Context())
	if userIDStr == "" {
		return uuid.Nil, errors.New("user ID not found in context")
	}
	return uuid.Parse(userIDStr)
}
