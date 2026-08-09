// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"strings"
	"testing"
	"time"

	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/notify/models"
)

// TestBuildNotificationMessage drives the real formatter.
//
// This file previously held a "deduplication" test that built a map of e-mail
// addresses inside the test body and counted it — production code was never called,
// and the package reported 0% coverage. The dedup it claimed to cover lives inside
// sendDeduplicatedEmailNotifications, a method that queries the database and sends
// mail; it is not reachable from a unit test as written, and is called out as still
// uncovered rather than papered over with another self-referential test.
func TestBuildNotificationMessage(t *testing.T) {
	calendar := &calendarModels.Calendar{Name: "Board game night"}
	date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		transitionType string
		newCount       int
		threshold      int
		wantContains   []string
		wantAbsent     []string
	}{
		{
			name:           "threshold reached",
			transitionType: "threshold_reached",
			newCount:       5, threshold: 5,
			wantContains: []string{"Board game night", "2026-03-05", "5/5", "Threshold reached"},
		},
		{
			name:           "threshold lost",
			transitionType: "threshold_lost",
			newCount:       4, threshold: 5,
			wantContains: []string{"Board game night", "2026-03-05", "4/5", "Threshold lost"},
			wantAbsent:   []string{"Threshold reached"},
		},
		{
			name:           "no transition falls back to a neutral message",
			transitionType: "none",
			newCount:       3, threshold: 5,
			wantContains: []string{"Board game night", "2026-03-05", "3/5", "Availability changed"},
			wantAbsent:   []string{"Threshold reached", "Threshold lost"},
		},
		{
			name:           "an unrecognised transition type also falls back",
			transitionType: "something_else",
			newCount:       1, threshold: 2,
			wantContains: []string{"Availability changed", "1/2"},
		},
	}

	service := &NotifyService{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.buildNotificationMessage(calendar, &models.ThresholdTransition{
				Date:           date,
				TransitionType: tt.transitionType,
				NewCount:       tt.newCount,
				Threshold:      tt.threshold,
			})

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("message %q does not contain %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("message %q unexpectedly contains %q", got, absent)
				}
			}
		})
	}
}

// TestBuildNotificationMessageEscapesNothing records that the plain-text message is not
// escaped — a calendar named with markup passes through verbatim. That is correct for
// text/plain delivery, and is the reason the HTML variant exists separately.
func TestBuildNotificationMessageEscapesNothing(t *testing.T) {
	calendar := &calendarModels.Calendar{Name: `<script>alert(1)</script>`}
	service := &NotifyService{}

	got := service.buildNotificationMessage(calendar, &models.ThresholdTransition{
		Date:           time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
		TransitionType: "threshold_reached",
		NewCount:       1, Threshold: 1,
	})

	if !strings.Contains(got, `<script>alert(1)</script>`) {
		t.Errorf("the plain-text builder altered the calendar name: %q", got)
	}
}
