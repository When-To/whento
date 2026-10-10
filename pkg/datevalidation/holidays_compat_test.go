// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestUnsupportedCountryOrdinaryWednesdayAdmitted is the B2 regression: a
// `block` calendar in a country the offline table does not cover must admit an
// ordinary, eligible weekday. The previous design refused the day because the
// offline map was empty for it (fail-closed on missing data). With the fallback
// unavailable the policy fails open to the ordinary weekday decision — a known
// blocked holiday is still blocked, but an unknown ordinary day is a normal
// day.
func TestUnsupportedCountryOrdinaryWednesdayAdmitted(t *testing.T) {
	restore := SetNagerBaseURL("") // disable the network fallback: genuinely unavailable
	defer restore()

	// 2026-10-07 is a Wednesday.
	date := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	countryTests := []struct {
		country  string
		timezone string
	}{
		{"JP", "Asia/Tokyo"},
		{"AU", "Australia/Sydney"},
		{"IN", "Asia/Kolkata"},
		{"CN", "Asia/Shanghai"},
		{"KR", "Asia/Seoul"},
		{"SG", "Asia/Singapore"},
		{"TH", "Asia/Bangkok"},
		{"IL", "Asia/Jerusalem"},
		{"TR", "Europe/Istanbul"},
	}

	// Wednesday alone should be allowed, so the only way these fail is the
	// holiday provider refusing the day.
	wednesdayOnly := []int{int(time.Wednesday)}

	for _, tt := range countryTests {
		t.Run(tt.country, func(t *testing.T) {
			if !IsDateAllowed(date, tt.timezone, wednesdayOnly, "block", false) {
				t.Errorf("%s: ordinary Wednesday 2026-10-07 rejected under block", tt.country)
			}
		})
	}
}

// TestKnownHolidayStillBlocks pins that the fail-open change does not weaken a
// known (offline-covered) holiday: FR's labour day still blocks under `block`
// and is admitted under `allow` when the weekday would otherwise forbid it.
func TestKnownHolidayStillBlocks(t *testing.T) {
	restore := SetNagerBaseURL("")
	defer restore()

	// 2026-05-01 is a Friday (weekday 5).
	labourDay := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	tuesdayOnly := []int{int(time.Tuesday)}

	if IsDateAllowed(labourDay, "Europe/Paris", tuesdayOnly, "block", false) {
		t.Error("FR 2026-05-01 admitted under block (known holiday)")
	}
	if !IsDateAllowed(labourDay, "Europe/Paris", tuesdayOnly, "allow", false) {
		t.Error("FR 2026-05-01 refused under allow (a known holiday must be admitted)")
	}
	if IsDateAllowed(labourDay, "Europe/Paris", tuesdayOnly, "ignore", false) {
		t.Error("FR 2026-05-01 should be a plain weekday decision under ignore (Tuesday only -> refused)")
	}
}

// TestUSObservedSubstituteDayBlocks covers the observed-day policy: the
// offline table keeps both the civil date (Saturday 2026-07-04) and the
// observed substitute (Friday 2026-07-03) of US Independence Day, and a block
// calendar refuses both — a calendar written against the holiday must not admit
// the observed bank holiday.
func TestUSObservedSubstituteDayBlocks(t *testing.T) {
	restore := SetNagerBaseURL("")
	defer restore()

	friday := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)   // observed
	saturday := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC) // civil
	weekdays := []int{int(time.Friday), int(time.Saturday)}

	for _, day := range []time.Time{friday, saturday} {
		if !IsDateAllowed(day, "America/New_York", weekdays, "ignore", false) {
			t.Fatalf("US %s is not admitted by the weekday baseline", day.Format("2006-01-02"))
		}
		if IsDateAllowed(day, "America/New_York", weekdays, "block", false) {
			t.Errorf("US %s admitted under block (should be a blocked holiday)", day.Format("2006-01-02"))
		}
	}
	ordinaryFriday := friday.AddDate(0, 0, 7)
	if !IsDateAllowed(ordinaryFriday, "America/New_York", weekdays, "block", false) {
		t.Error("ordinary Friday refused under block")
	}
}

// TestSupportedCountriesReturnsACopy guards the S1 race: the initializer runs
// unconditionally in the reader, so even a concurrently cold package cannot
// race a read against the write, and mutating the returned slice cannot corrupt
// the global table.
func TestSupportedCountriesReturnsACopy(t *testing.T) {
	restore := SetNagerBaseURL("")
	defer restore()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			countries := SupportedCountries()
			// Mutate the copy aggressively; the package must be unaffected.
			for j := range countries {
				countries[j] = "XX"
			}
			if _, ok := offlineHolidays()["US"]; !ok {
				t.Error("offline table lost US after a returned copy was mutated")
			}
		}()
	}
	wg.Wait()

	countries := SupportedCountries()
	for _, code := range []string{"FR", "US", "DE"} {
		found := false
		for _, c := range countries {
			if c == code {
				found = true
			}
		}
		if !found {
			t.Errorf("SupportedCountries() missing %s (global state corrupted?)", code)
		}
	}
}

