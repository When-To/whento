// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"testing"
	"time"
)

// Fixtures, verified against the holiday library rather than assumed. Weekdays follow
// Go's numbering (Sunday = 0), which is what IsDateAllowed compares against.
//
//	2026-03-17  Tuesday    ordinary day in FR and US
//	2026-07-14  Tuesday    Bastille Day: a holiday in FR, an ordinary day in US
//	2026-12-24  Thursday   not a holiday, but the eve of one in both FR and US
//	2026-12-25  Friday     Christmas: a holiday in both
const (
	ordinaryTuesday = "2026-03-17"
	frenchHoliday   = "2026-07-14"
	holidayEve      = "2026-12-24"
	sharedHoliday   = "2026-12-25"
)

func date(t *testing.T, iso string) time.Time {
	t.Helper()

	parsed, err := time.Parse("2006-01-02", iso)
	if err != nil {
		t.Fatalf("parse %q: %v", iso, err)
	}

	return parsed
}

var (
	weekdaysOnly = []int{1, 2, 3, 4, 5} // Monday..Friday
	noTuesdays   = []int{1, 3, 4, 5}    // Monday, Wednesday..Friday
	everyDay     = []int{0, 1, 2, 3, 4, 5, 6}
	noDays       = []int{}
)

func TestIsDateAllowed(t *testing.T) {
	tests := []struct {
		name             string
		date             string
		timezone         string
		allowedWeekdays  []int
		holidaysPolicy   string
		allowHolidayEves bool
		want             bool
	}{
		// --- the plain weekday check, with no holiday in play ---
		{
			name:            "ordinary day on an allowed weekday",
			date:            ordinaryTuesday,
			timezone:        "Europe/Paris",
			allowedWeekdays: weekdaysOnly,
			holidaysPolicy:  "ignore",
			want:            true,
		},
		{
			name:            "ordinary day on a disallowed weekday",
			date:            ordinaryTuesday,
			timezone:        "Europe/Paris",
			allowedWeekdays: noTuesdays,
			holidaysPolicy:  "ignore",
			want:            false,
		},
		{
			name:            "no weekday is allowed at all",
			date:            ordinaryTuesday,
			timezone:        "Europe/Paris",
			allowedWeekdays: noDays,
			holidaysPolicy:  "ignore",
			want:            false,
		},
		{
			name:            "nil allowed weekdays behaves like an empty list",
			date:            ordinaryTuesday,
			timezone:        "Europe/Paris",
			allowedWeekdays: nil,
			holidaysPolicy:  "ignore",
			want:            false,
		},

		// --- policy "block": a holiday is refused before the weekday is even consulted ---
		{
			name:            "block refuses a holiday that falls on an allowed weekday",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: weekdaysOnly,
			holidaysPolicy:  "block",
			want:            false,
		},
		{
			name:            "block refuses a holiday on a disallowed weekday",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: noTuesdays,
			holidaysPolicy:  "block",
			want:            false,
		},
		{
			name:             "block wins over the holiday-eve exception",
			date:             frenchHoliday,
			timezone:         "Europe/Paris",
			allowedWeekdays:  noTuesdays,
			holidaysPolicy:   "block",
			allowHolidayEves: true,
			want:             false,
		},
		{
			name:            "block leaves an ordinary day to the weekday check",
			date:            ordinaryTuesday,
			timezone:        "Europe/Paris",
			allowedWeekdays: weekdaysOnly,
			holidaysPolicy:  "block",
			want:            true,
		},

		// --- policy "allow": a holiday is accepted before the weekday is consulted ---
		{
			name:            "allow accepts a holiday on a disallowed weekday",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: noTuesdays,
			holidaysPolicy:  "allow",
			want:            true,
		},
		{
			name:            "allow accepts a holiday when no weekday is permitted",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: noDays,
			holidaysPolicy:  "allow",
			want:            true,
		},
		{
			name:            "allow leaves an ordinary day to the weekday check",
			date:            ordinaryTuesday,
			timezone:        "Europe/Paris",
			allowedWeekdays: noTuesdays,
			holidaysPolicy:  "allow",
			want:            false,
		},

		// --- policy "ignore": the holiday is simply not special ---
		{
			name:            "ignore treats a holiday as a normal allowed weekday",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: weekdaysOnly,
			holidaysPolicy:  "ignore",
			want:            true,
		},
		{
			name:            "ignore treats a holiday as a normal disallowed weekday",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: noTuesdays,
			holidaysPolicy:  "ignore",
			want:            false,
		},

		// --- an unrecognised policy must not become an accidental gate ---
		{
			name:            "an unknown policy falls through to the weekday check",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: weekdaysOnly,
			holidaysPolicy:  "no-such-policy",
			want:            true,
		},
		{
			name:            "an empty policy falls through to the weekday check",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: weekdaysOnly,
			holidaysPolicy:  "",
			want:            true,
		},
		{
			name:            "an unknown policy does not rescue a disallowed weekday",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: noTuesdays,
			holidaysPolicy:  "no-such-policy",
			want:            false,
		},

		// --- the holiday-eve exception, which only ever *adds* a day ---
		{
			name:             "an eve rescues a disallowed weekday when eves are enabled",
			date:             holidayEve,
			timezone:         "Europe/Paris",
			allowedWeekdays:  []int{1, 2, 3}, // Thursday excluded
			holidaysPolicy:   "ignore",
			allowHolidayEves: true,
			want:             true,
		},
		{
			name:             "an eve changes nothing when eves are disabled",
			date:             holidayEve,
			timezone:         "Europe/Paris",
			allowedWeekdays:  []int{1, 2, 3},
			holidaysPolicy:   "ignore",
			allowHolidayEves: false,
			want:             false,
		},
		{
			name:             "an eve is reachable under block, because the eve is not itself a holiday",
			date:             holidayEve,
			timezone:         "Europe/Paris",
			allowedWeekdays:  []int{1, 2, 3},
			holidaysPolicy:   "block",
			allowHolidayEves: true,
			want:             true,
		},
		{
			name:             "an eve is reachable under allow as well",
			date:             holidayEve,
			timezone:         "Europe/Paris",
			allowedWeekdays:  []int{1, 2, 3},
			holidaysPolicy:   "allow",
			allowHolidayEves: true,
			want:             true,
		},
		{
			name:             "an ordinary day is not rescued by the eve exception",
			date:             ordinaryTuesday,
			timezone:         "Europe/Paris",
			allowedWeekdays:  noTuesdays,
			holidaysPolicy:   "ignore",
			allowHolidayEves: true,
			want:             false,
		},

		// --- holidays are country-local, and the country comes from the timezone ---
		{
			name:            "Bastille Day is blocked in France",
			date:            frenchHoliday,
			timezone:        "Europe/Paris",
			allowedWeekdays: everyDay,
			holidaysPolicy:  "block",
			want:            false,
		},
		{
			name:            "Bastille Day is an ordinary working day in New York",
			date:            frenchHoliday,
			timezone:        "America/New_York",
			allowedWeekdays: everyDay,
			holidaysPolicy:  "block",
			want:            true,
		},
		{
			name:            "Christmas is blocked in both",
			date:            sharedHoliday,
			timezone:        "America/New_York",
			allowedWeekdays: everyDay,
			holidaysPolicy:  "block",
			want:            false,
		},

		// --- the silent skip: a timezone with no country disables holidays entirely ---
		{
			name:            "UTC maps to no country, so block never fires",
			date:            sharedHoliday,
			timezone:        "UTC",
			allowedWeekdays: everyDay,
			holidaysPolicy:  "block",
			want:            true,
		},
		{
			name:            "UTC maps to no country, so allow never fires either",
			date:            sharedHoliday,
			timezone:        "UTC",
			allowedWeekdays: noDays,
			holidaysPolicy:  "allow",
			want:            false,
		},
		{
			name:             "UTC maps to no country, so the eve exception is unreachable",
			date:             holidayEve,
			timezone:         "UTC",
			allowedWeekdays:  []int{1, 2, 3},
			holidaysPolicy:   "ignore",
			allowHolidayEves: true,
			want:             false,
		},
		{
			name:            "an empty timezone behaves like UTC",
			date:            sharedHoliday,
			timezone:        "",
			allowedWeekdays: everyDay,
			holidaysPolicy:  "block",
			want:            true,
		},
		{
			name:            "an unknown timezone behaves like UTC",
			date:            sharedHoliday,
			timezone:        "Mars/Olympus_Mons",
			allowedWeekdays: everyDay,
			holidaysPolicy:  "block",
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDateAllowed(
				date(t, tt.date),
				tt.timezone,
				tt.allowedWeekdays,
				tt.holidaysPolicy,
				tt.allowHolidayEves,
			)

			if got != tt.want {
				t.Errorf(
					"IsDateAllowed(%s, %q, %v, %q, eves=%v) = %v, want %v",
					tt.date, tt.timezone, tt.allowedWeekdays, tt.holidaysPolicy, tt.allowHolidayEves,
					got, tt.want,
				)
			}
		})
	}
}

