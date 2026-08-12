// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"

	"github.com/whento/pkg/httputil"
)

// trustedProxyIPs holds exact trusted proxy IPs.
var trustedProxyIPs map[string]struct{}

// trustedProxyCIDRs holds trusted proxy CIDR ranges.
var trustedProxyCIDRs []*net.IPNet

// SetTrustedProxies configures the set of trusted proxy IPs and CIDR ranges.
// Accepts individual IPs (e.g. "10.0.0.1") and CIDR notation (e.g. "172.17.0.0/16").
// If empty/nil, proxy headers are never trusted (RemoteAddr is always used).
func SetTrustedProxies(proxies []string) {
	if len(proxies) == 0 {
		trustedProxyIPs = nil
		trustedProxyCIDRs = nil
		return
	}
	ips := make(map[string]struct{}, len(proxies))
	var cidrs []*net.IPNet
	for _, p := range proxies {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			_, ipNet, err := net.ParseCIDR(p)
			if err == nil {
				cidrs = append(cidrs, ipNet)
			}
		} else {
			ips[p] = struct{}{}
		}
	}
	trustedProxyIPs = ips
	trustedProxyCIDRs = cidrs
}

// rateLimitBackend is the abstraction used by RateLimiter to decide whether a
// request is allowed. It has two implementations: redisBackend (authoritative,
// shared across instances) and memoryBackend (per-process fallback).
type rateLimitBackend interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, remaining int, resetAt int64, err error)
}

// RateLimiter provides rate limiting with Redis as the primary backend and an
// in-memory token bucket as a fallback. When Redis is unreachable, requests
// continue to be rate-limited locally instead of failing with 503.
//
// The per-call ctx timeout caps the worst case (~the Redis client's
// ReadTimeout) on a probe after Redis recovers. Process-wide circuit breaking
// and outage logging live in the Redis client hook (pkg/database).
type RateLimiter struct {
	primary      rateLimitBackend // nil if no Redis client was provided at startup
	memory       *memoryBackend   // always present, used as fallback or sole backend
	redisTimeout time.Duration
}

const defaultRedisTimeout = 500 * time.Millisecond

// NewRateLimiter creates a new rate limiter. If client is nil, the in-memory
// backend is used exclusively. Otherwise Redis is the primary backend and the
// memory backend takes over transparently on Redis errors.
func NewRateLimiter(client *redis.Client) *RateLimiter {
	rl := &RateLimiter{
		memory:       newMemoryBackend(),
		redisTimeout: defaultRedisTimeout,
	}
	if client != nil {
		rl.primary = &redisBackend{client: client}
	}
	return rl
}

// Stop releases background resources held by the rate limiter (the memory
// backend's eviction goroutine). Safe to call multiple times.
func (rl *RateLimiter) Stop() {
	rl.memory.stop()
}

// RateLimitConfig holds rate limit configuration
type RateLimitConfig struct {
	Requests int                          // Number of requests allowed
	Window   time.Duration                // Time window
	KeyFunc  func(r *http.Request) string // Function to extract rate limit key
}

