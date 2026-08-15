// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
)

// registerICSRoutes mounts /api/v1/ics: the feed endpoints a calendar client
// polls with nothing but the feed token, and the authenticated endpoints that
// configure the unified feed.
func registerICSRoutes(r chi.Router, d *deps, h *handlers) {
	l := d.limiter

	r.Route("/api/v1/ics", func(r chi.Router) {
		// Public routes with rate limiting
		r.Group(func(r chi.Router) {
			// ICS feed access: 30 requests/minute/IP
			l.use(r, perIP("ics-feed", 30, time.Minute))

			// ICS feed endpoint (accepts both /feed/{token} and /feed/{token}.ics)
			r.Get("/feed/{token}", h.ics.GetFeed)
			// Unified ICS feed endpoint
			r.Get("/unified/{token}", h.ics.GetUnifiedFeed)
		})

		// Authenticated routes for managing unified feed
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(d.jwtManager, d.cacheStore))
			l.use(r, perUser("ics-authenticated", 30, time.Minute))

			r.Get("/unified-feed", h.unifiedFeed.GetConfig)
			r.Post("/unified-feed", h.unifiedFeed.Create)
			r.Patch("/unified-feed/calendars", h.unifiedFeed.UpdateCalendars)
			r.Post("/unified-feed/regenerate-token", h.unifiedFeed.RegenerateToken)
		})
	})
}