// TestHolidayPolicyIsIndependentOfTimeOfDay guards the boundary that bit the frontend:
// a timestamp carries a clock, and the holiday lookup must key on the calendar date the
// caller means, not on whatever UTC instant the value happens to sit at.
func TestHolidayPolicyIsIndependentOfTimeOfDay(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	day := date(t, frenchHoliday)

	for _, at := range []time.Time{
		day,
		time.Date(2026, 7, 14, 0, 0, 0, 0, paris),
		time.Date(2026, 7, 14, 12, 30, 0, 0, paris),
		time.Date(2026, 7, 14, 23, 59, 59, 0, paris),
	} {
		if IsDateAllowed(at, "Europe/Paris", everyDay, "block", false) {
			t.Errorf("Bastille Day at %v was allowed under the block policy", at)
		}
	}
}

func TestIsWeekdayAllowed(t *testing.T) {
	tests := []struct {
		name     string
		weekday  int
		weekdays []int
		want     bool
	}{
		{name: "present", weekday: 3, weekdays: weekdaysOnly, want: true},
		{name: "absent", weekday: 0, weekdays: weekdaysOnly, want: false},
		{name: "sunday is zero, not seven", weekday: 0, weekdays: everyDay, want: true},
		{name: "saturday", weekday: 6, weekdays: everyDay, want: true},
		{name: "empty list allows nothing", weekday: 3, weekdays: noDays, want: false},
		{name: "nil list allows nothing", weekday: 3, weekdays: nil, want: false},
		{name: "out of range below", weekday: -1, weekdays: everyDay, want: false},
		{name: "out of range above", weekday: 7, weekdays: everyDay, want: false},
		{name: "duplicates are harmless", weekday: 2, weekdays: []int{2, 2, 2}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsWeekdayAllowed(tt.weekday, tt.weekdays); got != tt.want {
				t.Errorf("IsWeekdayAllowed(%d, %v) = %v, want %v", tt.weekday, tt.weekdays, got, tt.want)
			}
		})
	}
}

