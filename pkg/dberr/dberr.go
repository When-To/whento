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

// SQLSTATE codes the application reacts to. See
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	// CodeUniqueViolation is raised by a UNIQUE constraint or unique index, and is
	// how "this row already exists" reaches the application.
	CodeUniqueViolation = "23505"
	// CodeUndefinedTable is raised when a query names a table the schema does not
	// have yet, e.g. reading app_state on a database older than migration 019.
	CodeUndefinedTable = "42P01"
)

// HasCode reports whether err is a server error with exactly the given SQLSTATE,
// anywhere in its chain.
func HasCode(err error, code string) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == code
}

// IsUniqueViolation reports whether err is a unique constraint violation (23505).
func IsUniqueViolation(err error) bool {
	return HasCode(err, CodeUniqueViolation)
}