// Limit creates a rate limiting middleware. Requests are checked against
// Redis when available; on Redis failure the in-memory backend takes over so
// the endpoint stays protected without ever returning 503.
func (rl *RateLimiter) Limit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := cfg.KeyFunc(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// The raw key identifies a person: an IP address, a user id, or a
			// request path that in this API carries a calendar token and a
			// participant UUID. None of it may be written to Redis or held in
			// the in-memory map, so only its digest ever reaches a backend.
			allowed, remaining, resetAt, backend, err := rl.check(r.Context(), hashRateLimitKey(key), cfg.Requests, cfg.Window)
			if err != nil {
				// Should not happen: memory backend never errors. Allow the
				// request rather than block on an unexpected internal error.
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.Requests))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt))
			w.Header().Set("X-RateLimit-Backend", backend)

			if !allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", resetAt-time.Now().Unix()))
				httputil.Error(w, http.StatusTooManyRequests, httputil.ErrCodeRateLimited, "Rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Backend identifiers exposed via the X-RateLimit-Backend response header so
// operators can tell which backend served a given request.
const (
	backendRedis  = "redis"
	backendMemory = "memory"
)

// check dispatches to the primary (Redis) backend first and falls back to the
// memory backend on errors. The per-call ctx timeout caps the worst-case
// latency when Redis is slow or unreachable. The Redis client hook installed
// in pkg/database short-circuits subsequent calls during an outage. The
// returned backend identifier records which path actually served the request.
func (rl *RateLimiter) check(ctx context.Context, key string, limit int, window time.Duration) (bool, int, int64, string, error) {
	if rl.primary != nil {
		callCtx, cancel := context.WithTimeout(ctx, rl.redisTimeout)
		allowed, remaining, resetAt, err := rl.primary.Allow(callCtx, key, limit, window)
		cancel()
		if err == nil {
			return allowed, remaining, resetAt, backendRedis, nil
		}
	}
	allowed, remaining, resetAt, err := rl.memory.Allow(ctx, key, limit, window)
	return allowed, remaining, resetAt, backendMemory, err
}

// redisBackend implements rateLimitBackend on top of a Redis sorted set.
// It uses a pipeline to atomically prune the window, record the request, and
// read the resulting count.
type redisBackend struct {
	client *redis.Client
}

func (b *redisBackend) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, int64, error) {
	redisKey := fmt.Sprintf("ratelimit:%s", key)
	now := time.Now()
	windowStart := now.Add(-window).UnixMicro()

	pipe := b.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart))
	pipe.ZAdd(ctx, redisKey, redis.Z{
		Score:  float64(now.UnixMicro()),
		Member: now.UnixMicro(),
	})
	pipe.ZCard(ctx, redisKey)
	pipe.Expire(ctx, redisKey, window)

	results, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, 0, err
	}

	// The pipeline shape is fixed a few lines above, so neither guard should ever
	// fire. They exist because this runs inside a middleware: an out-of-range index
	// or a failed type assertion would panic on the request path and take every
	// request with it. Returning an error instead falls back to the memory backend,
	// which still enforces the limit.
	if len(results) < 3 {
		return false, 0, 0, fmt.Errorf("ratelimit: pipeline returned %d results, want at least 3", len(results))
	}
	countCmd, ok := results[2].(*redis.IntCmd)
	if !ok {
		return false, 0, 0, fmt.Errorf("ratelimit: unexpected ZCard result type %T", results[2])
	}

	count := countCmd.Val()
	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	resetAt := now.Add(window).Unix()
	return count <= int64(limit), remaining, resetAt, nil
}

// memoryBackend implements rateLimitBackend with a per-key token bucket from
// golang.org/x/time/rate. A background sweeper evicts entries that have not
// been used for evictAfter to avoid unbounded growth on volatile keys.
type memoryBackend struct {
	mu      sync.Mutex
	entries map[string]*memEntry
	done    chan struct{}
	once    sync.Once
}

type memEntry struct {
	limiter  *rate.Limiter
	lastUsed time.Time
}

const (
	sweepInterval = 5 * time.Minute
	evictAfter    = 10 * time.Minute
)

func newMemoryBackend() *memoryBackend {
	b := &memoryBackend{
		entries: make(map[string]*memEntry),
		done:    make(chan struct{}),
	}
	go b.sweepLoop(sweepInterval)
	return b
}

func (b *memoryBackend) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, int, int64, error) {
	if limit <= 0 || window <= 0 {
		return true, limit, time.Now().Add(window).Unix(), nil
	}

	now := time.Now()
	b.mu.Lock()
	entry, ok := b.entries[key]
	if !ok {
		entry = &memEntry{
			limiter: rate.NewLimiter(rate.Every(window/time.Duration(limit)), limit),
		}
		b.entries[key] = entry
	}
	entry.lastUsed = now
	lim := entry.limiter
	b.mu.Unlock()

	allowed := lim.AllowN(now, 1)
	tokens := lim.TokensAt(now)
	remaining := int(tokens)
	if remaining < 0 {
		remaining = 0
	}
	resetAt := now.Add(window).Unix()
	return allowed, remaining, resetAt, nil
}

