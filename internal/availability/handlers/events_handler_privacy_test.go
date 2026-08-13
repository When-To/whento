// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/whento/pkg/broadcast"
	"github.com/whento/pkg/logger"
)

// The calendar's public token is the credential: whoever holds it can read and
// write the calendar, and there is no second factor to stop them. This handler
// used to write it into four log lines, two of which arrived with the fix that
// clears the write deadline — which is exactly how a leak like this gets in, one
// helpful diagnostic at a time.
//
// These tests fail if any of them comes back.

// lockedBuffer lets the test read the log while the handler goroutine is still
// writing to it. slog handlers serialise their own writes, but not against a
// reader in another goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// captureLogsAtDebug redirects the default logger for one test. Debug is the
// level two of the four lines are written at, so anything less would let them
// pass unobserved.
func captureLogsAtDebug(t *testing.T) *lockedBuffer {
	t.Helper()

	original := logger.Default()
	t.Cleanup(func() { logger.SetDefault(original) })

	buf := &lockedBuffer{}
	logger.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	return buf
}

// deadlineRecorder answers SetWriteDeadline with an error of its choosing, which
// is what drives the `warn` branch — the one visible at the default log level.
type deadlineRecorder struct {
	*liveRecorder
	err error
}

func (d *deadlineRecorder) SetWriteDeadline(time.Time) error { return d.err }

// failingRecorder fails every write, which is what drives the two "stream write
// failed" lines.
type failingRecorder struct {
	*liveRecorder
	err error
}

func (f *failingRecorder) Write([]byte) (int, error) { return 0, f.err }

// TestStreamNeverLogsTheCalendarToken drives each branch that logs and checks
// the token is in none of them.
func TestStreamNeverLogsTheCalendarToken(t *testing.T) {
	// Distinctive enough that a substring search cannot miss it, and shaped like
	// the real thing: 64 hex characters, as generateToken produces.
	const token = "9f3c0d7e2a4b6185c3d0f7e9a2b4c6d81f3e5a7c9b0d2e4f6a8c1b3d5e7f9012"

	tests := []struct {
		name string
		// writer builds the ResponseWriter that provokes the branch.
		writer func(*liveRecorder) http.ResponseWriter
		// wantLine is a fragment of the message expected, so that a test that
		// silently stopped reaching its branch fails instead of passing empty.
		wantLine string
		// blocks is true when the handler stays in its loop and the test has to
		// cancel the request to get it back.
		blocks bool
	}{
		{
			name:     "write deadlines are not supported",
			writer:   func(rec *liveRecorder) http.ResponseWriter { return rec },
			wantLine: "write deadlines are not supported",
			blocks:   true,
		},
		{
			name: "the write deadline cannot be cleared",
			writer: func(rec *liveRecorder) http.ResponseWriter {
				return &deadlineRecorder{liveRecorder: rec, err: errors.New("connection is gone")}
			},
			wantLine: "could not clear the write deadline",
			blocks:   true,
		},
		{
			name: "the stream write fails",
			writer: func(rec *liveRecorder) http.ResponseWriter {
				return &failingRecorder{liveRecorder: rec, err: errors.New("broken pipe")}
			},
			wantLine: "stream write failed",
			blocks:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogsAtDebug(t)

			broker := broadcast.NewMemoryBroker()
			t.Cleanup(func() { _ = broker.Close() })
			handler := NewEventsHandler(&fakeCalendars{}, broker)

			req, cancel := streamRequest(token)
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				handler.Stream(tt.writer(newLiveRecorder()), req)
			}()

			waitFor(t, "the branch to log", func() bool {
				return strings.Contains(buf.String(), tt.wantLine)
			})

			if tt.blocks {
				cancel()
			}
			<-done

			written := buf.String()
			if strings.Contains(written, token) {
				t.Errorf("the log carries the calendar token in clear:\n%s", written)
			}
			// The point is not silence: an operator still has to be able to tell
			// a run of failures on one calendar from failures spread over many.
			if !strings.Contains(written, "calendar_ref") {
				t.Errorf("the log carries no correlation tag at all:\n%s", written)
			}
			if strings.Contains(written, `"token"`) {
				t.Errorf("a `token` field is back in the log:\n%s", written)
			}
		})
	}
}

// TestStreamFingerprintTellsCalendarsApart: a tag that were the same for every
// calendar would satisfy the test above and be worthless. Two tokens must not
// produce the same one.
func TestStreamFingerprintTellsCalendarsApart(t *testing.T) {
	first := logger.Fingerprint("token-one")
	second := logger.Fingerprint("token-two")

	if first == second {
		t.Error("two calendars share a correlation tag")
	}
	if first == "" || second == "" {
		t.Error("the correlation tag is empty")
	}
}
