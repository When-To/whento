// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

// Package sessioncookie is the single place that decides how the refresh-token
// cookie is written and cleared. Every flow that issues a session — password
// login, registration, token refresh, magic link, password reset, passkey and
// MFA — funnels through SetRefreshToken, so a change to cookie properties
// (name, path, SameSite, Secure rule, lifetime) happens once and cannot leave
// one login path with different semantics than the others.
//
// The cookie's lifetime is derived from the *token's* expiry, not from a second
// copy of a configured duration. The JWT and the cookie that carries it can
// therefore never disagree.
package sessioncookie

import (
	"math"
	"net/http"
	"time"
)

// Name is the cookie name every login flow shares.
const Name = "refresh_token"

// SetRefreshToken writes the refresh token as an httpOnly, SameSite=Strict
// cookie whose Expires and MaxAge come from the token's own expiry.
//
// expiresAt is the instant the refresh token stops being valid (the second
// value returned by jwt.Manager.GenerateRefreshToken). The cookie expires with
// it; a token and its cookie never outlive each other.
func SetRefreshToken(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     Name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
		MaxAge:   maxAgeSeconds(expiresAt, time.Now()),
	})
}

// ClearRefreshToken removes the refresh-token cookie. The browser is told to
// drop it immediately: MaxAge -1 plus an Expires in the past covers the two
// ways a client may interpret the removal.
func ClearRefreshToken(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     Name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
	})
}

// maxAgeSeconds converts a token expiry into the cookie's MaxAge, in seconds.
//
// Ceiling, not truncation: a token expiring in 172800.5 seconds must advertise
// a cookie for 172801 seconds, not 172800, so a boundary told to drop the
// cookie a moment before the token is still valid. An already-expired token is
// refused a cookie entirely (-1 clears it).
func maxAgeSeconds(expiresAt, now time.Time) int {
	lifetime := expiresAt.Sub(now)
	if lifetime <= 0 {
		return -1
	}
	return int(math.Ceil(lifetime.Seconds()))
}
