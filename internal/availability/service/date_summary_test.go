// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
)

// One test was dropped when the expansion moved into SQL, because the service no longer
// decides it: TestGetDateSummaryUnknownParticipant asserted that an availability whose
// participant has left the calendar is not reported. The occurrence query answers that
// with its JOIN on participants scoped to the calendar, covered by the repository's
// TestOccurrencesExpandRecurrencesAndCarryTimes.

// identifiedIDs collects the participant ids that actually survived masking.
func identifiedIDs(participants []models.PublicParticipantAvailabilitySummary) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(participants))
	for _, p := range participants {
		if p.ParticipantID != nil {
			out = append(out, *p.ParticipantID)
		}
	}

	return out
}

// containsUUID reports whether a marshalled response mentions an id anywhere at all.
//
// Checking the struct field is not enough to pin this defect. The endpoint leaked
// precisely because its response type spelled the field `uuid.UUID` rather than
// `*uuid.UUID`, so encoding/json wrote it out whatever the service believed it was
// hiding. Asserting on the bytes on the wire is the only assertion that would have
// failed before the fix.
func containsUUID(t *testing.T, payload any, id uuid.UUID) bool {
	t.Helper()

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	return strings.Contains(string(encoded), id.String())
}

// summariesWithBothParticipants gives Alice and Bob an overlapping answer on one date,
// so the date endpoint and the range endpoint are asked about exactly the same data.
func summariesWithBothParticipants(t *testing.T, locked bool) *summaryFixture {
	t.Helper()

	fixture := newSummaryFixture(t, func(c *repository.Calendar) { c.LockParticipants = locked })

	// The one seeding serves both endpoints, so they are asked about identical data.
	fixture.availRepo.occurrences = []models.Occurrence{
		available(fixture.alice, mustDate(t, "2026-03-05"), ptr("09:00"), ptr("17:00")),
		available(fixture.bob, mustDate(t, "2026-03-05"), ptr("10:00"), ptr("12:00")),
	}

	return fixture
}

// TestGetDateSummaryMasksParticipantIDs is the regression test for the leak.
//
// GetDateSummary used to answer with models.DateAvailabilitySummary, whose
// ParticipantID is a bare uuid.UUID and therefore always serialised. On a calendar
// whose owner had switched lock_participants on, anyone holding the public token could
// read every participant's id from this endpoint — while the range endpoint, over the
// same data, was careful to withhold it. Since a participant is authorised by the
// public token plus their id and nothing else, that handed out both halves of every
// participant's credential.
func TestGetDateSummaryMasksParticipantIDs(t *testing.T) {
	tests := []struct {
		name string
		// locked is the calendar's lock_participants setting.
		locked bool
		// asker names whose participant_id the caller presents, if any.
		asker func(f *summaryFixture) string
		// wantAlice and wantBob say whether each id may appear in the answer.
		wantAlice bool
		wantBob   bool
	}{
		{
			name:      "an unlocked calendar identifies everybody",
			locked:    false,
			asker:     func(*summaryFixture) string { return "" },
			wantAlice: true, wantBob: true,
		},
		{
			name:      "an unlocked calendar identifies everybody for a known asker too",
			locked:    false,
			asker:     func(f *summaryFixture) string { return f.alice.ID.String() },
			wantAlice: true, wantBob: true,
		},
		{
			name:      "a locked calendar identifies nobody to an anonymous caller",
			locked:    true,
			asker:     func(*summaryFixture) string { return "" },
			wantAlice: false, wantBob: false,
		},
		{
			name:      "a locked calendar still identifies the asker to themselves",
			locked:    true,
			asker:     func(f *summaryFixture) string { return f.alice.ID.String() },
			wantAlice: true, wantBob: false,
		},
		{
			name:      "a locked calendar identifies nobody to a malformed asker",
			locked:    true,
			asker:     func(*summaryFixture) string { return "not-a-uuid" },
			wantAlice: false, wantBob: false,
		},
		{
			name:      "a locked calendar identifies nobody to a stranger",
			locked:    true,
			asker:     func(*summaryFixture) string { return uuid.NewString() },
			wantAlice: false, wantBob: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := summariesWithBothParticipants(t, tt.locked)
			asker := tt.asker(fixture)

			got, err := fixture.service.GetDateSummary(context.Background(), "token", "2026-03-05", asker)
			if err != nil {
				t.Fatalf("GetDateSummary: %v", err)
			}

			if len(got.Participants) != 2 {
				t.Fatalf("got %d participants, want 2", len(got.Participants))
			}

			if containsUUID(t, got, fixture.alice.ID) != tt.wantAlice {
				t.Errorf("Alice's id on the wire = %v, want %v", !tt.wantAlice, tt.wantAlice)
			}
			if containsUUID(t, got, fixture.bob.ID) != tt.wantBob {
				t.Errorf("Bob's id on the wire = %v, want %v", !tt.wantBob, tt.wantBob)
			}

			// Masking is about identity: names, times and the count stay public, or
			// the calendar would stop being usable at all.
			names := []string{got.Participants[0].ParticipantName, got.Participants[1].ParticipantName}
			if names[0] != "Alice" || names[1] != "Bob" {
				t.Errorf("participant names = %v, want [Alice Bob]", names)
			}
			if deref(got.Participants[0].StartTime) != "09:00" {
				t.Error("a participant's times were masked along with their id")
			}
			if got.TotalCount != 2 {
				t.Errorf("TotalCount = %d, want 2", got.TotalCount)
			}
		})
	}
}

