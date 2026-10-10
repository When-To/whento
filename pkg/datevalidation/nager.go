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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Coverage describes where a year's holiday list came from: the bundled
// offline table, the Nager network fallback, or nowhere at all.
type Coverage int

const (
	// CoverageOffline means the bundled rickar/cal table served the list.
	CoverageOffline Coverage = iota
	// CoverageFallback means the Nager API served the list.
	CoverageFallback
	// CoverageUnavailable means no provider could serve the list.
	CoverageUnavailable
)

// String returns a stable identifier for the coverage ("offline", "fallback" or
// "unavailable") suitable for an API envelope.
func (c Coverage) String() string {
	switch c {
	case CoverageOffline:
		return "offline"
	case CoverageFallback:
		return "fallback"
	case CoverageUnavailable:
		return "unavailable"
	default:
		return "unknown"
	}
}

// Holiday is one public holiday as served by HolidaysForYear.
type Holiday struct {
	Date      time.Time // the holiday's calendar date
	Name      string    // the holiday name (local language for the offline source, English for the fallback)
	LocalName string    // the local-language name (same as Name for the offline source)
	Source    string    // "offline" or "fallback"
}

var (
	errInvalidCountry = errors.New("invalid country code")
	errInvalidYear    = errors.New("invalid year")
)

// The Nager fallback provider. The endpoint is a fixed, trusted base URL with a
// format verb for the year and the ISO country code:
//
//	nagerDefaultBaseURL = "https://date.nager.at/api/v3/PublicHolidays/%d/%s"
//
// Self-hosted deployments may pin a mirror via SetNagerBaseURL; tests point it
// at an httptest server.
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
// three fields below are kept; the rest of the payload (subnational scope,
// observance type, launch year) is deliberately ignored.
type nagerHoliday struct {
	Date      string `json:"date"`
	Name      string `json:"name"`
	LocalName string `json:"localName"`
}

type nagerCacheEntry struct {
	coverage Coverage
	holidays []Holiday
	err      error
	at       time.Time
}

// nagerResult is what the singleflight group hands back to every caller sharing
// one upstream request. The error rides inside the value rather than in
// singleflight's error slot so the load outcome is cached uniformly whether it
// succeeded or failed.
type nagerResult struct {
	holidays []Holiday
	coverage Coverage
	err      error
}

// nagerClient is the bounded, cached Nager fallback.
//
// Concurrency: one mutex guards the map and the FIFO eviction order. The
// singleflight group is separate and coalesces simultaneous in-flight requests
// for the same cache key, so N concurrent callers for one country+year cause
// exactly one upstream request; the rest wait on its result.
type nagerClient struct {
	baseURL string // fmt.Sprintf template; empty disables the fallback

	client     *http.Client
	successTTL time.Duration // how long a loaded year is trusted
	failureTTL time.Duration // cooldown before a failed year is retried

	mu    sync.Mutex
	cache map[string]nagerCacheEntry
	order []string // cache keys in insertion order, oldest first
	sf    singleflight.Group
}

// newNagerClient builds a fallback client. A successTTL/failureTTL of zero uses
// the package defaults; tests shrink them to exercise expiry.
func newNagerClient(baseURL string) *nagerClient {
	return &nagerClient{
		baseURL:    baseURL,
		client:     &http.Client{Timeout: nagerTimeout},
		successTTL: nagerSuccessTTL,
		failureTTL: nagerFailureTTL,
		cache:      make(map[string]nagerCacheEntry),
	}
}

// defaultNager is the process-wide fallback used by IsDateAllowed and
// HolidaysForYear. SetNagerBaseURL replaces it for tests and mirrors.
var defaultNager = newNagerClient(nagerDefaultBaseURL)

// SetNagerBaseURL points the built-in fallback provider at a different base URL
// template and returns a function that restores the previous setting.
//
// The production endpoint is the fixed, trusted date.nager.at API; this hook
// exists so tests can dial an httptest server and so a self-hosted deployment
// can pin a mirror of the same schema. An empty URL disables the fallback
// entirely (every fetch reports CoverageUnavailable without dialing anything).
// Each URL gets its own cached client, so switching URLs starts a fresh cache.
// It must not be called concurrently with lookups.
func SetNagerBaseURL(url string) (restore func()) {
	old := defaultNager
	defaultNager = newNagerClient(url)
	return func() { defaultNager = old }
}

// HolidaysForYear returns the public holidays of a country for a year. The
// bundled offline table answers first when it covers the country; otherwise the
// Nager fallback is consulted through its bounded cache. A country no provider
// can serve reports CoverageUnavailable with a non-nil error.
func HolidaysForYear(ctx context.Context, country string, year int) ([]Holiday, Coverage, error) {
	cc := strings.ToUpper(strings.TrimSpace(country))
	if !countryCodeRE.MatchString(cc) {
		return nil, CoverageUnavailable, fmt.Errorf("%w: %q", errInvalidCountry, country)
	}
	if holidays, ok := offlineHolidaysForYear(cc, year); ok {
		return holidays, CoverageOffline, nil
	}
	return defaultNager.fetch(ctx, cc, year)
}

// holidaysForYear is the internal, unvalidated form used by isHolidayErrCtx.
func holidaysForYear(ctx context.Context, country string, year int) ([]Holiday, Coverage, error) {
	if holidays, ok := offlineHolidaysForYear(country, year); ok {
		return holidays, CoverageOffline, nil
	}
	return defaultNager.fetch(ctx, country, year)
}

