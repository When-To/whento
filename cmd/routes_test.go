// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
	"github.com/whento/whento/internal/config"
)

// The router is assembled here exactly as run() assembles it, but from handlers
// whose dependencies are nil. Nothing below ever calls one: chi stores a method
// value at registration time, and a method value on a nil receiver is legal as
// long as it is not invoked. That is what makes the whole route table reachable
// from a unit test with no database, no Redis and no listener.

// testDeps returns the wiring newRouter reads: the configuration it takes CORS
// and trusted proxies from, the quota services the symmetric RegisterQuotaRoutes
// needs, and the rate limiter under test.
func testDeps(rateLimitEnabled bool) *deps {
	return &deps{
		cfg: &config.Config{
			CORSOrigins:      []string{"https://example.test"},
			RateLimitEnabled: rateLimitEnabled,
		},
		limiter: newRouteLimiter(middleware.NewRateLimiter(nil), rateLimitEnabled),
		quota:   &Services{},
	}
}

// testRouter builds the real application router around zero-value handlers and a
// stub frontend that answers 418, so an SPA fallback is unmistakable.
func testRouter(t *testing.T, rateLimitEnabled bool) chi.Router {
	t.Helper()

	d := testDeps(rateLimitEnabled)
	t.Cleanup(d.limiter.limiter.Stop)

	spa := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	return newRouter(d, &handlers{}, spa)
}

// walkRoutes returns "METHOD /path" for every registered route, sorted.
func walkRoutes(t *testing.T, r chi.Router) []string {
	t.Helper()

	var routes []string
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, fmt.Sprintf("%s %s", method, route))

		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}
	sort.Strings(routes)

	return routes
}

