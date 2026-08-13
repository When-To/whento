// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
)

// registerAuthRoutes mounts /api/v1/auth: the credential endpoints, the
// authenticated account endpoints, and the two ceremonies — passkey login and
// MFA verification — that belong to another module but happen *during* a login
// and are therefore reachable under this prefix.
func registerAuthRoutes(r chi.Router, d *deps, h *handlers) {
	l := d.limiter

	r.Route("/api/v1/auth", func(r chi.Router) {
		// Public routes with rate limiting
		r.Group(func(r chi.Router) {
			l.on(r, perPathIP(5, time.Minute)).Post("/login", h.auth.Login)
			l.on(r, perPathIP(3, time.Minute)).Post("/register", h.auth.Register)
			// Higher than its neighbours because the access token now lives only in
			// memory: every cold page load spends one refresh, where it used to take
			// one only after the token expired. At five a minute, reloading a few
			// times in a row signed the user out. Guessing the value this protects is
			// not the threat — it is an RSA-signed JWT — so the headroom costs nothing.
			l.on(r, perPathIP(30, time.Minute)).Post("/refresh", h.auth.Refresh)
			r.Post("/logout", h.auth.Logout)

			// Password reset (public - no auth required)
			l.on(r, perIP(3, 15*time.Minute)).Post("/forgot-password", h.passwordReset.ForgotPassword)
			l.on(r, perPathIP(5, 15*time.Minute)).Post("/reset-password", h.passwordReset.ResetPassword)

			// Magic link authentication (public)
			l.on(r, perIP(3, 15*time.Minute)).Post("/magic-link/request", h.magicLink.RequestMagicLink)
			l.on(r, perPathIP(5, 15*time.Minute)).Get("/magic-link/verify/{token}", h.magicLink.VerifyMagicLink)
			r.Get("/magic-link/available", h.magicLink.CheckAvailable)

			// Email verification (public - no auth required)
			l.on(r, perPathIP(5, 15*time.Minute)).Get("/verify-email/{token}", h.emailVerification.VerifyEmail)
		})

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(d.jwtManager, d.cacheStore))

			r.Get("/me", h.auth.GetMe)
			r.Patch("/me", h.auth.UpdateMe)
			r.Patch("/me/password", h.auth.ChangePassword)

			// Email verification (authenticated - requires login)
			r.Post("/send-verification", h.emailVerification.SendVerificationEmail)

			// Admin routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("admin"))

				r.Get("/admin/users", h.auth.ListUsers)
				r.Patch("/admin/users/{id}/role", h.auth.UpdateUserRole)
				r.Delete("/admin/users/{id}", h.auth.DeleteUser)
				r.Post("/admin/users/{id}/disable-2fa", h.adminMFA.AdminDisable2FA)
			})
		})

		registerPasskeyLoginRoutes(r, d, h)
		registerMFALoginRoutes(r, d, h)
	})
}
