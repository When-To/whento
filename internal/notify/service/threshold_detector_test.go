// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

var errCount = errors.New("count unavailable")

// stubCounter is a hand-written ParticipantCounter. It also records the arguments it
// was handed, so the tests can check the detector queries the date it was asked about.
type stubCounter struct {
	count int
	err   error

	calls          int
	lastCalendarID uuid.UUID
	lastDate       time.Time
}

var _ ParticipantCounter = (*stubCounter)(nil)

func (s *stubCounter) GetParticipantCountForDate(
	_ context.Context, calendarID uuid.UUID, date time.Time,
) (int, error) {
	s.calls++
	s.lastCalendarID = calendarID
	s.lastDate = date

	return s.count, s.err
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestDetectTransition drives the real detector rather than restating its rules.
//
// The previous version of this test reimplemented the transition logic inside the test
// body and asserted its own reimplementation, so it reported 0% coverage of this
// package and would have passed against any implementation at all.
func TestDetectTransition(t *testing.T) {
	tests := []struct {
		name          string
		previousCount int
		newCount      int
		threshold     int
		want          string
	}{
		{
			name:          "crossing up to the threshold",
			previousCount: 4, newCount: 5, threshold: 5,
			want: "threshold_reached",
		},
		{
			name:          "dropping below the threshold",
			previousCount: 5, newCount: 4, threshold: 5,
			want: "threshold_lost",
		},
		{
			name:          "still below",
			previousCount: 3, newCount: 4, threshold: 5,
			want: "none",
		},
		{
			name:          "still above",
			previousCount: 6, newCount: 7, threshold: 5,
			want: "none",
		},
		{
			name:          "unchanged at the threshold",
			previousCount: 5, newCount: 5, threshold: 5,
			want: "none",
		},
		{
			name:          "overshooting the threshold in one step",
			previousCount: 0, newCount: 9, threshold: 5,
			want: "threshold_reached",
		},
		{
			name:          "collapsing to nobody",
			previousCount: 9, newCount: 0, threshold: 5,
			want: "threshold_lost",
		},
		{
			// -1 means the caller does not know the previous count, so only the
			// current state can be judged. Reaching is reported; losing cannot be.
			name:          "unknown previous count, threshold met",
			previousCount: -1, newCount: 5, threshold: 5,
			want: "threshold_reached",
		},
		{
			name:          "unknown previous count, threshold not met",
			previousCount: -1, newCount: 3, threshold: 5,
			want: "none",
		},
		{
			// A threshold of one is the default for a new calendar, so this is the
			// most common path in practice.
			name:          "a threshold of one, first answer",
			previousCount: 0, newCount: 1, threshold: 1,
			want: "threshold_reached",
		},
		{
			// Zero is met by anyone, including nobody, so it never transitions.
			name:          "a threshold of zero is met from the start",
			previousCount: 0, newCount: 0, threshold: 0,
			want: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := &stubCounter{count: tt.newCount}
			detector := NewThresholdDetector(counter, quietLogger())

			calendarID := uuid.New()
			date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

			got, err := detector.DetectTransition(
				context.Background(), calendarID, date, tt.threshold, tt.previousCount,
			)
			if err != nil {
				t.Fatalf("DetectTransition: %v", err)
			}

			if got.TransitionType != tt.want {
				t.Errorf("TransitionType = %q, want %q", got.TransitionType, tt.want)
			}

			// The transition carries the numbers the notification message is built
			// from, so they have to survive the round trip intact.
			if got.CalendarID != calendarID {
				t.Errorf("CalendarID = %v, want %v", got.CalendarID, calendarID)
			}
			if !got.Date.Equal(date) {
				t.Errorf("Date = %v, want %v", got.Date, date)
			}
			if got.PreviousCount != tt.previousCount {
				t.Errorf("PreviousCount = %d, want %d", got.PreviousCount, tt.previousCount)
			}
			if got.NewCount != tt.newCount {
				t.Errorf("NewCount = %d, want %d", got.NewCount, tt.newCount)
			}
			if got.Threshold != tt.threshold {
				t.Errorf("Threshold = %d, want %d", got.Threshold, tt.threshold)
			}

			if counter.calls != 1 {
				t.Errorf("the repository was queried %d times, want once", counter.calls)
			}
			if !counter.lastDate.Equal(date) {
				t.Errorf("the count was fetched for %v, want %v", counter.lastDate, date)
			}
		})
	}
}

// TestDetectTransitionPropagatesCountFailure covers the branch where the count cannot
// be read. Returning a nil transition matters: the caller would otherwise send a
// notification built from zero values.
func TestDetectTransitionPropagatesCountFailure(t *testing.T) {
	detector := NewThresholdDetector(&stubCounter{err: errCount}, quietLogger())

	got, err := detector.DetectTransition(
		context.Background(), uuid.New(), time.Now(), 5, 0,
	)

	if !errors.Is(err, errCount) {
		t.Errorf("error = %v, want the counting failure", err)
	}
	if got != nil {
		t.Errorf("transition = %+v, want nil on failure", got)
	}
}

func TestGetCurrentCount(t *testing.T) {
	t.Run("returns the repository count", func(t *testing.T) {
		counter := &stubCounter{count: 7}
		detector := NewThresholdDetector(counter, quietLogger())

		calendarID := uuid.New()
		date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

		got, err := detector.GetCurrentCount(context.Background(), calendarID, date)
		if err != nil {
			t.Fatalf("GetCurrentCount: %v", err)
		}
		if got != 7 {
			t.Errorf("count = %d, want 7", got)
		}
		if counter.lastCalendarID != calendarID {
			t.Errorf("queried calendar %v, want %v", counter.lastCalendarID, calendarID)
		}
	})

	t.Run("propagates a failure", func(t *testing.T) {
		detector := NewThresholdDetector(&stubCounter{err: errCount}, quietLogger())

		if _, err := detector.GetCurrentCount(context.Background(), uuid.New(), time.Now()); !errors.Is(err, errCount) {
			t.Errorf("error = %v, want the counting failure", err)
		}
	})
}
