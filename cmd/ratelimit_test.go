// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
)

// newTestLimiter returns a routeLimiter over a fresh in-memory rate limiter, so
// that no two subtests share a bucket.
func newTestLimiter(t *testing.T, enabled bool) *routeLimiter {
	t.Helper()

	limiter := middleware.NewRateLimiter(nil)
	t.Cleanup(limiter.Stop)

	return newRouteLimiter(limiter, enabled)
}

// okHandler records that it was reached.
func okHandler(reached *int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		*reached++
		w.WriteHeader(http.StatusOK)
	}
}

func TestRouteLimiterMiddlewares(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    int
	}{
		{name: "enabled mounts one middleware", enabled: true, want: 1},
		{name: "disabled mounts none", enabled: false, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newTestLimiter(t, tc.enabled).middlewares(perIP(5, time.Minute))
			if len(got) != tc.want {
				t.Fatalf("middlewares() has %d entries, want %d", len(got), tc.want)
			}
		})
	}
}

// TestRouteLimiterOnIsIdentityWhenDisabled is the equivalence this refactor
// rests on: with rate limiting off, `l.on(r, rule).Get(...)` must be the very
// same call the old `else` branch made — `r.Get(...)` — and not a route wrapped
// in something neutral. Returning r itself is how that is guaranteed rather
// than argued.
func TestRouteLimiterOnIsIdentityWhenDisabled(t *testing.T) {
	r := chi.NewRouter()

	if got := newTestLimiter(t, false).on(r, perIP(5, time.Minute)); got != chi.Router(r) {
		t.Error("on() returned a different router with rate limiting disabled")
	}
	if got := newTestLimiter(t, true).on(r, perIP(5, time.Minute)); got == chi.Router(r) {
		t.Error("on() returned the same router with rate limiting enabled")
	}
}

// TestRouteLimiterHeadersAndRejection checks the observable behaviour on both
// sides of the flag, for both mounting styles: the per-route one (on) and the
// per-group one (use).
func TestRouteLimiterHeadersAndRejection(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		mount   func(l *routeLimiter, r chi.Router, h http.HandlerFunc)
	}{
		{
			name:    "on, enabled",
			enabled: true,
			mount: func(l *routeLimiter, r chi.Router, h http.HandlerFunc) {
				l.on(r, perIP(2, time.Minute)).Get("/x", h)
			},
		},
		{
			name:    "on, disabled",
			enabled: false,
			mount: func(l *routeLimiter, r chi.Router, h http.HandlerFunc) {
				l.on(r, perIP(2, time.Minute)).Get("/x", h)
			},
		},
		{
			name:    "use, enabled",
			enabled: true,
			mount: func(l *routeLimiter, r chi.Router, h http.HandlerFunc) {
				l.use(r, perIP(2, time.Minute))
				r.Get("/x", h)
			},
		},
		{
			name:    "use, disabled",
			enabled: false,
			mount: func(l *routeLimiter, r chi.Router, h http.HandlerFunc) {
				l.use(r, perIP(2, time.Minute))
				r.Get("/x", h)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reached := 0
			r := chi.NewRouter()
			tc.mount(newTestLimiter(t, tc.enabled), r, okHandler(&reached))

			// Three requests against a budget of two.
			var last *httptest.ResponseRecorder
			for range 3 {
				last = httptest.NewRecorder()
				r.ServeHTTP(last, httptest.NewRequest(http.MethodGet, "/x", nil))
			}

			if tc.enabled {
				if last.Code != http.StatusTooManyRequests {
					t.Errorf("third request = %d, want %d", last.Code, http.StatusTooManyRequests)
				}
				if reached != 2 {
					t.Errorf("handler reached %d times, want 2", reached)
				}
				if got := last.Header().Get("X-RateLimit-Limit"); got != "2" {
					t.Errorf("X-RateLimit-Limit = %q, want %q", got, "2")
				}
				if last.Header().Get("Retry-After") == "" {
					t.Error("Retry-After is missing on a rejected request")
				}

				return
			}

			if last.Code != http.StatusOK {
				t.Errorf("third request = %d, want %d", last.Code, http.StatusOK)
			}
			if reached != 3 {
				t.Errorf("handler reached %d times, want 3", reached)
			}
			// No bucket means no accounting, so none of the limiter's headers may
			// appear: a client that reads X-RateLimit-Remaining must not be told a
			// budget that is not being kept.
			for _, header := range []string{"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "X-RateLimit-Backend"} {
				if got := last.Header().Get(header); got != "" {
					t.Errorf("%s = %q with rate limiting disabled, want it absent", header, got)
				}
			}
		})
	}
}

// TestRateLimitRuleKeys checks that the three rule constructors bucket by what
// their names say. A key function cannot be compared, so each is established
// from what it does.
func TestRateLimitRuleKeys(t *testing.T) {
	t.Run("perIP shares one bucket across paths", func(t *testing.T) {
		r := chi.NewRouter()
		reached := 0
		l := newTestLimiter(t, true)
		l.use(r, perIP(2, time.Minute))
		r.Get("/a", okHandler(&reached))
		r.Get("/b", okHandler(&reached))

		codes := requestPaths(r, "/a", "/b", "/a")
		if codes[2] != http.StatusTooManyRequests {
			t.Errorf("third request across two paths = %d, want %d", codes[2], http.StatusTooManyRequests)
		}
	})

	t.Run("perPathIP buckets each path separately", func(t *testing.T) {
		r := chi.NewRouter()
		reached := 0
		l := newTestLimiter(t, true)
		l.use(r, perPathIP(2, time.Minute))
		r.Get("/a", okHandler(&reached))
		r.Get("/b", okHandler(&reached))

		codes := requestPaths(r, "/a", "/b", "/a", "/b")
		for i, code := range codes {
			if code != http.StatusOK {
				t.Errorf("request %d = %d, want %d", i, code, http.StatusOK)
			}
		}
	})

	t.Run("perUser is inert without an authenticated user", func(t *testing.T) {
		// UserKeyFunc reads the user id the Auth middleware puts on the context.
		// With no Auth above it the key is empty, and pkg/middleware lets an
		// empty key through — which is why every perUser bucket in this router
		// sits below middleware.Auth.
		r := chi.NewRouter()
		reached := 0
		l := newTestLimiter(t, true)
		l.use(r, perUser(1, time.Minute))
		r.Get("/a", okHandler(&reached))

		codes := requestPaths(r, "/a", "/a", "/a")
		for i, code := range codes {
			if code != http.StatusOK {
				t.Errorf("request %d = %d, want %d", i, code, http.StatusOK)
			}
		}
	})
}

// requestPaths issues one GET per path and returns the status codes.
func requestPaths(r chi.Router, paths ...string) []int {
	codes := make([]int, 0, len(paths))
	for _, path := range paths {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		codes = append(codes, rec.Code)
	}

	return codes
}
