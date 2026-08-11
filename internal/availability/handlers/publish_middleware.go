// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/whento/pkg/broadcast"
	"github.com/whento/pkg/logger"
)

// PublishChanges announces that a calendar changed, after any request that changed it.
//
// Deliberately a middleware rather than a call at the end of each service method. The
// participant API has nine write paths today — availabilities, recurrences, exceptions —
// and a tenth added next month would silently not notify anyone. Here there is one place
// to get right, and a new route under the same mount is covered by existing.
//
// It fires on the response rather than the request, so a rejected write announces
// nothing: a 403 or a validation failure changed no state, and telling every open browser
// to refetch would be pure noise.
func PublishChanges(broker broadcast.Broker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutating(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			recorder := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(recorder, r)

			if recorder.Status() < 200 || recorder.Status() >= 300 {
				return
			}

			// The token names the calendar and is what watchers subscribe with, so it is
			// the topic. Reading it after the handler is deliberate: chi fills the route
			// context as it matches, and a route that did not match has no token to read.
			token := chi.URLParam(r, "token")
			if token == "" {
				return
			}

			// The request context is still live here — it is cancelled when the whole
			// chain returns, not when the handler does — and publishing is a single
			// round trip, so there is no need to detach from it.
			if err := broker.Publish(r.Context(), token); err != nil {
				// Already delivered locally by the broker; this is the wider fan-out
				// failing. The write itself stands, so this is a warning, not a failure.
				logger.FromContext(r.Context()).Warn("could not announce a calendar change",
					"error", err)
			}
		})
	}
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
