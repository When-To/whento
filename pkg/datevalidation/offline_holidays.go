// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package datevalidation

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/rickar/cal/v2"
	"github.com/rickar/cal/v2/ar"
	"github.com/rickar/cal/v2/at"
	"github.com/rickar/cal/v2/au"
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
	"github.com/rickar/cal/v2/jp"
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
	"github.com/rickar/cal/v2/th"
	"github.com/rickar/cal/v2/ua"
	"github.com/rickar/cal/v2/us"
	"github.com/rickar/cal/v2/za"
)

// errHolidayUnavailable reports a country the offline library has no rules for.
// The block policy used to turn this into a fail-closed refusal; it now fails
// open to the normal weekday decision; the error-aware lookup still reports
// the unavailability explicitly.
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
)

func buildOfflineCalendars() {
	// Observation days are kept deliberately: when a civil holiday lands on a
	// weekend, the library also reports the substitute weekday banks observe,
	// and both dates count as holidays. A 4th of July on a Saturday blocks the
	// 4th and the observed Friday alike. Sub-countries (states, regions) are
	// out of scope for the current country-level model, so each map entry takes
	// the national set.
	//
	// Australia has regional slices only. Select the holidays common across
	// the country explicitly; do not treat state-specific days or ANZAC
	// substitute dates as national. JP and TH ship national Holidays slices.
	offlineCalendars = map[string]*cal.Calendar{
		"AR": calendarFrom(ar.Holidays),
		"AT": calendarFrom(at.Holidays),
		"AU": calendarFrom([]*cal.Holiday{
			au.NewYear, au.AustraliaDay, au.GoodFriday, au.EasterMonday,
			au.AnzacDay, au.ChristmasDay, au.BoxingDay, au.MourningDay2022,
		}),
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
		"JP": calendarFrom(jp.Holidays),
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
		"TH": calendarFrom(th.Holidays),
		"UA": calendarFrom(ua.Holidays),
		"US": calendarFrom(us.Holidays),
		"ZA": calendarFrom(za.Holidays),
	}
}

// calendarFrom compiles a country's national holiday definitions into a
// calendar. The pack's `Observed` substitution rules are left intact: the
// calendar therefore reports both the civil date and any observed substitute
// date as holidays.
func calendarFrom(holidays []*cal.Holiday) *cal.Calendar {
	c := &cal.Calendar{}
	for _, holiday := range holidays {
		copy := holiday.Clone(nil)
		if calculate := copy.Func; calculate != nil {
			// Some upstream calculators (notably Japan's vernal equinox)
			// mutate their argument. Keep compiled rules immutable and give
			// each concurrent calculation its own value.
			copy.Func = func(rule *cal.Holiday, year int) time.Time {
				local := *rule
				return calculate(&local, year)
			}
		}
		c.AddHoliday(copy)
	}
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
