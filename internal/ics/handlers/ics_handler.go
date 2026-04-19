// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/logger"
	"github.com/whento/whento/internal/ics/service"
)

// crlfRegexp matches CR, LF, and CRLF sequences
var crlfRegexp = regexp.MustCompile(`[\r\n]+`)

// sanitizeICSValue strips CR/LF characters to prevent CRLF injection into ICS properties
func sanitizeICSValue(s string) string {
	return crlfRegexp.ReplaceAllString(s, "")
}

type ICSHandler struct {
	icsService *service.ICSService
}

func NewICSHandler(icsService *service.ICSService) *ICSHandler {
	return &ICSHandler{
		icsService: icsService,
	}
}

// icsFeedEndpoint describes what varies between the single-calendar and
// unified ICS feed endpoints; shared request handling lives in serveICSFeed.
type icsFeedEndpoint struct {
	filename      string
	notFoundMsg   string
	quotaMsg      string
	errLogMessage string
	generate      func(ctx context.Context, token, host string) (string, error)
}

// extractTokenAndHost pulls the ICS token from the URL and resolves the host,
// honoring X-Forwarded-Host / X-Real-Host before r.Host, and sanitizes the
// host to prevent CRLF injection into ICS properties.
func extractTokenAndHost(r *http.Request) (token, host string) {
	token = strings.TrimSuffix(chi.URLParam(r, "token"), ".ics")

	host = r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Header.Get("X-Real-Host")
	}
	if host == "" {
		host = r.Host
	}
	host = sanitizeICSValue(host)
	return token, host
}

func (h *ICSHandler) serveICSFeed(w http.ResponseWriter, r *http.Request, ep icsFeedEndpoint) {
	log := logger.FromContext(r.Context())

	token, host := extractTokenAndHost(r)
	if token == "" {
		http.Error(w, "Token required", http.StatusBadRequest)
		return
	}

	icsContent, err := ep.generate(r.Context(), token, host)
	if err != nil {
		if errors.Is(err, service.ErrCalendarNotFound) {
			http.Error(w, ep.notFoundMsg, http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrQuotaExceeded) {
			http.Error(w, ep.quotaMsg, http.StatusForbidden)
			return
		}
		log.Error(ep.errLogMessage, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", ep.filename))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(icsContent)); err != nil {
		log.Error("Failed to write ICS response", "error", err)
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
	h.serveICSFeed(w, r, icsFeedEndpoint{
		filename:      "calendar.ics",
		notFoundMsg:   "Calendar not found",
		quotaMsg:      "Calendar owner has exceeded their quota. Please delete calendars or upgrade to access this feed.",
		errLogMessage: "Failed to generate ICS feed",
		generate:      h.icsService.GenerateFeed,
	})
}

// GetUnifiedFeed handles GET /api/v1/ics/unified/{token}.ics
// Generates an iCalendar feed combining events from multiple calendars
//
//	@Summary		Get unified ICS feed
//	@Description	Generates an iCalendar feed combining events from all selected calendars of a user.
//	@Tags			ICS
//	@Produce		text/calendar
//	@Param			token	path		string	true	"Unified feed ICS token (with or without .ics extension)"
//	@Success		200		{string}	string	"iCalendar feed content"
//	@Failure		400		{string}	string	"Token required"
//	@Failure		403		{string}	string	"Quota exceeded (over limit)"
//	@Failure		404		{string}	string	"Feed not found"
//	@Router			/api/v1/ics/unified/{token} [get]
func (h *ICSHandler) GetUnifiedFeed(w http.ResponseWriter, r *http.Request) {
	h.serveICSFeed(w, r, icsFeedEndpoint{
		filename:      "WhenTo-unified.ics",
		notFoundMsg:   "Feed not found",
		quotaMsg:      "Feed owner has exceeded their quota. Please delete calendars or upgrade to access this feed.",
		errLogMessage: "Failed to generate unified ICS feed",
		generate:      h.icsService.GenerateUnifiedFeed,
	})
}
