// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package sessioncookie

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestMaxAgeSeconds pins the rounding rule on its own, so the cookie attribute
// tests below do not have to depend on a wall clock.
func TestMaxAgeSeconds(t *testing.T) {
	base := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expiresAt time.Time
		want      int
	}{
		{"an exact number of seconds", base.Add(48 * time.Hour), 172800},
		{"a fractional lifetime is rounded up, not down", base.Add(48*time.Hour + 500*time.Millisecond), 172801},
		{"a lifetime just under a second", base.Add(999 * time.Millisecond), 1},
		{"an already-expired token clears the cookie", base.Add(-time.Second), -1},
		{"an exactly-expired token clears the cookie", base, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxAgeSeconds(tt.expiresAt, base); got != tt.want {
				t.Errorf("maxAgeSeconds(%v, %v) = %d, want %d", tt.expiresAt, base, got, tt.want)
			}
		})
	}
}

// TestSetRefreshToken checks the cookie this session actually returns.
func TestSetRefreshToken(t *testing.T) {
	// A real future expiry, so the assertions are valid at any wall-clock time.
	expiresAt := time.Now().Add(48 * time.Hour).Truncate(time.Second)

	t.Run("https request", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-Proto", "https")

		SetRefreshToken(rec, req, "the-refresh-token", expiresAt)

		var c *http.Cookie
		for _, cookie := range rec.Result().Cookies() {
			if cookie.Name == Name {
				c = cookie
			}
		}
		if c == nil {
			t.Fatal("no refresh_token cookie was set")
		}
		if c.Value != "the-refresh-token" {
			t.Errorf("value = %q", c.Value)
		}
		if c.Path != "/" {
			t.Errorf("path = %q, want /", c.Path)
		}
		if !c.HttpOnly {
			t.Error("cookie is not HttpOnly")
		}
		if c.SameSite != http.SameSiteStrictMode {
			t.Errorf("SameSite = %v, want Strict", c.SameSite)
		}
		if !c.Secure {
			t.Error("cookie is not Secure on an https request")
		}
		// HttpOnly cookies serialise at second precision.
		if !c.Expires.Equal(expiresAt) {
			t.Errorf("Expires = %v, want %v", c.Expires, expiresAt)
		}
		if got, want := c.MaxAge, maxAgeSeconds(expiresAt, time.Now()); got != want {
			t.Errorf("MaxAge = %d, want %d (the token's remaining lifetime, ceiling)", got, want)
		}
	})

	t.Run("plain http request is not marked Secure", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		SetRefreshToken(rec, req, "the-refresh-token", expiresAt)

		for _, c := range rec.Result().Cookies() {
			if c.Name == Name && c.Secure {
				t.Error("cookie is Secure on a plain http request")
			}
		}
	})
}

// TestClearRefreshToken checks the cookie is asked to be dropped both ways the
// standard library knows about.
func TestClearRefreshToken(t *testing.T) {
	rec := httptest.NewRecorder()
	SetRefreshToken(rec, httptest.NewRequest(http.MethodGet, "/", nil), "the-refresh-token", time.Now().Add(time.Hour))
	ClearRefreshToken(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	var c *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == Name {
			c = cookie
		}
	}
	if c == nil {
		t.Fatal("no refresh_token cookie was set on clear")
	}
	if c.Value != "" {
		t.Errorf("value = %q, want empty", c.Value)
	}
	if c.MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1", c.MaxAge)
	}
	if !c.Expires.Before(time.Now()) {
		t.Errorf("Expires = %v, want a time in the past", c.Expires)
	}
}
