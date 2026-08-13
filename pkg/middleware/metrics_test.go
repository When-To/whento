// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/metrics"
)

// scrape reads the exposition the way Prometheus would. The collectors are
// package-level in pkg/metrics, so tests here assert on the text rather than
// reaching into them, which is also the closest thing to what an operator sees.
func scrape(t *testing.T) string {
	t.Helper()

	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, metrics.MetricsPath, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("scrape returned %d", rec.Code)
	}

	return rec.Body.String()
}

// TestMetricsRecordsTheRouteNeverThePath is the metric-side twin of
// TestLoggerNeverLogsTheRequestPath. The path in this API is the credential; a
// label carrying it would put a replayable calendar link in every scrape, in
// every Prometheus retention window and on every dashboard — and would grow the
// series set by one per calendar ever shared.
func TestMetricsRecordsTheRouteNeverThePath(t *testing.T) {
	const (
		calendarToken = "Qb7WmT4xLp9ZsRd2Kv6Ynf3H"
		participantID = "2f9619ff-8b86-d011-b42d-00c04fc964aa"
	)

	tests := []struct {
		name    string
		pattern string
		target  string
		method  string
		status  int
		want    string
		secrets []string
	}{
		{
			name:    "a participant availability link",
			pattern: "/availabilities/calendar/{token}/participant/{pid}",
			target:  "/availabilities/calendar/" + calendarToken + "/participant/" + participantID,
			method:  http.MethodGet,
			status:  http.StatusOK,
			want:    `whento_http_requests_total{method="GET",route="/availabilities/calendar/{token}/participant/{pid}",status="200"} 1`,
			secrets: []string{calendarToken, participantID},
		},
		{
			name:    "a failed write",
			pattern: "/calendars/{id}",
			target:  "/calendars/" + calendarToken,
			method:  http.MethodPatch,
			status:  http.StatusInternalServerError,
			want:    `whento_http_requests_total{method="PATCH",route="/calendars/{id}",status="500"} 1`,
			secrets: []string{calendarToken},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := chi.NewRouter()
			router.Use(Metrics)
			router.Method(tt.method, tt.pattern, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d — the route did not match", rec.Code, tt.status)
			}

			body := scrape(t)
			if !strings.Contains(body, tt.want) {
				t.Errorf("the exposition does not contain %q", tt.want)
			}
			for _, secret := range tt.secrets {
				if strings.Contains(body, secret) {
					t.Errorf("the exposition leaks %q", secret)
				}
			}
		})
	}
}

// TestMetricsCountsErrors covers the "E" of RED, including the case chi cannot
// give a pattern for.
func TestMetricsCountsErrors(t *testing.T) {
	router := chi.NewRouter()
	router.Use(Metrics)
	router.Get("/metrics-test/boom", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics-test/boom", nil))

	// A miss has no pattern. The raw path must not stand in for it: an attacker
	// probing /calendars/<stolen-token> would otherwise mint a label per guess.
	missRec := httptest.NewRecorder()
	router.ServeHTTP(missRec, httptest.NewRequest(http.MethodGet, "/metrics-test/nope", nil))
	if missRec.Code != http.StatusNotFound {
		t.Fatalf("the miss returned %d, want 404", missRec.Code)
	}

	body := scrape(t)
	for _, want := range []string{
		`whento_http_errors_total{class="server",method="GET",route="/metrics-test/boom"} 1`,
		`whento_http_errors_total{class="client",method="GET",route="unrouted"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the exposition does not contain %q", want)
		}
	}
	if strings.Contains(body, "/metrics-test/nope") {
		t.Error("the exposition contains the path of an unmatched request")
	}
}

// TestMetricsRecordsAnImplicit200 covers a handler that writes nothing: the
// client still sees a 200, and the metric must say so rather than "unknown".
func TestMetricsRecordsAnImplicit200(t *testing.T) {
	router := chi.NewRouter()
	router.Use(Metrics)
	router.Get("/metrics-test/silent", func(http.ResponseWriter, *http.Request) {})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics-test/silent", nil))

	want := `whento_http_requests_total{method="GET",route="/metrics-test/silent",status="200"} 1`
	if body := scrape(t); !strings.Contains(body, want) {
		t.Errorf("the exposition does not contain %q", want)
	}
}

// TestMetricsBoundsTheMethodLabel: the method comes off the wire and any token
// is legal, so an unsanitised label is one an attacker can enumerate.
func TestMetricsBoundsTheMethodLabel(t *testing.T) {
	// Straight through the middleware rather than through chi, which refuses to
	// register a method it does not know. Go's HTTP server has no such qualm:
	// it hands the handler whatever token the client sent on the request line.
	handler := Metrics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("BREW", "/metrics-test/teapot", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", rec.Code)
	}

	body := scrape(t)
	if !strings.Contains(body, `method="other"`) {
		t.Error("an unknown method was not collapsed into the other bucket")
	}
	if strings.Contains(body, `method="BREW"`) {
		t.Error("an arbitrary method reached a label value")
	}
}

// TestMetricsPassesTheRequestThrough: instrumentation that swallows a request
// is worse than no instrumentation.
func TestMetricsPassesTheRequestThrough(t *testing.T) {
	var reached bool
	rec := httptest.NewRecorder()
	Metrics(okHandler(&reached)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !reached {
		t.Error("the metrics middleware swallowed the request")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// TestRateLimitRejectionsAreCounted checks the counter the audit asked for, and
// that the bucket key — an IP, here — never becomes a label.
func TestRateLimitRejectionsAreCounted(t *testing.T) {
	rl := NewRateLimiter(nil)
	defer rl.Stop()

	router := chi.NewRouter()
	router.Use(Metrics)
	router.With(rl.Limit(RateLimitConfig{
		Requests: 1,
		Window:   time.Minute,
		KeyFunc:  IPKeyFunc,
	})).Get("/metrics-test/limited", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const clientIP = "198.51.100.7"
	var lastCode int
	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "/metrics-test/limited", nil)
		req.RemoteAddr = clientIP + ":51234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("the limiter never refused: last status %d", lastCode)
	}

	body := scrape(t)
	if !strings.Contains(body, `whento_rate_limit_rejections_total{route="/metrics-test/limited"}`) {
		t.Error("the rejection was not counted against its route")
	}
	if strings.Contains(body, clientIP) {
		t.Error("the exposition leaks the client IP")
	}
}
