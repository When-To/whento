// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package config

import (
	"strings"
	"testing"
)

func TestValidateDatabaseURL(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantError bool
		errSubstr string
	}{
		{"valid postgres", "postgres://user:pass@host:5432/db?sslmode=disable", false, ""},
		{"valid postgresql", "postgresql://user:pass@host:5432/db", false, ""},
		{"empty", "", true, "empty"},
		{"wrong scheme", "mysql://user:pass@host:5432/db", true, "postgres://"},
		{"missing host", "postgres://user:pass@/db", true, "missing host"},
		{"valid with encoded special char password", "postgres://user:p%40ss@host:5432/db", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDatabaseURL(tt.in)
			if tt.wantError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantError && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("expected error to contain %q, got %q", tt.errSubstr, err.Error())
			}
		})
	}
}

func TestValidateRedisURL(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantError bool
		errSubstr string
	}{
		{"valid redis", "redis://:pwd@host:6379/0", false, ""},
		{"valid rediss (TLS)", "rediss://:pwd@host:6379", false, ""},
		{"valid no password", "redis://host:6379", false, ""},
		{"empty", "", true, "empty"},
		{"wrong scheme", "http://host:6379", true, "redis://"},
		{"missing host", "redis://", true, "missing host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRedisURL(tt.in)
			if tt.wantError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantError && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Fatalf("expected error to contain %q, got %q", tt.errSubstr, err.Error())
			}
		})
	}
}

func TestMaskURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"postgres with password", "postgres://user:secret@host:5432/db", "postgres://user:***@host:5432/db"},
		{"redis with password only", "redis://:secret@host:6379", "redis://:***@host:6379"},
		{"no userinfo", "postgres://host:5432/db", "postgres://host:5432/db"},
		{"username only (no password)", "postgres://user@host:5432/db", "postgres://user@host:5432/db"},
		{"non-URL string preserved", "not a url at all", "not a url at all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskURL(tt.in); got != tt.want {
				t.Fatalf("MaskURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildDatabaseURLEscapesPassword(t *testing.T) {
	t.Setenv("DB_HOST", "host")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "db")
	t.Setenv("DB_USER", "user")
	t.Setenv("DB_PASSWORD", "p@ss:w/ord?x")
	t.Setenv("DB_SSLMODE", "disable")

	got := buildDatabaseURL()
	if err := validateDatabaseURL(got); err != nil {
		t.Fatalf("built URL fails validation: %v (url=%s)", err, MaskURL(got))
	}
	// Spot-check that the literal special characters are not present unescaped.
	if strings.Contains(got, "p@ss:w/ord?x") {
		t.Fatalf("password was not escaped: %s", MaskURL(got))
	}
}

func TestBuildRedisURLEscapesPassword(t *testing.T) {
	t.Setenv("REDIS_HOST", "host")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("REDIS_PASSWORD", "p@ss:w/ord?x")
	t.Setenv("REDIS_DB", "0")

	got := buildRedisURL()
	if err := validateRedisURL(got); err != nil {
		t.Fatalf("built URL fails validation: %v (url=%s)", err, MaskURL(got))
	}
	if strings.Contains(got, "p@ss:w/ord?x") {
		t.Fatalf("password was not escaped: %s", MaskURL(got))
	}
}
