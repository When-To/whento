// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/broadcast"
)

// A stream that silently stops working looks exactly like a calendar where nobody has
// answered yet, so the failures worth pinning are the quiet ones: a notice for the wrong
// calendar, a connection reaped by a proxy for want of a heartbeat, and a subscription
// that outlives the browser that opened it.

type fakeCalendars struct {
	err   error
	token string
}

func (f *fakeCalendars) GetByPublicToken(_ context.Context, token string) (uuid.UUID, error) {
	f.token = token
	if f.err != nil {
		return uuid.Nil, f.err
	}

	return uuid.New(), nil
}

// streamRequest builds a request carrying the chi URL parameter, with a context the
// caller can cancel to play the part of a browser going away.
func streamRequest(token string) (*http.Request, context.CancelFunc) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/availabilities/calendar/"+token+"/events", nil)

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("token", token)
	ctx, cancel := context.WithCancel(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

	return req.WithContext(ctx), cancel
}

// liveRecorder is an httptest.ResponseRecorder that can be read while the handler is
// still writing. The plain recorder is only safe to read once ServeHTTP returns, which
// never happens for a stream.
type liveRecorder struct {
	mu     sync.Mutex
	header http.Header
	status int
	body   strings.Builder
}

func newLiveRecorder() *liveRecorder {
	return &liveRecorder{header: make(http.Header)}
}

func (r *liveRecorder) Header() http.Header { return r.header }

func (r *liveRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.body.Write(p)
}

func (r *liveRecorder) WriteHeader(status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

func (r *liveRecorder) Flush() {}

func (r *liveRecorder) snapshot() (int, string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.status, r.body.String()
}

// waitFor polls until the condition holds or the window expires. The handler writes from
// its own goroutine, so assertions cannot be made immediately.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("timed out waiting for %s", what)
}

func TestAnUnknownTokenIs404(t *testing.T) {
	handler := NewEventsHandler(&fakeCalendars{err: errors.New("no rows")}, broadcast.NewMemoryBroker())

	req, cancel := streamRequest("nope")
	defer cancel()
	rec := httptest.NewRecorder()
	handler.Stream(rec, req)

	// Answered before the stream opens. Once the status line is spent an unknown token
	// would look like a working stream that simply never says anything, and the browser
	// would keep it open for ever.
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "event-stream") {
		t.Errorf("Content-Type = %q; the rejection was sent as a stream", ct)
	}
}

func TestTheStreamAnnouncesItself(t *testing.T) {
	broker := broadcast.NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })
	handler := NewEventsHandler(&fakeCalendars{}, broker)

	req, cancel := streamRequest("public-token")
	rec := newLiveRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Stream(rec, req)
	}()

	waitFor(t, "the stream to open", func() bool {
		status, _ := rec.snapshot()

		return status == http.StatusOK
	})

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	// A cached stream is a stream that never updates.
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	// Nginx buffers proxied responses by default, which holds notices until the buffer
	// fills. Without this header the feature works in development and not behind a proxy.
	if rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Error("X-Accel-Buffering is not disabled; notices would be buffered by nginx")
	}

	_, body := rec.snapshot()
	if !strings.Contains(body, "retry:") {
		t.Errorf("the stream sends no reconnection delay:\n%q", body)
	}

	cancel()
	<-done
}

func TestANoticeReachesTheBrowser(t *testing.T) {
	broker := broadcast.NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })
	handler := NewEventsHandler(&fakeCalendars{}, broker)

	req, cancel := streamRequest("public-token")
	rec := newLiveRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Stream(rec, req)
	}()

	waitFor(t, "the stream to open", func() bool {
		status, _ := rec.snapshot()

		return status == http.StatusOK
	})

	if err := broker.Publish(context.Background(), "public-token"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	waitFor(t, "the update event", func() bool {
		_, body := rec.snapshot()

		return strings.Contains(body, "event: update")
	})

	cancel()
	<-done
}

// TestNoticesForOtherCalendarsAreNotDelivered is the privacy case. Calendars are
// addressed by a public token, and a notice crossing streams would tell the holder of
// one link that a different calendar had just been edited.
func TestNoticesForOtherCalendarsAreNotDelivered(t *testing.T) {
	broker := broadcast.NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })
	handler := NewEventsHandler(&fakeCalendars{}, broker)

	req, cancel := streamRequest("mine")
	rec := newLiveRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Stream(rec, req)
	}()

	waitFor(t, "the stream to open", func() bool {
		status, _ := rec.snapshot()

		return status == http.StatusOK
	})

	if err := broker.Publish(context.Background(), "theirs"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	if _, body := rec.snapshot(); strings.Contains(body, "event: update") {
		t.Errorf("a notice for another calendar was delivered:\n%q", body)
	}

	cancel()
	<-done
}