// middlewareCounts returns, per "METHOD /path", how many middlewares chi will
// run before the handler — the global stack, plus whatever the group and the
// route itself added. chi.Walk unwraps the ChainHandler that carries a With()
// middleware, so a per-route rate limit is counted here just like a per-group one.
func middlewareCounts(t *testing.T, r chi.Router) map[string]int {
	t.Helper()

	counts := make(map[string]int)
	err := chi.Walk(r, func(method, route string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		counts[fmt.Sprintf("%s %s", method, route)] = len(mws)

		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}

	return counts
}

// wantRoutes is the complete route table, and the point of it is to be tedious:
// splitting cmd/main.go into one file per domain moved every one of these lines,
// and a route silently dropped in the move is exactly the failure a reviewer
// cannot see in a diff of that size.
var wantRoutes = []string{
	// Health
	"GET /api/health",
	"GET /api/ready",

	// Auth, public
	"POST /api/v1/auth/login",
	"POST /api/v1/auth/register",
	"POST /api/v1/auth/refresh",
	"POST /api/v1/auth/logout",
	"POST /api/v1/auth/forgot-password",
	"POST /api/v1/auth/reset-password",
	"POST /api/v1/auth/magic-link/request",
	"GET /api/v1/auth/magic-link/verify/{token}",
	"GET /api/v1/auth/magic-link/available",
	"GET /api/v1/auth/verify-email/{token}",

	// Auth, authenticated
	"GET /api/v1/auth/me",
	"PATCH /api/v1/auth/me",
	"PATCH /api/v1/auth/me/password",
	"POST /api/v1/auth/send-verification",
	"GET /api/v1/auth/admin/users",
	"PATCH /api/v1/auth/admin/users/{id}/role",
	"DELETE /api/v1/auth/admin/users/{id}",
	"POST /api/v1/auth/admin/users/{id}/disable-2fa",

	// The passkey and MFA ceremonies that happen during a login, and therefore
	// live under /api/v1/auth rather than under their own module's prefix.
	"POST /api/v1/auth/passkey/login/begin",
	"POST /api/v1/auth/passkey/login/finish",
	"POST /api/v1/auth/mfa/verify",

	// Passkey management
	"POST /api/v1/passkey/register/begin",
	"POST /api/v1/passkey/register/finish",
	"GET /api/v1/passkey/list",
	"PATCH /api/v1/passkey/{id}/name",
	"DELETE /api/v1/passkey/{id}",

	// MFA management
	"POST /api/v1/mfa/setup/begin",
	"POST /api/v1/mfa/setup/finish",
	"POST /api/v1/mfa/disable",
	"GET /api/v1/mfa/status",
	"POST /api/v1/mfa/backup-codes/regenerate",

	// Calendars, public
	"GET /api/v1/calendars/public/{token}",
	"POST /api/v1/calendars/public/{token}/participants",
	"GET /api/v1/calendars/participants/verify-email/{token}",
	"POST /api/v1/calendars/{token}/participants/{pid}/email",
	"POST /api/v1/calendars/{token}/participants/{pid}/resend-verification",

	// Calendars, authenticated
	"POST /api/v1/calendars/",
	"GET /api/v1/calendars/",
	"GET /api/v1/calendars/{id}",
	"PATCH /api/v1/calendars/{id}",
	"DELETE /api/v1/calendars/{id}",
	"POST /api/v1/calendars/{id}/regenerate-token",
	"POST /api/v1/calendars/{id}/participants",
	"PATCH /api/v1/calendars/{id}/participants/{pid}",
	"DELETE /api/v1/calendars/{id}/participants/{pid}",
	"GET /api/v1/calendars/{id}/notify-config",
	"PATCH /api/v1/calendars/{id}/notify-config",
	"GET /api/v1/calendars/admin/users/{id}/calendars",

	// Availabilities
	"GET /api/v1/availabilities/calendar/{token}/events",
	"GET /api/v1/availabilities/calendar/{token}/participant/{pid}",
	"POST /api/v1/availabilities/calendar/{token}/participant/{pid}",
	"PATCH /api/v1/availabilities/calendar/{token}/participant/{pid}/{date}",
	"DELETE /api/v1/availabilities/calendar/{token}/participant/{pid}/{date}",
	"POST /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence",
	"GET /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrences",
	"PATCH /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}",
	"DELETE /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}",
	"POST /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}/exception",
	"DELETE /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}/exception/{date}",
	"GET /api/v1/availabilities/calendar/{token}/dates/{date}",
	"GET /api/v1/availabilities/calendar/{token}/range",

	// Quota. Half of the cloud/self-hosted symmetric pair: both variants must
	// mount this, which is why it is asserted from a test that compiles under
	// either build tag.
	"GET /api/v1/quota/limits",

	// ICS
	"GET /api/v1/ics/feed/{token}",
	"GET /api/v1/ics/unified/{token}",
	"GET /api/v1/ics/unified-feed",
	"POST /api/v1/ics/unified-feed",
	"PATCH /api/v1/ics/unified-feed/calendars",
	"POST /api/v1/ics/unified-feed/regenerate-token",

	// SEO and documentation
	"GET /robots.txt",
	"GET /sitemap.xml",
	"GET /swagger/*",
}

// catchAllPatterns are the two registerFallbacks mounts with Handle, which binds
// every method chi knows rather than one. They are expanded rather than listed
// so that a chi release adding a method does not fail this test for no reason.
var catchAllPatterns = []string{"/api/*", "/*"}

// expandCatchAlls returns wantRoutes plus one entry per method for each
// catch-all mount.
func expandCatchAlls(methods []string) []string {
	want := append([]string(nil), wantRoutes...)
	for _, pattern := range catchAllPatterns {
		for _, method := range methods {
			want = append(want, fmt.Sprintf("%s %s", method, pattern))
		}
	}

	return want
}

// walkMethods returns every HTTP method the router has registered anywhere,
// which is where the catch-all expansion above gets its method list.
func walkMethods(routes []string) []string {
	seen := make(map[string]bool)
	var methods []string
	for _, route := range routes {
		method, _, ok := strings.Cut(route, " ")
		if !ok || seen[method] {
			continue
		}
		seen[method] = true
		methods = append(methods, method)
	}
	sort.Strings(methods)

	return methods
}

// wantRateLimited is every route a bucket is mounted on, whether by the route
// itself or by the group it sits in. Everything else must be untouched by
// RATE_LIMIT_ENABLED.
var wantRateLimited = []string{
	// Per-route buckets
	"POST /api/v1/auth/login",
	"POST /api/v1/auth/register",
	"POST /api/v1/auth/refresh",
	"POST /api/v1/auth/forgot-password",
	"POST /api/v1/auth/reset-password",
	"POST /api/v1/auth/magic-link/request",
	"GET /api/v1/auth/magic-link/verify/{token}",
	"GET /api/v1/auth/verify-email/{token}",
	"POST /api/v1/auth/passkey/login/begin",
	"POST /api/v1/auth/passkey/login/finish",
	"POST /api/v1/auth/mfa/verify",
	"POST /api/v1/passkey/register/begin",
	"POST /api/v1/passkey/register/finish",
	"POST /api/v1/mfa/setup/begin",
	"POST /api/v1/mfa/setup/finish",
	"POST /api/v1/mfa/disable",
	"GET /api/v1/calendars/public/{token}",
	"POST /api/v1/calendars/public/{token}/participants",
	"GET /api/v1/calendars/participants/verify-email/{token}",
	// Both of these spend the same resource — outbound mail to an address the
	// caller chooses, with no authentication — so both are budgeted. AddEmail
	// was the only route in its group without a limiter, which made it an open
	// relay for anyone holding a public calendar link.
	"POST /api/v1/calendars/{token}/participants/{pid}/email",
	"POST /api/v1/calendars/{token}/participants/{pid}/resend-verification",

	// The authenticated calendar group: 100/minute/user
	"POST /api/v1/calendars/",
	"GET /api/v1/calendars/",
	"GET /api/v1/calendars/{id}",
	"PATCH /api/v1/calendars/{id}",
	"DELETE /api/v1/calendars/{id}",
	"POST /api/v1/calendars/{id}/regenerate-token",
	"POST /api/v1/calendars/{id}/participants",
	"PATCH /api/v1/calendars/{id}/participants/{pid}",
	"DELETE /api/v1/calendars/{id}/participants/{pid}",
	"GET /api/v1/calendars/{id}/notify-config",
	"PATCH /api/v1/calendars/{id}/notify-config",
	"GET /api/v1/calendars/admin/users/{id}/calendars",

	// The availability stream, in a bucket of its own, and the participant API
	"GET /api/v1/availabilities/calendar/{token}/events",
	"GET /api/v1/availabilities/calendar/{token}/participant/{pid}",
	"POST /api/v1/availabilities/calendar/{token}/participant/{pid}",
	"PATCH /api/v1/availabilities/calendar/{token}/participant/{pid}/{date}",
	"DELETE /api/v1/availabilities/calendar/{token}/participant/{pid}/{date}",
	"POST /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence",
	"GET /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrences",
	"PATCH /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}",
	"DELETE /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}",
	"POST /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}/exception",
	"DELETE /api/v1/availabilities/calendar/{token}/participant/{pid}/recurrence/{rid}/exception/{date}",
	"GET /api/v1/availabilities/calendar/{token}/dates/{date}",
	"GET /api/v1/availabilities/calendar/{token}/range",

	// Both ICS groups
	"GET /api/v1/ics/feed/{token}",
	"GET /api/v1/ics/unified/{token}",
	"GET /api/v1/ics/unified-feed",
	"POST /api/v1/ics/unified-feed",
	"PATCH /api/v1/ics/unified-feed/calendars",
	"POST /api/v1/ics/unified-feed/regenerate-token",
}

func TestRouterRegistersEveryRoute(t *testing.T) {
	// Rate limiting must not add, remove or rename a single route.
	tests := []struct {
		name             string
		rateLimitEnabled bool
	}{
		{name: "rate limiting enabled", rateLimitEnabled: true},
		{name: "rate limiting disabled", rateLimitEnabled: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := walkRoutes(t, testRouter(t, tc.rateLimitEnabled))

			want := expandCatchAlls(walkMethods(got))
			sort.Strings(want)

			for _, missing := range difference(want, got) {
				t.Errorf("route not registered: %s", missing)
			}
			for _, extra := range difference(got, want) {
				t.Errorf("unexpected route registered: %s", extra)
			}
		})
	}
}

