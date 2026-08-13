// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package dberr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgErr builds the value pgx puts in the error chain for a server-side failure.
func pgErr(code, constraint string) *pgconn.PgError {
	return &pgconn.PgError{
		Severity:       "ERROR",
		Code:           code,
		Message:        "duplicate key value violates unique constraint",
		ConstraintName: constraint,
		TableName:      "users",
	}
}

// impostor carries the digits the old substring matcher looked for, without ever
// having come from the server. It is the false positive this package exists to kill.
type impostor struct{ msg string }

func (e impostor) Error() string { return e.msg }

func TestPgError(t *testing.T) {
	target := pgErr(CodeUniqueViolation, "users_email_key")

	tests := []struct {
		name    string
		err     error
		wantOK  bool
		wantPtr *pgconn.PgError
	}{
		{name: "nil error", err: nil, wantOK: false},
		{name: "plain error", err: errors.New("connection refused"), wantOK: false},
		{name: "bare pg error", err: target, wantOK: true, wantPtr: target},
		{
			name:    "wrapped once",
			err:     fmt.Errorf("failed to create user: %w", target),
			wantOK:  true,
			wantPtr: target,
		},
		{
			name:    "wrapped twice",
			err:     fmt.Errorf("service: %w", fmt.Errorf("repository: %w", target)),
			wantOK:  true,
			wantPtr: target,
		},
		{
			name:    "joined with an unrelated error",
			err:     errors.Join(errors.New("context deadline exceeded"), target),
			wantOK:  true,
			wantPtr: target,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PgError(tt.err)
			if ok != tt.wantOK {
				t.Fatalf("PgError(%v) ok = %v, want %v", tt.err, ok, tt.wantOK)
			}
			if got != tt.wantPtr {
				t.Errorf("PgError(%v) = %v, want %v", tt.err, got, tt.wantPtr)
			}
		})
	}
}

func TestCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil error", err: nil, want: ""},
		{name: "not a server error", err: errors.New("dial tcp: i/o timeout"), want: ""},
		{name: "unique violation", err: pgErr(CodeUniqueViolation, "users_email_key"), want: "23505"},
		{
			name: "foreign key violation, wrapped",
			err:  fmt.Errorf("wrapped: %w", pgErr(CodeForeignKeyViolation, "availabilities_participant_id_fkey")),
			want: "23503",
		},
		{name: "check violation", err: pgErr(CodeCheckViolation, "recurrences_day_of_week_check"), want: "23514"},
		{
			name: "message merely contains the code",
			err:  impostor{msg: "could not connect to host 23505.db.example.com"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Code(tt.err); got != tt.want {
				t.Errorf("Code(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestHasCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
		want bool
	}{
		{name: "nil error", err: nil, code: CodeUniqueViolation, want: false},
		{
			name: "matching code",
			err:  pgErr(CodeUniqueViolation, "users_email_key"),
			code: CodeUniqueViolation,
			want: true,
		},
		{
			name: "different code on a real server error",
			err:  pgErr(CodeForeignKeyViolation, "availabilities_participant_id_fkey"),
			code: CodeUniqueViolation,
			want: false,
		},
		{
			name: "empty code never matches a server error",
			err:  pgErr(CodeUniqueViolation, "users_email_key"),
			code: "",
			want: false,
		},
		{
			name: "empty code against a plain error",
			err:  errors.New("boom"),
			code: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasCode(tt.err, tt.code); got != tt.want {
				t.Errorf("HasCode(%v, %q) = %v, want %v", tt.err, tt.code, got, tt.want)
			}
		})
	}
}

func TestConstraintName(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil error", err: nil, want: ""},
		{name: "plain error", err: errors.New("boom"), want: ""},
		{
			name: "named constraint",
			err:  pgErr(CodeUniqueViolation, "participants_calendar_id_name_key"),
			want: "participants_calendar_id_name_key",
		},
		{
			name: "named constraint, wrapped",
			err:  fmt.Errorf("failed to create participant: %w", pgErr(CodeUniqueViolation, "participants_email_key")),
			want: "participants_email_key",
		},
		{
			name: "server error with no constraint attributed",
			err:  pgErr(CodeCheckViolation, ""),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConstraintName(tt.err); got != tt.want {
				t.Errorf("ConstraintName(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestClassifiers pins the three predicates against the same table, so that an error of
// one class can never be reported as another.
func TestClassifiers(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantUnique     bool
		wantForeignKey bool
		wantCheck      bool
	}{
		{name: "nil error", err: nil},
		{name: "client-side failure", err: errors.New("failed to connect to host")},
		{
			name:       "unique violation",
			err:        pgErr(CodeUniqueViolation, "availabilities_participant_id_date_key"),
			wantUnique: true,
		},
		{
			name:       "unique violation wrapped by the repository",
			err:        fmt.Errorf("failed to create availability: %w", pgErr(CodeUniqueViolation, "")),
			wantUnique: true,
		},
		{
			name:           "foreign key violation",
			err:            pgErr(CodeForeignKeyViolation, "availabilities_participant_id_fkey"),
			wantForeignKey: true,
		},
		{
			name:      "check violation",
			err:       pgErr(CodeCheckViolation, "availabilities_source_check"),
			wantCheck: true,
		},
		{
			name: "not-null violation is none of the three",
			err:  pgErr("23502", ""),
		},
		{
			// The old implementation searched the whole message for "23505" and
			// would have called every one of these a duplicate.
			name: "message contains 23505 but is not a server error",
			err:  impostor{msg: `pq: relation "audit_23505" does not exist`},
		},
		{
			name: "the exact literal the old equality check compared against",
			err:  impostor{msg: "ERROR: duplicate key value violates unique constraint"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUniqueViolation(tt.err); got != tt.wantUnique {
				t.Errorf("IsUniqueViolation(%v) = %v, want %v", tt.err, got, tt.wantUnique)
			}
			if got := IsForeignKeyViolation(tt.err); got != tt.wantForeignKey {
				t.Errorf("IsForeignKeyViolation(%v) = %v, want %v", tt.err, got, tt.wantForeignKey)
			}
			if got := IsCheckViolation(tt.err); got != tt.wantCheck {
				t.Errorf("IsCheckViolation(%v) = %v, want %v", tt.err, got, tt.wantCheck)
			}
		})
	}
}
