// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/logger"
)

// QuotaLocker serialises calendar creation with a PostgreSQL advisory lock.
//
// The lock exists to close a TOCTOU window: two concurrent requests from the same
// user both read a calendar count below the allowance and both create a calendar.
// `pg_advisory_xact_lock` is held for the life of a transaction, so the transaction
// here carries nothing else — it is the lock's lifetime and nothing more.
//
// This used to live in the HTTP handler, which opened the transaction, ran the raw
// SQL, and dropped both the rollback and the commit errors on the floor through two
// deferred closures whose order was load-bearing. The SQL belongs here; what the
// handler needs is a scope, which is what WithQuotaLock gives it.
type QuotaLocker struct {
	pool *pgxpool.Pool
}

// NewQuotaLocker creates a quota locker over the given pool.
func NewQuotaLocker(pool *pgxpool.Pool) *QuotaLocker {
	return &QuotaLocker{pool: pool}
}

// WithQuotaLock runs fn while holding the advisory lock named by key.
//
// It returns an error only when the lock could not be taken — that is, only when fn
// did not run. A failure to release afterwards is logged and does not surface,
// because by then fn has already written its HTTP response and there is nothing
// left to say to the caller. The transaction holds no data, so a lost commit costs
// nothing beyond the lock being released by the backend when the connection is
// returned; hiding it entirely, which is what the old code did, cost the operator
// the only sign that the database connection was failing.
func (l *QuotaLocker) WithQuotaLock(ctx context.Context, key int64, fn func(context.Context) error) error {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin quota lock transaction: %w", err)
	}

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key); err != nil {
		l.rollback(ctx, tx)
		return fmt.Errorf("acquire quota advisory lock: %w", err)
	}

	fnErr := fn(ctx)

	// Committing is what releases the advisory lock, so it happens on the failure
	// path too — fn's error is the caller's business, not the lock's.
	if err := tx.Commit(ctx); err != nil {
		logger.FromContext(ctx).Warn("Failed to release the quota advisory lock", "error", err)
		l.rollback(ctx, tx)
	}

	return fnErr
}

// rollback aborts tx, ignoring the one error that means "already finished".
func (l *QuotaLocker) rollback(ctx context.Context, tx pgx.Tx) {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		logger.FromContext(ctx).Warn("Failed to roll back the quota lock transaction", "error", err)
	}
}
