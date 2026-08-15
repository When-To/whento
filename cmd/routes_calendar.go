// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
)

// registerCalendarRoutes mounts /api/v1/calendars: the public, token-addressed
// half that a participant reaches from a shared link, and the authenticated half
// that an owner reaches from their account.
func registerCalendarRoutes(r chi.Router, d *deps, h *handlers) {
	l := d.limiter

	r.Route("/api/v1/calendars", func(r chi.Router) {
		// Public routes
		r.Group(func(r chi.Router) {
			// Public calendar access: 60 requests/minute/IP
			l.on(r, perIP("calendar-public-read", 60, time.Minute)).Get("/public/{token}", h.calendar.GetPublicCalendar)

			// Anonymous participant registration: 10 requests/minute/IP
			l.on(r, perIP("calendar-anonymous-participant", 10, time.Minute)).Post("/public/{token}/participants", h.participant.AddAnonymousParticipant)

			// Public participant email verification: 5 requests/15 minutes/IP
			l.on(r, perPathIP("participant-verify-email", 5, 15*time.Minute)).Get("/participants/verify-email/{token}", h.participantEmail.VerifyEmail)

			// Public participant email management: 5 requests/15 minutes/IP.
			//
			// This route was the only one in the group without a limiter, and it is
			// the one that sends mail to an address the caller picks — unauthenticated,
			// so an open relay for anyone holding a public calendar link. The budget is
			// deliberately tight rather than following the looser per-IP defaults,
			// because what it spends is outbound mail to a third party.
			//
			// Ten rather than five: a household or an office is one public address, and
			// several participants filling in their own address one after another is
			// ordinary use, not abuse. Five was also never really five — until this
			// bucket was named, opening the calendar page spent four of them on reads.
			l.on(r, perIP("participant-email-add", 10, 15*time.Minute)).Post("/{token}/participants/{pid}/email", h.participantEmail.AddEmail)

			// Resend verification: 3 requests/15 minutes/IP
			l.on(r, perIP("participant-email-resend", 3, 15*time.Minute)).Post("/{token}/participants/{pid}/resend-verification", h.participantEmail.ResendVerification)
		})

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(d.jwtManager, d.cacheStore))

			// Authenticated routes: 100 requests/minute/user
			l.use(r, perUser("calendar-authenticated", 100, time.Minute))

			// Calendar CRUD
			r.Post("/", h.calendar.CreateCalendar)
			r.Get("/", h.calendar.ListMyCalendars)
			r.Get("/{id}", h.calendar.GetCalendar)
			r.Patch("/{id}", h.calendar.UpdateCalendar)
			r.Delete("/{id}", h.calendar.DeleteCalendar)

			// Token regeneration
			r.Post("/{id}/regenerate-token", h.calendar.RegenerateToken)

			// Participant management
			r.Post("/{id}/participants", h.participant.AddParticipant)
			r.Patch("/{id}/participants/{pid}", h.participant.UpdateParticipant)
			r.Delete("/{id}/participants/{pid}", h.participant.RemoveParticipant)

			// Notification config (owner only)
			r.Get("/{id}/notify-config", h.notifyConfig.GetConfig)
			r.Patch("/{id}/notify-config", h.notifyConfig.UpdateConfig)

			// Admin routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("admin"))

				r.Get("/admin/users/{id}/calendars", h.calendar.ListUserCalendars)
			})
		})
	})
}
