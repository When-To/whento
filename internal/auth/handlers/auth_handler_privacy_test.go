// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/logger"
)

// "Login attempt on locked account" was logged at warn with the address, so it
// showed up at the default LOG_LEVEL. A lockout line is by definition written
// when someone is being attacked, which is exactly when the log is most likely
// to be copied into a ticket, a chat message or a screenshot.

// lockedOutCache reports the account as already past the attempt limit, which is
// what makes Login take the ErrAccountLocked branch.
type lockedOutCache struct{ attempts int }

func (c *lockedOutCache) Get(_ context.Context, _ string, dest interface{}) error {
	if target, ok := dest.(*int); ok {
		*target = c.attempts
	}

	return nil
}

func (c *lockedOutCache) Set(context.Context, string, interface{}, time.Duration) error { return nil }
func (c *lockedOutCache) Delete(context.Context, ...string) error                       { return nil }
func (c *lockedOutCache) Exists(context.Context, string) (bool, error)                  { return true, nil }
func (c *lockedOutCache) IsEnabled() bool                                               { return true }

var _ cache.Cache = (*lockedOutCache)(nil)

// captureDefaultLogger redirects the package-level logger, which is what
// logger.FromContext hands the handler.
func captureDefaultLogger(t *testing.T) *bytes.Buffer {
	t.Helper()

	original := logger.Default()
	t.Cleanup(func() { logger.SetDefault(original) })

	var buf bytes.Buffer
	logger.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	return &buf
}

// TestLoginNeverLogsTheEmailAddress covers the locked-account line, and asserts
// the line is still there — dropping the log entirely would pass a "no address"
// check and leave an operator blind during a credential-stuffing run.
func TestLoginNeverLogsTheEmailAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"an ordinary address", "ada@example.test"},
		{"an address with a plus tag", "ada+calendar@example.test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureDefaultLogger(t)

			// 10 is maxLoginAttempts; anything at or above it locks the account.
			r := newRig(t, rigOptions{allowedRegister: true, appCache: &lockedOutCache{attempts: 99}})

			rec := httptest.NewRecorder()
			r.handler.Login(rec, post("/api/v1/auth/login",
				`{"email":"`+tt.address+`","password":"Correct-Horse-9"}`))

			written := buf.String()
			if !strings.Contains(written, "locked account") {
				t.Fatalf("the lockout was not logged at all, so the branch was not reached:\n%s", written)
			}
			if strings.Contains(written, tt.address) {
				t.Errorf("the log carries the address in clear:\n%s", written)
			}
			if local, _, found := strings.Cut(tt.address, "@"); found && strings.Contains(written, local) {
				t.Errorf("the log carries the local part %q:\n%s", local, written)
			}
			// Still diagnosable: an administrator has to be able to tell one
			// account under attack from a spray across many.
			if !strings.Contains(written, "account_ref") {
				t.Errorf("the lockout line carries no correlation tag:\n%s", written)
			}
		})
	}
}

// TestLockoutTagsDistinguishAccounts: a tag shared by every account would pass
// the test above while telling an operator nothing.
func TestLockoutTagsDistinguishAccounts(t *testing.T) {
	if logger.Fingerprint("ada@example.test") == logger.Fingerprint("grace@example.test") {
		t.Error("two accounts share a correlation tag")
	}
}
