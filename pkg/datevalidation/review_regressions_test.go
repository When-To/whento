// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type holidayRoundTripper func(*http.Request) (*http.Response, error)

func (fn holidayRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestNagerFallbackIsDisabledUntilExplicitlyEnabled(t *testing.T) {
	if defaultNager.baseURL != "" {
		t.Fatal("package default unexpectedly permits third-party holiday requests")
	}
	restore := SetNagerFallbackEnabled(true)
	if defaultNager.baseURL != nagerDefaultBaseURL {
		t.Fatal("operator opt-in did not enable the fixed Nager endpoint")
	}
	restore()
	defer SetNagerFallbackEnabled(false)()

	var hits atomic.Int64
	previous := http.DefaultTransport
	http.DefaultTransport = holidayRoundTripper(func(*http.Request) (*http.Response, error) {
		hits.Add(1)
		return nil, errors.New("unexpected outbound request")
	})
	defer func() { http.DefaultTransport = previous }()

	if _, err := isHolidayErrCtx(context.Background(), date(t, "2026-08-15"), "kr"); !errors.Is(err, errHolidayUnavailable) {
		t.Fatalf("disabled fallback = %v", err)
	}
	if !IsDateAllowed(date(t, "2026-08-15"), "Asia/Seoul", everyDay, "block", false) {
		t.Fatal("uncovered holiday must fail open to an allowed weekday")
	}
	if IsDateAllowed(date(t, "2026-08-15"), "Asia/Seoul", noDays, "allow", false) {
		t.Fatal("uncovered holiday must not grant a forbidden weekday")
	}
	if IsDateAllowed(date(t, "2026-08-14"), "Asia/Seoul", noDays, "ignore", true) {
		t.Fatal("unknown holiday eve must not grant a forbidden weekday")
	}
	if !IsHoliday(date(t, "2026-07-14"), "fr") {
		t.Fatal("disabled fallback must not weaken offline coverage or lowercase normalization")
	}
	if hits.Load() != 0 {
		t.Fatalf("disabled fallback made %d outbound requests", hits.Load())
	}
}

func TestBundledHolidayReviewMatrixWithoutNetwork(t *testing.T) {
	restore := SetNagerFallbackEnabled(false)
	defer restore()
	for _, tc := range []struct {
		zone, day string
		blocked   bool
	}{
		{"Europe/Paris", "2026-07-14", true},
		{"Europe/Paris", "2026-05-14", true},
		{"Europe/Berlin", "2026-10-03", true},
		{"Europe/Brussels", "2026-07-21", true},
		{"America/New_York", "2026-07-03", true},
		{"America/New_York", "2026-07-04", true},
		{"America/New_York", "2026-10-12", true},
		{"Europe/London", "2026-12-26", true},
		{"Europe/London", "2026-12-28", true},
		{"Europe/Berlin", "2026-06-04", false},
		{"Europe/Berlin", "2026-11-01", false},
		{"Europe/Berlin", "2026-01-06", false},
		{"Europe/London", "2026-03-17", false},
		{"Australia/Sydney", "2026-01-26", true},
		{"Australia/Sydney", "2026-04-25", true},
		{"Australia/Sydney", "2026-04-27", false}, // regional ANZAC substitute
		{"Australia/Perth", "2026-03-02", false},  // WA Labour Day
		{"Australia/Sydney", "2026-06-08", false}, // not a uniform national birthday date
		{"Asia/Tokyo", "2026-01-01", true},
		{"Asia/Tokyo", "2026-03-20", true},
		{"Asia/Tokyo", "2026-09-22", true}, // supplied exceptional national holiday
		{"Asia/Bangkok", "2026-01-01", true},
		{"Asia/Bangkok", "2026-04-13", true},
		{"Asia/Bangkok", "2026-04-14", true},
		{"Asia/Bangkok", "2026-04-15", true},
		{"Asia/Bangkok", "2026-12-07", true}, // observed Father's Day
	} {
		t.Run(tc.zone+"/"+tc.day, func(t *testing.T) {
			allowed := IsDateAllowed(date(t, tc.day), tc.zone, everyDay, "block", false)
			if allowed == tc.blocked {
				t.Fatalf("allowed = %t; expected blocked = %t", allowed, tc.blocked)
			}
		})
	}
	if n := len(offlineHolidays()); n != 47 {
		t.Fatalf("offline table has %d countries; want 47", n)
	}
	for _, country := range []string{"AU", "JP", "TH"} {
		if !isOfflineCountry(country) {
			t.Fatalf("missing offline country %s", country)
		}
	}
}

func TestJapanConcurrentYearsKeepCompiledRulesImmutable(t *testing.T) {
	type fixture struct {
		day  time.Time
		want bool
	}
	var fixtures []fixture
	// Sequential lookups are the oracle the concurrent ones must agree with.
	for year := 2024; year <= 2030; year++ {
		for day := 20; day <= 21; day++ {
			d := time.Date(year, time.March, day, 0, 0, 0, 0, time.UTC)
			want, err := offlineIsHoliday(d, "JP")
			if err != nil {
				t.Fatal(err)
			}
			fixtures = append(fixtures, fixture{d, want})
		}
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, tc := range fixtures {
				got, err := offlineIsHoliday(tc.day, "JP")
				if err != nil || got != tc.want {
					t.Errorf("JP %s = %t, %v; want %t", tc.day, got, err, tc.want)
				}
			}
		}()
	}
	wg.Wait()
	for _, h := range offlineHolidays()["JP"].Holidays {
		if h.Name == "Vernal Equinox Day" && h.Day != 0 {
			t.Fatalf("shared vernal equinox rule mutated to day %d", h.Day)
		}
	}
}

type holidayWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *holidayWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestNagerFirstCallerCancellationDoesNotAbortOtherWaiters(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			_, _ = io.WriteString(w, `[{"date":"2026-08-15","name":"Liberation Day"}]`)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer unblock()
	client := newNagerClient(server.URL + "/%d/%s")
	firstCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() {
		_, err := client.fetch(firstCtx, "KR", 2026)
		first <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("shared fetch never started")
	}
	secondCtx := &holidayWaitContext{Context: context.Background(), waiting: make(chan struct{})}
	second := make(chan nagerCacheEntry, 1)
	go func() {
		holidays, err := client.fetch(secondCtx, "KR", 2026)
		second <- nagerCacheEntry{holidays: holidays, err: err}
	}()
	select {
	case <-secondCtx.waiting:
	case <-time.After(time.Second):
		t.Fatal("second caller did not join the shared fetch")
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller = %v; want its own cancellation", err)
	}
	unblock()
	result := <-second
	if result.err != nil || len(result.holidays) != 1 {
		t.Fatalf("second caller inherited cancellation: %+v", result)
	}
	_, err := client.fetch(context.Background(), "KR", 2026)
	if err != nil || hits.Load() != 1 {
		t.Fatalf("successful shared result not cached: hits=%d err=%v", hits.Load(), err)
	}
}

func TestNagerRefusesAllRedirects(t *testing.T) {
	var foreignHits atomic.Int64
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		foreignHits.Add(1)
		_, _ = io.WriteString(w, "[]")
	}))
	defer foreign.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, foreign.URL, status)
			}))
			defer server.Close()
			client := newNagerClient(server.URL + "/%d/%s")
			_, err := client.fetch(context.Background(), "KR", 2026)
			if !errors.Is(err, errHolidayUnavailable) || !strings.Contains(err.Error(), "unexpected status") {
				t.Fatalf("redirect = %v; want refused redirect response", err)
			}
		})
	}
	if foreignHits.Load() != 0 {
		t.Fatalf("redirect target received %d requests", foreignHits.Load())
	}
}

func TestNagerTransportAndDecodeErrorsKeepTheirCauses(t *testing.T) {
	client := newNagerClient("https://example.invalid/%d/%s")
	cause := errors.New("transport test failure")
	client.client.Transport = holidayRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, cause
	})
	_, err := client.fetch(context.Background(), "KR", 2026)
	var transport *url.Error
	if !errors.Is(err, cause) || !errors.Is(err, errHolidayUnavailable) || !errors.As(err, &transport) {
		t.Fatalf("transport cause lost: %v", err)
	}
	client = newNagerClient("https://example.invalid/%d/%s")
	client.client.Transport = holidayRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("not json"))}, nil
	})
	_, err = client.fetch(context.Background(), "KR", 2026)
	var syntax *json.SyntaxError
	if !errors.Is(err, errHolidayUnavailable) || !errors.As(err, &syntax) {
		t.Fatalf("decode cause lost: %v", err)
	}
}

func TestNagerOwnTimeoutIsBoundedAndFailureCached(t *testing.T) {
	client := newNagerClient("https://example.invalid/%d/%s")
	client.client.Timeout = 10 * time.Millisecond
	var hits atomic.Int64
	client.client.Transport = holidayRoundTripper(func(r *http.Request) (*http.Response, error) {
		hits.Add(1)
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > nagerTimeout {
			t.Error("shared fetch does not have its own bounded deadline")
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	_, err := client.fetch(context.Background(), "KR", 2026)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, errHolidayUnavailable) {
		t.Fatalf("timeout = %v", err)
	}
	_, err = client.fetch(context.Background(), "KR", 2026)
	if err == nil || hits.Load() != 1 {
		t.Fatalf("provider timeout not cooled down: hits=%d err=%v", hits.Load(), err)
	}
}
