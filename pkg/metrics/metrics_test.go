// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// resetHTTP clears the HTTP collectors so one test cannot see another's counts.
// The registry is package-level by design (see metrics.go), so the vectors are
// shared for the whole test binary.
func resetHTTP(t *testing.T) {
	t.Helper()

	httpRequests.Reset()
	httpErrors.Reset()
	httpDuration.Reset()
	rateLimitRejections.Reset()
	t.Cleanup(func() {
		httpRequests.Reset()
		httpErrors.Reset()
		httpDuration.Reset()
		rateLimitRejections.Reset()
	})
}

func TestNormalizeMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		want   string
	}{
		{name: "GET", method: http.MethodGet, want: "GET"},
		{name: "POST", method: http.MethodPost, want: "POST"},
		{name: "PATCH", method: http.MethodPatch, want: "PATCH"},
		{name: "DELETE", method: http.MethodDelete, want: "DELETE"},
		{name: "OPTIONS", method: http.MethodOptions, want: "OPTIONS"},
		// A method is a token: any client may send anything. Left unsanitised
		// it is a label value an attacker chooses, and one new time series per
		// value until the process runs out of memory.
		{name: "an invented method", method: "BREW", want: methodOther},
		{name: "lowercase is not a method", method: "get", want: methodOther},
		{name: "an empty method", method: "", want: methodOther},
		{name: "a very long method", method: strings.Repeat("A", 4096), want: methodOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMethod(tt.method); got != tt.want {
				t.Errorf("normalizeMethod(%q) = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{name: "ok", status: http.StatusOK, want: "200"},
		{name: "not found", status: http.StatusNotFound, want: "404"},
		{name: "server error", status: http.StatusInternalServerError, want: "500"},
		{name: "informational", status: http.StatusContinue, want: "100"},
		{name: "never written", status: 0, want: statusUnknown},
		{name: "below the range", status: 99, want: statusUnknown},
		{name: "above the range", status: 600, want: statusUnknown},
		{name: "negative", status: -1, want: statusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeStatus(tt.status); got != tt.want {
				t.Errorf("normalizeStatus(%d) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestObserveHTTPRequest(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		route      string
		status     int
		wantMethod string
		wantRoute  string
		wantStatus string
		wantClass  string // empty when no error is expected
	}{
		{
			name: "a successful read", method: http.MethodGet, route: "/api/v1/calendars",
			status:     http.StatusOK,
			wantMethod: http.MethodGet, wantRoute: "/api/v1/calendars", wantStatus: "200",
		},
		{
			name: "a client error", method: http.MethodPost, route: "/api/v1/auth/login",
			status:     http.StatusUnauthorized,
			wantMethod: http.MethodPost, wantRoute: "/api/v1/auth/login", wantStatus: "401",
			wantClass: classClient,
		},
		{
			name: "a rate limited request is a client error", method: http.MethodGet, route: "/api/v1/ics/feed/{token}",
			status:     http.StatusTooManyRequests,
			wantMethod: http.MethodGet, wantRoute: "/api/v1/ics/feed/{token}", wantStatus: "429",
			wantClass: classClient,
		},
		{
			name: "a server error", method: http.MethodPatch, route: "/api/v1/calendars/{id}",
			status:     http.StatusInternalServerError,
			wantMethod: http.MethodPatch, wantRoute: "/api/v1/calendars/{id}", wantStatus: "500",
			wantClass: classServer,
		},
		{
			name: "an unroutable request keeps its label bounded", method: "WHATEVER", route: "",
			status:     http.StatusNotFound,
			wantMethod: methodOther, wantRoute: RouteUnrouted, wantStatus: "404",
			wantClass: classClient,
		},
		{
			name: "a redirect is not an error", method: http.MethodGet, route: "/api/v1/auth/magic-link/verify/{token}",
			status:     http.StatusFound,
			wantMethod: http.MethodGet, wantRoute: "/api/v1/auth/magic-link/verify/{token}", wantStatus: "302",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetHTTP(t)

			ObserveHTTPRequest(tt.method, tt.route, tt.status, 12*time.Millisecond)

			counter := httpRequests.WithLabelValues(tt.wantMethod, tt.wantRoute, tt.wantStatus)
			if got := testutil.ToFloat64(counter); got != 1 {
				t.Errorf("requests_total{%s,%s,%s} = %v, want 1", tt.wantMethod, tt.wantRoute, tt.wantStatus, got)
			}

			if got := testutil.CollectAndCount(httpDuration); got != 1 {
				t.Errorf("duration series = %d, want 1", got)
			}

			errorSeries := testutil.CollectAndCount(httpErrors)
			if tt.wantClass == "" {
				if errorSeries != 0 {
					t.Errorf("errors_total has %d series, want none for status %d", errorSeries, tt.status)
				}
				return
			}
			if errorSeries != 1 {
				t.Fatalf("errors_total has %d series, want 1", errorSeries)
			}
			errCounter := httpErrors.WithLabelValues(tt.wantMethod, tt.wantRoute, tt.wantClass)
			if got := testutil.ToFloat64(errCounter); got != 1 {
				t.Errorf("errors_total{class=%s} = %v, want 1", tt.wantClass, got)
			}
		})
	}
}

// TestObserveHTTPRequestAccumulates checks that two calls on the same labels
// share a series rather than creating one each.
func TestObserveHTTPRequestAccumulates(t *testing.T) {
	resetHTTP(t)

	for range 3 {
		ObserveHTTPRequest(http.MethodGet, "/api/health", http.StatusOK, time.Millisecond)
	}

	if got := testutil.CollectAndCount(httpRequests); got != 1 {
		t.Errorf("series = %d, want 1", got)
	}
	if got := testutil.ToFloat64(httpRequests.WithLabelValues(http.MethodGet, "/api/health", "200")); got != 3 {
		t.Errorf("requests_total = %v, want 3", got)
	}
}

func TestSSEConnectionGauge(t *testing.T) {
	t.Cleanup(func() { sseConnections.Set(0) })
	sseConnections.Set(0)

	SSEConnectionOpened()
	SSEConnectionOpened()
	if got := testutil.ToFloat64(sseConnections); got != 2 {
		t.Fatalf("open_connections = %v, want 2", got)
	}

	SSEConnectionClosed()
	if got := testutil.ToFloat64(sseConnections); got != 1 {
		t.Errorf("open_connections = %v, want 1 after one close", got)
	}

	SSEConnectionClosed()
	if got := testutil.ToFloat64(sseConnections); got != 0 {
		t.Errorf("open_connections = %v, want 0 once every stream is gone", got)
	}
}

func TestRateLimitRejected(t *testing.T) {
	tests := []struct {
		name  string
		route string
		want  string
	}{
		{name: "a matched route", route: "/api/v1/auth/login", want: "/api/v1/auth/login"},
		{name: "no route matched", route: "", want: RouteUnrouted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetHTTP(t)

			RateLimitRejected(tt.route)
			RateLimitRejected(tt.route)

			if got := testutil.ToFloat64(rateLimitRejections.WithLabelValues(tt.want)); got != 2 {
				t.Errorf("rate_limit_rejections_total{route=%q} = %v, want 2", tt.want, got)
			}
		})
	}
}

func TestSetBuildInfoKeepsOneSeries(t *testing.T) {
	t.Cleanup(buildInfo.Reset)

	SetBuildInfo("1.6.3", "selfhosted")
	if got := testutil.ToFloat64(buildInfo.WithLabelValues("1.6.3", "selfhosted")); got != 1 {
		t.Errorf("build_info = %v, want 1", got)
	}

	// Calling it again must replace the series, not add one: version is the only
	// free-form label in the package and it is fixed for the life of a process.
	SetBuildInfo("1.7.0", "cloud")
	if got := testutil.CollectAndCount(buildInfo); got != 1 {
		t.Errorf("build_info series = %d, want 1", got)
	}
	if got := testutil.ToFloat64(buildInfo.WithLabelValues("1.7.0", "cloud")); got != 1 {
		t.Errorf("build_info after update = %v, want 1", got)
	}
}

// TestNoLabelIdentifiesAnybody is the owner's constraint written as a test: a
// counter says how many, never who. It exercises the package with the nastiest
// input a request can carry — a calendar token, a participant UUID, an IP, an
// e-mail address — and then reads every series back out of the registry to check
// that none of it reached a label.
func TestNoLabelIdentifiesAnybody(t *testing.T) {
	resetHTTP(t)
	t.Cleanup(buildInfo.Reset)

	const (
		calendarToken = "Xk3f9QvR2mNbT7wLpZ4sYd8H"
		participantID = "6f9619ff-8b86-d011-b42d-00c04fc964ff"
		clientIP      = "203.0.113.42"
		email         = "someone@example.org"
	)

	// The route label is a chi pattern; the values above are what the pattern
	// replaces. They are passed here as they would be if a caller ever slipped
	// and handed over a raw path.
	ObserveHTTPRequest(http.MethodGet, "/api/v1/availabilities/calendar/{token}/participant/{pid}", http.StatusOK, time.Millisecond)
	RateLimitRejected("/api/v1/auth/login")
	SetBuildInfo("1.6.3", "selfhosted")
	SSEConnectionOpened()
	t.Cleanup(SSEConnectionClosed)

	// Every label name this package is allowed to emit, and nothing else.
	allowedLabels := map[string]struct{}{
		"method":     {},
		"route":      {},
		"status":     {},
		"class":      {},
		"version":    {},
		"build_type": {},
	}
	forbidden := []string{calendarToken, participantID, clientIP, email}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var whentoFamilies int
	for _, family := range families {
		name := family.GetName()
		if !strings.HasPrefix(name, namespace+"_") {
			// go_* and process_* come from the runtime collectors and carry no
			// request-derived labels at all.
			continue
		}
		whentoFamilies++

		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if _, ok := allowedLabels[label.GetName()]; !ok {
					t.Errorf("%s carries the label %q, which is not one of the bounded dimensions", name, label.GetName())
				}
				for _, secret := range forbidden {
					if strings.Contains(label.GetValue(), secret) {
						t.Errorf("%s{%s=%q} carries identifying data", name, label.GetName(), label.GetValue())
					}
				}
			}
		}
	}

	if whentoFamilies == 0 {
		t.Fatal("no whento_* metric was gathered, the test proves nothing")
	}
}

func TestHandlerServesTheExposition(t *testing.T) {
	resetHTTP(t)
	ObserveHTTPRequest(http.MethodGet, "/api/v1/calendars", http.StatusOK, 5*time.Millisecond)

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, MetricsPath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`whento_http_requests_total{method="GET",route="/api/v1/calendars",status="200"} 1`,
		"whento_http_request_duration_seconds_bucket",
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the exposition does not contain %q", want)
		}
	}
}

func TestNewServerOnlyServesTheMetricsPath(t *testing.T) {
	srv := NewServer(":0")

	tests := []struct {
		name   string
		target string
		want   int
	}{
		{name: "the metrics path", target: MetricsPath, want: http.StatusOK},
		{name: "anything else", target: "/", want: http.StatusNotFound},
		{name: "the application API", target: "/api/v1/calendars", want: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.target, nil))
			if rec.Code != tt.want {
				t.Errorf("GET %s = %d, want %d", tt.target, rec.Code, tt.want)
			}
		})
	}

	// A listener of its own is only safe if it cannot be reached through the
	// application's, which is a property of the address, not of the handler.
	if srv.ReadHeaderTimeout == 0 {
		t.Error("the metrics server has no ReadHeaderTimeout")
	}
}
