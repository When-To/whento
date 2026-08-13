// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package email

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// Every verification, magic link, password reset and threshold notification the
// instance ever sends passes through Send. Logging the recipient here therefore
// produced more addresses in the log stream than every other site put together,
// at info and error level — both visible at the default LOG_LEVEL.

// TestSendNeverLogsARecipientAddress drives the failure path (an unreachable
// host, which is the branch that logs at error) and the addresses must not
// appear in either line.
func TestSendNeverLogsARecipientAddress(t *testing.T) {
	tests := []struct {
		name string
		to   []string
	}{
		{"one recipient", []string{"ada@example.test"}},
		{"a recipient with a plus tag", []string{"ada+calendar@example.test"}},
		{"several recipients", []string{"ada@example.test", "grace@example.test"}},
	}

	host, port := closedPort(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			service := NewService(Config{
				Host:        host,
				Port:        port,
				FromAddress: "no-reply@example.test",
				DialTimeout: 150 * time.Millisecond,
				Timeout:     300 * time.Millisecond,
			}, log)

			if err := service.Send(Email{To: tt.to, Subject: "Verify your email", Body: "."}); err == nil {
				t.Fatal("Send to a closed port returned no error, so the logging branch was not reached")
			}

			written := buf.String()
			if !strings.Contains(written, "Failed to send email") {
				t.Fatalf("the failure was not logged:\n%s", written)
			}

			for _, address := range tt.to {
				if strings.Contains(written, address) {
					t.Errorf("the log carries %q in clear:\n%s", address, written)
				}
				if local, _, found := strings.Cut(address, "@"); found && strings.Contains(written, local) {
					t.Errorf("the log carries the local part %q:\n%s", local, written)
				}
			}
			// What replaced the address still has to answer "did this failure hit
			// one person or the whole calendar?".
			if !strings.Contains(written, "recipient_count") {
				t.Errorf("the failure line no longer says how many recipients were involved:\n%s", written)
			}
			if !strings.Contains(written, "recipient_ref") {
				t.Errorf("the failure line carries no correlation tag:\n%s", written)
			}
			if strings.Contains(written, `"to"`) {
				t.Errorf("a `to` field is back in the log:\n%s", written)
			}
		})
	}
}

// TestSendLogsTheSubjectButNotTheBody: subjects are fixed translated strings and
// are worth keeping; the body carries the participant's name and the link.
func TestSendNeverLogsTheBody(t *testing.T) {
	const secretBody = "https://whento.example/reset-password/deadbeefcafe"

	host, port := closedPort(t)

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	service := NewService(Config{
		Host:        host,
		Port:        port,
		FromAddress: "no-reply@example.test",
		DialTimeout: 150 * time.Millisecond,
		Timeout:     300 * time.Millisecond,
	}, log)

	_ = service.Send(Email{To: []string{"ada@example.test"}, Subject: "Reset your password", Body: secretBody})

	if strings.Contains(buf.String(), secretBody) {
		t.Errorf("the log carries the message body, links included:\n%s", buf.String())
	}
}