// TestRateLimitingAddsExactlyOneMiddlewarePerLimitedRoute pins down which routes
// the limiter is mounted on, read back from the router chi actually built.
//
// It compares middleware counts rather than identities, because a middleware is
// a closure and two closures cannot be compared. What it establishes is the
// property the refactor had to preserve: switching RATE_LIMIT_ENABLED off
// removes exactly one middleware from exactly the routes below, and nothing at
// all from any other route — no reordering, no leftover pass-through.
func TestRateLimitingAddsExactlyOneMiddlewarePerLimitedRoute(t *testing.T) {
	limited := make(map[string]bool, len(wantRateLimited))
	for _, route := range wantRateLimited {
		limited[route] = true
	}

	on := middlewareCounts(t, testRouter(t, true))
	off := middlewareCounts(t, testRouter(t, false))

	if len(on) != len(off) {
		t.Fatalf("route count differs with rate limiting on (%d) and off (%d)", len(on), len(off))
	}

	for route, withLimit := range on {
		withoutLimit, ok := off[route]
		if !ok {
			t.Errorf("%s: registered only when rate limiting is enabled", route)

			continue
		}

		want := 0
		if limited[route] {
			want = 1
		}
		if got := withLimit - withoutLimit; got != want {
			t.Errorf("%s: rate limiting adds %d middleware(s), want %d", route, got, want)
		}
	}

	// A route named in wantRateLimited but absent from the table would otherwise
	// pass silently.
	for route := range limited {
		if _, ok := on[route]; !ok {
			t.Errorf("%s: listed as rate limited but not registered", route)
		}
	}
}

// TestFallbacks covers what registerFallbacks mounts: an /api path no route
// claims must answer with the JSON envelope rather than a page of HTML, and
// anything else must reach the SPA.
func TestFallbacks(t *testing.T) {
	r := testRouter(t, false)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "no sub-router prefix matches", path: "/api/v1/nope", wantStatus: http.StatusNotFound},
		{name: "sub-router matches but no route inside it", path: "/api/v1/calendars/xyz/summary", wantStatus: http.StatusNotFound},
		{name: "unknown /api path entirely", path: "/api/health-check", wantStatus: http.StatusNotFound},
		{name: "non-API path falls through to the SPA", path: "/calendar/anything", wantStatus: http.StatusTeapot},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Errorf("GET %s = %d, want %d", tc.path, rec.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusNotFound {
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Errorf("GET %s Content-Type = %q, want application/json", tc.path, ct)
				}
			}
		})
	}
}

// difference returns the members of a that are not in b. Both are sorted, but it
// does not rely on that.
func difference(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, s := range b {
		inB[s] = true
	}

	var only []string
	for _, s := range a {
		if !inB[s] {
			only = append(only, s)
		}
	}

	return only
}
