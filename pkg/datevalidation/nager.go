// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

var (
	errInvalidCountry = errors.New("invalid country code")
	errInvalidYear    = errors.New("invalid year")
)

// The Nager fallback provider. The endpoint is a fixed, trusted base URL with a
// format verb for the year and the ISO country code:
//
//	nagerDefaultBaseURL = "https://date.nager.at/api/v3/PublicHolidays/%d/%s"
//
// Production uses this fixed endpoint only after operator opt-in; tests use
// setNagerBaseURL to point it at an httptest server.
const (
	nagerDefaultBaseURL = "https://date.nager.at/api/v3/PublicHolidays/%d/%s"
	nagerMinYear        = 1900
	nagerMaxYear        = 2100
	nagerCacheSize      = 256
	nagerSuccessTTL     = 24 * time.Hour
	nagerFailureTTL     = time.Minute
	nagerBodyLimit      = 512 * 1024
	nagerTimeout        = 2 * time.Second
)

// countryCodeRE admits exactly an ISO 3166-1 alpha-2 country code.
var countryCodeRE = regexp.MustCompile(`^[A-Z]{2}$`)

// nagerHoliday is one element of the Nager PublicHolidays response. Only the
// date is kept; the rest of the payload (names, subnational scope, observance
// type, launch year) is deliberately ignored.
type nagerHoliday struct {
	Date string `json:"date"`
}

// nagerCacheEntry is a cached load outcome, and also what the singleflight group
// hands every caller sharing one upstream request: the error rides inside the value
// so a failure is cached the same way as a success.
type nagerCacheEntry struct {
	holidays []time.Time
	err      error
	at       time.Time
}

// nagerClient is the bounded, cached Nager fallback.
//
// Concurrency: one mutex guards the map and the FIFO eviction order. The
// singleflight group is separate and coalesces simultaneous in-flight requests
// for the same cache key, so N concurrent callers for one country+year cause
// exactly one upstream request; the rest wait on its result.
type nagerClient struct {
	baseURL string // fmt.Sprintf template; empty disables the fallback

	client *http.Client

	mu    sync.Mutex
	cache map[string]nagerCacheEntry
	order []string // cache keys in insertion order, oldest first
	sf    singleflight.Group
}

// newNagerClient builds a fallback client.
func newNagerClient(baseURL string) *nagerClient {
	return &nagerClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: nagerTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		cache: make(map[string]nagerCacheEntry),
	}
}

// defaultNager is the process-wide fallback used by IsDateAllowed. setNagerBaseURL replaces it for tests and mirrors.
var defaultNager = newNagerClient("")

// SetNagerFallbackEnabled configures the process-wide network fallback before
// serving requests. It is disabled unless the operator explicitly enables it.
// Like setNagerBaseURL, this startup setting must not change during lookups.
func SetNagerFallbackEnabled(enabled bool) (restore func()) {
	baseURL := ""
	if enabled {
		baseURL = nagerDefaultBaseURL
	}
	return setNagerBaseURL(baseURL)
}

// setNagerBaseURL points the built-in fallback provider at a different base URL
// template and returns a function that restores the previous setting.
//
// The production endpoint is the fixed, trusted date.nager.at API; this hook
// exists so tests can dial an httptest server and so a self-hosted deployment
// can pin a mirror of the same schema. An empty URL disables the fallback
// entirely (every fetch reports errHolidayUnavailable without dialing anything).
// Each URL gets its own cached client, so switching URLs starts a fresh cache.
// It must not be called concurrently with lookups.
func setNagerBaseURL(url string) (restore func()) {
	old := defaultNager
	defaultNager = newNagerClient(url)
	return func() { defaultNager = old }
}

