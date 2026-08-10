// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// The package is small, but two of its behaviours are load-bearing: the level parsed
// from configuration decides what is ever written, and FromContext is what puts a
// request id on every line of a request. A silent mistake in either is only visible
// when you need the logs and they are not there.

func TestNewLevel(t *testing.T) {
	tests := []struct {
		name         string
		level        string
		wantDebug    bool
		wantInfo     bool
		wantWarn     bool
		wantErrorLog bool
	}{
		{name: "debug shows everything", level: "debug", wantDebug: true, wantInfo: true, wantWarn: true, wantErrorLog: true},
		{name: "info hides debug", level: "info", wantInfo: true, wantWarn: true, wantErrorLog: true},
		{name: "warn hides info", level: "warn", wantWarn: true, wantErrorLog: true},
		{name: "error hides warn", level: "error", wantErrorLog: true},
		// An unrecognised value must not silence the logger; it falls back to info.
		{name: "an unknown level falls back to info", level: "loud", wantInfo: true, wantWarn: true, wantErrorLog: true},
		{name: "an empty level falls back to info", level: "", wantInfo: true, wantWarn: true, wantErrorLog: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			// New writes to stdout, so the level is checked through the handler it
			// builds rather than by capturing output.
			logger := New(tt.level, "json")
			handler := logger.Handler()

			checks := []struct {
				level slog.Level
				want  bool
				name  string
			}{
				{slog.LevelDebug, tt.wantDebug, "debug"},
				{slog.LevelInfo, tt.wantInfo, "info"},
				{slog.LevelWarn, tt.wantWarn, "warn"},
				{slog.LevelError, tt.wantErrorLog, "error"},
			}
			for _, check := range checks {
				if got := handler.Enabled(context.Background(), check.level); got != check.want {
					t.Errorf("%s enabled = %v, want %v", check.name, got, check.want)
				}
			}
			_ = buf
		})
	}
}

func TestNewFormat(t *testing.T) {
	// The format decides whether logs are machine-readable. Production runs json; a
	// silent fallback to text would break every log pipeline.
	json := New("info", "json").Handler()
	if _, ok := json.(*slog.JSONHandler); !ok {
		t.Errorf("format json produced %T", json)
	}

	text := New("info", "text").Handler()
	if _, ok := text.(*slog.TextHandler); !ok {
		t.Errorf("format text produced %T", text)
	}

	// Anything else is treated as text, which is the readable default for a terminal.
	other := New("info", "something-else").Handler()
	if _, ok := other.(*slog.TextHandler); !ok {
		t.Errorf("an unknown format produced %T, want a text handler", other)
	}
}

func TestDefaultAndSetDefault(t *testing.T) {
	original := Default()
	t.Cleanup(func() { SetDefault(original) })

	if original == nil {
		t.Fatal("Default() is nil before anything sets it; init should have")
	}

	var buf bytes.Buffer
	replacement := slog.New(slog.NewJSONHandler(&buf, nil))
	SetDefault(replacement)

	if Default() != replacement {
		t.Error("Default() did not return the logger just set")
	}

	// The package-level helpers write through the default, so replacing it has to
	// redirect them too.
	Info("hello", "key", "value")

	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("Info did not reach the replacement logger: %q", buf.String())
	}
}

func TestPackageLevelHelpersRespectTheLevel(t *testing.T) {
	original := Default()
	t.Cleanup(func() { SetDefault(original) })

	var buf bytes.Buffer
	SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	Debug("a debug line")
	Info("an info line")
	Warn("a warn line")
	Error("an error line")

	output := buf.String()
	for _, hidden := range []string{"a debug line", "an info line"} {
		if strings.Contains(output, hidden) {
			t.Errorf("%q was written despite the warn level", hidden)
		}
	}
	for _, shown := range []string{"a warn line", "an error line"} {
		if !strings.Contains(output, shown) {
			t.Errorf("%q was not written", shown)
		}
	}
}

func TestFromContextCarriesRequestAndUser(t *testing.T) {
	original := Default()
	t.Cleanup(func() { SetDefault(original) })

	var buf bytes.Buffer
	SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	ctx := WithUserID(WithRequestID(context.Background(), "req-123"), "user-456")
	FromContext(ctx).Info("handled")

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("the log line is not JSON: %v (%q)", err, buf.String())
	}

	// This is what makes a request traceable across the log; without it a failure in
	// production cannot be tied to the request that caused it.
	if line["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", line["request_id"])
	}
	if line["user_id"] != "user-456" {
		t.Errorf("user_id = %v, want user-456", line["user_id"])
	}
}

func TestFromContextWithNothingAttached(t *testing.T) {
	original := Default()
	t.Cleanup(func() { SetDefault(original) })

	var buf bytes.Buffer
	SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	FromContext(context.Background()).Info("no context")

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("the log line is not JSON: %v", err)
	}

	// Absent values must be omitted rather than logged empty, which would make an
	// unauthenticated request look like one from user "".
	if _, present := line["request_id"]; present {
		t.Error("request_id was logged despite not being in the context")
	}
	if _, present := line["user_id"]; present {
		t.Error("user_id was logged despite not being in the context")
	}
}

func TestFromContextWithOnlyOneValue(t *testing.T) {
	original := Default()
	t.Cleanup(func() { SetDefault(original) })

	var buf bytes.Buffer
	SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	FromContext(WithRequestID(context.Background(), "req-only")).Info("partial")

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("the log line is not JSON: %v", err)
	}

	if line["request_id"] != "req-only" {
		t.Errorf("request_id = %v", line["request_id"])
	}
	if _, present := line["user_id"]; present {
		t.Error("user_id was logged for an unauthenticated request")
	}
}

func TestContextKeysAreNotPlainStrings(t *testing.T) {
	// A plain string key can be overwritten by any other package storing under the
	// same literal. The values must survive a context that also carries "request_id"
	// as a string key.
	ctx := context.WithValue(context.Background(), "request_id", "impostor") //nolint:staticcheck // the point of the test
	ctx = WithRequestID(ctx, "genuine")

	original := Default()
	t.Cleanup(func() { SetDefault(original) })

	var buf bytes.Buffer
	SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	FromContext(ctx).Info("collision")

	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("the log line is not JSON: %v", err)
	}
	if line["request_id"] != "genuine" {
		t.Errorf("request_id = %v, want the value set through WithRequestID", line["request_id"])
	}
}