// fetch returns a country+year from the bounded cache or, on a miss, through a
// single singleflight request coalescing every concurrent caller.
func (n *nagerClient) fetch(ctx context.Context, country string, year int) ([]Holiday, Coverage, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if !countryCodeRE.MatchString(country) {
		return nil, CoverageUnavailable, fmt.Errorf("%w: %q", errInvalidCountry, country)
	}
	if year < nagerMinYear || year > nagerMaxYear {
		return nil, CoverageUnavailable, fmt.Errorf("%w: %d", errInvalidYear, year)
	}
	if n.baseURL == "" {
		return nil, CoverageUnavailable, fmt.Errorf("%w: no fallback endpoint configured", errHolidayUnavailable)
	}

	key := country + ":" + strconv.Itoa(year)

	n.mu.Lock()
	if e, ok := n.cache[key]; ok && n.valid(e) {
		n.mu.Unlock()
		return cloneHolidays(e.holidays), e.coverage, e.err
	}
	n.mu.Unlock()

	// One in-flight request per key: the first caller's context drives the
	// upstream request; waiters share its outcome.
	ch := n.sf.DoChan(key, func() (any, error) {
		holidays, coverage, err := n.load(ctx, country, year)
		return nagerResult{holidays: holidays, coverage: coverage, err: err}, nil
	})

	select {
	case res := <-ch:
		r, ok := res.Val.(nagerResult)
		if !ok {
			return nil, CoverageUnavailable, fmt.Errorf("unexpected singleflight result type %T", res.Val)
		}
		return cloneHolidays(r.holidays), r.coverage, r.err
	case <-ctx.Done():
		return nil, CoverageUnavailable, ctx.Err()
	}
}

// valid reports whether a cached entry is still within its TTL: a loaded year
// is trusted for successTTL, a failed one for failureTTL only.
func (n *nagerClient) valid(e nagerCacheEntry) bool {
	ttl := n.successTTL
	if e.coverage == CoverageUnavailable {
		ttl = n.failureTTL
	}
	return time.Since(e.at) < ttl
}

// load performs the upstream request and caches the outcome. An abort — the
// context gone while the request is in flight — is not evidence about the
// provider, so it is returned to the caller but never cached. In particular a
// failure is never cached as a successful empty list.
func (n *nagerClient) load(ctx context.Context, country string, year int) ([]Holiday, Coverage, error) {
	holidays, coverage, err := n.dial(ctx, country, year)
	if ctx.Err() != nil {
		return holidays, coverage, err
	}
	key := country + ":" + strconv.Itoa(year)

	n.mu.Lock()
	if _, exists := n.cache[key]; !exists {
		n.order = append(n.order, key)
	}
	n.cache[key] = nagerCacheEntry{coverage: coverage, holidays: holidays, err: err, at: time.Now()}
	for len(n.cache) > nagerCacheSize {
		oldest := n.order[0]
		n.order = n.order[1:]
		delete(n.cache, oldest)
	}
	n.mu.Unlock()

	return holidays, coverage, err
}

// dial talks to the Nager API. Any provider, schema or HTTP error is reported
// as CoverageUnavailable — never as "an ordinary day".
func (n *nagerClient) dial(ctx context.Context, country string, year int) ([]Holiday, Coverage, error) {
	url := fmt.Sprintf(n.baseURL, year, country)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, CoverageUnavailable, fmt.Errorf("%w: build request: %v", errHolidayUnavailable, err)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, CoverageUnavailable, fmt.Errorf("%w: %v", errHolidayUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, CoverageUnavailable, fmt.Errorf("%w: unexpected status %d", errHolidayUnavailable, resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, nagerBodyLimit+1))
	if err != nil {
		return nil, CoverageUnavailable, fmt.Errorf("%w: read body: %v", errHolidayUnavailable, err)
	}
	if len(raw) > nagerBodyLimit {
		return nil, CoverageUnavailable, fmt.Errorf("%w: response body exceeds %d bytes", errHolidayUnavailable, nagerBodyLimit)
	}

	var upstream []nagerHoliday
	if err := json.Unmarshal(raw, &upstream); err != nil {
		return nil, CoverageUnavailable, fmt.Errorf("%w: decode: %v", errHolidayUnavailable, err)
	}

	// Ordering is by date only; subnational filtering is never guessed at.
	holidays := make([]Holiday, 0, len(upstream))
	for _, h := range upstream {
		d, err := parseNagerDate(h.Date)
		if err != nil {
			return nil, CoverageUnavailable, fmt.Errorf("%w: %v", errHolidayUnavailable, err)
		}
		if d.Year() != year {
			continue
		}
		holidays = append(holidays, Holiday{Date: d, Name: h.Name, LocalName: h.LocalName, Source: "fallback"})
	}
	sort.Slice(holidays, func(i, j int) bool {
		return holidays[i].Date.Before(holidays[j].Date)
	})
	return holidays, CoverageFallback, nil
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

// cloneHolidays returns a defensive copy so a caller mutating the slice it got
// cannot corrupt the cache or the offline table.
func cloneHolidays(in []Holiday) []Holiday {
	if in == nil {
		return nil
	}
	out := make([]Holiday, len(in))
	copy(out, in)
	return out
}

// sameDate reports whether two times fall on the same calendar date.
func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
