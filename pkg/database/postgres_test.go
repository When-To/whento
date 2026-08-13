// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package database

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/metrics"
)

// newUnconnectedPool builds a pool against an address nothing listens on. pgx
// opens connections lazily, so this yields a real *pgxpool.Pool — and therefore
// a real *pgxpool.Stat — without a database.
func newUnconnectedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig("postgres://whento:whento@127.0.0.1:1/whento?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.MaxConns = 7
	cfg.MinConns = 0

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func TestPoolStatsFunc(t *testing.T) {
	t.Run("a live pool reports its own configuration", func(t *testing.T) {
		stats := PoolStatsFunc(newUnconnectedPool(t))()

		if stats.MaxConns != 7 {
			t.Errorf("MaxConns = %d, want 7", stats.MaxConns)
		}
		if stats.AcquiredConns != 0 {
			t.Errorf("AcquiredConns = %d, want 0 on an idle pool", stats.AcquiredConns)
		}
		if stats.AcquireDuration < 0 {
			t.Errorf("AcquireDuration = %v, want a non-negative duration", stats.AcquireDuration)
		}
	})

	t.Run("no pool yields a zero snapshot", func(t *testing.T) {
		// A scrape must never be able to panic the process, whatever state
		// startup left things in.
		stats := PoolStatsFunc(nil)()

		if stats != (metrics.PoolStats{}) {
			t.Errorf("stats = %+v, want the zero value", stats)
		}
	})

	t.Run("the snapshot is taken when called, not when built", func(t *testing.T) {
		pool := newUnconnectedPool(t)
		snapshot := PoolStatsFunc(pool)

		// Acquiring cannot succeed here, but it does move the pool's counters,
		// which is enough to show the function reads them afresh.
		before := snapshot()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if conn, err := pool.Acquire(ctx); err == nil {
			conn.Release()
			t.Skip("something is listening on 127.0.0.1:1, the pool connected")
		}

		after := snapshot()
		if after.CanceledAcquireCount <= before.CanceledAcquireCount &&
			after.EmptyAcquireCount <= before.EmptyAcquireCount &&
			after.NewConnsCount <= before.NewConnsCount {
			t.Errorf("no counter moved: before %+v, after %+v", before, after)
		}
	})
}

func TestClose(t *testing.T) {
	// Close is called from a defer that runs whether or not the pool was ever
	// created.
	Close(nil)
}
