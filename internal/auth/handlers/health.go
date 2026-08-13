// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/whento/pkg/httputil"
)

// Probe reports whether a dependency answers. *pgxpool.Pool satisfies it as
// is; database.NewRedisPinger adapts the Redis client.
type Probe interface {
	Ping(ctx context.Context) error
}

const (
	// dbProbeTimeout caps the readiness probe's database ping. It has to be
	// short: an orchestrator that waits on this endpoint is deciding whether to
	// take the instance out of service, and a probe that hangs is a probe that
	// answers neither way.
	dbProbeTimeout = 2 * time.Second

	// cacheProbeTimeout caps the Redis ping. Redis is a soft dependency, so its
	// budget is smaller still — and the client's own circuit breaker usually
	// answers instantly during an outage.
	cacheProbeTimeout = 1 * time.Second

	// readinessTTL is how long a probe result is reused. /api/ready is public
	// and unauthenticated, so without this every caller gets a free round trip
	// to PostgreSQL and the endpoint becomes an amplifier. One second is short
	// enough that no orchestrator notices, and long enough that a flood costs
	// one query per second rather than one per request.
	readinessTTL = 1 * time.Second
)

// Dependency states reported by /api/ready.
const (
	stateOK       = "ok"
	stateDown     = "down"
	stateDisabled = "disabled"
)

// HealthHandler answers the liveness and readiness probes.
//
// The split matters. Liveness says "this process is running" and must probe
// nothing: a failing liveness probe gets the container killed, so wiring it to
// the database means a database blip restarts every instance at once. Readiness
// says "this instance can serve traffic" and is where the dependencies belong.
type HealthHandler struct {
	// db is the hard dependency: no PostgreSQL, no service.
	db Probe

	// cache is the soft one, and nil when Redis was never configured. Redis is
	// optional in this project (the cache falls back to a NoOp and the rate
	// limiter to its in-memory backend), so losing it degrades the instance
	// without making it unfit to serve.
	cache Probe

	// Probe budgets and result reuse, fields rather than constants so tests can
	// shorten them.
	dbTimeout    time.Duration
	cacheTimeout time.Duration
	ttl          time.Duration
	now          func() time.Time

	// mu also serialises the probes themselves: two simultaneous requests
	// produce one ping, not two.
	mu         sync.Mutex
	cached     readyResponse
	cachedAt   time.Time
	cachedHTTP int
}

// NewHealthHandler creates a health handler. cache may be nil when Redis is not
// configured, which is reported as "disabled" rather than as a failure.
func NewHealthHandler(db, cache Probe) *HealthHandler {
	return &HealthHandler{
		db:           db,
		cache:        cache,
		dbTimeout:    dbProbeTimeout,
		cacheTimeout: cacheProbeTimeout,
		ttl:          readinessTTL,
		now:          time.Now,
	}
}

// liveResponse is the liveness payload. Deliberately one field: this endpoint
// is unauthenticated, and a version or a hostname here is free reconnaissance.
type liveResponse struct {
	Status string `json:"status"`
}

// readyResponse is the readiness payload. The per-dependency states are the
// whole point — "not ready" without saying which dependency is missing sends
// an operator to read logs — but they are three fixed words. No error message
// from PostgreSQL or Redis is ever forwarded: those carry host names, user
// names and version numbers.
type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Health returns liveness. It probes nothing, on purpose: it answers "the
// process is alive and its HTTP server is accepting requests", which is what
// the Docker HEALTHCHECK asks and all it should act on.
func (h *HealthHandler) Health(w http.ResponseWriter, _ *http.Request) {
	httputil.JSON(w, http.StatusOK, liveResponse{Status: "healthy"})
}

// Ready returns readiness: 200 while PostgreSQL answers, 503 as soon as it does
// not. Redis is reported but never decides the verdict.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	resp, status := h.readiness(r.Context())
	httputil.JSON(w, status, resp)
}

// readiness returns the probe result, reusing a recent one when it has not yet
// expired.
func (h *HealthHandler) readiness(ctx context.Context) (readyResponse, int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ttl > 0 && !h.cachedAt.IsZero() && h.now().Sub(h.cachedAt) < h.ttl {
		return h.cached, h.cachedHTTP
	}

	resp, status := h.probe(ctx)

	// A probe cut short because the caller hung up says nothing about the
	// database; caching it would answer 503 to everyone else for a second.
	if ctx.Err() == nil {
		h.cached, h.cachedHTTP, h.cachedAt = resp, status, h.now()
	}

	return resp, status
}

// probe pings the dependencies and builds the answer.
func (h *HealthHandler) probe(ctx context.Context) (readyResponse, int) {
	resp := readyResponse{Status: "ready", Checks: map[string]string{}}
	httpStatus := http.StatusOK

	if err := ping(ctx, h.db, h.dbTimeout); err != nil {
		resp.Checks["database"] = stateDown
		resp.Status = "not_ready"
		httpStatus = http.StatusServiceUnavailable
	} else {
		resp.Checks["database"] = stateOK
	}

	switch {
	case h.cache == nil:
		resp.Checks["cache"] = stateDisabled
	case ping(ctx, h.cache, h.cacheTimeout) != nil:
		// Soft dependency: reported, never fatal. An instance without Redis
		// serves every request, only with a colder cache and per-instance rate
		// limit buckets.
		resp.Checks["cache"] = stateDown
	default:
		resp.Checks["cache"] = stateOK
	}

	return resp, httpStatus
}

// ping runs one probe under its own deadline. A nil probe counts as a failure:
// a dependency that was never wired up is not a dependency that is answering.
func ping(ctx context.Context, p Probe, timeout time.Duration) error {
	if p == nil {
		return errNoProbe
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return p.Ping(probeCtx)
}

// errNoProbe is returned when a dependency has no probe wired to it.
var errNoProbe = errors.New("health: dependency not configured")
