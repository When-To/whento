// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestCircuitBreaker_TripsOnError(t *testing.T) {
	h := newCircuitBreakerHook(time.Hour)

	calls := 0
	process := h.ProcessHook(func(_ context.Context, cmd redis.Cmder) error {
		calls++
		cmd.SetErr(errors.New("dial timeout"))
		return errors.New("dial timeout")
	})

	// First call: hits the underlying op and trips.
	_ = process(context.Background(), redis.NewStatusCmd(context.Background(), "PING"))
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	// Subsequent calls: short-circuited, never reach the underlying op.
	for i := 0; i < 5; i++ {
		err := process(context.Background(), redis.NewStatusCmd(context.Background(), "PING"))
		if !errors.Is(err, ErrRedisCircuitOpen) {
			t.Fatalf("expected ErrRedisCircuitOpen on call %d, got %v", i+2, err)
		}
	}
	if calls != 1 {
		t.Fatalf("underlying op should not be called while breaker is open; got %d", calls)
	}
}

func TestCircuitBreaker_ClosesAfterCooldown(t *testing.T) {
	h := newCircuitBreakerHook(10 * time.Millisecond)

	calls := 0
	process := h.ProcessHook(func(_ context.Context, cmd redis.Cmder) error {
		calls++
		if calls == 1 {
			err := errors.New("dial timeout")
			cmd.SetErr(err)
			return err
		}
		return nil
	})

	_ = process(context.Background(), redis.NewStatusCmd(context.Background(), "PING"))

	// Wait for cooldown to expire, then the next call must reach the op
	// (probe) and succeed.
	time.Sleep(15 * time.Millisecond)
	err := process(context.Background(), redis.NewStatusCmd(context.Background(), "PING"))
	if err != nil {
		t.Fatalf("probe should succeed, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (initial + probe), got %d", calls)
	}

	// Breaker is now closed, more calls reach the op.
	_ = process(context.Background(), redis.NewStatusCmd(context.Background(), "PING"))
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestCircuitBreaker_IgnoresRedisNil(t *testing.T) {
	h := newCircuitBreakerHook(time.Hour)

	process := h.ProcessHook(func(_ context.Context, cmd redis.Cmder) error {
		cmd.SetErr(redis.Nil)
		return redis.Nil
	})

	// redis.Nil is a "key not found" sentinel, not an outage signal.
	_ = process(context.Background(), redis.NewStatusCmd(context.Background(), "GET"))
	if h.openedAt.Load() != 0 {
		t.Fatalf("breaker must not trip on redis.Nil")
	}
}

// serverErr satisfies redis.Error, the interface go-redis uses for replies
// the server actively returns (-ERR ...) as opposed to network failures.
type serverErr string

func (e serverErr) Error() string { return string(e) }
func (serverErr) RedisError()     {}

func TestCircuitBreaker_IgnoresServerSideErrors(t *testing.T) {
	h := newCircuitBreakerHook(time.Hour)

	// A server-side rejection (unknown command, wrong type, etc.) means the
	// server is talking to us fine. Tripping on these would break connection
	// setup against older Redis versions that don't implement every
	// subcommand go-redis tries (e.g. CLIENT MAINT_NOTIFICATIONS).
	srvErr := serverErr("ERR unknown subcommand 'maint_notifications'")
	process := h.ProcessHook(func(_ context.Context, cmd redis.Cmder) error {
		cmd.SetErr(srvErr)
		return srvErr
	})

	_ = process(context.Background(), redis.NewStatusCmd(context.Background(), "CLIENT"))
	if h.openedAt.Load() != 0 {
		t.Fatalf("breaker must not trip on a redis.Error (server is responding)")
	}
}

func TestCircuitBreaker_PipelineShortCircuit(t *testing.T) {
	h := newCircuitBreakerHook(time.Hour)
	h.openedAt.Store(time.Now().Add(time.Hour).UnixNano())

	calls := 0
	pipe := h.ProcessPipelineHook(func(_ context.Context, _ []redis.Cmder) error {
		calls++
		return nil
	})

	cmds := []redis.Cmder{
		redis.NewStatusCmd(context.Background(), "ZADD"),
		redis.NewIntCmd(context.Background(), "ZCARD"),
	}
	err := pipe(context.Background(), cmds)
	if !errors.Is(err, ErrRedisCircuitOpen) {
		t.Fatalf("expected ErrRedisCircuitOpen, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("underlying pipeline must not run while breaker is open; got %d calls", calls)
	}
	for i, c := range cmds {
		if !errors.Is(c.Err(), ErrRedisCircuitOpen) {
			t.Fatalf("cmd %d: expected per-cmd ErrRedisCircuitOpen, got %v", i, c.Err())
		}
	}
}

func TestNewRedisPinger(t *testing.T) {
	t.Run("no client yields no probe", func(t *testing.T) {
		// Returning a typed nil here would be worse than returning nothing: the
		// caller would store it in an interface, the interface would test
		// non-nil, and readiness would report a hard failure for a dependency
		// the operator deliberately did not deploy.
		if p := NewRedisPinger(nil); p != nil {
			t.Errorf("NewRedisPinger(nil) = %v, want nil", p)
		}
	})

	t.Run("a nil probe still answers rather than panicking", func(t *testing.T) {
		var p *RedisPinger
		if err := p.Ping(context.Background()); !errors.Is(err, ErrRedisUnavailable) {
			t.Errorf("Ping() = %v, want ErrRedisUnavailable", err)
		}
	})

	t.Run("an unreachable Redis reports an error and does not hang", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{
			Addr:        "127.0.0.1:1",
			DialTimeout: 100 * time.Millisecond,
			MaxRetries:  -1,
		})
		t.Cleanup(func() { _ = client.Close() })

		p := NewRedisPinger(client)
		if p == nil {
			t.Fatal("NewRedisPinger returned nil for a real client")
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		start := time.Now()
		if err := p.Ping(ctx); err == nil {
			t.Skip("something is listening on 127.0.0.1:1")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("the probe took %v; a readiness check must fail fast", elapsed)
		}
	})
}
