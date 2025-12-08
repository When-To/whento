// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// AllowedHours represents the JSONB structure for allowed hours in the database
type AllowedHours struct {
	Weekdays    map[string]TimeSlot `json:"weekdays"`
	Holidays    TimeSlot            `json:"holidays"`
	HolidayEves TimeSlot            `json:"holiday_eves"`
}

// TimeSlot represents a time range with start and end
type TimeSlot struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// BuildAllowedHoursJSON creates the JSONB string from separate request fields
func BuildAllowedHoursJSON(
	weekdayTimes map[string]TimeRange,
	holidayMinTime, holidayMaxTime string,
	holidayEveMinTime, holidayEveMaxTime string,
) (*string, error) {
	allowedHours := AllowedHours{
		Weekdays: make(map[string]TimeSlot),
		Holidays: TimeSlot{
			Start: holidayMinTime,
			End:   holidayMaxTime,
		},
		HolidayEves: TimeSlot{
			Start: holidayEveMinTime,
			End:   holidayEveMaxTime,
		},
	}

	// Convert weekday_times to the JSONB format
	for day, timeRange := range weekdayTimes {
		allowedHours.Weekdays[day] = TimeSlot{
			Start: timeRange.MinTime,
			End:   timeRange.MaxTime,
		}
	}

	// If no weekday times provided, set defaults for all days
	if len(allowedHours.Weekdays) == 0 {
		for i := 0; i <= 6; i++ {
			allowedHours.Weekdays[fmt.Sprintf("%d", i)] = TimeSlot{
				Start: "00:00",
				End:   "23:59",
			}
		}
	}

	// Keep holidays and holiday_eves empty if not provided
	// No default values - empty means unrestricted

	jsonBytes, err := json.Marshal(allowedHours)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal allowed_hours: %w", err)
	}

	jsonStr := string(jsonBytes)
	return &jsonStr, nil
}

// ParseAllowedHoursJSON parses the JSONB string and extracts separate fields
func ParseAllowedHoursJSON(allowedHoursJSON *string) (
	weekdayTimes map[string]TimeRange,
	holidayMinTime, holidayMaxTime string,
	holidayEveMinTime, holidayEveMaxTime string,
	err error,
) {
	if allowedHoursJSON == nil || *allowedHoursJSON == "" {
		return nil, "", "", "", "", nil
	}

	var allowedHours AllowedHours
	if err := json.Unmarshal([]byte(*allowedHoursJSON), &allowedHours); err != nil {
		return nil, "", "", "", "", fmt.Errorf("failed to unmarshal allowed_hours: %w", err)
	}

	// Convert weekdays to map[string]TimeRange
	weekdayTimes = make(map[string]TimeRange)
	for day, slot := range allowedHours.Weekdays {
		weekdayTimes[day] = TimeRange{
			MinTime: slot.Start,
			MaxTime: slot.End,
		}
	}

	return weekdayTimes,
		allowedHours.Holidays.Start,
		allowedHours.Holidays.End,
		allowedHours.HolidayEves.Start,
		allowedHours.HolidayEves.End,
		nil
}

// NormalizeTimeRange ensures MinTime is always before MaxTime by swapping if necessary
// Returns a new TimeRange with normalized values
func NormalizeTimeRange(tr TimeRange) TimeRange {
	// If either is empty, return as-is
	if tr.MinTime == "" || tr.MaxTime == "" {
		return tr
	}

	// Parse times
	minT, err1 := time.Parse("15:04", tr.MinTime)
	maxT, err2 := time.Parse("15:04", tr.MaxTime)

	// If parsing fails, return as-is (validation will catch this later)
	if err1 != nil || err2 != nil {
		return tr
	}

	// If min is after max, swap them
	if minT.After(maxT) {
		return TimeRange{
			MinTime: tr.MaxTime,
			MaxTime: tr.MinTime,
		}
	}

	// Otherwise return as-is
	return tr
}

// NormalizeWeekdayTimes normalizes all TimeRanges in the weekday times map
func NormalizeWeekdayTimes(weekdayTimes map[string]TimeRange) map[string]TimeRange {
	if len(weekdayTimes) == 0 {
		return weekdayTimes
	}

	normalized := make(map[string]TimeRange)
	for day, timeRange := range weekdayTimes {
		normalized[day] = NormalizeTimeRange(timeRange)
	}
	return normalized
}

// NormalizeHolidayTimes normalizes holiday min/max times by swapping if necessary
// Returns normalized min and max times
func NormalizeHolidayTimes(minTime, maxTime string) (string, string) {
	// If either is empty, return as-is
	if minTime == "" || maxTime == "" {
		return minTime, maxTime
	}

	// Parse times
	min, err1 := time.Parse("15:04", minTime)
	max, err2 := time.Parse("15:04", maxTime)

	// If parsing fails, return as-is
	if err1 != nil || err2 != nil {
		return minTime, maxTime
	}

	// If min is after max, swap them
	if min.After(max) {
		return maxTime, minTime
	}

	// Otherwise return as-is
	return minTime, maxTime
}
