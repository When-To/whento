// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
)

// The tests below assert on the raw response body rather than on a decoded struct.
//
// The defect they pin was invisible from the service's own types: GetDateSummary
// returned models.DateAvailabilitySummary, whose ParticipantID is a plain uuid.UUID
// with no omitempty, so encoding/json wrote every id out no matter what the calendar's
// lock_participants setting said. Only the bytes that leave the handler prove it is
// gone — and those bytes are the whole authorisation story here, because a participant
// is authorised by the public token plus their id and nothing else.

const dateSummaryPath = "/api/v1/public/calendars/tok/availabilities/dates/2026-03-05"

// twoParticipantsOnADate builds a calendar with Alice and Bob both answering 2026-03-05.
func twoParticipantsOnADate(t *testing.T, locked bool) (http.Handler, *repository.Participant, *repository.Participant) {
	t.Helper()

	calendarID := uuid.New()
	alice := &repository.Participant{ID: uuid.New(), CalendarID: calendarID, Name: "Alice"}
	bob := &repository.Participant{ID: uuid.New(), CalendarID: calendarID, Name: "Bob"}

	calendar := &repository.Calendar{
		ID:               calendarID,
		Threshold:        2,
		AllowedWeekdays:  []int{0, 1, 2, 3, 4, 5, 6},
		Timezone:         "Europe/Paris",
		LockParticipants: locked,
	}

	date, err := time.Parse("2006-01-02", "2026-03-05")
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}

	newAvailability := func(participantID uuid.UUID, start, end string) *models.Availability {
		availability := &models.Availability{
			ParticipantID: participantID,
			Date:          date,
			StartTime:     &start,
			EndTime:       &end,
		}
		availability.ID = uuid.New()

		return availability
	}

	router := newHandler(
		t,
		calendar,
		nil,
		[]*models.Availability{
			newAvailability(alice.ID, "09:00", "17:00"),
			newAvailability(bob.ID, "10:00", "12:00"),
		},
		[]*repository.Participant{alice, bob},
	)

	return router, alice, bob
}

func TestGetDateSummaryDoesNotLeakParticipantIDs(t *testing.T) {
	tests := []struct {
		name   string
		locked bool
		// query builds the query string from the participant the caller claims to be.
		query func(alice *repository.Participant) string
		// wantAlice and wantBob say whether each id may appear in the body.
		wantAlice bool
		wantBob   bool
	}{
		{
			name: "an open calendar names everybody", locked: false,
			query:     func(*repository.Participant) string { return "" },
			wantAlice: true, wantBob: true,
		},
		{
			name: "a locked calendar names nobody to an anonymous visitor", locked: true,
			query:     func(*repository.Participant) string { return "" },
			wantAlice: false, wantBob: false,
		},
		{
			name: "a locked calendar still names the caller to themselves", locked: true,
			query: func(alice *repository.Participant) string {
				return "?participant_id=" + alice.ID.String()
			},
			wantAlice: true, wantBob: false,
		},
		{
			name: "a locked calendar ignores an unparseable participant_id", locked: true,
			query:     func(*repository.Participant) string { return "?participant_id=not-a-uuid" },
			wantAlice: false, wantBob: false,
		},
		{
			name: "a locked calendar ignores an empty participant_id", locked: true,
			query:     func(*repository.Participant) string { return "?participant_id=" },
			wantAlice: false, wantBob: false,
		},
		{
			name: "a locked calendar ignores a stranger's participant_id", locked: true,
			query: func(*repository.Participant) string {
				return "?participant_id=" + uuid.NewString()
			},
			wantAlice: false, wantBob: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, alice, bob := twoParticipantsOnADate(t, tt.locked)

			req := httptest.NewRequest(http.MethodGet, dateSummaryPath+tt.query(alice), nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
			}

			body := rec.Body.String()

			if got := strings.Contains(body, alice.ID.String()); got != tt.wantAlice {
				t.Errorf("Alice's id present = %v, want %v (body %s)", got, tt.wantAlice, body)
			}
			if got := strings.Contains(body, bob.ID.String()); got != tt.wantBob {
				t.Errorf("Bob's id present = %v, want %v (body %s)", got, tt.wantBob, body)
			}

			// Whatever is masked, the calendar has to remain readable: the frontend
			// popup labels rows by name, not by id.
			if !strings.Contains(body, `"Alice"`) || !strings.Contains(body, `"Bob"`) {
				t.Errorf("a participant name was masked along with the ids (body %s)", body)
			}
		})
	}
}

// TestDateAndRangeSummariesLeakTheSameIDs pins the two public summary endpoints
// together. They answer over identical data, so whichever discloses more is the one an
// attacker would use; they must agree, in both directions, or the pair has drifted
// again.
func TestDateAndRangeSummariesLeakTheSameIDs(t *testing.T) {
	const rangePath = "/api/v1/public/calendars/tok/availabilities/range?start=2026-03-05&end=2026-03-05"

	tests := []struct {
		name      string
		locked    bool
		withAsker bool
	}{
		{name: "open, anonymous"},
		{name: "open, the caller names themselves", withAsker: true},
		{name: "locked, anonymous", locked: true},
		{name: "locked, the caller names themselves", locked: true, withAsker: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, alice, bob := twoParticipantsOnADate(t, tt.locked)

			asker := ""
			if tt.withAsker {
				asker = "&participant_id=" + alice.ID.String()
			}

			bodyOf := func(url string) string {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
				if rec.Code != http.StatusOK {
					t.Fatalf("GET %s: status = %d (body %s)", url, rec.Code, rec.Body)
				}

				return rec.Body.String()
			}

			dateBody := bodyOf(dateSummaryPath + "?" + strings.TrimPrefix(asker, "&"))
			rangeBody := bodyOf(rangePath + asker)

			for _, participant := range []*repository.Participant{alice, bob} {
				onDate := strings.Contains(dateBody, participant.ID.String())
				inRange := strings.Contains(rangeBody, participant.ID.String())
				if onDate != inRange {
					t.Errorf(
						"%s: the date endpoint discloses their id = %v, the range endpoint = %v",
						participant.Name, onDate, inRange,
					)
				}
			}
		})
	}
}
