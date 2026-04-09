// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
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

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	client *redis.Client
}

// NewRateLimiter creates a new rate limiter
// If client is nil, rate limiting is disabled (allows all requests)
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// RateLimitConfig holds rate limit configuration
type RateLimitConfig struct {
	Requests   int                          // Number of requests allowed
	Window     time.Duration                // Time window
	KeyFunc    func(r *http.Request) string // Function to extract rate limit key
	FailClosed bool                         // If true, block requests when Redis is unavailable (critical routes)
}

// Limit creates a rate limiting middleware
// If Redis client is nil, this middleware does nothing (allows all requests)
func (rl *RateLimiter) Limit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If Redis is not available, behavior depends on FailClosed setting
			if rl.client == nil {
				if cfg.FailClosed {
					http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			key := cfg.KeyFunc(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed, remaining, resetAt, err := rl.check(r.Context(), key, cfg.Requests, cfg.Window)
			if err != nil {
				if cfg.FailClosed {
					http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
					return
				}
				// On error, allow the request on non-critical routes
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", cfg.Requests))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt))

			if !allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", resetAt-time.Now().Unix()))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) check(ctx context.Context, key string, limit int, window time.Duration) (bool, int, int64, error) {
	redisKey := fmt.Sprintf("ratelimit:%s", key)
	now := time.Now()
	windowStart := now.Add(-window).UnixMicro()

	pipe := rl.client.Pipeline()

	// Remove old entries
	pipe.ZRemRangeByScore(ctx, redisKey, "0", fmt.Sprintf("%d", windowStart))

	// Add current request
	pipe.ZAdd(ctx, redisKey, redis.Z{
		Score:  float64(now.UnixMicro()),
		Member: now.UnixMicro(),
	})

	// Count requests in window
	pipe.ZCard(ctx, redisKey)

	// Set expiry
	pipe.Expire(ctx, redisKey, window)

	results, err := pipe.Exec(ctx)
	if err != nil {
		return true, limit, now.Add(window).Unix(), err
	}

	count := results[2].(*redis.IntCmd).Val()
	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	resetAt := now.Add(window).Unix()

	return count <= int64(limit), remaining, resetAt, nil
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

// CombinedKeyFunc combines path and IP for endpoint-specific limiting
func CombinedKeyFunc(r *http.Request) string {
	return fmt.Sprintf("%s:%s", r.URL.Path, IPKeyFunc(r))
}