// TestSummaryEndpointsAgreeOnMasking is the property that was violated: two endpoints
// answering about the same participants on the same calendar must withhold the same
// ids. Whichever of them is laxer is the one an attacker uses, so pinning them
// together is what stops the pair drifting apart again.
func TestSummaryEndpointsAgreeOnMasking(t *testing.T) {
	tests := []struct {
		name   string
		locked bool
		// withAsker presents Alice's own id on both calls when true.
		withAsker bool
	}{
		{name: "unlocked, anonymous caller", locked: false},
		{name: "unlocked, the caller names themselves", locked: false, withAsker: true},
		{name: "locked, anonymous caller", locked: true},
		{name: "locked, the caller names themselves", locked: true, withAsker: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := summariesWithBothParticipants(t, tt.locked)

			asker := ""
			if tt.withAsker {
				asker = fixture.alice.ID.String()
			}

			byDateSummary, err := fixture.service.GetDateSummary(
				context.Background(), "token", "2026-03-05", asker,
			)
			if err != nil {
				t.Fatalf("GetDateSummary: %v", err)
			}

			byRange, err := fixture.service.GetRangeSummary(
				context.Background(), "token", "2026-03-05", "2026-03-05", asker,
			)
			if err != nil {
				t.Fatalf("GetRangeSummary: %v", err)
			}

			rangeSummary, ok := byDate(byRange)["2026-03-05"]
			if !ok {
				t.Fatal("2026-03-05 is missing from the range summary")
			}

			for _, id := range []uuid.UUID{fixture.alice.ID, fixture.bob.ID} {
				onDate := containsUUID(t, byDateSummary, id)
				inRange := containsUUID(t, rangeSummary, id)
				if onDate != inRange {
					t.Errorf(
						"%v: the date endpoint discloses it = %v, the range endpoint = %v; the two must agree",
						id, onDate, inRange,
					)
				}
			}

			if len(identifiedIDs(byDateSummary.Participants)) != len(identifiedIDs(rangeSummary.Participants)) {
				t.Errorf(
					"the date endpoint identified %v, the range endpoint %v",
					identifiedIDs(byDateSummary.Participants),
					identifiedIDs(rangeSummary.Participants),
				)
			}
		})
	}
}

