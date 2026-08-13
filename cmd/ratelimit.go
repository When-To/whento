// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
)

// rateLimitRule is one bucket: how many requests, over what window, counted
// against whom. It is pkg/middleware's RateLimitConfig minus the question of
// whether rate limiting is switched on at all, which is the routeLimiter's.
type rateLimitRule struct {
	requests int
	window   time.Duration
	keyFunc  func(r *http.Request) string
}

// perIP buckets by client address. Proxy headers are honoured only for
// connections coming from a configured trusted proxy; see middleware.IPKeyFunc.
func perIP(requests int, window time.Duration) rateLimitRule {
	return rateLimitRule{requests: requests, window: window, keyFunc: middleware.IPKeyFunc}
}

// perUser buckets by authenticated user id, and therefore only means anything
// below middleware.Auth: without it the key is empty and the limiter lets the
// request through.
func perUser(requests int, window time.Duration) rateLimitRule {
	return rateLimitRule{requests: requests, window: window, keyFunc: middleware.UserKeyFunc}
}

// perPathIP buckets by exact request path *and* client address, so two
// calendars — or two participants of one calendar — never share a budget.
func perPathIP(requests int, window time.Duration) rateLimitRule {
	return rateLimitRule{requests: requests, window: window, keyFunc: middleware.CombinedKeyFunc}
}

// routeLimiter decides once whether rate limiting applies, so that no route has
// to decide it again.
//
// Every rate-limited route used to be registered twice — once under
// `if cfg.RateLimitEnabled` with the bucket and once in the `else` branch
// without it — thirteen branches, twenty-six registrations, and two places for
// a path or a handler to drift apart. The routes now register once and the
// difference lives here.
type routeLimiter struct {
	limiter *middleware.RateLimiter
	enabled bool
}

// newRouteLimiter pairs the limiter with the configuration flag that governs it.
func newRouteLimiter(limiter *middleware.RateLimiter, enabled bool) *routeLimiter {
	return &routeLimiter{limiter: limiter, enabled: enabled}
}

// middlewares turns a rule into a chi middleware stack: exactly one middleware
// when rate limiting is enabled, and an empty stack when it is not.
//
// Empty rather than a neutral pass-through middleware, deliberately. Both serve
// the request identically, but chi's Use and With are variadic, so an empty
// stack is *not registered at all*: the middleware chain, its ordering, the
// response headers and what chi.Walk reports are then bit-for-bit what the
// old `else` branch produced. A pass-through would sit in the chain and show up
// in Walk, which is a difference — small, but a difference this refactor has no
// reason to introduce.
func (l *routeLimiter) middlewares(rule rateLimitRule) chi.Middlewares {
	if !l.enabled {
		return nil
	}

	return chi.Middlewares{l.limiter.Limit(middleware.RateLimitConfig{
		Requests: rule.requests,
		Window:   rule.window,
		KeyFunc:  rule.keyFunc,
	})}
}

// on returns the router the next single route should be registered on: r itself
// when rate limiting is off — the exact call the old `else` branch made — and an
// inline sub-router carrying the bucket when it is on.
//
//	l.on(r, perIP(60, time.Minute)).Get("/public/{token}", h.calendar.GetPublicCalendar)
func (l *routeLimiter) on(r chi.Router, rule rateLimitRule) chi.Router {
	mws := l.middlewares(rule)
	if len(mws) == 0 {
		return r
	}

	return r.With(mws...)
}

// use applies the bucket to every route registered on r afterwards, and adds
// nothing when rate limiting is off.
//
// Like any chi Use it has to come before the first route on that router.
func (l *routeLimiter) use(r chi.Router, rule rateLimitRule) {
	r.Use(l.middlewares(rule)...)
}
