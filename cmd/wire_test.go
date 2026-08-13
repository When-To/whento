// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/email"
	"github.com/whento/whento/internal/config"
)

// buildHandlers is thirty-odd constructors in a fixed order, and the order is
// the part that can break: a repository read by two modules, or a service that
// has to exist before the one that notifies through it. None of those
// constructors touches the database — they store what they are given — so the
// whole graph can be built from a nil pool and inspected.

func testWireDeps(t *testing.T) *deps {
	t.Helper()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		AppURL: "https://whento.test",
		// The only constructor that validates its configuration is the passkey
		// service, which builds a WebAuthn config out of these three.
		WebAuthnRPName:   "WhenTo",
		WebAuthnRPID:     "whento.test",
		WebAuthnRPOrigin: "https://whento.test",
	}

	return &deps{
		cfg:        cfg,
		log:        log,
		pool:       nil,
		cacheStore: cache.NewRedisCache(nil),
		jwtManager: nil,
		mailer:     email.NewService(email.Config{}, log),
		broker:     nil,
		limiter:    newRouteLimiter(nil, false),
		quota:      &Services{},
	}
}

func TestBuildHandlers(t *testing.T) {
	t.Run("wires every handler the router mounts", func(t *testing.T) {
		h, err := buildHandlers(testWireDeps(t))
		if err != nil {
			t.Fatalf("buildHandlers: %v", err)
		}

		// A nil field here is a handler the router would register a route for
		// and then panic on at the first request.
		nilChecks := []struct {
			name string
			got  any
		}{
			{name: "health", got: h.health},
			{name: "auth", got: h.auth},
			{name: "emailVerification", got: h.emailVerification},
			{name: "passwordReset", got: h.passwordReset},
			{name: "magicLink", got: h.magicLink},
			{name: "adminMFA", got: h.adminMFA},
			{name: "passkey", got: h.passkey},
			{name: "mfa", got: h.mfa},
			{name: "calendar", got: h.calendar},
			{name: "participant", got: h.participant},
			{name: "notifyConfig", got: h.notifyConfig},
			{name: "participantEmail", got: h.participantEmail},
			{name: "availability", got: h.availability},
			{name: "recurrence", got: h.recurrence},
			{name: "events", got: h.events},
			{name: "ics", got: h.ics},
			{name: "unifiedFeed", got: h.unifiedFeed},
			{name: "seo", got: h.seo},
		}

		for _, tc := range nilChecks {
			if isNilPointer(tc.got) {
				t.Errorf("handlers.%s is nil", tc.name)
			}
		}
	})

	t.Run("reports the passkey service refusing its configuration", func(t *testing.T) {
		d := testWireDeps(t)
		// A relying-party id that is not a domain is the one thing in this graph
		// that fails at construction time, and it must surface as an error rather
		// than a half-wired router.
		d.cfg.WebAuthnRPID = "https://whento.test:8080/"

		if _, err := buildHandlers(d); err == nil {
			t.Fatal("buildHandlers returned no error for an unusable WebAuthn configuration")
		}
	})
}

// isNilPointer reports whether an interface holds a nil pointer. A plain
// `== nil` on the interface would miss it: an interface holding a typed nil is
// itself not nil.
func isNilPointer(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)

	return rv.Kind() == reflect.Pointer && rv.IsNil()
}
