// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rickar/cal/v2"
	"github.com/rickar/cal/v2/ar"
	"github.com/rickar/cal/v2/at"
	"github.com/rickar/cal/v2/be"
	"github.com/rickar/cal/v2/bg"
	"github.com/rickar/cal/v2/br"
	"github.com/rickar/cal/v2/ca"
	"github.com/rickar/cal/v2/ch"
	"github.com/rickar/cal/v2/cy"
	"github.com/rickar/cal/v2/cz"
	"github.com/rickar/cal/v2/de"
	"github.com/rickar/cal/v2/dk"
	"github.com/rickar/cal/v2/ee"
	"github.com/rickar/cal/v2/es"
	"github.com/rickar/cal/v2/fi"
	"github.com/rickar/cal/v2/fr"
	"github.com/rickar/cal/v2/gb"
	"github.com/rickar/cal/v2/gr"
	"github.com/rickar/cal/v2/hr"
	"github.com/rickar/cal/v2/hu"
	"github.com/rickar/cal/v2/ie"
	"github.com/rickar/cal/v2/is"
	"github.com/rickar/cal/v2/it"
	"github.com/rickar/cal/v2/ke"
	"github.com/rickar/cal/v2/lt"
	"github.com/rickar/cal/v2/lu"
	"github.com/rickar/cal/v2/lv"
	"github.com/rickar/cal/v2/mt"
	"github.com/rickar/cal/v2/mw"
	"github.com/rickar/cal/v2/mx"
	"github.com/rickar/cal/v2/nc"
	"github.com/rickar/cal/v2/nl"
	"github.com/rickar/cal/v2/no"
	"github.com/rickar/cal/v2/nz"
	"github.com/rickar/cal/v2/pl"
	"github.com/rickar/cal/v2/pt"
	"github.com/rickar/cal/v2/ro"
	"github.com/rickar/cal/v2/rs"
	"github.com/rickar/cal/v2/ru"
	"github.com/rickar/cal/v2/se"
	"github.com/rickar/cal/v2/si"
	"github.com/rickar/cal/v2/sk"
	"github.com/rickar/cal/v2/ua"
	"github.com/rickar/cal/v2/us"
	"github.com/rickar/cal/v2/za"
)

// errHolidayUnavailable reports a country the offline library has no rules for.
// The block policy used to turn this into a fail-closed refusal; it now fails
// open to the normal weekday decision, and the caller-facing API reports the
// unavailability explicitly through CoverageUnavailable instead.
var errHolidayUnavailable = errors.New("holiday data unavailable for country")

// offlineHolidayCalendars is a compiled table of ISO 3166-1 alpha-2 country
// codes → a calendar of that country's national public holidays. The data comes
// from the rickar/cal library, which ships per-country definitions and computes
// movable dates (Easter, nth weekdays) locally: nothing here touches a network,
// so a self-hosted instance never depends on a third-party API to know whether
// it may slouch on Bastille Day.
var (
	offlineOnce      sync.Once
	offlineCalendars map[string]*cal.Calendar
	offlineCountries []string
)

func buildOfflineCalendars() {
	// Observation days are kept deliberately: when a civil holiday lands on a
	// weekend, the library also reports the substitute weekday banks observe,
	// and both dates count as holidays. A 4th of July on a Saturday blocks the
	// 4th and the observed Friday alike. Sub-countries (states, regions) are
	// out of scope for the current country-level model, so each map entry takes
	// the national set.
	//
	// Not every library pack ships a single national `Holidays` slice: the
	// Australia/Japan/Thailand packs are region-only, so their ISO codes are
	// absent here and those countries are served by the Nager fallback instead
	// of an empty offline guess.
	offlineCalendars = map[string]*cal.Calendar{
		"AR": calendarFrom(ar.Holidays),
		"AT": calendarFrom(at.Holidays),
		"BE": calendarFrom(be.Holidays),
		"BG": calendarFrom(bg.Holidays),
		"BR": calendarFrom(br.Holidays),
		"CA": calendarFrom(ca.Holidays),
		"CH": calendarFrom(ch.Holidays),
		"CY": calendarFrom(cy.Holidays),
		"CZ": calendarFrom(cz.Holidays),
		"DE": calendarFrom(de.Holidays),
		"DK": calendarFrom(dk.Holidays),
		"EE": calendarFrom(ee.Holidays),
		"ES": calendarFrom(es.Holidays),
		"FI": calendarFrom(fi.Holidays),
		"FR": calendarFrom(fr.Holidays),
		"GB": calendarFrom(gb.Holidays),
		"GR": calendarFrom(gr.Holidays),
		"HR": calendarFrom(hr.Holidays),
		"HU": calendarFrom(hu.Holidays),
		"IE": calendarFrom(ie.Holidays),
		"IS": calendarFrom(is.Holidays),
		"IT": calendarFrom(it.Holidays),
		"KE": calendarFrom(ke.Holidays),
		"LT": calendarFrom(lt.Holidays),
		"LU": calendarFrom(lu.Holidays),
		"LV": calendarFrom(lv.Holidays),
		"MT": calendarFrom(mt.Holidays),
		"MW": calendarFrom(mw.Holidays),
		"MX": calendarFrom(mx.Holidays),
		"NC": calendarFrom(nc.Holidays),
		"NL": calendarFrom(nl.Holidays),
		"NO": calendarFrom(no.Holidays),
		"NZ": calendarFrom(nz.Holidays),
		"PL": calendarFrom(pl.Holidays),
		"PT": calendarFrom(pt.Holidays),
		"RO": calendarFrom(ro.Holidays),
		"RS": calendarFrom(rs.Holidays),
		"RU": calendarFrom(ru.Holidays),
		"SE": calendarFrom(se.Holidays),
		"SI": calendarFrom(si.Holidays),
		"SK": calendarFrom(sk.Holidays),
		"UA": calendarFrom(ua.Holidays),
		"US": calendarFrom(us.Holidays),
		"ZA": calendarFrom(za.Holidays),
	}

	offlineCountries = make([]string, 0, len(offlineCalendars))
	for code := range offlineCalendars {
		offlineCountries = append(offlineCountries, code)
	}
	sort.Strings(offlineCountries)
}

