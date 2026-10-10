// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package holidays_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/whento/pkg/datevalidation"
	"github.com/whento/whento/internal/holidays"
)

func get(t *testing.T, target string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	holidays.NewHandler().Year(w, req)
	return w.Code, w.Body.String()
}

// envelope is the {success, data, error} shape every WhenTo JSON response uses.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

func decodeYearResponse(t *testing.T, body string) holidays.YearResponse {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", body, err)
	}
	var resp holidays.YearResponse
	if err := json.Unmarshal(env.Data, &resp); err != nil {
		t.Fatalf("decode data %q: %v", string(env.Data), err)
	}
	return resp
}

func statusOf(t *testing.T, body string) bool {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("decode envelope %q: %v", body, err)
	}
	return env.Success
}

// TestYearServesTheOfflineDataset pins the endpoint's contract against the
// fixtures the backend policy tests use: Bastille Day is French, Christmas is
// shared, and the offline source is reported.
func TestYearServesTheOfflineDataset(t *testing.T) {
	status, body := get(t, "/api/v1/holidays?country=FR&year=2026")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if !resp.Supported || resp.CountryCode != "FR" || resp.Source != "offline" {
		t.Errorf("France should read as offline-supported: %+v", resp)
	}

	byDate := map[string]holidays.Holiday{}
	for _, h := range resp.Holidays {
		byDate[h.Date] = h
	}
	for _, fixture := range []string{"2026-07-14", "2026-12-25"} {
		if _, ok := byDate[fixture]; !ok {
			t.Errorf("the dataset misses fixture %s: %+v", fixture, resp.Holidays)
		}
	}
	if h := byDate["2026-07-14"]; h.Source != "offline" {
		t.Errorf("offline entries must carry source=offline: %+v", h)
	}
}

func TestYearDefaultsToTheCurrentYear(t *testing.T) {
	status, body := get(t, "/api/v1/holidays?country=FR")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if !resp.Supported {
		t.Errorf("France should be covered: %+v", resp)
	}
	if resp.Year == 0 {
		t.Error("year is unset")
	}
}

func TestYearByTimezone(t *testing.T) {
	status, body := get(t, "/api/v1/holidays?timezone=Europe%2FParis&year=2026")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if resp.CountryCode != "FR" || resp.Source != "offline" {
		t.Errorf("Europe/Paris should resolve to offline FR: %+v", resp)
	}
}

func TestYearRejectsAnInvalidYear(t *testing.T) {
	for _, raw := range []string{"not-a-year", "0", "10000", "-5"} {
		status, _ := get(t, "/api/v1/holidays?country=FR&year="+raw)
		if status != http.StatusBadRequest {
			t.Errorf("year=%q: status = %d, want 400", raw, status)
		}
	}
}

func TestYearRejectsAnInvalidCountry(t *testing.T) {
	for _, raw := range []string{"fr", "FRA", "F1", "f"} {
		status, _ := get(t, "/api/v1/holidays?country="+raw+"&year=2026")
		if status != http.StatusBadRequest {
			t.Errorf("country=%q: status = %d, want 400", raw, status)
		}
	}
}

func TestYearRejectsAnInvalidTimezone(t *testing.T) {
	status, _ := get(t, "/api/v1/holidays?timezone=Mars%2FOlympus_Mons&year=2026")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestYearHandlesAValidTimezoneWithNoCountry(t *testing.T) {
	status, body := get(t, "/api/v1/holidays?timezone=UTC&year=2026")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if resp.Supported || resp.CountryCode != "" || len(resp.Holidays) != 0 || resp.Source != "unavailable" {
		t.Errorf("UTC must read as unsupported and empty: %+v", resp)
	}
}

// TestYearReportsTheFallbackSource drives an uncovered country (Japan) through
// the network fallback, served by a local httptest server so the test never
// touches the public Nager API. The fallback list satisfies the same envelope,
// but supported=false: "supported" means the offline dataset covers the country.
func TestYearReportsTheFallbackSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"date":"2026-01-01","localName":"元日","name":"New Year's Day"},
			{"date":"2026-01-12","localName":"成人の日","name":"Coming of Age Day"}
		]`))
	}))
	defer server.Close()

	restore := datevalidation.SetNagerBaseURL(server.URL + "/api/v3/PublicHolidays/%d/%s")
	defer restore()

	status, body := get(t, "/api/v1/holidays?country=JP&year=2026")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if resp.Supported || resp.Source != "fallback" || resp.CountryCode != "JP" {
		t.Errorf("JP must read as fallback-supported=false: %+v", resp)
	}
	if len(resp.Holidays) != 2 {
		t.Fatalf("expected 2 fallback holidays, got %d: %+v", len(resp.Holidays), resp.Holidays)
	}
	if resp.Holidays[0].Date != "2026-01-01" || resp.Holidays[0].Source != "fallback" {
		t.Errorf("fallback entry malformed: %+v", resp.Holidays[0])
	}
	if resp.Holidays[0].LocalName != "元日" {
		t.Errorf("local name not carried through: %+v", resp.Holidays[0])
	}
}

// TestYearReportsUnavailableWhenNoProviderServes sharpens the honest-empty
// answer: with the fallback disabled, an uncovered country reads source
// "unavailable" rather than pretending the year has no holidays.
func TestYearReportsUnavailableWhenNoProviderServes(t *testing.T) {
	restore := datevalidation.SetNagerBaseURL("")
	defer restore()

	status, body := get(t, "/api/v1/holidays?country=JP&year=2026")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if resp.Supported || resp.Source != "unavailable" || len(resp.Holidays) != 0 {
		t.Errorf("JP with no fallback must read unavailable: %+v", resp)
	}
}

func TestSupportedListsTheOfflineTable(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/holidays/supported", nil)
	w := httptest.NewRecorder()
	holidays.NewHandler().Supported(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var env envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var resp struct {
		Countries []string `json:"countries"`
	}
	if err := json.Unmarshal(env.Data, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Countries) == 0 {
		t.Fatal("supported list is empty")
	}
	// FR and US, the two policy-fixture countries, must be advertised.
	saw := map[string]bool{}
	for _, c := range resp.Countries {
		saw[c] = true
	}
	if !saw["FR"] || !saw["US"] {
		t.Errorf("FR/US missing from supported list: %v", resp.Countries)
	}
	// supported means "offline dataset available", and Japan ships no national
	// offline dataset, so it must not be advertised.
	if saw["JP"] {
		t.Errorf("JP must not be in the offline-supported list: %v", resp.Countries)
	}
}

func TestEnvelopeWrapsSuccessfully(t *testing.T) {
	status, body := get(t, "/api/v1/holidays?country=FR&year=2026")
	if status != http.StatusOK || !statusOf(t, body) {
		t.Errorf("a served year must be a success envelope: status=%d body=%s", status, body)
	}

	status, body = get(t, "/api/v1/holidays?country=fr&year=2026")
	if status != http.StatusBadRequest || statusOf(t, body) {
		t.Errorf("a validation error must be a failure envelope: status=%d body=%s", status, body)
	}
}
