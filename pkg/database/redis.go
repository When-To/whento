// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package database

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/whento/pkg/logger"
)

// RedisConfig holds Redis configuration
type RedisConfig struct {
	URL string
}

// ErrRedisCircuitOpen is returned for commands that are short-circuited
// because Redis is currently considered unhealthy.
var ErrRedisCircuitOpen = errors.New("redis circuit breaker open")

const circuitBreakerCooldown = 5 * time.Second

// NewRedisClient creates a new Redis client.
//
// The defaults (5s dial, 3s read/write, 3 retries) make every operation block
// ~19s while go-redis cycles through retries when Redis is unreachable, which
// stalls login and other Redis-touching routes far past the browser timeout.
// Two layers of protection are applied:
//
//   - aggressive client timeouts and MaxRetries=-1 so a single hung call fails
//     in roughly 1s instead of 19s;
//   - a process-wide circuit breaker installed as a hook: after a failure,
//     every subsequent command/pipeline returns ErrRedisCircuitOpen instantly
//     for circuitBreakerCooldown. The next call after the cooldown probes
//     Redis; on success the breaker closes again. This caps the cost of an
//     outage at ~one slow probe per cooldown window.
//
// Callers (cache, rate limiter) handle the degraded path themselves.
func NewRedisClient(ctx context.Context, cfg *RedisConfig) (*redis.Client, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	opts.DialTimeout = 500 * time.Millisecond
	opts.ReadTimeout = 1 * time.Second
	opts.WriteTimeout = 1 * time.Second
	opts.MaxRetries = -1

	client := redis.NewClient(opts)
	client.AddHook(newCircuitBreakerHook(circuitBreakerCooldown))

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return client, nil
}

// circuitBreakerHook short-circuits Redis commands while the breaker is open.
// openedAt encodes three states:
//   - 0: closed (healthy or never tripped)
//   - future unix-nanos: open (short-circuit until that time)
//   - past unix-nanos: half-open, the next call probes Redis
//
// Transitions are logged once per direction so a long outage produces a single
// Warn and a single Info on recovery — not one per call.
type circuitBreakerHook struct {
	cooldown time.Duration
	openedAt atomic.Int64
}

func newCircuitBreakerHook(cooldown time.Duration) *circuitBreakerHook {
	return &circuitBreakerHook{cooldown: cooldown}
}

func (h *circuitBreakerHook) tripIfNeeded(prev int64, err error) {
	if !isOutageError(err) {
		return
	}
	next := time.Now().Add(h.cooldown).UnixNano()
	if h.openedAt.CompareAndSwap(prev, next) && prev == 0 {
		logger.Warn("redis: circuit breaker tripped", "error", err.Error(), "cooldown", h.cooldown.String())
	}
}

// isOutageError reports whether err signals that Redis is unreachable, as
// opposed to a normal cache miss (redis.Nil) or a server-side application
// error (redis.Error — e.g. unknown command, wrong type). Application errors
// mean the server is talking to us just fine, so the breaker stays closed.
func isOutageError(err error) bool {
	if err == nil || errors.Is(err, redis.Nil) {
		return false
	}
	var redisErr redis.Error
	return !errors.As(err, &redisErr)
}

func (h *circuitBreakerHook) closeIfNeeded(prev int64) {
	if prev == 0 {
		return
	}
	if h.openedAt.CompareAndSwap(prev, 0) {
		logger.Info("redis: circuit breaker closed (recovered)")
	}
}

func (h *circuitBreakerHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *circuitBreakerHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		prev := h.openedAt.Load()
		if prev != 0 && time.Now().UnixNano() < prev {
			cmd.SetErr(ErrRedisCircuitOpen)
			return ErrRedisCircuitOpen
		}
		err := next(ctx, cmd)
		if isOutageError(err) {
			h.tripIfNeeded(prev, err)
			return err
		}
		h.closeIfNeeded(prev)
		return err
	}
}

func (h *circuitBreakerHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		prev := h.openedAt.Load()
		if prev != 0 && time.Now().UnixNano() < prev {
			for _, cmd := range cmds {
				cmd.SetErr(ErrRedisCircuitOpen)
			}
			return ErrRedisCircuitOpen
		}
		err := next(ctx, cmds)
		if isOutageError(err) {
			h.tripIfNeeded(prev, err)
			return err
		}
		h.closeIfNeeded(prev)
		return err
	}
}

// RedisPinger adapts a Redis client to the Ping(ctx) error shape a readiness
// probe expects. go-redis returns a *StatusCmd, which is one indirection too
// many for a caller that only wants to know whether Redis answered.
//
// The circuit breaker installed by NewRedisClient applies here too: during an
// outage the probe returns ErrRedisCircuitOpen immediately instead of waiting
// out a dial timeout, which is what keeps a readiness endpoint cheap.
type RedisPinger struct {
	client *redis.Client
}

// NewRedisPinger returns a probe for client. It returns nil when client is nil
// so that a caller can test the result and leave its interface field nil,
// rather than end up holding a non-nil interface wrapping a nil pointer.
func NewRedisPinger(client *redis.Client) *RedisPinger {
	if client == nil {
		return nil
	}
	return &RedisPinger{client: client}
}

// Ping reports whether Redis answered under ctx.
func (p *RedisPinger) Ping(ctx context.Context) error {
	if p == nil || p.client == nil {
		return ErrRedisUnavailable
	}
	return p.client.Ping(ctx).Err()
}

// ErrRedisUnavailable is returned when a probe is asked about a Redis client
// that was never created.
var ErrRedisUnavailable = errors.New("redis: no client")

// CloseRedis closes the Redis client
func CloseRedis(client *redis.Client) error {
	if client != nil {
		return client.Close()
	}
	return nil
}