func (b *memoryBackend) sweepLoop(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-b.done:
			return
		case <-t.C:
			b.sweep(time.Now())
		}
	}
}

// sweep removes entries that have not been used for evictAfter.
func (b *memoryBackend) sweep(now time.Time) {
	cutoff := now.Add(-evictAfter)
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, e := range b.entries {
		if e.lastUsed.Before(cutoff) {
			delete(b.entries, k)
		}
	}
}

func (b *memoryBackend) stop() {
	b.once.Do(func() { close(b.done) })
}

// rateLimitKeySalt keys the HMAC that turns a rate limit key into a bucket
// name. It is random per process, so a stolen Redis dump cannot be matched
// back to an IP, a user id or a calendar token by hashing candidates: without
// the salt there is nothing to compare against.
var rateLimitKeySalt = newRateLimitKeySalt()

func newRateLimitKeySalt() []byte {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		// crypto/rand does not fail on any platform Go supports. Refusing to
		// start beats silently limiting with a predictable all-zero salt.
		panic("middleware: cannot generate the rate limit key salt: " + err.Error())
	}
	return salt
}

// SetRateLimitKeySalt pins the salt used to derive bucket names, which several
// instances sharing one Redis must do to share their buckets: with the default
// per-process random salt each instance hashes the same client to a different
// bucket, so N instances grant N times the configured allowance. Pass a secret
// with at least as much entropy as the limit is worth, and call this before
// serving any request.
func SetRateLimitKeySalt(secret string) {
	if secret == "" {
		return
	}
	sum := sha256.Sum256([]byte(secret))
	rateLimitKeySalt = sum[:]
}

// hashRateLimitKey derives the stored bucket name from a rate limit key.
//
// The key itself is personal data — an IP address, a user id, or a path such
// as /calendar/{token}/participant/{pid} whose values are the credential — and
// the owner's constraint is that none of it is stored. 128 bits of a salted
// HMAC keeps collisions out of reach (a collision would merge two clients'
// budgets) while storing nothing that can be read back.
func hashRateLimitKey(key string) string {
	mac := hmac.New(sha256.New, rateLimitKeySalt)
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// IPKeyFunc returns the real client IP as rate limit key.
// X-Forwarded-For and X-Real-IP are only trusted when the direct
// connection comes from a configured trusted proxy.
func IPKeyFunc(r *http.Request) string {
	remoteIP := remoteAddrIP(r)

	if isFromTrustedProxy(remoteIP) {
		// Trust the rightmost-minus-one entry in X-Forwarded-For (the one
		// set by the trusted proxy), or fall back to X-Real-IP.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			// Use the last entry (closest to the proxy that added it)
			clientIP := strings.TrimSpace(parts[len(parts)-1])
			if clientIP != "" {
				return clientIP
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}

// remoteAddrIP extracts the IP (without port) from r.RemoteAddr.
func remoteAddrIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isFromTrustedProxy checks if the remote IP is in the trusted proxy set
// or falls within a trusted CIDR range.
func isFromTrustedProxy(ip string) bool {
	if trustedProxyIPs == nil && len(trustedProxyCIDRs) == 0 {
		return false
	}
	if _, ok := trustedProxyIPs[ip]; ok {
		return true
	}
	if len(trustedProxyCIDRs) > 0 {
		parsed := net.ParseIP(ip)
		if parsed != nil {
			for _, cidr := range trustedProxyCIDRs {
				if cidr.Contains(parsed) {
					return true
				}
			}
		}
	}
	return false
}

// UserKeyFunc returns user ID as rate limit key (requires Auth middleware)
func UserKeyFunc(r *http.Request) string {
	return GetUserID(r.Context())
}

// CombinedKeyFunc combines path and IP for endpoint-specific limiting.
// The path is deliberately the exact one, not the route pattern, so that two
// calendars do not share a budget. It is never stored as such: Limit hashes
// the key before any backend sees it (see hashRateLimitKey).
func CombinedKeyFunc(r *http.Request) string {
	return fmt.Sprintf("%s:%s", r.URL.Path, IPKeyFunc(r))
}
