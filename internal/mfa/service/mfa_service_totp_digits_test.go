// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"testing"

	"github.com/pquerna/otp"
)

// TestTOTPDigitsClampsWithoutWrapping pins the conversion CodeQL flagged: the
// configured value arrives as a uint parsed by strconv.ParseUint, and otp.Digits
// is an int, so an unbounded value would wrap negative instead of failing.
// Config validation bounds this to 6-8, but it lives in another package and
// nothing makes this function unreachable without it.
func TestTOTPDigitsClampsWithoutWrapping(t *testing.T) {
	tests := []struct {
		name       string
		configured uint
		want       otp.Digits
	}{
		{"six, the default", 6, otp.DigitsSix},
		{"seven is legal and must survive", 7, otp.Digits(7)},
		{"eight", 8, otp.DigitsEight},
		{"below the range falls back", 5, otp.DigitsSix},
		{"zero falls back", 0, otp.DigitsSix},
		{"above the range falls back", 9, otp.DigitsSix},
		{"a value that would wrap negative", uint(1) << 32, otp.DigitsSix},
		{"the largest uint", ^uint(0), otp.DigitsSix},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := totpDigits(tt.configured)
			if got != tt.want {
				t.Errorf("totpDigits(%d) = %d, want %d", tt.configured, got, tt.want)
			}
			if got <= 0 {
				t.Errorf("totpDigits(%d) = %d, which is not a usable digit count", tt.configured, got)
			}
		})
	}
}
