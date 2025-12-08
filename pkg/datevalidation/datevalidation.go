// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"strings"
	"time"

	"github.com/go-playground/tz"
	holidays "github.com/omidnikrah/go-holidays"
)

// IsDateAllowed checks if a date is allowed for availability based on calendar settings
// It considers:
// 1. Regular allowed weekdays
// 2. Holidays policy:
//   - "ignore": Holidays are treated as normal days (check weekday only)
//   - "allow": Holidays are explicitly allowed even if weekday is not
//   - "block": Holidays are explicitly blocked (return false)
//
// 3. Holiday eves (if allow_holiday_eves is true)
func IsDateAllowed(date time.Time, timezone string, allowedWeekdays []int, holidaysPolicy string, allowHolidayEves bool) bool {
	// Get country code from timezone for holiday checking
	countryCode := getCountryFromTimezone(timezone)

	// Check if it's a holiday (if we have country information)
	isHolidayDate := false
	if countryCode != "" {
		isHolidayDate = isHoliday(date, countryCode)
	}

	// Apply holidays_policy
	switch holidaysPolicy {
	case "block":
		// If it's a holiday and policy is block, reject immediately
		if isHolidayDate {
			return false
		}
	case "allow":
		// If it's a holiday and policy is allow, accept immediately
		if isHolidayDate {
			return true
		}
	case "ignore":
		// Fall through - treat as normal day (check weekday)
	}

	// Check if the weekday is in the allowed list
	weekday := int(date.Weekday())
	if isWeekdayAllowed(weekday, allowedWeekdays) {
		return true
	}

	// If weekday is not allowed, check holiday eve exception
	if countryCode != "" && allowHolidayEves && isHolidayEve(date, countryCode) {
		return true
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

// getCountryFromTimezone is a private alias for backward compatibility
func getCountryFromTimezone(timezone string) string {
	return GetCountryFromTimezone(timezone)
}

// IsHoliday checks if a given date is a public holiday in the specified country
func IsHoliday(date time.Time, countryCode string) bool {
	// Check if the date is a holiday using go-holidays library
	isHoliday := holidays.IsHoliday(countryCode, date)
	return isHoliday
}

// isHoliday is a private alias for backward compatibility
func isHoliday(date time.Time, countryCode string) bool {
	return IsHoliday(date, countryCode)
}

// IsHolidayEve checks if a given date is the day before a public holiday
func IsHolidayEve(date time.Time, countryCode string) bool {
	// Check if the next day is a holiday
	nextDay := date.AddDate(0, 0, 1)
	return IsHoliday(nextDay, countryCode)
}

// isHolidayEve is a private alias for backward compatibility
func isHolidayEve(date time.Time, countryCode string) bool {
	return IsHolidayEve(date, countryCode)
}