// TestGetDateSummaryMasksUntimedOccurrences covers the second way a participant reaches
// this response. An occurrence a recurrence produced is an ordinary row by the time the
// service sees it — usually an untimed one, since a weekly rule is most often set for a
// whole day — and masking has to cover it just the same. It used to arrive through a
// branch of its own, so masking the explicitly answered ones alone left the leak open for
// anyone whose availability comes from a weekly rule.
func TestGetDateSummaryMasksUntimedOccurrences(t *testing.T) {
	fixture := newSummaryFixture(t, func(c *repository.Calendar) { c.LockParticipants = true })

	fixture.availRepo.occurrences = []models.Occurrence{
		available(fixture.alice, mustDate(t, "2026-03-05"), nil, nil),
		available(fixture.bob, mustDate(t, "2026-03-05"), nil, nil),
	}

	got, err := fixture.service.GetDateSummary(
		context.Background(), "token", "2026-03-05", fixture.alice.ID.String(),
	)
	if err != nil {
		t.Fatalf("GetDateSummary: %v", err)
	}

	if len(got.Participants) != 2 {
		t.Fatalf("got %d participants, want 2 untimed occurrences", len(got.Participants))
	}
	if !containsUUID(t, got, fixture.alice.ID) {
		t.Error("the asker cannot see their own id, so they cannot edit their own slot")
	}
	if containsUUID(t, got, fixture.bob.ID) {
		t.Error("an untimed occurrence's participant id leaked past lock_participants")
	}
}

// TestGetDateSummaryEmptySerialisesAsArray pins the shape of the two early returns.
// The old code accumulated into a nil slice, so a date nobody answered came back with
// `"participants": null` and any client mapping over it threw.
func TestGetDateSummaryEmptySerialisesAsArray(t *testing.T) {
	tests := []struct {
		name string
		// minDuration over zero exercises the short-overlap early return; zero
		// exercises the ordinary empty-day path.
		minDuration int
		wantCount   int
	}{
		{name: "nobody answered", minDuration: 0, wantCount: 0},
		{name: "the overlap is below the minimum", minDuration: 12, wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSummaryFixture(t, func(c *repository.Calendar) {
				c.MinDurationHours = tt.minDuration
			})
			if tt.minDuration > 0 {
				fixture.availRepo.occurrences = []models.Occurrence{
					available(fixture.alice, mustDate(t, "2026-03-05"), ptr("09:00"), ptr("10:00")),
				}
			}

			got, err := fixture.service.GetDateSummary(context.Background(), "token", "2026-03-05", "")
			if err != nil {
				t.Fatalf("GetDateSummary: %v", err)
			}

			if got.TotalCount != tt.wantCount {
				t.Errorf("TotalCount = %d, want %d", got.TotalCount, tt.wantCount)
			}

			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			if !strings.Contains(string(encoded), `"participants":[]`) {
				t.Errorf("participants = %s, want an empty array rather than null", encoded)
			}
		})
	}
}

func TestGetDateSummaryErrors(t *testing.T) {
	t.Run("an unknown token", func(t *testing.T) {
		svc := NewAvailabilityService(
			&mockAvailabilityRepo{},
			&mockCalendarInfoRepo{err: repository.ErrCalendarNotFound},
			&mockParticipantsRepo{},
			&mockRecurrenceRepo{},
			&mockNotifyService{},
			cache.NewRedisCache(nil),
		)

		_, err := svc.GetDateSummary(context.Background(), "nope", "2026-03-05", "")
		if !errors.Is(err, ErrCalendarNotFound) {
			t.Errorf("error = %v, want ErrCalendarNotFound", err)
		}
	})

	dateTests := []struct {
		name string
		date string
	}{
		{name: "an unparseable date", date: "05/03/2026"},
		{name: "an empty date", date: ""},
		{name: "a date that does not exist", date: "2026-02-30"},
	}

	for _, tt := range dateTests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newSummaryFixture(t, nil)

			_, err := fixture.service.GetDateSummary(context.Background(), "token", tt.date, "")
			if !errors.Is(err, ErrInvalidDate) {
				t.Errorf("error = %v, want ErrInvalidDate", err)
			}
		})
	}

	t.Run("a store failure is propagated", func(t *testing.T) {
		fixture := newSummaryFixture(t, nil)
		fixture.availRepo.occurrencesErr = errStore

		_, err := fixture.service.GetDateSummary(context.Background(), "token", "2026-03-05", "")
		if !errors.Is(err, errStore) {
			t.Errorf("error = %v, want the store failure", err)
		}
	})
}
