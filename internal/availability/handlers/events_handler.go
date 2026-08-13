// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/broadcast"
	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/logger"
)

const (
	// heartbeatInterval keeps the connection from being reaped. Reverse proxies close an
	// idle response after a minute or so by default — nginx's proxy_read_timeout is 60s —
	// and a stream that carries nothing between two quiet answers looks exactly like a
	// hung backend. A comment line is the cheapest thing that counts as traffic.
	heartbeatInterval = 25 * time.Second

	// retryHint tells the browser how long to wait before reconnecting. EventSource
	// reconnects on its own; this only sets the delay, and three seconds keeps a restart
	// of the server from turning into a stampede.
	retryHint = 3 * time.Second
)

// CalendarLookup is the one thing this handler asks of the calendar repository: does
// this public token name a calendar? Everything else about the stream is the token.
type CalendarLookup interface {
	GetByPublicToken(ctx context.Context, token string) (uuid.UUID, error)
}

// EventsHandler streams change notices for one calendar.
//
// The notices carry no data. A browser that receives one refetches through the ordinary
// read path, which keeps a single read model and means the stream can never hand out a
// field its holder is not entitled to. It also makes the stream cheap: a notice is four
// bytes on the wire whatever changed.
type EventsHandler struct {
	calendars CalendarLookup
	broker    broadcast.Broker

	// heartbeat is heartbeatInterval in production. It is a field only so that a test
	// can watch several heartbeats go by without taking half a minute over it.
	heartbeat time.Duration
}

// NewEventsHandler creates the SSE handler for participant calendars.
func NewEventsHandler(calendars CalendarLookup, broker broadcast.Broker) *EventsHandler {
	return &EventsHandler{calendars: calendars, broker: broker, heartbeat: heartbeatInterval}
}

// Stream handles GET /api/v1/availabilities/calendar/{token}/events
//
//	@Summary		Stream calendar changes
//	@Description	Server-sent events announcing that the calendar changed. Each `update` event is a notice with no payload: fetch the range summary again. Public endpoint, authorised by the calendar token like the rest of the participant API.
//	@Tags			Availabilities
//	@Produce		text/event-stream
//	@Param			token	path	string	true	"Calendar public token"
//	@Success		200		{string}	string	"event: update"
//	@Failure		404		{object}	httputil.ErrorResponse	"Calendar not found"
//	@Failure		500		{object}	httputil.ErrorResponse	"Streaming unsupported"
//	@Router			/api/v1/availabilities/calendar/{token}/events [get]
func (h *EventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	log := logger.FromContext(r.Context())

	// Resolved before a single byte goes out: once the stream has started the status
	// line is spent, and an unknown token would look to the browser like a working
	// stream that never says anything.
	if _, err := h.calendars.GetByPublicToken(r.Context(), token); err != nil {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Calendar not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Without flushing, every notice sits in a buffer until the stream ends, which
		// is the opposite of the point.
		httputil.Error(w, http.StatusInternalServerError, httputil.ErrCodeInternal, "Streaming unsupported")
		return
	}

	// The server arms a write deadline once, when it reads the request headers, and
	// nothing rearms it. For an ordinary answer that is the whole point; for a stream
	// meant to stay open for hours it is fatal — every write after WriteTimeout fails,
	// starting with the first heartbeat, and the browser reconnects on the retry hint
	// for ever. Clearing the deadline is what makes this connection long-lived.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		if errors.Is(err, http.ErrNotSupported) {
			// A ResponseWriter that cannot be unwrapped down to the connection: the
			// stream still works, it just keeps whatever deadline the server set.
			log.Debug("events: write deadlines are not supported by this response writer", "token", token)
		} else {
			log.Warn("events: could not clear the write deadline", "token", token, "error", err)
		}
	}

	// Subscribe before announcing the stream is open. A write landing in between would
	// otherwise be missed by a browser that believes it is now up to date.
	notices, stop := h.broker.Subscribe(r.Context(), token)
	defer stop()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	// Nginx buffers proxied responses by default, which holds notices back until the
	// buffer fills. This is the documented opt-out.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintf(w, "retry: %d\n\n", retryHint.Milliseconds()); err != nil {
		log.Debug("events: stream write failed", "token", token, "error", err)
		return
	}
	flusher.Flush()

	interval := h.heartbeat
	if interval <= 0 {
		interval = heartbeatInterval
	}

	heartbeat := time.NewTicker(interval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			// The browser went away: a closed tab, a lost network, a reload.
			return

		case _, open := <-notices:
			if !open {
				// The broker is shutting down. Closing cleanly lets EventSource
				// reconnect to whichever instance comes back.
				return
			}
			if _, err := fmt.Fprint(w, "event: update\ndata: {}\n\n"); err != nil {
				log.Debug("events: stream write failed", "token", token, "error", err)
				return
			}
			flusher.Flush()

		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
