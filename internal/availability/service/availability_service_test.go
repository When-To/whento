// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/logger"
	"github.com/whento/whento/internal/availability/models"
)

func TestCalculateMaxSimultaneousParticipants(t *testing.T) {
	tests := []struct {
		name         string
		participants []models.ParticipantAvailabilitySummary
		expected     int
		description  string
	}{
		{
			name:         "Empty participants",
			participants: []models.ParticipantAvailabilitySummary{},
			expected:     0,
			description:  "No participants should return 0",
		},
		{
			name: "Two participants with no overlap",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("08:00"),
					EndTime:         stringPtr("12:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("14:00"),
					EndTime:         stringPtr("18:00"),
				},
			},
			expected:    1,
			description: "Two participants without overlap should have max 1 simultaneous",
		},
		{
			name: "Three participants with partial overlap",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("08:00"),
					EndTime:         stringPtr("18:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("08:00"),
					EndTime:         stringPtr("12:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Charlie",
					StartTime:       stringPtr("14:00"),
					EndTime:         stringPtr("18:00"),
				},
			},
			expected:    2,
			description: "Three participants with one all-day and two partial should have max 2 simultaneous",
		},
		{
			name: "All participants available all day",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       nil,
					EndTime:         nil,
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       nil,
					EndTime:         nil,
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Charlie",
					StartTime:       nil,
					EndTime:         nil,
				},
			},
			expected:    3,
			description: "Three participants all day should have max 3 simultaneous",
		},
		{
			name: "Complete overlap",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("10:00"),
					EndTime:         stringPtr("16:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("10:00"),
					EndTime:         stringPtr("16:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Charlie",
					StartTime:       stringPtr("10:00"),
					EndTime:         stringPtr("16:00"),
				},
			},
			expected:    3,
			description: "Three participants with complete overlap should have max 3 simultaneous",
		},
		{
			name: "Staggered availability",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("08:00"),
					EndTime:         stringPtr("14:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("10:00"),
					EndTime:         stringPtr("16:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Charlie",
					StartTime:       stringPtr("12:00"),
					EndTime:         stringPtr("18:00"),
				},
			},
			expected:    3,
			description: "Three participants with staggered times should have max 3 simultaneous during overlap (12:00-14:00)",
		},
		{
			name: "Available from midnight",
			participants: []models.ParticipantAvailabilitySummary{
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Alice",
					StartTime:       stringPtr("00:00"),
					EndTime:         stringPtr("12:00"),
				},
				{
					ParticipantID:   uuid.New(),
					ParticipantName: "Bob",
					StartTime:       stringPtr("00:00"),
					EndTime:         stringPtr("08:00"),
				},
			},
			expected:    2,
			description: "Two participants from midnight should be counted correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateMaxSimultaneousParticipants(tt.participants)
			if result != tt.expected {
				t.Errorf("%s: expected %d, got %d", tt.description, tt.expected, result)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestRecurrencesOverlap(t *testing.T) {
	tests := []struct {
		name       string
		startDateA string
		endDateA   string // empty string means no end date (infinite)
		startDateB string
		endDateB   string // empty string means no end date (infinite)
		expected   bool
	}{
		{
			name:       "Both infinite (no end dates) - always overlap",
			startDateA: "2025-01-01",
			endDateA:   "",
			startDateB: "2025-06-01",
			endDateB:   "",
			expected:   true,
		},
		{
			name:       "A infinite, B ends before A starts - no overlap",
			startDateA: "2025-06-01",
			endDateA:   "",
			startDateB: "2025-01-01",
			endDateB:   "2025-05-31",
			expected:   false,
		},
		{
			name:       "A infinite, B ends after A starts - overlap",
			startDateA: "2025-06-01",
			endDateA:   "",
			startDateB: "2025-01-01",
			endDateB:   "2025-12-31",
			expected:   true,
		},
		{
			name:       "A infinite, B ends exactly when A starts - overlap",
			startDateA: "2025-06-01",
			endDateA:   "",
			startDateB: "2025-01-01",
			endDateB:   "2025-06-01",
			expected:   true,
		},
		{
			name:       "B infinite, A ends before B starts - no overlap",
			startDateA: "2025-01-01",
			endDateA:   "2025-05-31",
			startDateB: "2025-06-01",
			endDateB:   "",
			expected:   false,
		},
		{
			name:       "B infinite, A ends after B starts - overlap",
			startDateA: "2025-01-01",
			endDateA:   "2025-12-31",
			startDateB: "2025-06-01",
			endDateB:   "",
			expected:   true,
		},
		{
			name:       "Both have end dates, no overlap (A before B)",
			startDateA: "2025-01-01",
			endDateA:   "2025-03-31",
			startDateB: "2025-06-01",
			endDateB:   "2025-08-31",
			expected:   false,
		},
		{
			name:       "Both have end dates, no overlap (B before A)",
			startDateA: "2025-06-01",
			endDateA:   "2025-08-31",
			startDateB: "2025-01-01",
			endDateB:   "2025-03-31",
			expected:   false,
		},
		{
			name:       "Both have end dates, overlap",
			startDateA: "2025-01-01",
			endDateA:   "2025-06-30",
			startDateB: "2025-04-01",
			endDateB:   "2025-12-31",
			expected:   true,
		},
		{
			name:       "Both have end dates, A contains B",
			startDateA: "2025-01-01",
			endDateA:   "2025-12-31",
			startDateB: "2025-03-01",
			endDateB:   "2025-06-30",
			expected:   true,
		},
		{
			name:       "Both have end dates, touching (A ends when B starts)",
			startDateA: "2025-01-01",
			endDateA:   "2025-06-01",
			startDateB: "2025-06-01",
			endDateB:   "2025-12-31",
			expected:   true,
		},
		{
			name:       "Both have end dates, adjacent (A ends before B starts)",
			startDateA: "2025-01-01",
			endDateA:   "2025-05-31",
			startDateB: "2025-06-01",
			endDateB:   "2025-12-31",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recurrencesOverlap(tt.startDateA, tt.endDateA, tt.startDateB, tt.endDateB)
			if result != tt.expected {
				t.Errorf("recurrencesOverlap(%s, %s, %s, %s) = %v, expected %v",
					tt.startDateA, tt.endDateA, tt.startDateB, tt.endDateB, result, tt.expected)
			}
		})
	}
}

