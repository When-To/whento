// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

// Package metrics exposes the process' Prometheus collectors and the handler
// that serves them.
//
// # No personal data, by construction
//
// A counter says how many, never who. Every label in this package is a
// dimension with a small, fixed set of values decided by the code — HTTP
// method, chi route *pattern*, status code, build variant. Nothing that
// identifies a person, a browser or a resource is ever a label value:
//
//   - no IP address and no User-Agent: they are not collected anywhere in this
//     process (see pkg/middleware.Logger, which drops them on purpose);
//   - no request path: in this API the path *is* the credential
//     (/calendar/{token}/participant/{pid}), so the route pattern is recorded
//     instead — placeholders kept, values dropped;
//   - no calendar token, participant id, user id or e-mail address: no
//     function in this package accepts one.
//
// The two label values that come off the wire are sanitised before they are
// used: an arbitrary HTTP method collapses to "other" (a client may send any
// token, and an unbounded label would grow the series set without limit), and
// a status outside 100-599 collapses to "unknown".
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "whento"

// registry is private on purpose: nothing outside this package may register a
// collector, which is what keeps the guarantee above auditable from one file.
var registry = prometheus.NewRegistry()

var (
	// httpRequests is the "rate" and "duration"-free half of the RED trio: one
	// count per completed request, broken down by outcome.
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Total number of completed HTTP requests, by method, chi route pattern and status code.",
	}, []string{"method", "route", "status"})

	// httpErrors is the "errors" of RED. It is derivable from httpRequests, and
	// kept separate anyway: an alert on error rate should not have to enumerate
	// status codes, and the client/server split is the one that decides whether
	// anybody gets woken up.
	httpErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "http",
		Name:      "errors_total",
		Help:      "Total number of HTTP requests answered with 4xx (class=client) or 5xx (class=server).",
	}, []string{"method", "route", "class"})

	// httpDuration is the "duration" of RED. The buckets run past the default
	// 10s ceiling because SSE streams are ordinary requests to this middleware
	// and stay open for hours; they land in +Inf, which is where a
	// never-ending request belongs.
	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "HTTP request latency in seconds, by method and chi route pattern.",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"method", "route"})

	// sseConnections tracks how many event streams are open right now. It is
	// the one number that says whether a shutdown is going to have to wait.
	sseConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: "sse",
		Name:      "open_connections",
		Help:      "Number of server-sent event streams currently open.",
	})

	// rateLimitRejections counts refusals only. It says a route is being
	// hammered; it cannot say by whom, and that is deliberate.
	rateLimitRejections = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "rate_limit_rejections_total",
		Help:      "Total number of requests refused with 429 by the rate limiter, by chi route pattern.",
	}, []string{"route"})

	// buildInfo is the usual constant-1 gauge: the values live in the labels so
	// a dashboard can join on them. Both are fixed for the life of the process.
	buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "build_info",
		Help:      "Always 1, labelled with the version and build variant of the running binary.",
	}, []string{"version", "build_type"})
)

func init() {
	registry.MustRegister(
		httpRequests,
		httpErrors,
		httpDuration,
		sseConnections,
		rateLimitRejections,
		buildInfo,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// Error classes for httpErrors.
const (
	classClient = "client"
	classServer = "server"
)

// methodOther is the bucket every non-standard HTTP method falls into.
const methodOther = "other"

// statusUnknown labels a status code outside the range HTTP defines.
const statusUnknown = "unknown"

// RouteUnrouted is the route label used when no chi route matched. Callers
// must never substitute the raw path: it carries the credential in this API.
const RouteUnrouted = "unrouted"

// ObserveHTTPRequest records one completed request.
//
// route must be a chi route pattern (or RouteUnrouted), never r.URL.Path.
func ObserveHTTPRequest(method, route string, status int, d time.Duration) {
	m := normalizeMethod(method)
	if route == "" {
		route = RouteUnrouted
	}

	httpRequests.WithLabelValues(m, route, normalizeStatus(status)).Inc()
	httpDuration.WithLabelValues(m, route).Observe(d.Seconds())

	switch {
	case status >= 500 && status <= 599:
		httpErrors.WithLabelValues(m, route, classServer).Inc()
	case status >= 400 && status < 500:
		httpErrors.WithLabelValues(m, route, classClient).Inc()
	}
}

// SSEConnectionOpened records a stream that just opened.
func SSEConnectionOpened() { sseConnections.Inc() }

// SSEConnectionClosed records a stream that just closed.
func SSEConnectionClosed() { sseConnections.Dec() }

// RateLimitRejected records one request refused by the rate limiter.
func RateLimitRejected(route string) {
	if route == "" {
		route = RouteUnrouted
	}
	rateLimitRejections.WithLabelValues(route).Inc()
}

// SetBuildInfo publishes the version and build variant of this binary. Both
// values are fixed at link time, so this adds exactly one series.
func SetBuildInfo(version, buildType string) {
	buildInfo.Reset()
	buildInfo.WithLabelValues(version, buildType).Set(1)
}

// normalizeMethod keeps the method label bounded. Any token is a valid method
// on the wire, so an unsanitised r.Method is a label a client can enumerate.
func normalizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		return method
	default:
		return methodOther
	}
}

// normalizeStatus keeps the status label inside the range HTTP defines.
func normalizeStatus(status int) string {
	if status < 100 || status > 599 {
		return statusUnknown
	}
	return strconv.Itoa(status)
}

// Handler serves the Prometheus exposition format.
//
// It is deliberately not a chi route: see NewServer. Mount it on the
// application router and every metric this process holds becomes readable by
// anyone who can reach the app.
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorHandling:       promhttp.HTTPErrorOnError,
		MaxRequestsInFlight: 4,
		Timeout:             10 * time.Second,
	})
}

// MetricsPath is where NewServer exposes the exposition format.
const MetricsPath = "/metrics"

// NewServer builds the HTTP server that exposes MetricsPath, to be run on a
// listener of its own.
//
// A separate listener rather than a route on the application router, because
// the exposition is operational data — route patterns, error rates, pool
// saturation, uptime, Go build details — and none of it is meant for the
// public. Keeping it on its own port means exposure is decided by what the
// operator publishes (a docker-compose port mapping, a Kubernetes Service),
// not by middleware ordering on a router that also serves anonymous traffic;
// there is no auth code to get wrong, and no way for a later route change to
// expose it by accident. The caller enables it explicitly; unset means no
// listener at all.
func NewServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(MetricsPath, Handler())

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
