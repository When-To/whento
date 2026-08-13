// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/whento/pkg/metrics"
)

// Metrics records the RED trio — request count, error count, latency histogram —
// for every request that goes through the router.
//
// It labels requests with the chi route *pattern*, exactly as Logger does and
// for the same two reasons. The first is privacy: the path in this API carries
// the calendar token and the participant id, so a metric keyed on the path is a
// metric that names a calendar and a person. The second is that Prometheus
// cannot hold an unbounded label anyway — one series per calendar token would
// grow the series set with every link ever shared.
//
// Health endpoints are counted here even though Logger drops them: a counter
// costs nothing per request, and "the readiness probe started failing" is
// precisely what an operator wants a graph of.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// The route pattern is only known once chi has matched, inside
		// next.ServeHTTP — hence reading it from the deferred call.
		//
		//nolint:contextcheck // as in Logger: the closure does read r.Context(),
		// contextcheck cannot see through a deferred literal.
		defer func() {
			metrics.ObserveHTTPRequest(r.Method, routePattern(r), responseStatus(ww), time.Since(start))
		}()

		next.ServeHTTP(ww, r)
	})
}

// responseStatus reports the status the client saw. A handler that returns
// without touching the header still produces a 200 on the wire, which the
// wrapper records as 0.
func responseStatus(ww middleware.WrapResponseWriter) int {
	if status := ww.Status(); status != 0 {
		return status
	}
	return http.StatusOK
}
