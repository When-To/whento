// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
)

// registerMFALoginRoutes mounts the second factor step of a login, under
// /api/v1/auth: it is reached with the temporary token the password step
// returned, not with an access token, so it is public as far as the router is
// concerned.
//
// r is the /api/v1/auth sub-router; the path below is relative to it.
func registerMFALoginRoutes(r chi.Router, d *deps, h *handlers) {
	l := d.limiter

	r.Group(func(r chi.Router) {
		// MFA verification: 5 requests/5 minutes/IP
		l.on(r, perPathIP(5, 5*time.Minute)).Post("/mfa/verify", h.mfa.VerifyLogin)
	})
}

// registerMFARoutes mounts /api/v1/mfa: enrolling, disabling and inspecting the
// second factor of an account that is already signed in.
func registerMFARoutes(r chi.Router, d *deps, h *handlers) {
	l := d.limiter

	r.Route("/api/v1/mfa", func(r chi.Router) {
		r.Use(middleware.Auth(d.jwtManager, d.cacheStore))

		// MFA setup/disable: 5 requests/minute/user
		l.on(r, perUser(5, time.Minute)).Post("/setup/begin", h.mfa.BeginSetup)
		l.on(r, perUser(5, time.Minute)).Post("/setup/finish", h.mfa.FinishSetup)
		l.on(r, perUser(3, time.Minute)).Post("/disable", h.mfa.Disable)

		r.Get("/status", h.mfa.GetStatus)
		r.Post("/backup-codes/regenerate", h.mfa.RegenerateBackupCodes)
	})
}
