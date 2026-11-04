// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/go-playground/tz"
)

// isHolidayErrCtx is the error-aware holiday check: it distinguishes "this date
// is not a public holiday; data present" (false, nil) from "no holiday data
// could be obtained for this country" (false, err). The offline table answers
// for the bundled countries; everything else uses the opt-in Nager fallback
// with its bounded cache. Callers decide what a missing dataset means for their
// policy; the lookup itself never lies. The context only reaches the network
// fallback; the offline table never touches it.
func isHolidayErrCtx(ctx context.Context, date time.Time, countryCode string) (bool, error) {
	cc := strings.ToUpper(strings.TrimSpace(countryCode))
	if cc == "" {
		return false, nil
	}
	if isOfflineCountry(cc) {
		return offlineIsHoliday(date, cc)
	}
	holidays, err := defaultNager.fetch(ctx, cc, date.Year())
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(holidays, func(h time.Time) bool { return sameDate(h, date) }), nil
}

// IsDateAllowed checks if a date is allowed for availability based on calendar settings
// It considers:
// 1. Regular allowed weekdays
// 2. Holidays policy:
//   - "ignore": Holidays are treated as normal days (check weekday only)
//   - "allow": Holidays are explicitly allowed even if weekday is not
//   - "block": Holidays are explicitly blocked (return false)
//
// 3. Holiday eves (if allow_holiday_eves is true)
//
// The block policy is fail-open on genuinely unavailable holiday data: if
// neither the offline table nor the fallback could say whether the day is a
// holiday, the ordinary weekday decision applies — an eligible Wednesday is
// admitted rather than refused on a guess. A day that IS a known holiday is
// still refused. Unavailability is surfaced through the error-aware lookup
// instead of being collapsed into a refusal here.
func IsDateAllowed(date time.Time, timezone string, allowedWeekdays []int, holidaysPolicy string, allowHolidayEves bool) bool {
	// Get country code from timezone for holiday checking.
	countryCode := GetCountryFromTimezone(timezone)
	// The lookup error is deliberately discarded here: under the block policy it
	// means "the providers could not classify the day", which fails open to the
	// weekday decision below. Unavailability reaches callers through the
	// error-aware isHolidayErrCtx instead.
	isHolidayDate := IsHoliday(date, countryCode)

	// Apply holidays_policy.
	switch holidaysPolicy {
	case "block":
		// A known holiday is refused outright.
		if isHolidayDate {
			return false
		}
		// Fall through to the weekday decision: a day the providers could not
		// classify is admitted when the ordinary weekday rules allow it rather
		// than refused on a guess. A known blocked holiday is still blocked.
	case "allow":
		// If it's a known holiday and policy is allow, accept immediately.
		if isHolidayDate {
			return true
		}
	case "ignore":
		// Fall through - treat as normal day (check weekday).
	}

	// Check if the weekday is in the allowed list.
	weekday := int(date.Weekday())
	if IsWeekdayAllowed(weekday, allowedWeekdays) {
		return true
	}

	// If weekday is not allowed, check holiday eve exception. An eve is only
	// ever an *addition*, so a failed eve lookup is safe to treat as "not an
	// eve": it can cost a day, never admit a blocked one.
	return countryCode != "" && allowHolidayEves && IsHolidayEve(date, countryCode)
}

// IsWeekdayAllowed checks if a weekday is in the list of allowed weekdays
func IsWeekdayAllowed(weekday int, allowedWeekdays []int) bool {
	return slices.Contains(allowedWeekdays, weekday)
}

// GetCountryFromTimezone converts a timezone to a country code
func GetCountryFromTimezone(timezone string) string {
	// Get all countries and their zones from tz library
	countries := tz.GetCountries()

	// Search for the timezone in all countries
	for _, country := range countries {
		for _, zone := range country.Zones {
			if zone.Name == timezone {
				return strings.ToUpper(country.Code)
			}
		}
	}

	return ""
}

// IsHoliday checks if a given date is a public holiday in the specified country.
//
// This is a bool convenience kept for callers who only need "yes or no" (the
// availability time-window helpers). It deliberately swallows lookup errors — a
// country without usable data reads as "not a holiday" here. Do not use it for
// the "block" policy; IsDateAllowed handles that one.
func IsHoliday(date time.Time, countryCode string) bool {
	isHolidayDate, _ := isHolidayErrCtx(context.Background(), date, countryCode)
	return isHolidayDate
}

// IsHolidayEve checks if a given date is the day before a public holiday
func IsHolidayEve(date time.Time, countryCode string) bool {
	// Check if the next day is a holiday
	nextDay := date.AddDate(0, 0, 1)
	return IsHoliday(nextDay, countryCode)
}
