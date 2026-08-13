// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package logger

import (
	"bytes"
	"strings"
	"testing"
)

// TestFingerprintNeverEchoesItsInput is the only property the callers depend on:
// whatever they hand it — an email address, a calendar token, a participant id —
// none of it comes back out. Everything else here is about the tag still being
// useful.
func TestFingerprintNeverEchoesItsInput(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"an email address", "someone@example.org"},
		{"a calendar public token", "9f3c0d7e2a4b6185c3d0f7e9a2b4c6d81f3e5a7c9b0d2e4f6a8c1b3d5e7f9012"},
		{"a participant id", "6f9619ff-8b86-d011-b42d-00c04fc964ff"},
		{"a value with punctuation", "first.last+tag@sub.example.co.uk"},
		{"a display name", "Ada Lovelace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fingerprint(tt.value)

			if strings.Contains(got, tt.value) {
				t.Errorf("Fingerprint(%q) = %q, which contains the input", tt.value, got)
			}
			// A short input could otherwise hide inside the hex by chance; check
			// the pieces too, since an address is the thing most likely to be
			// grepped for in a log archive.
			for _, part := range strings.FieldsFunc(tt.value, func(r rune) bool {
				return r == '@' || r == '.' || r == '-' || r == '+' || r == ' '
			}) {
				if len(part) >= 3 && strings.Contains(got, part) {
					t.Errorf("Fingerprint(%q) = %q, which contains the fragment %q", tt.value, got, part)
				}
			}
			// Fixed width, whatever the input: a tag whose length varied with the
			// value would say something about the value.
			if len(got) != fingerprintLength*2 {
				t.Errorf("Fingerprint(%q) = %q, want %d hex characters", tt.value, got, fingerprintLength*2)
			}
		})
	}
}

// TestFingerprintIsStableAndDistinguishing: the tag exists so that an operator
// can say "these lines are the same subject" and "these are not". Both halves
// have to hold, or the field is decoration.
func TestFingerprintIsStableAndDistinguishing(t *testing.T) {
	// Via a variable: staticcheck flags a literal `f(x) != f(x)` as a mistake,
	// which is exactly the property being asserted here.
	first, second := "someone@example.org", "someone@example.org"
	if Fingerprint(first) != Fingerprint(second) {
		t.Error("the same value produced two different fingerprints in one process")
	}
	if Fingerprint("someone@example.org") == Fingerprint("someone.else@example.org") {
		t.Error("two different values produced the same fingerprint")
	}
}

// TestFingerprintOfNothingIsNothing: a missing field must not become the
// fingerprint of the empty string, which every caller with a missing value would
// share and which would read like a real subject appearing everywhere.
func TestFingerprintOfNothingIsNothing(t *testing.T) {
	if got := Fingerprint(""); got != "" {
		t.Errorf("Fingerprint(\"\") = %q, want an empty string", got)
	}
}

// TestFingerprintSaltIsRandomPerProcess is what makes a fingerprint useless
// outside the run that produced it: no salt, no dictionary attack on a log
// archive.
func TestFingerprintSaltIsRandomPerProcess(t *testing.T) {
	if bytes.Equal(newFingerprintSalt(), make([]byte, 32)) {
		t.Error("the generated salt is all zeroes")
	}
	if bytes.Equal(newFingerprintSalt(), newFingerprintSalt()) {
		t.Error("two generated salts are identical")
	}
}
