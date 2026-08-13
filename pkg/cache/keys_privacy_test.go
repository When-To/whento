// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package cache

import (
	"strings"
	"testing"
)

// The values these keys are built from are the things the owner said must never
// be stored: a calendar's public token (which is the credential), an ICS token,
// a participant id, a user id, an email address. A Redis key is stored — it is
// what `KEYS *` lists and what `dump.rdb` writes to disk — so a key that carries
// one of them is a copy of it at rest, whatever the value alongside contains.

// TestKeysNeverContainTheirInput is the guard. Every helper is fed a value that
// is unmistakable in a string search, and the resulting key must not contain it.
func TestKeysNeverContainTheirInput(t *testing.T) {
	const (
		token         = "9f3c0d7e2a4b6185c3d0f7e9a2b4c6d81f3e5a7c9b0d2e4f6a8c1b3d5e7f9012"
		participantID = "6f9619ff-8b86-d011-b42d-00c04fc964ff"
		userID        = "3ac7e1b2-4d5f-4a6b-8c9d-0e1f2a3b4c5d"
		address       = "someone@example.org"
	)

	tests := []struct {
		name   string
		key    string
		secret string
	}{
		{"a calendar id", CalendarByIDKey(participantID), participantID},
		{"a calendar public token", CalendarByPublicTokenKey(token), token},
		{"an ICS token", CalendarByICSTokenKey(token), token},
		{"a calendar's participants", CalendarParticipantsKey(participantID), participantID},
		{"a user's calendars", UserCalendarsKey(userID), userID},
		{"a participant's availabilities", ParticipantAvailabilitiesKey(participantID), participantID},
		{"a date summary", CalendarDateSummaryKey(participantID, "2026-03-05"), participantID},
		{"a range summary", CalendarRangeSummaryKey(participantID, "2026-03-01", "2026-03-31"), participantID},
		{"an ICS feed", ICSFeedKey(token), token},
		{"a password change marker", UserPasswordChangedKey(userID), userID},
		// Not a key helper, but the primitive every caller outside this package
		// uses to build one — the login lockout counter among them, whose
		// variable part is an email address.
		{"an email address hashed directly", HashKeyPart(address), address},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(tt.key, tt.secret) {
				t.Errorf("the key %q carries %q in clear", tt.key, tt.secret)
			}
			if tt.key == "" {
				t.Error("the key is empty")
			}
		})
	}
}

// TestKeysStayReadableEnoughToOperate: hashing the variable part must not turn
// the key space into noise. The prefix says what a key is for, which is what
// makes an unexpected `KEYS calendar:*` count diagnosable.
func TestKeysStayReadableEnoughToOperate(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		prefix string
	}{
		{"calendar by id", CalendarByIDKey("x"), "calendar:id:"},
		{"calendar by token", CalendarByPublicTokenKey("x"), "calendar:token:"},
		{"ics feed", ICSFeedKey("x"), "ics:feed:"},
		{"password change", UserPasswordChangedKey("x"), "auth:pwd_changed:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.HasPrefix(tt.key, tt.prefix) {
				t.Errorf("key = %q, want the prefix %q", tt.key, tt.prefix)
			}
		})
	}
}

// TestHashKeyPartIsDeterministic is the property that makes hashing safe to do
// at all. Two instances sharing a Redis have to derive the same key for the same
// row: with per-instance keys, one instance's invalidation never reaches the
// other, which keeps serving a stale copy until the TTL runs out.
func TestHashKeyPartIsDeterministic(t *testing.T) {
	// Via a variable: staticcheck flags a literal `f(x) != f(x)` as a mistake,
	// which is exactly the property being asserted here.
	first, second := "abc", "abc"
	if HashKeyPart(first) != HashKeyPart(second) {
		t.Error("the same input produced two different digests")
	}
	if HashKeyPart("abc") == HashKeyPart("abd") {
		t.Error("two different inputs produced the same digest")
	}
	if HashKeyPart("") != "" {
		t.Errorf("an empty part produced %q, want an empty digest", HashKeyPart(""))
	}
}

// TestSetKeySaltChangesTheDigests covers the operator-facing knob: pinning a
// secret is what makes the digests untestable against a list of candidate
// addresses, and an empty value must leave the default alone rather than
// silently hash under an empty key.
func TestSetKeySaltChangesTheDigests(t *testing.T) {
	previous := keySalt
	t.Cleanup(func() { keySalt = previous })

	withDefault := HashKeyPart("someone@example.org")

	SetKeySalt("a pinned secret")
	withSecret := HashKeyPart("someone@example.org")
	if withSecret == withDefault {
		t.Error("pinning a salt did not change the digest")
	}

	SetKeySalt("")
	if HashKeyPart("someone@example.org") != withSecret {
		t.Error("an empty secret overwrote the pinned salt")
	}
}
