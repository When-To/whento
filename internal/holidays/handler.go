// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

// Package holidays serves the canonical, offline public-holiday dataset.
//
// The frontend used to ship a 1.4 MB holiday library and its own timezone→country
// table. Both duplicated (and could disagree with) the backend's block/allow
// policy, so the data now lives in exactly one place — pkg/datevalidation's
// offline table — and this handler exposes it. The frontend fetches the same
// dates the policy enforces, which makes "the shading says holiday but the
// backend admitted/refused it" impossible by construction.
package holidays

import (
	"net/http"
	"strconv"
	"time"

	"github.com/whento/pkg/datevalidation"
	"github.com/whento/pkg/httputil"
)

// Holiday is one public holiday, as served to the frontend.
type Holiday struct {
	Date string `json:"date"` // ISO 8601 (YYYY-MM-DD)
	Name string `json:"name"`
}

// YearResponse is the response of GET /api/v1/holidays.
type YearResponse struct {
	CountryCode string    `json:"country_code"` // ISO 3166-1 alpha-2, or ""
	Supported   bool      `json:"supported"`    // false when the offline table covers no data for this timezone
	Holidays    []Holiday `json:"holidays"`
}

// SupportedResponse is the response of GET /api/v1/holidays/supported.
type SupportedResponse struct {
	Countries []string `json:"countries"`
}

// Handler serves the holidays endpoints. Stateless: the dataset is compiled in
// and shared, so the handler is cheap to construct per request.
type Handler struct{}

// NewHandler creates the holidays handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Year handles GET /api/v1/holidays?timezone=Europe%2FParis&year=2026.
//
// The timezone is resolved to a country server-side, so the frontend needs no
// timezone→country table of its own. An unknown timezone yields an empty
// response with supported=false; a covered country yields its holidays for the
// requested year (defaulting to the current one).
//
//	@Summary		Public holidays for a timezone
//	@Description	Serves the offline public-holiday dataset for the country a timezone maps to, for one year.
//	@Tags			Holidays
//	@Produce		json
//	@Param			timezone	query		string	false	"IANA timezone (e.g. Europe/Paris)"
//	@Param			year		query		int		false	"Year (defaults to the current one)"
//	@Success		200			{object}	YearResponse
//	@Failure		400			{object}	httputil.ErrorResponse
//	@Router			/api/v1/holidays [get]
func (h *Handler) Year(w http.ResponseWriter, r *http.Request) {
	timezone := r.URL.Query().Get("timezone")
	countryCode := datevalidation.GetCountryFromTimezone(timezone)

	year := currentYear()
	if raw := r.URL.Query().Get("year"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 9999 {
			httputil.Error(w, http.StatusBadRequest, httputil.ErrCodeBadRequest, "Invalid year")
			return
		}
		year = parsed
	}

	response := YearResponse{
		CountryCode: countryCode,
		Supported:   false,
		Holidays:    []Holiday{},
	}
	if countryCode == "" {
		httputil.JSON(w, http.StatusOK, response)
		return
	}

	holidays, err := datevalidation.ListHolidays(countryCode, year)
	if err != nil {
		// Unsupported country: honest empty answer rather than a refusal that
		// would break the participant calendar. The block policy still fails
		// closed server-side for these.
		httputil.JSON(w, http.StatusOK, response)
		return
	}

	response.Supported = true
	for _, holiday := range holidays {
		response.Holidays = append(response.Holidays, Holiday{
			Date: holiday.Date.Format("2006-01-02"),
			Name: holiday.Name,
		})
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

// currentYear is a variable so tests can pin the default year.
var currentYear = func() int { return time.Now().Year() }