// syncBuffer collects log output written from the detached goroutine while the test
// reads it from its own.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.String()
}

// recordingNotifier stands in for the notify service and reports what the detached
// goroutine handed it.
type recordingNotifier struct {
	err       error
	panicWith any

	called      chan struct{}
	mu          sync.Mutex
	hadDeadline bool
	ctxErr      error
}

func newRecordingNotifier(err error, panicWith any) *recordingNotifier {
	return &recordingNotifier{err: err, panicWith: panicWith, called: make(chan struct{})}
}

func (n *recordingNotifier) CheckThresholdAndNotify(ctx context.Context, _ uuid.UUID, _ time.Time, _ int) error {
	n.mu.Lock()
	_, n.hadDeadline = ctx.Deadline()
	n.ctxErr = ctx.Err()
	n.mu.Unlock()

	close(n.called)

	if n.panicWith != nil {
		panic(n.panicWith)
	}

	return n.err
}

// TestNotifyThresholdAsync covers the three ways the fire-and-forget threshold check used
// to fail silently.
//
// The block this helper replaces logged nothing at all — its error branch was empty under
// a comment reading "log only" — ran on a context.Background() that could never time out,
// and had no recover(), so a panic in the notify path took the process down with it
// (middleware.Recoverer only wraps the request goroutine).
func TestNotifyThresholdAsync(t *testing.T) {
	tests := []struct {
		name string
		// err is what the notify service returns.
		err error
		// panicWith, when set, makes the notify service panic instead.
		panicWith any
		// cancelRequest cancels the caller's context before the check is started.
		cancelRequest bool
		// wantLogged is a fragment the log line must contain.
		wantLogged string
	}{
		{
			name:       "a failed notification is logged",
			err:        errors.New("smtp: connection refused"),
			wantLogged: "threshold notification failed",
		},
		{
			name:       "a panic is recovered instead of killing the process",
			panicWith:  "notify exploded",
			wantLogged: "threshold notification panicked",
		},
		{
			name:          "the check still runs once the request is over",
			cancelRequest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &syncBuffer{}
			previous := logger.Default()
			logger.SetDefault(slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { logger.SetDefault(previous) })

			notifier := newRecordingNotifier(tt.err, tt.panicWith)
			svc := NewAvailabilityService(nil, nil, nil, nil, notifier, nil)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			svc.notifyThresholdAsync(ctx, uuid.New(), time.Now(), 2)

			if tt.cancelRequest {
				// The handler has returned and chi has cancelled the request context.
				// The notification must not be cancelled along with it.
				cancel()
			}

			select {
			case <-notifier.called:
			case <-time.After(2 * time.Second):
				t.Fatal("the threshold check was never run")
			}

			notifier.mu.Lock()
			hadDeadline, ctxErr := notifier.hadDeadline, notifier.ctxErr
			notifier.mu.Unlock()

			if !hadDeadline {
				t.Error("the detached context has no deadline: a wedged call would pin this goroutine for ever")
			}
			if ctxErr != nil {
				t.Errorf("the detached context was already done: %v", ctxErr)
			}

			if tt.wantLogged == "" {
				return
			}

			deadline := time.Now().Add(2 * time.Second)
			for !strings.Contains(out.String(), tt.wantLogged) {
				if time.Now().After(deadline) {
					t.Fatalf("nothing was logged about the failure; log was:\n%s", out.String())
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}
