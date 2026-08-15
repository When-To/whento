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
			l.use(r, perPathIP("availability-stream", 300, time.Minute))

			r.Get("/calendar/{token}/events", h.events.Stream)
		})

		// Public routes with rate limiting (all availability endpoints are public).
		//
		// 60/minute/IP covered reads and writes together, and that is far too tight
		// for how the product is actually used. Filling in a few months of
		// availability sends one request per date — ParticipantView's batch handler
		// fires them all at once through Promise.allSettled — so selecting a quarter
		// is roughly ninety simultaneous writes. The reader was rate limited for
		// using the feature. Worse, the bucket is per IP, so a household or an
		// office behind one address shared those sixty, and the summary refetches
		// that every SSE notice triggers spent from the same budget.
		//
		// Raised to 400/minute, which absorbs a full quarter selected in one gesture
		// plus the refetches that follow it, and still bounds a client stuck in a
		// loop. Kept as one bucket rather than split between reads and writes: both
		// need the same order of headroom, and a second budget would be one more
		// thing to reason about for no protection gained.
		//
		// This is not the abuse control on these routes — possession of the
		// calendar's public token is, and someone holding it can already do all of
		// this through the interface. The limiter is here to bound a runaway client,
		// not to ration normal use.
		r.Group(func(r chi.Router) {
			l.use(r, perIP("availability-write", 400, time.Minute))

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
