// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/pkg/metrics"
)

// Pool defaults. Exported so callers reading these from the environment can
// advertise the same values as their own defaults instead of guessing, and so
// the numbers live in exactly one place.
//
// 25 connections is a single instance against a stock PostgreSQL, whose
// max_connections is 100: a deployment running several replicas has to divide
// DefaultMaxConns by the replica count, which is the whole reason these are
// configurable.
const (
	DefaultMaxConns        int32 = 25
	DefaultMinConns        int32 = 5
	DefaultMaxConnLifetime       = time.Hour
	DefaultMaxConnIdleTime       = 30 * time.Minute
)

// Config holds database configuration
type Config struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		MaxConns:        DefaultMaxConns,
		MinConns:        DefaultMinConns,
		MaxConnLifetime: DefaultMaxConnLifetime,
		MaxConnIdleTime: DefaultMaxConnIdleTime,
	}
}

// NewPool creates a new PostgreSQL connection pool
func NewPool(ctx context.Context, cfg *Config) (*pgxpool.Pool, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Apply default values if not set
	if cfg.MaxConns == 0 {
		cfg.MaxConns = DefaultMaxConns
	}
	if cfg.MinConns == 0 {
		cfg.MinConns = DefaultMinConns
	}
	if cfg.MaxConnLifetime == 0 {
		cfg.MaxConnLifetime = DefaultMaxConnLifetime
	}
	if cfg.MaxConnIdleTime == 0 {
		cfg.MaxConnIdleTime = DefaultMaxConnIdleTime
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}

// Close closes the database pool
func Close(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}

// PoolStatsFunc adapts a pgx pool to the snapshot function metrics.RegisterPool
// expects. The snapshot is taken when the function is called — at scrape time —
// not when this is wired up.
//
// A nil pool yields a zero snapshot rather than a panic: a scrape must never be
// able to take the process down.
func PoolStatsFunc(pool *pgxpool.Pool) func() metrics.PoolStats {
	return func() metrics.PoolStats {
		if pool == nil {
			return metrics.PoolStats{}
		}
		s := pool.Stat()
		return metrics.PoolStats{
			MaxConns:          s.MaxConns(),
			TotalConns:        s.TotalConns(),
			AcquiredConns:     s.AcquiredConns(),
			IdleConns:         s.IdleConns(),
			ConstructingConns: s.ConstructingConns(),

			AcquireCount:            s.AcquireCount(),
			AcquireDuration:         s.AcquireDuration(),
			EmptyAcquireCount:       s.EmptyAcquireCount(),
			CanceledAcquireCount:    s.CanceledAcquireCount(),
			NewConnsCount:           s.NewConnsCount(),
			MaxLifetimeDestroyCount: s.MaxLifetimeDestroyCount(),
			MaxIdleDestroyCount:     s.MaxIdleDestroyCount(),
		}
	}
}
