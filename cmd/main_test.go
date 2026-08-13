// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/whento/whento/internal/config"
)

func TestStartMetricsServer(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		// wantWarn is the substring the warning must carry when the port was
		// left unset, or "" when no warning is expected.
		wantWarn string
	}{
		{
			name: "disabled starts nothing",
			cfg:  &config.Config{MetricsEnabled: false},
		},
		{
			name: "enabled on an explicit port",
			cfg:  &config.Config{MetricsEnabled: true, MetricsPort: "0"},
		},
		{
			// The exposition must never fall back to the application listener,
			// so an unset port picks a private default and says so.
			name:     "enabled without a port warns and uses the default",
			cfg:      &config.Config{MetricsEnabled: true},
			wantWarn: defaultMetricsPort,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The listener runs in a goroutine of its own and logs from there,
			// so the sink has to be safe to write and read concurrently.
			logged := &syncBuffer{}
			log := slog.New(slog.NewJSONHandler(logged, nil))

			stop := startMetricsServer(tc.cfg, log)
			if stop == nil {
				t.Fatal("startMetricsServer returned no stop function")
			}
			stop()

			out := logged.String()
			if tc.wantWarn == "" {
				if strings.Contains(out, "METRICS_PORT is unset") {
					t.Errorf("unexpected default-port warning: %s", out)
				}

				return
			}
			if !strings.Contains(out, "METRICS_PORT is unset") || !strings.Contains(out, tc.wantWarn) {
				t.Errorf("missing default-port warning naming %s: %s", tc.wantWarn, out)
			}
		})
	}
}

// TestLogBuildInfo checks the one line that ties a bug report to a commit. The
// values are link-time variables, so what is asserted is that all four keys are
// present, not what they hold.
func TestLogBuildInfo(t *testing.T) {
	var logged bytes.Buffer
	logBuildInfo(slog.New(slog.NewJSONHandler(&logged, nil)))

	var line map[string]any
	if err := json.Unmarshal(logged.Bytes(), &line); err != nil {
		t.Fatalf("build info line is not JSON: %v (%s)", err, logged.String())
	}

	for _, key := range []string{"version", "build_type", "build_date", "vcs_ref"} {
		if _, ok := line[key]; !ok {
			t.Errorf("build info line has no %q field: %s", key, logged.String())
		}
	}

	if got := line["build_type"]; got != buildType {
		t.Errorf("build_type = %v, want %v", got, buildType)
	}
}

// TestBuildTypeIsOneOfTheTwoVariants is the smallest possible guard on the
// cloud/self-hosted pair: whichever tag this test was compiled under, exactly
// one of init_cloud.go and init_selfhosted.go supplied buildType, and it has to
// be a value the SEO handler and the build-info metric understand.
func TestBuildTypeIsOneOfTheTwoVariants(t *testing.T) {
	switch buildType {
	case "cloud", "selfhosted":
	default:
		t.Fatalf("buildType = %q, want \"cloud\" or \"selfhosted\"", buildType)
	}
}

// syncBuffer is a bytes.Buffer that survives being written from the metrics
// listener's goroutine while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}
