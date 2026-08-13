// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

// Package dberr classifies PostgreSQL errors by their SQLSTATE code.
//
// pgx reports every server-side failure as a *pgconn.PgError somewhere in the error
// chain, and that value carries the SQLSTATE verbatim in its Code field. Matching on
// that field with errors.As is the only reliable test. Matching on the error *message*
// is not, for three independent reasons:
//
//   - the text is localised by the server's lc_messages and reworded between major
//     versions, so an equality check against an English sentence is a coin toss;
//   - it is not the whole message either — pgx renders PgError as
//     `ERROR: duplicate key value violates unique constraint "users_email_key" (SQLSTATE 23505)`,
//     which never equals the truncated literal it used to be compared against;
//   - searching for the five digits "23505" anywhere in the text matches any message
//     that happens to contain them — a constraint name, a column value, an id — and
//     turns an unrelated failure into a reported duplicate.
//
// Callers should keep translating these into their own domain sentinels rather than
// leaking a database concept upwards: repositories return ErrXAlreadyExists, services
// compare with errors.Is.
package dberr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// SQLSTATE codes for the integrity-constraint violations this schema can raise.
// Class 23 — Integrity Constraint Violation, see
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	// CodeUniqueViolation is raised by a UNIQUE constraint or unique index, and is
	// how "this row already exists" reaches the application.
	CodeUniqueViolation = "23505"
	// CodeForeignKeyViolation is raised when a referenced row is missing, or when a
	// referenced row is deleted without ON DELETE CASCADE.
	CodeForeignKeyViolation = "23503"
	// CodeCheckViolation is raised by a CHECK constraint, such as the
	// `day_of_week BETWEEN 0 AND 6` and `source IN ('manual', 'recurrence')` guards.
	CodeCheckViolation = "23514"
)

// PgError extracts the *pgconn.PgError from anywhere in err's chain. It reports false
// for a nil error, for a client-side failure such as a broken connection, and for any
// error the driver did not produce.
func PgError(err error) (*pgconn.PgError, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr, true
	}

	return nil, false
}

// Code returns the SQLSTATE carried by err, or "" when err did not come from the server.
func Code(err error) string {
	if pgErr, ok := PgError(err); ok {
		return pgErr.Code
	}

	return ""
}

// HasCode reports whether err is a server error with exactly the given SQLSTATE.
func HasCode(err error, code string) bool {
	pgErr, ok := PgError(err)

	return ok && pgErr.Code == code
}

// ConstraintName returns the name of the constraint the server blamed, or "" when err
// is not a server error or the failure was not attributed to a named constraint. Use it
// to tell two unique constraints on the same table apart — for example a participant's
// name colliding versus their email.
func ConstraintName(err error) string {
	if pgErr, ok := PgError(err); ok {
		return pgErr.ConstraintName
	}

	return ""
}

// IsUniqueViolation reports whether err is a unique constraint violation (23505).
func IsUniqueViolation(err error) bool {
	return HasCode(err, CodeUniqueViolation)
}

// IsForeignKeyViolation reports whether err is a foreign key violation (23503).
func IsForeignKeyViolation(err error) bool {
	return HasCode(err, CodeForeignKeyViolation)
}

// IsCheckViolation reports whether err is a check constraint violation (23514).
func IsCheckViolation(err error) bool {
	return HasCode(err, CodeCheckViolation)
}
