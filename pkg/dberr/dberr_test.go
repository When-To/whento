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
			err:  pgErr("23503", "availabilities_participant_id_fkey"),
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

// TestIsUniqueViolation pins the predicate against errors of other classes and against
// messages that merely contain the digits.
func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil},
		{name: "client-side failure", err: errors.New("failed to connect to host")},
		{
			name: "unique violation",
			err:  pgErr(CodeUniqueViolation, "availabilities_participant_id_date_key"),
			want: true,
		},
		{
			name: "unique violation wrapped by the repository",
			err:  fmt.Errorf("failed to create availability: %w", pgErr(CodeUniqueViolation, "")),
			want: true,
		},
		{
			name: "joined with an unrelated error",
			err:  errors.Join(errors.New("context deadline exceeded"), pgErr(CodeUniqueViolation, "")),
			want: true,
		},
		{name: "foreign key violation", err: pgErr("23503", "availabilities_participant_id_fkey")},
		{name: "check violation", err: pgErr("23514", "availabilities_source_check")},
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
			if got := IsUniqueViolation(tt.err); got != tt.want {
				t.Errorf("IsUniqueViolation(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
