// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"time"

	"github.com/go-chi/chi/v5"

	availabilityHandlers "github.com/whento/whento/internal/availability/handlers"
)

// registerAvailabilityRoutes mounts /api/v1/availabilities. Every route here is
// public: the calendar token plus the participant id in the path *are* the
// authorisation.
func registerAvailabilityRoutes(r chi.Router, d *deps, h *handlers) {
	l := d.limiter

	r.Route("/api/v1/availabilities", func(r chi.Router) {
		// Live updates: the browser subscribes here and refetches on each notice.
		//
		// Kept out of the 60/min bucket below. An EventSource is a long-lived
		// connection that the browser re-establishes on its own after every hiccup,
		// deploy or idle timeout, so sharing a bucket with the participant API meant
		// a handful of reconnects could lock a participant out of the calendar they
		// were reconnecting to. Its own bucket, keyed on path+IP so one calendar's
		// reconnect loop cannot starve another, and sized for several tabs behind a
		// shared NAT rather than for API call volume.
		r.Group(func(r chi.Router) {
			l.use(r, perPathIP(300, time.Minute))

			r.Get("/calendar/{token}/events", h.events.Stream)
		})

		// Public routes with rate limiting (all availability endpoints are public)
		r.Group(func(r chi.Router) {
			// Rate limiting: 60 requests/minute/IP for public availability access
			l.use(r, perIP(60, time.Minute))

			// One notice per successful write, for every route below. Placed here
			// rather than in the nine service methods so a tenth write route cannot
			// silently stop notifying anyone.
			r.Use(availabilityHandlers.PublishChanges(d.broker))

			// Participant availability management
			r.Get("/calendar/{token}/participant/{pid}", h.availability.GetParticipantAvailabilities)
			r.Post("/calendar/{token}/participant/{pid}", h.availability.CreateAvailability)
			r.Patch("/calendar/{token}/participant/{pid}/{date}", h.availability.UpdateAvailability)
			r.Delete("/calendar/{token}/participant/{pid}/{date}", h.availability.DeleteAvailability)

			// Recurrence management
			r.Post("/calendar/{token}/participant/{pid}/recurrence", h.recurrence.CreateRecurrence)
			r.Get("/calendar/{token}/participant/{pid}/recurrences", h.recurrence.GetParticipantRecurrences)
			r.Patch("/calendar/{token}/participant/{pid}/recurrence/{rid}", h.recurrence.UpdateRecurrence)
			r.Delete("/calendar/{token}/participant/{pid}/recurrence/{rid}", h.recurrence.DeleteRecurrence)

			// Recurrence exceptions
			r.Post("/calendar/{token}/participant/{pid}/recurrence/{rid}/exception", h.recurrence.CreateException)
			r.Delete("/calendar/{token}/participant/{pid}/recurrence/{rid}/exception/{date}", h.recurrence.DeleteException)

			// Date summaries
			r.Get("/calendar/{token}/dates/{date}", h.availability.GetDateSummary)
			r.Get("/calendar/{token}/range", h.availability.GetRangeSummary)
		})
	})
}
