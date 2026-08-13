// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubProbe is a hand-written dependency probe. delay lets a test starve the
// probe budget without waiting for the real one.
type stubProbe struct {
	mu    sync.Mutex
	err   error
	delay time.Duration
	calls int
}

func (s *stubProbe) Ping(ctx context.Context) error {
	s.mu.Lock()
	s.calls++
	err, delay := s.err, s.delay
	s.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return err
}

func (s *stubProbe) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// envelope mirrors the {success, data} shape httputil writes.
type envelope struct {
	Success bool `json:"success"`
	Data    struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	} `json:"data"`
}

func decodeReady(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()

	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("the response is not JSON: %v (%q)", err, rec.Body.String())
	}

	return env
}

// TestReadyProbesItsDependencies is the defect this replaces: the old handler
// answered a static 200 whatever the state of the world, so an instance with a
// dead database advertised itself as healthy for ever and no orchestrator ever
// took it out of service.
func TestReadyProbesItsDependencies(t *testing.T) {
	dbDown := errors.New("connection refused")

	tests := []struct {
		name       string
		db         Probe
		cache      Probe
		wantStatus int
		wantBody   string
		wantChecks map[string]string
	}{
		{
			name:       "everything answers",
			db:         &stubProbe{},
			cache:      &stubProbe{},
			wantStatus: http.StatusOK,
			wantBody:   "ready",
			wantChecks: map[string]string{"database": stateOK, "cache": stateOK},
		},
		{
			name:       "the database is gone",
			db:         &stubProbe{err: dbDown},
			cache:      &stubProbe{},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "not_ready",
			wantChecks: map[string]string{"database": stateDown, "cache": stateOK},
		},
		{
			// Redis is optional in this project: the cache falls back to a NoOp
			// and the rate limiter to its in-memory backend. Losing it degrades
			// the instance, it does not disqualify it.
			name:       "Redis is down, the instance still serves",
			db:         &stubProbe{},
			cache:      &stubProbe{err: errors.New("i/o timeout")},
			wantStatus: http.StatusOK,
			wantBody:   "ready",
			wantChecks: map[string]string{"database": stateOK, "cache": stateDown},
		},
		{
			name:       "Redis was never configured",
			db:         &stubProbe{},
			cache:      nil,
			wantStatus: http.StatusOK,
			wantBody:   "ready",
			wantChecks: map[string]string{"database": stateOK, "cache": stateDisabled},
		},
		{
			name:       "both are gone",
			db:         &stubProbe{err: dbDown},
			cache:      &stubProbe{err: dbDown},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "not_ready",
			wantChecks: map[string]string{"database": stateDown, "cache": stateDown},
		},
		{
			// Defensive: a handler wired without a database probe must not
			// claim to be ready.
			name:       "no database probe at all",
			db:         nil,
			cache:      nil,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "not_ready",
			wantChecks: map[string]string{"database": stateDown, "cache": stateDisabled},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHealthHandler(tt.db, tt.cache)

			rec := httptest.NewRecorder()
			h.Ready(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			env := decodeReady(t, rec)
			if env.Data.Status != tt.wantBody {
				t.Errorf("status field = %q, want %q", env.Data.Status, tt.wantBody)
			}
			for dep, want := range tt.wantChecks {
				if got := env.Data.Checks[dep]; got != want {
					t.Errorf("checks[%s] = %q, want %q", dep, got, want)
				}
			}
		})
	}
}

// TestReadyDisclosesNothingExploitable: this endpoint is public and
// unauthenticated. It has to say which dependency is missing — "not ready" with
// no detail sends an operator to read logs — and nothing beyond that. Driver
// errors carry host names, user names and version numbers.
func TestReadyDisclosesNothingExploitable(t *testing.T) {
	const leak = "dial tcp 10.1.2.3:5432: connect: connection refused (PostgreSQL 16.2, user=whento)"

	h := NewHealthHandler(&stubProbe{err: errors.New(leak)}, &stubProbe{err: errors.New(leak)})

	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))

	body := rec.Body.String()
	for _, secret := range []string{"10.1.2.3", "5432", "PostgreSQL", "16.2", "whento", "connection refused"} {
		if strings.Contains(body, secret) {
			t.Errorf("the readiness response leaks %q: %s", secret, body)
		}
	}
}

