// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/testutil/dbtest"
)

// Skips when DATABASE_URL is unset; see internal/testutil/dbtest.
//
// The advisory lock is the one piece of this refactoring that cannot be proved with
// a fake: whether it excludes anything is a property of PostgreSQL, not of the Go
// around it.

func TestQuotaLockerRunsAndReleases(t *testing.T) {
	pool := dbtest.Pool(t)
	locker := repository.NewQuotaLocker(pool)

	// A key of its own, so a concurrent package cannot collide with it.
	const key = int64(-918273645)

	t.Run("the body runs and the lock is released afterwards", func(t *testing.T) {
		ran := false
		if err := locker.WithQuotaLock(dbtest.Context(t), key, func(context.Context) error {
			ran = true
			return nil
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ran {
			t.Fatal("the body never ran")
		}

		// Taking it again immediately proves the first transaction committed: an
		// xact lock outlives its transaction by nothing.
		second := make(chan error, 1)
		go func() {
			second <- locker.WithQuotaLock(context.Background(), key, func(context.Context) error { return nil })
		}()
		select {
		case err := <-second:
			if err != nil {
				t.Fatalf("second acquisition failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the lock was never released")
		}
	})

	t.Run("the body's error is returned unchanged", func(t *testing.T) {
		sentinel := errors.New("quota refused")
		err := locker.WithQuotaLock(dbtest.Context(t), key, func(context.Context) error { return sentinel })
		if !errors.Is(err, sentinel) {
			t.Fatalf("error = %v, want %v", err, sentinel)
		}
	})

	t.Run("a second holder waits for the first", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		firstDone := make(chan error, 1)

		go func() {
			firstDone <- locker.WithQuotaLock(context.Background(), key, func(context.Context) error {
				close(entered)
				<-release
				return nil
			})
		}()

		<-entered

		secondEntered := make(chan struct{})
		secondDone := make(chan error, 1)
		go func() {
			secondDone <- locker.WithQuotaLock(context.Background(), key, func(context.Context) error {
				close(secondEntered)
				return nil
			})
		}()

		select {
		case <-secondEntered:
			t.Fatal("two callers held the quota lock at once")
		case <-time.After(300 * time.Millisecond):
			// Still waiting, which is the point.
		}

		close(release)

		if err := <-firstDone; err != nil {
			t.Fatalf("first holder failed: %v", err)
		}
		select {
		case err := <-secondDone:
			if err != nil {
				t.Fatalf("second holder failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the second holder never acquired the lock")
		}
	})
}

// TestQuotaLockerRefusesOnAClosedPool covers the failure the handler translates
// into a 500: the lock could not be taken, so the body must not have run.
func TestQuotaLockerRefusesOnAClosedPool(t *testing.T) {
	dbtest.Pool(t) // skips when there is no database to open a second pool against

	// A pool of its own, so closing it cannot disturb the shared one.
	own, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Skipf("cannot open a second pool: %v", err)
	}
	own.Close()

	locker := repository.NewQuotaLocker(own)
	ran := false
	err = locker.WithQuotaLock(context.Background(), 1, func(context.Context) error {
		ran = true
		return nil
	})
	if err == nil {
		t.Fatal("expected a failure when the pool is closed")
	}
	if ran {
		t.Error("the body ran without the lock")
	}
}