func TestGetCountryFromTimezone(t *testing.T) {
	tests := []struct {
		name     string
		timezone string
		want     string
	}{
		{name: "france", timezone: "Europe/Paris", want: "FR"},
		{name: "united states", timezone: "America/New_York", want: "US"},
		{name: "united kingdom", timezone: "Europe/London", want: "GB"},
		{name: "japan", timezone: "Asia/Tokyo", want: "JP"},
		{name: "UTC has no country", timezone: "UTC", want: ""},
		{name: "empty", timezone: "", want: ""},
		{name: "unknown zone", timezone: "Mars/Olympus_Mons", want: ""},
		{name: "case matters", timezone: "europe/paris", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetCountryFromTimezone(tt.timezone); got != tt.want {
				t.Errorf("GetCountryFromTimezone(%q) = %q, want %q", tt.timezone, got, tt.want)
			}
		})
	}
}

func TestIsHolidayAndIsHolidayEve(t *testing.T) {
	tests := []struct {
		name        string
		date        string
		countryCode string
		wantHoliday bool
		wantEve     bool
	}{
		{name: "bastille day in france", date: frenchHoliday, countryCode: "FR", wantHoliday: true},
		{name: "bastille day in the united states", date: frenchHoliday, countryCode: "US"},
		{name: "christmas in france", date: sharedHoliday, countryCode: "FR", wantHoliday: true},
		{name: "christmas in the united states", date: sharedHoliday, countryCode: "US", wantHoliday: true},
		{name: "christmas eve is an eve, not a holiday", date: holidayEve, countryCode: "FR", wantEve: true},
		{name: "an ordinary tuesday is neither", date: ordinaryTuesday, countryCode: "FR"},
		{name: "an empty country code matches nothing", date: sharedHoliday, countryCode: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHoliday(date(t, tt.date), tt.countryCode); got != tt.wantHoliday {
				t.Errorf("IsHoliday(%s, %q) = %v, want %v", tt.date, tt.countryCode, got, tt.wantHoliday)
			}
			if got := IsHolidayEve(date(t, tt.date), tt.countryCode); got != tt.wantEve {
				t.Errorf("IsHolidayEve(%s, %q) = %v, want %v", tt.date, tt.countryCode, got, tt.wantEve)
			}
		})
	}
}

// TestIsHolidayEveLooksExactlyOneDayAhead pins the offset. An off-by-one here would
// shift every eve by a day, which is invisible in a spot check.
func TestIsHolidayEveLooksExactlyOneDayAhead(t *testing.T) {
	christmas := date(t, sharedHoliday)

	if !IsHolidayEve(christmas.AddDate(0, 0, -1), "FR") {
		t.Error("the day before Christmas is not reported as an eve")
	}
	if IsHolidayEve(christmas.AddDate(0, 0, -2), "FR") {
		t.Error("two days before Christmas was reported as an eve")
	}
	if IsHolidayEve(christmas, "FR") {
		t.Error("Christmas itself was reported as an eve")
	}
}
