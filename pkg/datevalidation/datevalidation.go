// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"strings"
	"time"

	"github.com/go-playground/tz"
)

// isHolidayErr is the error-aware holiday check: it distinguishes "this date is
// not a public holiday" (false, nil) from "no holiday data exists for this
// country" (false, err). It is backed by the offline table in
// offline_holidays.go — there is no network in this package, so the only error
// an unknown country can produce is a missing dataset. Callers decide what a
// missing region means for their policy; the lookup itself never lies.
func isHolidayErr(date time.Time, countryCode string) (bool, error) {
	if countryCode == "" {
		return false, nil
	}
	return offlineIsHoliday(date, countryCode)
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
// The holiday lookup is error-aware for the "block" policy: if a country's
// holiday data is unavailable, a date that *might* be a holiday is refused
// rather than admitted. Fail-closed is the whole point of a block policy — a
// missing dataset must take the date out of the calendar, never put a holiday
// into it. The "allow" and "ignore" policies are unaffected: an unavailable
// dataset just means no date is ever treated as a holiday, which adds nothing
// for "allow" and changes nothing for "ignore".
func IsDateAllowed(date time.Time, timezone string, allowedWeekdays []int, holidaysPolicy string, allowHolidayEves bool) bool {
	// Get country code from timezone for holiday checking.
	countryCode := GetCountryFromTimezone(timezone)
	isHolidayDate, holidayErr := isHolidayErr(date, countryCode)

	// Apply holidays_policy.
	switch holidaysPolicy {
	case "block":
		// A known holiday is refused outright.
		if isHolidayDate {
			return false
		}
		// An *unknown* day is refused too: a calendar configured to block
		// holidays must not silently admit one because the data source was
		// unavailable. A timezone with no country has no holiday data at all
		// and is left to the weekday check, as it always was.
		if countryCode != "" && holidayErr != nil {
			return false
		}
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
	if isWeekdayAllowed(weekday, allowedWeekdays) {
		return true
	}

	// If weekday is not allowed, check holiday eve exception. An eve is only
	// ever an *addition*, so a failed eve lookup is safe to treat as "not an
	// eve": it can cost a day, never admit a blocked one.
	if countryCode != "" && allowHolidayEves {
		nextDay := date.AddDate(0, 0, 1)
		if isEve, err := isHolidayErr(nextDay, countryCode); err == nil && isEve {
			return true
		}
	}

	return false
}

// IsWeekdayAllowed checks if a weekday is in the list of allowed weekdays
func IsWeekdayAllowed(weekday int, allowedWeekdays []int) bool {
	for _, allowed := range allowedWeekdays {
		if allowed == weekday {
			return true
		}
	}
	return false
}

// isWeekdayAllowed is a private alias for backward compatibility
func isWeekdayAllowed(weekday int, allowedWeekdays []int) bool {
	return IsWeekdayAllowed(weekday, allowedWeekdays)
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
// availability time-window helpers). It deliberately swallows lookup errors —
// a country without offline data reads as "not a holiday" here. Do not use it for
// the "block" policy; IsDateAllowed handles that one fail-closed.
func IsHoliday(date time.Time, countryCode string) bool {
	isHolidayDate, _ := isHolidayErr(date, countryCode)
	return isHolidayDate
}

// IsHolidayEve checks if a given date is the day before a public holiday
func IsHolidayEve(date time.Time, countryCode string) bool {
	// Check if the next day is a holiday
	nextDay := date.AddDate(0, 0, 1)
	return IsHoliday(nextDay, countryCode)
}
