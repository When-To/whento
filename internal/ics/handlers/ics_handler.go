// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/logger"
	"github.com/whento/whento/internal/ics/service"
)

type ICSHandler struct {
	icsService *service.ICSService
}

func NewICSHandler(icsService *service.ICSService) *ICSHandler {
	return &ICSHandler{
		icsService: icsService,
	}
}

// GetFeed handles GET /api/v1/ics/feed/{token}.ics
// Generates an iCalendar feed for a calendar using its ICS token
//
//	@Summary		Get ICS feed
//	@Description	Generates an iCalendar feed for subscription in Google Calendar, Apple Calendar, Outlook, etc. Uses the calendar's ICS token.
//	@Tags			ICS
//	@Produce		text/calendar
//	@Param			token	path		string	true	"ICS token (with or without .ics extension)"
//	@Success		200		{string}	string	"iCalendar feed content"
//	@Failure		400		{string}	string	"Token required"
//	@Failure		403		{string}	string	"Quota exceeded (over limit)"
//	@Failure		404		{string}	string	"Calendar not found"
//	@Router			/api/v1/ics/feed/{token} [get]
func (h *ICSHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	// Get token from URL parameter
	token := chi.URLParam(r, "token")

	// Strip .ics extension if present
	token = strings.TrimSuffix(token, ".ics")

	if token == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	// Extract host from request
	// Priority order:
	// 1. X-Forwarded-Host header (when behind a proxy)
	// 2. X-Real-Host header (alternative proxy header)
	// 3. r.Host (direct request)
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Header.Get("X-Real-Host")
	}
	if host == "" {
		host = r.Host
	}

	// Generate ICS feed with the actual host from the request
	icsContent, err := h.icsService.GenerateFeed(r.Context(), token, host)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			http.Error(w, "Calendar not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrQuotaExceeded) {
			http.Error(w, "Calendar owner has exceeded their quota. Please delete calendars or upgrade to access this feed.", http.StatusForbidden)
			return
		}
		log.Error("Failed to generate ICS feed", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set headers for iCalendar response
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=\"calendar.ics\"")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Write response
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(icsContent)); err != nil {
		log.Error("Failed to write ICS response", "error", err)
	}
}