// fetch returns a country+year from the bounded cache or, on a miss, through a
// single singleflight request coalescing every concurrent caller.
func (n *nagerClient) fetch(ctx context.Context, country string, year int) ([]time.Time, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if !countryCodeRE.MatchString(country) {
		return nil, fmt.Errorf("%w: %q", errInvalidCountry, country)
	}
	if year < nagerMinYear || year > nagerMaxYear {
		return nil, fmt.Errorf("%w: %d", errInvalidYear, year)
	}
	if n.baseURL == "" {
		return nil, fmt.Errorf("%w: no fallback endpoint configured", errHolidayUnavailable)
	}

	key := country + ":" + strconv.Itoa(year)

	if e, ok := n.cached(key); ok {
		return slices.Clone(e.holidays), e.err
	}

	// The shared request has its own bounded lifetime. Canceling any caller
	// stops only that caller's wait, not the fetch other callers rely on.
	ch := n.sf.DoChan(key, func() (any, error) {
		// Another fetch may have filled the cache between our first check
		// and joining this group; do not issue an unnecessary second request.
		if e, ok := n.cached(key); ok {
			return e, nil
		}
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nagerTimeout)
		defer cancel()
		return n.load(fetchCtx, key, country, year), nil
	})

	select {
	case res := <-ch:
		r, ok := res.Val.(nagerCacheEntry)
		if !ok {
			return nil, fmt.Errorf("unexpected singleflight result type %T", res.Val)
		}
		return slices.Clone(r.holidays), r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *nagerClient) cached(key string) (nagerCacheEntry, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	entry, ok := n.cache[key]
	return entry, ok && n.valid(entry)
}

// valid reports whether a cached entry is still within its TTL: a loaded year
// is trusted for nagerSuccessTTL, a failed one for nagerFailureTTL only.
func (n *nagerClient) valid(e nagerCacheEntry) bool {
	ttl := nagerSuccessTTL
	if e.err != nil {
		ttl = nagerFailureTTL
	}
	return time.Since(e.at) < ttl
}

// load caches the shared request's outcome, including its own timeout as a
// provider failure. Caller cancellation never reaches this context. A failure
// is never cached as a successful empty list.
func (n *nagerClient) load(ctx context.Context, key, country string, year int) nagerCacheEntry {
	holidays, err := n.dial(ctx, country, year)
	entry := nagerCacheEntry{holidays: holidays, err: err, at: time.Now()}

	n.mu.Lock()
	if _, exists := n.cache[key]; !exists {
		n.order = append(n.order, key)
	}
	n.cache[key] = entry
	for len(n.cache) > nagerCacheSize {
		oldest := n.order[0]
		n.order = n.order[1:]
		delete(n.cache, oldest)
	}
	n.mu.Unlock()

	return entry
}

// dial talks to the Nager API. Any provider, schema or HTTP error is reported
// as errHolidayUnavailable — never as "an ordinary day".
func (n *nagerClient) dial(ctx context.Context, country string, year int) ([]time.Time, error) {
	url := fmt.Sprintf(n.baseURL, year, country)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", errHolidayUnavailable, err)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errHolidayUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status %d", errHolidayUnavailable, resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, nagerBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %w", errHolidayUnavailable, err)
	}
	if len(raw) > nagerBodyLimit {
		return nil, fmt.Errorf("%w: response body exceeds %d bytes", errHolidayUnavailable, nagerBodyLimit)
	}

	var upstream []nagerHoliday
	if err := json.Unmarshal(raw, &upstream); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", errHolidayUnavailable, err)
	}

	// Ordering is by date only; subnational filtering is never guessed at.
	holidays := make([]time.Time, 0, len(upstream))
	for _, h := range upstream {
		d, err := parseNagerDate(h.Date)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errHolidayUnavailable, err)
		}
		if d.Year() != year {
			continue
		}
		holidays = append(holidays, d)
	}
	slices.SortFunc(holidays, time.Time.Compare)
	return holidays, nil
}

// parseNagerDate accepts the v3 "YYYY-MM-DD" wire format and, defensively, the
// RFC3339 timestamps earlier revisions used.
func parseNagerDate(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unrecognized holiday date %q", s)
}

// sameDate reports whether two times fall on the same calendar date.
func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
