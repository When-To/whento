// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"testing"
	"time"
)

// Fixtures, verified against the offline holiday library rather than assumed.
// Weekdays follow Go's numbering (Sunday = 0), which is what IsDateAllowed
// compares against. The 2026 dates were checked against the rickar/cal
// dataset the package now ships: Bastille Day (14 July) is a French holiday
// and an ordinary US day, Christmas is shared, and both countries' eve of
// Christmas is 24 December.
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

// TestBlockFailsClosedWithoutHolidayData pins the availability-defect behaviour
// this module used to have: a calendar set to "block" admitted every holiday
// whenever the source was unreachable, because the lookup collapsed its error
// into "not a holiday". Under the block policy an unknown day must be refused,
// not admitted. With the offline source there is no network failure, but a
// country the bundled dataset does not cover is the same problem in another
// costume: Asia/Tokyo maps to Japan, which ships region-only definitions, so it
// is absent from the national table and every date must fail closed.
func TestBlockFailsClosedWithoutHolidayData(t *testing.T) {
	tests := []struct {
		name     string
		timezone string
	}{
		{"a country with no national data in the offline table", "Asia/Tokyo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// everyDay means the weekday check alone would admit it: only the
			// missing holiday data can refuse it.
			if got := IsDateAllowed(date(t, "2026-06-01"), tc.timezone, everyDay, "block", false); got {
				t.Errorf("IsDateAllowed(2026-06-01, %s, everyDay, block) = true, want false: "+
					"a block calendar must fail closed when the holiday data is unavailable", tc.timezone)
			}
		})
	}
}

// TestTheSameDateIsAllowedWithData is the control for the test above: a country
// whose national dataset the offline table covers is a known ordinary day, and
// the block policy leaves it to the weekday check — so the fail-closed path only
// ever triggers on genuinely missing data, never on a normal working day.
func TestTheSameDateIsAllowedWithData(t *testing.T) {
	if got := IsDateAllowed(date(t, "2026-06-01"), "Europe/Paris", everyDay, "block", false); !got {
		t.Error("IsDateAllowed(2026-06-01, Europe/Paris, everyDay, block) = false, want true")
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

// TestListHolidaysServesTheBackendDataset pins what the endpoint the frontend
// consumes will return: the same offline dataset the block/allow policy runs on,
// in date order, with the library's local-language name.
func TestListHolidaysServesTheBackendDataset(t *testing.T) {
	holidays, err := ListHolidays("FR", 2026)
	if err != nil {
		t.Fatalf("ListHolidays(FR, 2026): %v", err)
	}
	if len(holidays) == 0 {
		t.Fatal("ListHolidays returned nothing for a covered country")
	}

	// Bastille Day and Christmas must be in the year's list, matching the
	// fixtures the availability tests use.
	var sawBastille, sawChristmas bool
	var last time.Time
	for _, h := range holidays {
		iso := h.Date.Format("2006-01-02")
		if iso == frenchHoliday {
			sawBastille = true
		}
		if iso == sharedHoliday {
			sawChristmas = true
		}
		if h.Name == "" {
			t.Errorf("holiday on %s has no name", iso)
		}
		if last.After(h.Date) {
			t.Errorf("holidays are not sorted: %s after %s", h.Date.Format("2006-01-02"), last.Format("2006-01-02"))
		}
		last = h.Date
	}
	if !sawBastille || !sawChristmas {
		t.Errorf("year list misses fixtures: Bastille=%v Christmas=%v", sawBastille, sawChristmas)
	}
}

// TestListHolidaysRejectsUncoveredCountries: a country without a national
// dataset is an error, not an empty list — the caller can distinguish "no
// holidays that year" from "no data".
func TestListHolidaysRejectsUncoveredCountries(t *testing.T) {
	if _, err := ListHolidays("XX", 2026); err == nil {
		t.Error("ListHolidays for an unknown country did not fail")
	}
	// AU ships region-only definitions, so it has no national set either.
	if _, err := ListHolidays("AU", 2026); err == nil {
		t.Error("ListHolidays for a region-only country did not fail")
	}
}

// TestSupportedCountries lists exactly the offline table's keys, sorted.
func TestSupportedCountries(t *testing.T) {
	countries := SupportedCountries()
	if len(countries) == 0 {
		t.Fatal("SupportedCountries returned nothing")
	}
	if !contains(countries, "FR") || !contains(countries, "US") {
		t.Errorf("SupportedCountries missing FR/US: %v", countries)
	}
	for i := 1; i < len(countries); i++ {
		if countries[i-1] >= countries[i] {
			t.Errorf("countries are not sorted at %q", countries[i])
		}
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
