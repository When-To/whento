// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

// Package dbtest provides a database-backed harness for repository tests.
//
// Every repository in this project is hand-written SQL over a *pgxpool.Pool. There is
// nothing pure in them to unit test, which is why they all sat at 0% — a mock of the
// pool would only assert that the code calls the functions it calls, not that the SQL
// is correct, that the constraints hold, or that a scan matches the columns selected.
// Those are the only things worth checking here, and they need a real server.
//
// Two rules keep this usable:
//
//   - **Skip, never fail, when there is no database.** `make test` on a laptop with no
//     Postgres must stay green; CI supplies DATABASE_URL and the migrations, and that is
//     where these actually run.
//   - **Every test owns its rows.** Nothing truncates a shared table, because `go test
//     ./...` runs package binaries concurrently and one package wiping `users` would
//     break another mid-run. Fixtures use unique identifiers and register their own
//     cleanup, so tests are safe alongside each other and alongside a running dev server
//     on the same database.
package dbtest

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	once     sync.Once
	shared   *pgxpool.Pool
	openErr  error
	schemaOK bool
)

// Pool returns a connection pool against the test database.
//
// The test is skipped when DATABASE_URL is unset, and failed when it is set but points
// at a database with no schema — that is a misconfigured CI job rather than a missing
// one, and silently skipping would hide it.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set; skipping the database-backed tests")
	}

	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		shared, openErr = pgxpool.New(ctx, url)
		if openErr != nil {
			return
		}

		// A migrated database has a users table. Checking once tells a misconfigured
		// job apart from an absent one.
		var exists bool
		openErr = shared.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = 'users'
			)`).Scan(&exists)
		schemaOK = exists
	})

	if openErr != nil {
		t.Fatalf("DATABASE_URL is set but the database is unreachable: %v", openErr)
	}
	if !schemaOK {
		t.Fatal("DATABASE_URL is set but the schema is missing; run `make migrate-up` first")
	}

	return shared
}

// Cleanup registers a statement to run when the test finishes, whether it passed or
// failed. Fixtures use it so that nothing is left behind and no test has to truncate a
// table another package might be using.
func Cleanup(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Logf("cleanup failed (%s): %v", sql, err)
		}
	})
}

// Context returns a context with a timeout suited to a single query, so a hung
// connection fails the test rather than the whole run.
func Context(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	return ctx
}
