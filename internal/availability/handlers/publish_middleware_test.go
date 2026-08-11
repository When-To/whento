// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

// This middleware is the reason a browser hears about anything at all. It is placed on
// the route mount rather than at the end of nine service methods, so what has to hold is
// that it fires for the writes and stays quiet for everything else — a notice after a
// rejected write is noise, and a missed notice is the feature not working.

type recordingBroker struct {
	mu     sync.Mutex
	topics []string
	err    error
}

func (b *recordingBroker) Publish(_ context.Context, topic string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.topics = append(b.topics, topic)

	return b.err
}

func (b *recordingBroker) Subscribe(context.Context, string) (<-chan struct{}, func()) {
	ch := make(chan struct{})

	return ch, func() { close(ch) }
}

func (b *recordingBroker) Close() error { return nil }

func (b *recordingBroker) published() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return append([]string(nil), b.topics...)
}

// route mounts the middleware the way main.go does, so the chi URL parameter is filled
// by real routing rather than by hand.
func route(broker *recordingBroker, status int) http.Handler {
	router := chi.NewRouter()
	router.Route("/calendar/{token}", func(r chi.Router) {
		r.Use(PublishChanges(broker))
		r.HandleFunc("/participant/{pid}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		})
	})

	return router
}

func TestAWriteAnnouncesItsCalendar(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			broker := &recordingBroker{}
			rec := httptest.NewRecorder()
			route(broker, http.StatusOK).ServeHTTP(rec,
				httptest.NewRequest(method, "/calendar/public-token/participant/p1", nil))

			if got := broker.published(); len(got) != 1 || got[0] != "public-token" {
				t.Errorf("published %v, want [public-token]", got)
			}
		})
	}
}

// TestAReadAnnouncesNothing matters because the read path is by far the busiest: the
// grid refetches its range summary on every navigation. Publishing there would have
// every viewer telling every other viewer to refetch, for ever.
func TestAReadAnnouncesNothing(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			broker := &recordingBroker{}
			rec := httptest.NewRecorder()
			route(broker, http.StatusOK).ServeHTTP(rec,
				httptest.NewRequest(method, "/calendar/public-token/participant/p1", nil))

			if got := broker.published(); len(got) != 0 {
				t.Errorf("a %s published %v", method, got)
			}
		})
	}
}

// TestARejectedWriteAnnouncesNothing covers the case the middleware exists to get right.
// A 403 or a validation failure changed no state, so telling every open browser to
// refetch would be pure noise — and on a calendar under a burst of rejected writes, a
// lot of it.
func TestARejectedWriteAnnouncesNothing(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusConflict, http.StatusTooManyRequests,
		http.StatusInternalServerError,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			broker := &recordingBroker{}
			rec := httptest.NewRecorder()
			route(broker, status).ServeHTTP(rec,
				httptest.NewRequest(http.MethodPost, "/calendar/public-token/participant/p1", nil))

			if got := broker.published(); len(got) != 0 {
				t.Errorf("a %d published %v", status, got)
			}
		})
	}
}

func TestEverySuccessStatusAnnounces(t *testing.T) {
	// 201 for a created availability, 204 for a deleted one: both are real changes.
	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			broker := &recordingBroker{}
			rec := httptest.NewRecorder()
			route(broker, status).ServeHTTP(rec,
				httptest.NewRequest(http.MethodDelete, "/calendar/public-token/participant/p1", nil))

			if got := broker.published(); len(got) != 1 {
				t.Errorf("a %d published %v, want one notice", status, got)
			}
		})
	}
}

// TestTheTopicIsTheCalendarToken pins what watchers subscribe with. A notice published
// under anything else reaches nobody, and the feature fails silently.
func TestTheTopicIsTheCalendarToken(t *testing.T) {
	broker := &recordingBroker{}
	rec := httptest.NewRecorder()
	route(broker, http.StatusCreated).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/calendar/abc123/participant/p1", nil))

	if got := broker.published(); len(got) != 1 || got[0] != "abc123" {
		t.Errorf("published %v, want [abc123]", got)
	}
}

// TestAFailedFanOutDoesNotFailTheWrite records the priority: the participant's answer is
// saved, and the worst case of a Redis outage is that other browsers refresh a little
// later than they might have.
func TestAFailedFanOutDoesNotFailTheWrite(t *testing.T) {
	broker := &recordingBroker{err: errors.New("connection refused")}
	rec := httptest.NewRecorder()
	route(broker, http.StatusCreated).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/calendar/public-token/participant/p1", nil))

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want the handler's 201 — the write must stand", rec.Code)
	}
}

// TestTheResponseIsUntouched guards the wrapping. The middleware wraps the writer to
// read the status back, and a wrapper that swallowed the body would break every write
// endpoint at once.
func TestTheResponseIsUntouched(t *testing.T) {
	broker := &recordingBroker{}

	router := chi.NewRouter()
	router.Route("/calendar/{token}", func(r chi.Router) {
		r.Use(PublishChanges(broker))
		r.Post("/participant/{pid}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true,"data":{"date":"2027-03-10"}}`))
		})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/calendar/public-token/participant/p1", nil))

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"2027-03-10"`) {
		t.Errorf("the body did not survive the wrapper: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

// TestARouteWithoutATokenAnnouncesNothing covers the mount being reused elsewhere. With
// no token there is no topic, and publishing under the empty string would put unrelated
// writes on one shared channel.
func TestARouteWithoutATokenAnnouncesNothing(t *testing.T) {
	broker := &recordingBroker{}

	router := chi.NewRouter()
	router.Use(PublishChanges(broker))
	router.Post("/something-else", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/something-else", nil))

	if got := broker.published(); len(got) != 0 {
		t.Errorf("published %v for a route with no calendar token", got)
	}
}
