// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"time"

	"github.com/go-chi/chi/v5"
)

// registerHolidayRoutes mounts the public holidays endpoints: the year dataset
// and the offline-coverage list. The frontend keeps its own date-holidays
// renderer, so nothing in the SPA calls these; they exist as an independently
// testable API over the same providers the block/allow policy runs on.
func registerHolidayRoutes(r chi.Router, d *deps, h *handlers) {
	r.Group(func(r chi.Router) {
		d.limiter.use(r, perIP("holidays", 60, time.Minute))

		r.Get("/api/v1/holidays", h.holidays.Year)
		r.Get("/api/v1/holidays/supported", h.holidays.Supported)
	})
}
