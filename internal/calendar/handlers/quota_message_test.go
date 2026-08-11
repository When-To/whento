// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"strings"
	"testing"
)

// This message is the only thing a user gets when their fourth calendar is refused, and
// the frontend shows it verbatim. It is worth a test because the string it replaced
// outlived the thing it described: "please upgrade your subscription" survived the
// removal of subscriptions, and "upgrade your license at whento.be/pricing" survived the
// removal of licensing and of that page.

func TestQuotaMessage(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		wants []string
	}{
		{
			name:  "the hosted limit",
			limit: 3,
			// The number matters: "limit reached" without it leaves the user guessing
			// whether they are one over or ten.
			wants: []string{"3", "hosted service", "Host your own instance"},
		},
		{
			name:  "a different hosted limit",
			limit: 10,
			wants: []string{"10"},
		},
		{
			// Self-hosted reports an unlimited allowance and never refuses, so this is
			// unreachable today. It stays a plain sentence rather than an invitation to
			// self-host, which would be absurd advice to someone already doing it.
			name:  "no limit configured",
			limit: 0,
			wants: []string{"Calendar limit reached."},
		},
		{
			name:  "a negative limit",
			limit: -1,
			wants: []string{"Calendar limit reached."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quotaMessage(tt.limit)

			for _, want := range tt.wants {
				if !strings.Contains(got, want) {
					t.Errorf("message = %q, want it to mention %q", got, want)
				}
			}
		})
	}
}

// TestQuotaMessageOffersNothingThatDoesNotExist is the guard that would have caught the
// two strings this replaced. There is no paid plan, no licence key and no pricing page;
// pointing a blocked user at any of them sends them somewhere that does not exist.
func TestQuotaMessageOffersNothingThatDoesNotExist(t *testing.T) {
	for _, limit := range []int{-1, 0, 1, 3, 100} {
		message := strings.ToLower(quotaMessage(limit))

		for _, gone := range []string{"licen", "subscription", "upgrade", "plan", "pricing", "whento.be"} {
			if strings.Contains(message, gone) {
				t.Errorf("limit %d yields %q, which offers %q — removed from the product", limit, message, gone)
			}
		}
	}
}

// TestQuotaMessageIsSelfContained keeps it usable. The frontend prints it as-is, with no
// formatting of its own, so an unresolved verb or a stray newline lands in front of the
// user.
func TestQuotaMessageIsSelfContained(t *testing.T) {
	for _, limit := range []int{0, 3} {
		message := quotaMessage(limit)

		if strings.Contains(message, "%!") {
			t.Errorf("limit %d left an unresolved format verb: %q", limit, message)
		}
		if strings.ContainsAny(message, "\n\r\t") {
			t.Errorf("limit %d produced a message with whitespace control characters: %q", limit, message)
		}
		if !strings.HasSuffix(strings.TrimSpace(message), ".") {
			t.Errorf("limit %d produced %q, which is not a finished sentence", limit, message)
		}
	}
}
