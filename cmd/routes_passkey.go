// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
)

// registerPasskeyLoginRoutes mounts the public half of the WebAuthn ceremony,
// under /api/v1/auth because it *is* a login: it is called by a visitor who has
// no token yet, so it cannot sit behind middleware.Auth like the routes in
// registerPasskeyRoutes.
//
// r is the /api/v1/auth sub-router; the paths below are relative to it.
func registerPasskeyLoginRoutes(r chi.Router, d *deps, h *handlers) {
	l := d.limiter

	r.Group(func(r chi.Router) {
		// Passkey login (usernameless/passwordless): 5 requests/minute/IP
		l.on(r, perPathIP(5, time.Minute)).Post("/passkey/login/begin", h.passkey.BeginDiscoverableAuthentication)
		l.on(r, perPathIP(5, time.Minute)).Post("/passkey/login/finish", h.passkey.FinishAuthentication)
	})
}

// registerPasskeyRoutes mounts /api/v1/passkey: managing the credentials of an
// account that is already signed in.
func registerPasskeyRoutes(r chi.Router, d *deps, h *handlers) {
	l := d.limiter

	r.Route("/api/v1/passkey", func(r chi.Router) {
		r.Use(middleware.Auth(d.jwtManager, d.cacheStore))

		// Passkey operations: 5 requests/minute/user
		l.on(r, perUser(5, time.Minute)).Post("/register/begin", h.passkey.BeginRegistration)
		l.on(r, perUser(5, time.Minute)).Post("/register/finish", h.passkey.FinishRegistration)

		r.Get("/list", h.passkey.List)
		r.Patch("/{id}/name", h.passkey.Rename)
		r.Delete("/{id}", h.passkey.Delete)
	})
}
