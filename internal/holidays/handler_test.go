// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package holidays_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

// TestYearServesTheOfflineDataset pins the endpoint's contract against the
// fixtures the backend policy tests use: Bastille Day is French, Christmas is
// shared, and an uncovered timezone is an honest empty answer.
func TestYearServesTheOfflineDataset(t *testing.T) {
	status, body := get(t, "/api/v1/holidays?timezone=Europe%2FParis&year=2026")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if !resp.Supported || resp.CountryCode != "FR" {
		t.Errorf("France should be supported: %+v", resp)
	}

	var sawBastille, sawChristmas bool
	for _, h := range resp.Holidays {
		switch h.Date {
		case "2026-07-14":
			sawBastille = true
		case "2026-12-25":
			sawChristmas = true
		}
	}
	if !sawBastille || !sawChristmas {
		t.Errorf("the dataset misses fixtures: Bastille=%v Christmas=%v", sawBastille, sawChristmas)
	}
}

func TestYearDefaultsToTheCurrentYear(t *testing.T) {
	status, body := get(t, "/api/v1/holidays?timezone=Europe%2FParis")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if !resp.Supported {
		t.Errorf("Paris should be covered: %+v", resp)
	}
}

func TestYearRejectsABadYear(t *testing.T) {
	status, _ := get(t, "/api/v1/holidays?timezone=Europe%2FParis&year=not-a-year")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestYearHandlesAnUncoveredTimezone(t *testing.T) {
	status, body := get(t, "/api/v1/holidays?timezone=UTC&year=2026")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	resp := decodeYearResponse(t, body)
	if resp.Supported || resp.CountryCode != "" || len(resp.Holidays) != 0 {
		t.Errorf("UTC must read as unsupported and empty: %+v", resp)
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
	// FR and US, the two fixture countries, must be advertised.
	saw := map[string]bool{}
	for _, c := range resp.Countries {
		saw[c] = true
	}
	if !saw["FR"] || !saw["US"] {
		t.Errorf("FR/US missing from supported list: %v", resp.Countries)
	}
}