// calendarFrom compiles a country's national holiday definitions into a
// calendar. The pack's `Observed` substitution rules are left intact: the
// calendar therefore reports both the civil date and any observed substitute
// date as holidays.
func calendarFrom(holidays []*cal.Holiday) *cal.Calendar {
	c := &cal.Calendar{}
	c.AddHoliday(holidays...)
	return c
}

// offlineHolidays returns the country → calendar table, built once.
func offlineHolidays() map[string]*cal.Calendar {
	offlineOnce.Do(buildOfflineCalendars)
	return offlineCalendars
}

// isOfflineCountry reports whether the bundled offline table covers a country.
func isOfflineCountry(countryCode string) bool {
	_, ok := offlineHolidays()[countryCode]
	return ok
}

// offlineIsHoliday reports whether a date is a public holiday in a country,
// from the offline table. Both the civil date and any observed substitute date
// of a holiday count. An unknown country is an error, not a "false": the caller
// decides what a missing region means for its policy.
func offlineIsHoliday(date time.Time, countryCode string) (bool, error) {
	calendar, ok := offlineHolidays()[strings.ToUpper(strings.TrimSpace(countryCode))]
	if !ok {
		return false, errHolidayUnavailable
	}
	actual, observed, _ := calendar.IsHoliday(date)
	return actual || observed, nil
}

// offlineHolidaysForYear returns the public holidays of a country for a year,
// computed locally: both the actual civil dates and any observed substitute
// dates, deduplicated when a date is both. The second return reports whether
// the offline table has national data for the country at all.
func offlineHolidaysForYear(countryCode string, year int) ([]Holiday, bool) {
	calendar, ok := offlineHolidays()[strings.ToUpper(strings.TrimSpace(countryCode))]
	if !ok {
		return nil, false
	}

	// date→name, deduplicating a date that is both an actual and an observed
	// day of a holiday (and, across holidays, a shared date).
	seen := make(map[string]string)
	add := func(d time.Time, name string) {
		if d.IsZero() || d.Year() != year {
			return
		}
		key := d.Format("2006-01-02")
		if _, dup := seen[key]; !dup {
			seen[key] = name
		}
	}

	for _, h := range calendar.Holidays {
		actual, observed := h.Calc(year)
		add(actual, h.Name)
		add(observed, h.Name)

		// Holidays that wrap the year boundary: a New Year's Day observed on
		// the previous 31 December, or a December civil date observed on the
		// following 1 January, land outside the year the holiday is calculated
		// for. Mirror what cal.Calendar.IsHoliday does so the year list and the
		// per-date lookup can never disagree.
		if h.Observed != nil {
			actualMonth := time.Month(0)
			if !actual.IsZero() {
				actualMonth = actual.Month()
			}
			switch actualMonth {
			case time.January:
				_, wrapped := h.Calc(year + 1)
				add(wrapped, h.Name)
			case time.December:
				_, wrapped := h.Calc(year - 1)
				add(wrapped, h.Name)
			}
		}
	}

	out := make([]Holiday, 0, len(seen))
	for key, name := range seen {
		d, err := time.Parse("2006-01-02", key)
		if err != nil {
			continue
		}
		out = append(out, Holiday{Date: d, Name: name, LocalName: name, Source: "offline"})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out, true
}

// SupportedCountries lists the ISO codes the offline table covers, sorted and
// as a fresh copy: a caller mutating the returned slice cannot corrupt the
// package-level table. The initializer is run unconditionally, so the result
// is well-defined even on a cold package with concurrent callers.
func SupportedCountries() []string {
	offlineOnce.Do(buildOfflineCalendars)
	return append([]string(nil), offlineCountries...)
}
