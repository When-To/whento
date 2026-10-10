// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

// Package holidays serves the canonical public-holiday dataset over HTTP.
//
// The endpoints are independent of the frontend's own date-holidays renderer:
// they expose the same pkg/datevalidation providers the backend block/allow
// policy runs on — the bundled offline table first, the Nager API as a bounded,
// cached fallback for countries it does not cover. "Supported" always means the
// offline dataset is available for the country, never just that the country
// exists in a timezone map.
package holidays

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/whento/pkg/datevalidation"
	"github.com/whento/pkg/httputil"
)

// Holiday is one public holiday, as served to clients.
type Holiday struct {
	Date      string `json:"date"` // ISO 8601 (YYYY-MM-DD)
	Name      string `json:"name"`
	LocalName string `json:"local_name,omitempty"` // absent when it equals Name
	Source    string `json:"source"`               // "offline" or "fallback"
}

// YearResponse is the response of GET /api/v1/holidays.
type YearResponse struct {
	CountryCode string    `json:"country_code"` // ISO 3166-1 alpha-2, or ""
	Year        int       `json:"year"`
	Source      string    `json:"source"`    // "offline", "fallback" or "unavailable"
	Supported   bool      `json:"supported"` // true only when the offline dataset covers the country
	Holidays    []Holiday `json:"holidays"`
}

// SupportedResponse is the response of GET /api/v1/holidays/supported.
type SupportedResponse struct {
	Countries []string `json:"countries"`
}

var countryCodeRE = regexp.MustCompile(`^[A-Z]{2}$`)

// Handler serves the holidays endpoints. Stateless: the dataset is compiled in
// and shared, so the handler is cheap to construct per request.
type Handler struct{}

// NewHandler creates the holidays handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Year handles GET /api/v1/holidays?country=FR&year=2026, or with timezone in
// place of country (/api/v1/holidays?timezone=Europe%2FParis&year=2026).
//
// The timezone is resolved to a country server-side, so clients need no
// timezone→country table of their own. The country query parameter takes
// precedence when both are given. Holidays for the offline table always come
// from the local dataset; for a country it does not cover the Nager fallback is
// consulted, and the source field reports which provider answered. A country no
// provider can serve is an honest empty answer with source "unavailable", not
// an error.
//
//	@Summary		Public holidays for a country or timezone
//	@Description	Serves the public-holiday dataset for a country, for one year. Country wins over timezone when both are given.
//	@Tags			Holidays
//	@Produce		json
//	@Param			country		query		string	false	"ISO 3166-1 alpha-2 country code (e.g. FR)"
//	@Param			timezone	query		string	false	"IANA timezone (e.g. Europe/Paris)"
//	@Param			year		query		int		false	"Year (defaults to the current one)"
//	@Success		200			{object}	YearResponse
//	@Failure		400			{object}	httputil.ErrorResponse
//	@Router			/api/v1/holidays [get]
func (h *Handler) Year(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	year := currentYear()
	if raw := query.Get("year"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 9999 {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, "Invalid year")
			return
		}
		year = parsed
	}

	countryCode := ""
	if country := query.Get("country"); country != "" {
		if !countryCodeRE.MatchString(country) {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, "Invalid country code")
			return
		}
		countryCode = country
	} else if timezone := query.Get("timezone"); timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeValidation, "Invalid timezone")
			return
		}
		countryCode = datevalidation.GetCountryFromTimezone(timezone)
	}

	response := YearResponse{
		CountryCode: countryCode,
		Year:        year,
		Source:      "unavailable",
		Supported:   false,
		Holidays:    []Holiday{},
	}
	if countryCode == "" {
		httputil.JSON(w, http.StatusOK, response)
		return
	}

	holidays, coverage, err := datevalidation.HolidaysForYear(r.Context(), countryCode, year)
	if err != nil {
		// No provider could serve the country: an honest empty answer with the
		// source reporting the gap, rather than a refusal that would break a
		// client's calendar. The block policy still decides per-date server-side.
		httputil.JSON(w, http.StatusOK, response)
		return
	}

	response.Source = coverage.String()
	response.Supported = isSupported(countryCode)
	for _, holiday := range holidays {
		entry := Holiday{
			Date:   holiday.Date.Format("2006-01-02"),
			Name:   holiday.Name,
			Source: holiday.Source,
		}
		if holiday.LocalName != "" && holiday.LocalName != entry.Name {
			entry.LocalName = holiday.LocalName
		}
		response.Holidays = append(response.Holidays, entry)
	}
	httputil.JSON(w, http.StatusOK, response)
}

// Supported handles GET /api/v1/holidays/supported.
//
//	@Summary		Countries the offline dataset covers
//	@Description	Lists the ISO codes the bundled offline holiday table has national data for.
//	@Tags			Holidays
//	@Produce		json
//	@Success		200	{object}	SupportedResponse
//	@Router			/api/v1/holidays/supported [get]
func (h *Handler) Supported(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, SupportedResponse{Countries: datevalidation.SupportedCountries()})
}

// isSupported reports whether a country's holidays were actually available from
// the bundled offline dataset (as opposed to the network fallback).
func isSupported(countryCode string) bool {
	for _, code := range datevalidation.SupportedCountries() {
		if code == countryCode {
			return true
		}
	}
	return false
}

// currentYear is a variable so tests can pin the default year.
var currentYear = func() int { return time.Now().Year() }