// TestHealthProbesNothing: liveness answers "the process is alive". Wiring it to
// the database would have a database blip restart every container at once —
// the Docker HEALTHCHECK calls exactly this endpoint.
func TestHealthProbesNothing(t *testing.T) {
	db := &stubProbe{err: errors.New("down")}
	cacheProbe := &stubProbe{err: errors.New("down")}
	h := NewHealthHandler(db, cacheProbe)

	rec := httptest.NewRecorder()
	h.Health(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 even with every dependency down", rec.Code)
	}
	if db.count() != 0 || cacheProbe.count() != 0 {
		t.Errorf("liveness probed its dependencies: db=%d cache=%d", db.count(), cacheProbe.count())
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"status":"healthy"`) {
		t.Errorf("body = %s, want a plain healthy status", body)
	}
	// "service":"auth" was a leftover from a microservice split that never
	// happened: there is one binary, and it does not answer for a service.
	if strings.Contains(body, "service") {
		t.Errorf("the liveness payload still names a service: %s", body)
	}
}

// TestReadyHonoursTheProbeBudget: an orchestrator waiting on this endpoint is
// deciding whether to take the instance out of service, and a probe that hangs
// answers neither way. The deadline is the handler's, not the database's.
func TestReadyHonoursTheProbeBudget(t *testing.T) {
	db := &stubProbe{delay: time.Hour}
	h := NewHealthHandler(db, nil)
	h.dbTimeout = 20 * time.Millisecond

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.Ready(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
		done <- rec
	}()

	select {
	case rec := <-done:
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503 when the database does not answer in time", rec.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the readiness probe never returned")
	}
}

func TestReadyReusesARecentResult(t *testing.T) {
	tests := []struct {
		name      string
		ttl       time.Duration
		requests  int
		wantPings int
	}{
		{
			// /api/ready is public: without reuse, every caller buys a round
			// trip to PostgreSQL and the endpoint becomes an amplifier.
			name: "within the window, one probe serves every caller",
			ttl:  time.Minute, requests: 5, wantPings: 1,
		},
		{
			name: "with reuse disabled, every request probes",
			ttl:  0, requests: 3, wantPings: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &stubProbe{}
			h := NewHealthHandler(db, nil)
			h.ttl = tt.ttl

			for range tt.requests {
				rec := httptest.NewRecorder()
				h.Ready(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200", rec.Code)
				}
			}

			if got := db.count(); got != tt.wantPings {
				t.Errorf("the database was pinged %d times, want %d", got, tt.wantPings)
			}
		})
	}
}

// TestReadyReprobesOnceTheResultIsStale: reuse must not turn into a stale
// answer. An instance whose database died has to start failing.
func TestReadyReprobesOnceTheResultIsStale(t *testing.T) {
	db := &stubProbe{}
	h := NewHealthHandler(db, nil)
	h.ttl = time.Second

	now := time.Now()
	h.now = func() time.Time { return now }

	first := httptest.NewRecorder()
	h.Ready(first, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}

	// The database dies, and the clock moves past the reuse window.
	db.mu.Lock()
	db.err = errors.New("connection refused")
	db.mu.Unlock()
	now = now.Add(2 * time.Second)

	second := httptest.NewRecorder()
	h.Ready(second, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
	if second.Code != http.StatusServiceUnavailable {
		t.Errorf("second status = %d, want 503 once the result went stale", second.Code)
	}
	if db.count() != 2 {
		t.Errorf("the database was pinged %d times, want 2", db.count())
	}
}

// TestReadyDoesNotCacheAnAbandonedProbe: a caller that hangs up mid-probe says
// nothing about the database. Caching that failure would answer 503 to everyone
// else for the rest of the window.
func TestReadyDoesNotCacheAnAbandonedProbe(t *testing.T) {
	db := &stubProbe{delay: time.Hour}
	h := NewHealthHandler(db, nil)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Ready(httptest.NewRecorder(), req)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the readiness probe ignored the cancelled request")
	}

	h.mu.Lock()
	cached := !h.cachedAt.IsZero()
	h.mu.Unlock()
	if cached {
		t.Error("the result of an abandoned probe was cached")
	}
}

// TestReadyServesConcurrentCallersWithOneProbe: the lock is not only there for
// the cache. Two simultaneous probes on a struggling database are one more than
// it needs.
func TestReadyServesConcurrentCallersWithOneProbe(t *testing.T) {
	db := &stubProbe{delay: 20 * time.Millisecond}
	h := NewHealthHandler(db, nil)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.Ready(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
		}()
	}
	wg.Wait()

	if got := db.count(); got != 1 {
		t.Errorf("the database was pinged %d times for 8 simultaneous callers, want 1", got)
	}
}
