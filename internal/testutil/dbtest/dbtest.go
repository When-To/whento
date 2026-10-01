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
	CleanupContext(context.Background(), t, pool, sql, args...)
}

// CleanupContext is Cleanup with an explicit cleanup context, for helpers (and
// subtest fixtures) that already hold a request-scoped context.
//
// The cleanup must run even when the operation under test gives up: a test that
// hits its deadline, cancels the context, or fails before its cleanup phase is
// exactly the one whose fixtures most need removing. Deriving the cleanup
// timeout from the raw request context would reverse that guarantee (a cancelled
// parent cancels the cleanup with it, so one failing test would contaminate the
// shared database for every later test and package). context.WithoutCancel
// detaches the parent's cancellation and deadline while still inheriting from
// it, so the relationship stays explicit for contextcheck and the timeout below
// is the only thing that can bound the cleanup.
func CleanupContext(ctx context.Context, t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()

	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		if _, err := pool.Exec(cctx, sql, args...); err != nil {
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

// singletonAccountsLockKey is the advisory-lock key for the instance-wide account
// state (the users table and app_state). It deliberately avoids the keys production
// code uses (CreateFirstUser takes the transaction lock 1, the admin snapshot takes
// the transaction lock 2) and the key the quota tests use (-918273645).
const singletonAccountsLockKey int64 = 910111213

// singletonConn tracks whether this process already holds the singleton lock, and
// on which pooled connection. Tests within one binary run sequentially (none of the
// database-backed tests use t.Parallel), so a single slot is enough: a second
// request from the same test — a fixture helper called after the test itself took
// the lock — must be a no-op, because re-locking the same key on a *different*
// session would deadlock against itself.
var (
	singletonMu   sync.Mutex
	singletonConn *pgxpool.Conn
)

// LockSingletonAccounts gives the calling test exclusive ownership of the
// instance-wide account state — the users table and the app_state singleton — for
// the rest of the test's lifetime. The first-user tests need this because "is this
// instance bootstrapped" is decided from a global count and a shared flag, and `go
// test ./...` runs package binaries concurrently: a count snapshotted before the
// assertion can be invalidated by another package inserting and cleaning up its own
// user row (reproduced as "CreateFirstUser on a populated instance = <nil>, want
// ErrFirstUserExists"). Every user-creating fixture in every package takes this
// same session-level advisory lock on a dedicated connection, so a first-user test
// cannot start while another package's fixture is alive, and no fixture can be
// created while a first-user test runs. Row ownership stays per test; this only
// serialises the singleton.
//
// The lock is released in test cleanup (after the fixture cleanups, which run
// LIFO-before it, so rows are gone before the ownership ends). Calling it from a
// helper that a test already covered is a no-op.
func LockSingletonAccounts(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	singletonMu.Lock()
	held := singletonConn
	if held != nil {
		singletonMu.Unlock()
		return
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		singletonMu.Unlock()
		t.Fatalf("acquire a connection for the singleton account lock: %v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, singletonAccountsLockKey); err != nil {
		conn.Release()
		singletonMu.Unlock()
		t.Fatalf("take the singleton account lock: %v", err)
	}
	singletonConn = conn
	singletonMu.Unlock()

	t.Cleanup(func() {
		singletonMu.Lock()
		conn := singletonConn
		singletonConn = nil
		singletonMu.Unlock()
		if conn == nil {
			return
		}
		// The deadline is detached from the test's context (which may already be
		// done after a failure) so the lock is not left behind on a pooled
		// connection for the rest of the run.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if _, err := conn.Exec(cctx, `SELECT pg_advisory_unlock($1)`, singletonAccountsLockKey); err != nil {
			t.Logf("release the singleton account lock: %v", err)
		}
		conn.Release()
	})
}