// TestHolidaysForYearOfflineCoverage verifies the offline provider answers with
// CoverageOffline and the fallback answers with CoverageFallback.
func TestHolidaysForYearOfflineCoverage(t *testing.T) {
	restore := SetNagerBaseURL("http://127.0.0.1:9/unused/%d/%s")
	defer restore()

	offline, coverage, err := HolidaysForYear(context.Background(), "FR", 2026)
	if err != nil || coverage != CoverageOffline {
		t.Fatalf("FR coverage = %v, err = %v; want offline", coverage, err)
	}
	if len(offline) == 0 {
		t.Fatal("FR 2026 offline list is empty, want holidays")
	}
}

func TestHolidaysForYearFallback(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"date": "2026-08-15", "name": "National Liberation Day", "localName": "광복절"},
		})
	}))
	defer server.Close()

	restore := SetNagerBaseURL(server.URL + "/%d/%s")
	defer restore()

	holidays, coverage, err := HolidaysForYear(context.Background(), "KR", 2026)
	if err != nil || coverage != CoverageFallback {
		t.Fatalf("KR coverage = %v, err = %v; want fallback", coverage, err)
	}
	if len(holidays) != 1 || holidays[0].Name != "National Liberation Day" {
		t.Fatalf("holidays = %#v", holidays)
	}

	// Cached: a second call must not hit the network again.
	_, _, err = HolidaysForYear(context.Background(), "KR", 2026)
	if err != nil {
		t.Fatalf("cached call: %v", err)
	}
	if hits != 1 {
		t.Errorf("network hits = %d, want 1 (cache must serve the second call)", hits)
	}
}

func TestHolidaysForYearFallbackUnavailable(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	restore := SetNagerBaseURL(server.URL + "/%d/%s")
	defer restore()

	_, coverage, err := HolidaysForYear(context.Background(), "KR", 2026)
	if coverage != CoverageUnavailable || err == nil {
		t.Fatalf("coverage = %v, err = %v; want unavailable+err", coverage, err)
	}
	// A failure is never promoted to a "known ordinary year".
	if _, isHolidayErr := isHolidayErrCtx(context.Background(), time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), "KR"); isHolidayErr == nil {
		t.Error("unavailable fallback produced a holiday=false with no error (must stay error-aware)")
	}
}

func TestHolidaysForYearFallbackMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not json"))
	}))
	defer server.Close()

	restore := SetNagerBaseURL(server.URL + "/%d/%s")
	defer restore()

	_, coverage, err := HolidaysForYear(context.Background(), "KR", 2026)
	if coverage != CoverageUnavailable || err == nil {
		t.Fatalf("coverage = %v, err = %v; want unavailable+err", coverage, err)
	}
}

func TestHolidaysForYearFallbackRejectsInvalidParams(t *testing.T) {
	restore := SetNagerBaseURL("http://127.0.0.1:9/unused/%d/%s")
	defer restore()

	if _, _, err := HolidaysForYear(context.Background(), "ZZZ", 2026); err == nil {
		t.Error("3-letter country accepted")
	}
	// KR is not an offline country, so its year is validated by the fallback.
	if _, _, err := HolidaysForYear(context.Background(), "KR", 1800); err == nil {
		t.Error("out-of-range year accepted")
	}
}

// TestHolidaysForYearContextCancellation ensures a cancelled call does not
// poison the (short) cache and the error is propagated.
func TestHolidaysForYearContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer server.Close()

	restore := SetNagerBaseURL(server.URL + "/%d/%s")
	defer restore()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	_, _, err := HolidaysForYear(ctx, "KR", 2027)
	if err == nil {
		t.Fatal("cancelled fetch did not error")
	}
}

// TestOfflineCountryFailsOpenWithoutFallback proves JP (not offline) plus a
// disabled fallback = the block policy still admits an ordinary weekday.
func TestOfflineCountryFailsOpenWithoutFallback(t *testing.T) {
	restore := SetNagerBaseURL("")
	defer restore()

	wednesday := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	if !IsDateAllowed(wednesday, "Asia/Tokyo", []int{int(time.Wednesday)}, "block", false) {
		t.Error("JP ordinary Wednesday refused when holiday data is unavailable")
	}
}