// TestTheBrowserGoingAwayEndsTheStream is what stops the leak: a goroutine and a
// subscription per abandoned tab, held until the process restarts.
func TestTheBrowserGoingAwayEndsTheStream(t *testing.T) {
	broker := broadcast.NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })
	handler := NewEventsHandler(&fakeCalendars{}, broker)

	req, cancel := streamRequest("public-token")
	rec := newLiveRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Stream(rec, req)
	}()

	waitFor(t, "the stream to open", func() bool {
		status, _ := rec.snapshot()

		return status == http.StatusOK
	})

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("the handler outlived the request context")
	}
}

// TestTheBrokerShuttingDownEndsTheStream covers server shutdown. Returning cleanly lets
// EventSource reconnect to whichever instance comes back, instead of the handler sitting
// on a channel nobody will ever write to.
func TestTheBrokerShuttingDownEndsTheStream(t *testing.T) {
	broker := broadcast.NewMemoryBroker()
	handler := NewEventsHandler(&fakeCalendars{}, broker)

	req, cancel := streamRequest("public-token")
	defer cancel()
	rec := newLiveRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Stream(rec, req)
	}()

	waitFor(t, "the stream to open", func() bool {
		status, _ := rec.snapshot()

		return status == http.StatusOK
	})

	if err := broker.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("the handler survived the broker shutting down")
	}
}

// TestTheTokenIsCheckedAgainstTheRepository guards the one authorisation step there is.
// Possession of the token is the whole credential, so a stream that opened without
// resolving it would hand a live feed to any string at all.
func TestTheTokenIsCheckedAgainstTheRepository(t *testing.T) {
	calendars := &fakeCalendars{}
	broker := broadcast.NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })
	handler := NewEventsHandler(calendars, broker)

	req, cancel := streamRequest("the-token-from-the-url")
	rec := newLiveRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Stream(rec, req)
	}()

	waitFor(t, "the stream to open", func() bool {
		status, _ := rec.snapshot()

		return status == http.StatusOK
	})

	if calendars.token != "the-token-from-the-url" {
		t.Errorf("the repository was asked about %q, want the token from the URL", calendars.token)
	}

	cancel()
	<-done
}

// TestTheStreamOutlivesTheServerWriteTimeout is the case every test above is blind to.
//
// httptest.NewRecorder is not a connection: it has no deadline, so a stream written into
// one survives anything. A real http.Server arms its WriteTimeout once, when it reads the
// request headers, and never rearms it — so every write past that point fails, starting
// with the very first heartbeat, and the browser reconnects on the retry hint for ever.
// Nothing about that is visible until the handler is mounted on an actual server whose
// WriteTimeout is shorter than the test is willing to wait.
func TestTheStreamOutlivesTheServerWriteTimeout(t *testing.T) {
	const (
		writeTimeout = 300 * time.Millisecond
		heartbeat    = 50 * time.Millisecond
	)

	tests := []struct {
		name string
		// publish sends a notice once the server's write deadline has certainly passed.
		publish bool
		// want is the line the browser must still be able to receive afterwards.
		want string
	}{
		{
			name: "a heartbeat still lands after the write timeout",
			want: ": keep-alive",
		},
		{
			name:    "a notice still lands after the write timeout",
			publish: true,
			want:    "event: update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker := broadcast.NewMemoryBroker()
			t.Cleanup(func() { _ = broker.Close() })

			handler := NewEventsHandler(&fakeCalendars{}, broker)
			handler.heartbeat = heartbeat

			router := chi.NewRouter()
			router.Get("/calendar/{token}/events", handler.Stream)

			server := httptest.NewUnstartedServer(router)
			server.Config.WriteTimeout = writeTimeout
			server.Start()
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/calendar/public-token/events", nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}

			started := time.Now()
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatalf("open the stream: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			// The notice is sent well past the deadline the server armed when it read
			// the request headers.
			if tt.publish {
				timer := time.AfterFunc(writeTimeout+2*heartbeat, func() {
					_ = broker.Publish(context.Background(), "public-token")
				})
				defer timer.Stop()
			}

			// Read continuously rather than sleeping first: a line that is still sitting
			// in the socket buffer says nothing about when it was written, and the
			// heartbeats sent before the deadline are exactly that.
			settled := writeTimeout + 2*heartbeat
			reader := bufio.NewReader(resp.Body)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					t.Fatalf(
						"the stream died %v after it opened, with a WriteTimeout of %v: %v",
						time.Since(started).Round(time.Millisecond), writeTimeout, err,
					)
				}

				// Only a line that arrived past the deadline proves anything.
				if strings.HasPrefix(line, tt.want) && time.Since(started) > settled {
					return
				}
			}
		})
	}
}
