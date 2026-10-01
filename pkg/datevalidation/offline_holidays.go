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
// The block policy turns this into a fail-closed refusal rather than silently
// admitting a day that might be a holiday.
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
	// Observation days (e.g. a US federal holiday bridged to a Monday) are
	// deliberately stripped: an availability rule is written against the actual
	// holiday date. A 4th of July on a Saturday still blocks the 4th — not the
	// Monday banks observe. Sub-countries (states, regions) are out of scope for
	// the current country-level model, so each map entry takes the national set.
	//
	// Not every library pack ships a single national `Holidays` slice: the
	// Australia/Japan/Thailand packs are region-only, so their ISO codes are
	// absent here and the block policy fails closed for them rather than
	// guessing at the national set.
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
// calendar, clearing any observance substitution so only the actual dates count.
func calendarFrom(holidays []*cal.Holiday) *cal.Calendar {
	c := &cal.Calendar{}
	for _, h := range holidays {
		clone := h.Clone(nil)
		clone.Observed = nil
		c.AddHoliday(clone)
	}
	return c
}

// offlineHolidays returns the country → calendar table, built once.
func offlineHolidays() map[string]*cal.Calendar {
	offlineOnce.Do(buildOfflineCalendars)
	return offlineCalendars
}

// offlineIsHoliday reports whether a date is a public holiday in a country,
// from the offline table. An unknown country is an error, not a "false": the
// caller decides what a missing region means for its policy.
func offlineIsHoliday(date time.Time, countryCode string) (bool, error) {
	calendar, ok := offlineHolidays()[strings.ToUpper(strings.TrimSpace(countryCode))]
	if !ok {
		return false, errHolidayUnavailable
	}
	actual, _, _ := calendar.IsHoliday(date)
	return actual, nil
}

// OfflineHoliday is one public holiday as served by ListHolidays.
type OfflineHoliday struct {
	Date time.Time
	Name string
}

// ListHolidays returns the public holidays of a country for a year, computed
// locally. It is what the calendars "holiday" endpoint serves so the frontend
// can render the same data the block/allow policy runs on.
func ListHolidays(countryCode string, year int) ([]OfflineHoliday, error) {
	calendar, ok := offlineHolidays()[strings.ToUpper(strings.TrimSpace(countryCode))]
	if !ok {
		return nil, errHolidayUnavailable
	}

	var out []OfflineHoliday
	for _, h := range calendar.Holidays {
		actual, _ := h.Calc(year)
		if actual.IsZero() {
			continue
		}
		out = append(out, OfflineHoliday{Date: actual, Name: h.Name})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.Before(out[j].Date)
	})
	return out, nil
}

// SupportedCountries lists the ISO codes the offline table covers, so the
// frontend can hide or flag regions without offline data instead of offering a
// policy that would fail closed on every date.
func SupportedCountries() []string {
	if offlineCountries == nil {
		offlineOnce.Do(buildOfflineCalendars)
	}
	return offlineCountries
}
