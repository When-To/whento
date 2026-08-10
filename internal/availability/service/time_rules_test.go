// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/availability/models"
	"github.com/whento/whento/internal/availability/repository"
)

func mustDate(t *testing.T, iso string) time.Time {
	t.Helper()

	parsed, err := time.Parse("2006-01-02", iso)
	if err != nil {
		t.Fatalf("parse %q: %v", iso, err)
	}

	return parsed
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}

	return *s
}

func TestParseAndFormatDate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "a plain date", in: "2026-07-14"},
		{name: "the first of january", in: "2026-01-01"},
		{name: "a leap day", in: "2028-02-29"},
		{name: "a non-existent leap day", in: "2026-02-29", wantErr: true},
		{name: "day and month transposed", in: "14-07-2026", wantErr: true},
		{name: "a full timestamp", in: "2026-07-14T10:00:00Z", wantErr: true},
		{name: "empty", in: "", wantErr: true},
		{name: "month thirteen", in: "2026-13-01", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDate(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDate(%q) succeeded, want an error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDate(%q): %v", tt.in, err)
			}

			// Round trip: what parses must format back to the same string.
			if round := formatDate(got); round != tt.in {
				t.Errorf("formatDate(parseDate(%q)) = %q", tt.in, round)
			}
		})
	}
}

func TestIsValidTime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "midnight", in: "00:00", want: true},
		{name: "end of day", in: "23:59", want: true},
		{name: "midday", in: "12:30", want: true},
		{name: "hour twenty-four", in: "24:00"},
		{name: "minute sixty", in: "12:60"},
		// Go's parser accepts a single-digit hour against the "15" layout, so this
		// is valid whether or not the API meant it to be. The frontend always sends
		// a padded value; the leniency only matters for hand-made requests.
		{name: "no leading zero is accepted", in: "9:00", want: true},
		{name: "with seconds", in: "09:00:00"},
		{name: "empty", in: ""},
		{name: "words", in: "noon"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidTime(tt.in); got != tt.want {
				t.Errorf("isValidTime(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsValidTimeRange pins that the range must be strictly increasing: a zero-length
// slot is not a valid availability.
func TestIsValidTimeRange(t *testing.T) {
	tests := []struct {
		name       string
		start, end string
		want       bool
	}{
		{name: "a normal range", start: "09:00", end: "17:00", want: true},
		{name: "one minute long", start: "09:00", end: "09:01", want: true},
		{name: "the whole day", start: "00:00", end: "23:59", want: true},
		{name: "zero length", start: "09:00", end: "09:00"},
		{name: "inverted", start: "17:00", end: "09:00"},
		{name: "an invalid start", start: "25:00", end: "17:00"},
		{name: "an invalid end", start: "09:00", end: "17:70"},
		{name: "both empty", start: "", end: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidTimeRange(tt.start, tt.end); got != tt.want {
				t.Errorf("isValidTimeRange(%q, %q) = %v, want %v", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestNormalizeTimeRange(t *testing.T) {
	tests := []struct {
		name                     string
		start, end               *string
		wantStart, wantEnd       string
		wantStartNil, wantEndNil bool
	}{
		{name: "ordered", start: ptr("09:00"), end: ptr("17:00"), wantStart: "09:00", wantEnd: "17:00"},
		{name: "inverted, so swapped", start: ptr("17:00"), end: ptr("09:00"), wantStart: "09:00", wantEnd: "17:00"},
		{name: "equal", start: ptr("12:00"), end: ptr("12:00"), wantStart: "12:00", wantEnd: "12:00"},
		{name: "a nil start is passed through", start: nil, end: ptr("17:00"), wantStartNil: true, wantEnd: "17:00"},
		{name: "a nil end is passed through", start: ptr("09:00"), end: nil, wantStart: "09:00", wantEndNil: true},
		{name: "both nil", start: nil, end: nil, wantStartNil: true, wantEndNil: true},
		{name: "an empty start", start: ptr(""), end: ptr("17:00"), wantStart: "", wantEnd: "17:00"},
		{name: "unparseable values are left alone", start: ptr("later"), end: ptr("09:00"), wantStart: "later", wantEnd: "09:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd := normalizeTimeRange(tt.start, tt.end)

			if tt.wantStartNil {
				if gotStart != nil {
					t.Errorf("start = %q, want nil", *gotStart)
				}
			} else if gotStart == nil || *gotStart != tt.wantStart {
				t.Errorf("start = %s, want %q", deref(gotStart), tt.wantStart)
			}

			if tt.wantEndNil {
				if gotEnd != nil {
					t.Errorf("end = %q, want nil", *gotEnd)
				}
			} else if gotEnd == nil || *gotEnd != tt.wantEnd {
				t.Errorf("end = %s, want %q", deref(gotEnd), tt.wantEnd)
			}
		})
	}
}

func TestCalculateDuration(t *testing.T) {
	tests := []struct {
		name       string
		start, end string
		want       float64
	}{
		{name: "a whole hour", start: "09:00", end: "10:00", want: 1},
		{name: "half an hour", start: "09:00", end: "09:30", want: 0.5},
		{name: "a working day", start: "09:00", end: "17:00", want: 8},
		{name: "zero length", start: "09:00", end: "09:00", want: 0},
		// A negative span is clamped rather than reported: the caller sums these.
		{name: "inverted is clamped to zero", start: "17:00", end: "09:00", want: 0},
		{name: "an unparseable start", start: "morning", end: "17:00", want: 0},
		{name: "an unparseable end", start: "09:00", end: "evening", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateDuration(tt.start, tt.end); got != tt.want {
				t.Errorf("calculateDuration(%q, %q) = %v, want %v", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestCompareTime(t *testing.T) {
	tests := []struct {
		name   string
		t1, t2 string
		want   int
	}{
		{name: "earlier", t1: "09:00", t2: "17:00", want: -1},
		{name: "later", t1: "17:00", t2: "09:00", want: 1},
		{name: "equal", t1: "12:00", t2: "12:00", want: 0},
		{name: "one minute apart", t1: "12:00", t2: "12:01", want: -1},
		// Unparseable input collapses to "equal", which silently disables every
		// clamp built on this comparison. Pinned so the behaviour is at least known.
		{name: "an unparseable first operand reads as equal", t1: "noon", t2: "12:00", want: 0},
		{name: "an unparseable second operand reads as equal", t1: "12:00", t2: "noon", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareTime(tt.t1, tt.t2); got != tt.want {
				t.Errorf("compareTime(%q, %q) = %d, want %d", tt.t1, tt.t2, got, tt.want)
			}
		})
	}
}

func TestCombineTimeRanges(t *testing.T) {
	tests := []struct {
		name             string
		special, weekday repository.TimeRange
		want             repository.TimeRange
	}{
		{
			name:    "widest of both",
			special: repository.TimeRange{Start: "10:00", End: "22:00"},
			weekday: repository.TimeRange{Start: "09:00", End: "17:00"},
			want:    repository.TimeRange{Start: "09:00", End: "22:00"},
		},
		{
			name:    "the special range already contains the weekday one",
			special: repository.TimeRange{Start: "08:00", End: "23:00"},
			weekday: repository.TimeRange{Start: "09:00", End: "17:00"},
			want:    repository.TimeRange{Start: "08:00", End: "23:00"},
		},
		{
			name:    "an unconfigured special range yields the weekday one",
			special: repository.TimeRange{},
			weekday: repository.TimeRange{Start: "09:00", End: "17:00"},
			want:    repository.TimeRange{Start: "09:00", End: "17:00"},
		},
		{
			name:    "a half-configured special range counts as unconfigured",
			special: repository.TimeRange{Start: "10:00"},
			weekday: repository.TimeRange{Start: "09:00", End: "17:00"},
			want:    repository.TimeRange{Start: "09:00", End: "17:00"},
		},
		{
			name:    "an unconfigured weekday range yields the special one",
			special: repository.TimeRange{Start: "10:00", End: "22:00"},
			weekday: repository.TimeRange{},
			want:    repository.TimeRange{Start: "10:00", End: "22:00"},
		},
		{
			name:    "identical ranges",
			special: repository.TimeRange{Start: "09:00", End: "17:00"},
			weekday: repository.TimeRange{Start: "09:00", End: "17:00"},
			want:    repository.TimeRange{Start: "09:00", End: "17:00"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := combineTimeRanges(tt.special, tt.weekday); got != tt.want {
				t.Errorf("combineTimeRanges(%+v, %+v) = %+v, want %+v", tt.special, tt.weekday, got, tt.want)
			}
		})
	}
}

// calendarWith builds the repository view of a calendar that the time helpers read.
func calendarWith(weekdays []int, hours repository.AllowedHours, policy string, eves bool) *repository.Calendar {
	return &repository.Calendar{
		ID:               uuid.New(),
		AllowedWeekdays:  weekdays,
		Timezone:         "Europe/Paris",
		HolidaysPolicy:   policy,
		AllowHolidayEves: eves,
		AllowedHours:     hours,
	}
}

// TestGetAllowedTimeRangeForDate is the backend half of the window the frontend
// resolves in dateRules.resolveTimeWindow. The two must agree, or a slot the UI offers
// gets silently clamped on the way in.
//
// Fixtures: 2026-07-14 is Bastille Day, a Tuesday, in France. 2026-12-24 is a Thursday
// and the eve of Christmas.
func TestGetAllowedTimeRangeForDate(t *testing.T) {
	businessHours := repository.AllowedHours{
		Weekdays: map[string]repository.TimeRange{
			"2": {Start: "09:00", End: "17:00"}, // Tuesday
			"4": {Start: "09:00", End: "17:00"}, // Thursday
		},
		Holidays:    repository.TimeRange{Start: "10:00", End: "22:00"},
		HolidayEves: repository.TimeRange{Start: "08:00", End: "20:00"},
	}

	tests := []struct {
		name     string
		date     string
		calendar *repository.Calendar
		want     repository.TimeRange
	}{
		{
			name:     "an ordinary allowed weekday uses its own hours",
			date:     "2026-03-17", // a Tuesday, not a holiday
			calendar: calendarWith([]int{2}, businessHours, "ignore", false),
			want:     repository.TimeRange{Start: "09:00", End: "17:00"},
		},
		{
			name:     "a disallowed weekday yields nothing",
			date:     "2026-03-18", // a Wednesday
			calendar: calendarWith([]int{2}, businessHours, "ignore", false),
			want:     repository.TimeRange{},
		},
		{
			name: "an allowed weekday with no configured hours is fully open",
			date: "2026-03-18",
			calendar: calendarWith([]int{3}, repository.AllowedHours{
				Weekdays: map[string]repository.TimeRange{},
			}, "ignore", false),
			want: repository.TimeRange{Start: "00:00", End: "23:59"},
		},
		{
			name:     "a holiday under the allow policy, on an allowed weekday, widens the window",
			date:     "2026-07-14",
			calendar: calendarWith([]int{2}, businessHours, "allow", false),
			want:     repository.TimeRange{Start: "09:00", End: "22:00"},
		},
		{
			name:     "a holiday under the allow policy, on a disallowed weekday, uses only the holiday hours",
			date:     "2026-07-14",
			calendar: calendarWith([]int{1}, businessHours, "allow", false),
			want:     repository.TimeRange{Start: "10:00", End: "22:00"},
		},
		{
			// Under "ignore" the holiday is not special, so the day falls back to
			// its plain weekday hours rather than the holiday window.
			name:     "a holiday under the ignore policy keeps the weekday hours",
			date:     "2026-07-14",
			calendar: calendarWith([]int{2}, businessHours, "ignore", false),
			want:     repository.TimeRange{Start: "09:00", End: "17:00"},
		},
		{
			name:     "a holiday eve on an allowed weekday widens the window",
			date:     "2026-12-24",
			calendar: calendarWith([]int{4}, businessHours, "ignore", true),
			want:     repository.TimeRange{Start: "08:00", End: "20:00"},
		},
		{
			name:     "a holiday eve on a disallowed weekday uses only the eve hours",
			date:     "2026-12-24",
			calendar: calendarWith([]int{1}, businessHours, "ignore", true),
			want:     repository.TimeRange{Start: "08:00", End: "20:00"},
		},
		{
			name:     "an eve is ignored when eves are disabled",
			date:     "2026-12-24",
			calendar: calendarWith([]int{4}, businessHours, "ignore", false),
			want:     repository.TimeRange{Start: "09:00", End: "17:00"},
		},
		{
			// A timezone with no country disables every holiday lookup, so the
			// special windows become unreachable — the same silent skip that
			// pkg/datevalidation has.
			name: "a timezone with no country falls back to weekday hours",
			date: "2026-07-14",
			calendar: func() *repository.Calendar {
				c := calendarWith([]int{2}, businessHours, "allow", true)
				c.Timezone = "UTC"
				return c
			}(),
			want: repository.TimeRange{Start: "09:00", End: "17:00"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAllowedTimeRangeForDate(mustDate(t, tt.date), tt.calendar)
			if got != tt.want {
				t.Errorf("getAllowedTimeRangeForDate(%s) = %+v, want %+v", tt.date, got, tt.want)
			}
		})
	}
}

// TestAdjustTimesByAllowedHours covers the clamp itself, including the two asymmetric
// defaults: a configured maximum with no minimum implies a 00:00 start, and a
// configured minimum with no maximum implies a 23:59 end.
func TestAdjustTimesByAllowedHours(t *testing.T) {
	openTuesday := func(window repository.TimeRange) *repository.Calendar {
		return calendarWith([]int{2}, repository.AllowedHours{
			Weekdays: map[string]repository.TimeRange{"2": window},
		}, "ignore", false)
	}

	const tuesday = "2026-03-17"

	tests := []struct {
		name               string
		window             repository.TimeRange
		start, end         *string
		wantStart, wantEnd string
		wantStartNil       bool
		wantEndNil         bool
	}{
		{
			name:   "a request inside the window is untouched",
			window: repository.TimeRange{Start: "09:00", End: "17:00"},
			start:  ptr("10:00"), end: ptr("16:00"),
			wantStart: "10:00", wantEnd: "16:00",
		},
		{
			name:   "a start before the window is pulled forward",
			window: repository.TimeRange{Start: "09:00", End: "17:00"},
			start:  ptr("07:00"), end: ptr("16:00"),
			wantStart: "09:00", wantEnd: "16:00",
		},
		{
			name:   "an end after the window is pulled back",
			window: repository.TimeRange{Start: "09:00", End: "17:00"},
			start:  ptr("10:00"), end: ptr("23:00"),
			wantStart: "10:00", wantEnd: "17:00",
		},
		{
			name:   "a request straddling the window is clamped on both sides",
			window: repository.TimeRange{Start: "09:00", End: "17:00"},
			start:  ptr("06:00"), end: ptr("23:00"),
			wantStart: "09:00", wantEnd: "17:00",
		},
		{
			name:   "an all-day request adopts the window",
			window: repository.TimeRange{Start: "09:00", End: "17:00"},
			start:  nil, end: nil,
			wantStart: "09:00", wantEnd: "17:00",
		},
		{
			name:   "empty strings behave like absent times",
			window: repository.TimeRange{Start: "09:00", End: "17:00"},
			start:  ptr(""), end: ptr(""),
			wantStart: "09:00", wantEnd: "17:00",
		},
		{
			name:   "boundaries are inclusive",
			window: repository.TimeRange{Start: "09:00", End: "17:00"},
			start:  ptr("09:00"), end: ptr("17:00"),
			wantStart: "09:00", wantEnd: "17:00",
		},
		{
			name:   "a maximum with no minimum implies a midnight start",
			window: repository.TimeRange{End: "17:00"},
			start:  nil, end: nil,
			wantStart: "00:00", wantEnd: "17:00",
		},
		{
			name:   "a minimum with no maximum implies an end-of-day end",
			window: repository.TimeRange{Start: "09:00"},
			start:  nil, end: nil,
			wantStart: "09:00", wantEnd: "23:59",
		},
		{
			name:   "an unconfigured window passes the request straight through",
			window: repository.TimeRange{},
			start:  ptr("06:00"), end: ptr("23:00"),
			wantStart: "06:00", wantEnd: "23:00",
		},
		{
			name:   "an unconfigured window leaves an all-day request all-day",
			window: repository.TimeRange{},
			start:  nil, end: nil,
			wantStartNil: true, wantEndNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStart, gotEnd := adjustTimesByAllowedHours(
				mustDate(t, tuesday), tt.start, tt.end, openTuesday(tt.window),
			)

			if tt.wantStartNil {
				if gotStart != nil {
					t.Errorf("start = %q, want nil", *gotStart)
				}
			} else if gotStart == nil || *gotStart != tt.wantStart {
				t.Errorf("start = %s, want %q", deref(gotStart), tt.wantStart)
			}

			if tt.wantEndNil {
				if gotEnd != nil {
					t.Errorf("end = %q, want nil", *gotEnd)
				}
			} else if gotEnd == nil || *gotEnd != tt.wantEnd {
				t.Errorf("end = %s, want %q", deref(gotEnd), tt.wantEnd)
			}
		})
	}
}

// TestAdjustTimesByAllowedHoursForWeekday is the recurrence counterpart: a recurrence
// has a weekday but no concrete date, so it cannot consult holidays at all. The two
// clamps must otherwise agree, or a recurrence and a one-off on the same day would be
// stored with different windows.
func TestAdjustTimesByAllowedHoursForWeekday(t *testing.T) {
	calendar := calendarWith([]int{2}, repository.AllowedHours{
		Weekdays:    map[string]repository.TimeRange{"2": {Start: "09:00", End: "17:00"}},
		Holidays:    repository.TimeRange{Start: "00:00", End: "23:59"},
		HolidayEves: repository.TimeRange{Start: "00:00", End: "23:59"},
	}, "allow", true)

	t.Run("clamps to the weekday window", func(t *testing.T) {
		start, end := adjustTimesByAllowedHoursForWeekday(2, ptr("06:00"), ptr("23:00"), calendar)

		if deref(start) != "09:00" || deref(end) != "17:00" {
			t.Errorf("got %s-%s, want 09:00-17:00", deref(start), deref(end))
		}
	})

	t.Run("a disallowed weekday leaves the request alone", func(t *testing.T) {
		start, end := adjustTimesByAllowedHoursForWeekday(3, ptr("06:00"), ptr("23:00"), calendar)

		if deref(start) != "06:00" || deref(end) != "23:00" {
			t.Errorf("got %s-%s, want the request unchanged", deref(start), deref(end))
		}
	})

	t.Run("agrees with the dated clamp on an ordinary day", func(t *testing.T) {
		// 2026-03-17 is a Tuesday and not a holiday, so both paths must produce
		// the same window from the same request.
		byDate, byDateEnd := adjustTimesByAllowedHours(mustDate(t, "2026-03-17"), ptr("06:00"), ptr("23:00"), calendar)
		byWeekday, byWeekdayEnd := adjustTimesByAllowedHoursForWeekday(2, ptr("06:00"), ptr("23:00"), calendar)

		if deref(byDate) != deref(byWeekday) || deref(byDateEnd) != deref(byWeekdayEnd) {
			t.Errorf(
				"the dated and weekday clamps disagree: %s-%s vs %s-%s",
				deref(byDate), deref(byDateEnd), deref(byWeekday), deref(byWeekdayEnd),
			)
		}
	})
}

func TestGetAllowedTimeRangeForWeekday(t *testing.T) {
	calendar := calendarWith([]int{2, 4}, repository.AllowedHours{
		Weekdays: map[string]repository.TimeRange{"2": {Start: "09:00", End: "17:00"}},
	}, "ignore", false)

	tests := []struct {
		name    string
		weekday int
		want    repository.TimeRange
	}{
		{name: "configured hours are returned", weekday: 2, want: repository.TimeRange{Start: "09:00", End: "17:00"}},
		// An allowed weekday with no configured hours is open all day — the same
		// answer getAllowedTimeRangeForDate gives for it.
		{name: "an allowed weekday with no hours is open all day", weekday: 4, want: repository.TimeRange{Start: "00:00", End: "23:59"}},
		// A weekday the calendar rejects has no window: an empty range means "do not
		// clamp", and inventing one would only obscure that validation refuses it.
		{name: "a forbidden weekday yields no window", weekday: 3, want: repository.TimeRange{}},
		{name: "sunday is zero, and also forbidden here", weekday: 0, want: repository.TimeRange{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getAllowedTimeRangeForWeekday(tt.weekday, calendar); got != tt.want {
				t.Errorf("getAllowedTimeRangeForWeekday(%d) = %+v, want %+v", tt.weekday, got, tt.want)
			}
		})
	}
}

// TestFilterParticipantSummariesPrivacy mirrors the calendar-side leak test: under
// lock_participants, only the asker may be identified.
func TestFilterParticipantSummariesPrivacy(t *testing.T) {
	alice := uuid.New()
	bob := uuid.New()

	summaries := []models.ParticipantAvailabilitySummary{
		{ParticipantID: alice, ParticipantName: "Alice", StartTime: ptr("09:00"), EndTime: ptr("17:00")},
		{ParticipantID: bob, ParticipantName: "Bob", StartTime: ptr("10:00"), EndTime: ptr("12:00")},
	}

	tests := []struct {
		name       string
		locked     bool
		asker      string
		wantFirst  bool // the first entry keeps its ID
		wantSecond bool
	}{
		{name: "unlocked, no asker", locked: false, asker: "", wantFirst: true, wantSecond: true},
		{name: "unlocked, with an asker", locked: false, asker: alice.String(), wantFirst: true, wantSecond: true},
		{name: "locked, no asker", locked: true, asker: "", wantFirst: false, wantSecond: false},
		{name: "locked, the asker is identified", locked: true, asker: alice.String(), wantFirst: true, wantSecond: false},
		{name: "locked, a malformed asker id", locked: true, asker: "not-a-uuid", wantFirst: false, wantSecond: false},
		{name: "locked, an unrelated asker", locked: true, asker: uuid.NewString(), wantFirst: false, wantSecond: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterParticipantSummaries(tt.locked, tt.asker, summaries)

			if len(got) != 2 {
				t.Fatalf("got %d summaries, want 2", len(got))
			}
			if (got[0].ParticipantID != nil) != tt.wantFirst {
				t.Errorf("Alice identified = %v, want %v", got[0].ParticipantID != nil, tt.wantFirst)
			}
			if (got[1].ParticipantID != nil) != tt.wantSecond {
				t.Errorf("Bob identified = %v, want %v", got[1].ParticipantID != nil, tt.wantSecond)
			}

			// Names and times are public either way — masking is about identity.
			if got[0].ParticipantName != "Alice" || got[1].ParticipantName != "Bob" {
				t.Error("a participant name was masked")
			}
			if deref(got[1].StartTime) != "10:00" {
				t.Error("a participant's times were masked")
			}
		})
	}
}

func TestIsDuplicateError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil},
		{name: "the exact sentinel message", err: errors.New("availability already exists for this date"), want: true},
		{name: "a different message", err: errors.New("something else"), want: false},
		// The check is an exact string match, so a wrapped error no longer matches.
		// Pinned because it is the kind of thing a later refactor breaks silently.
		{name: "the same message, wrapped", err: errors.New("insert failed: availability already exists for this date"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDuplicateError(tt.err); got != tt.want {
				t.Errorf("isDuplicateError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestCalculateDurationForDate(t *testing.T) {
	tests := []struct {
		name         string
		participants []models.ParticipantAvailabilitySummary
		want         float64
	}{
		// No participants means no times, which the "all day" branch reads as a
		// 24-hour event. Pinned as documentation: GetDateSummary only reaches this
		// with a non-empty list, so the value is never observed, but a future caller
		// that does pass an empty list would get 24 rather than 0.
		{name: "nobody at all is reported as a full day", participants: nil, want: 24},
		{
			name: "everyone is available all day",
			participants: []models.ParticipantAvailabilitySummary{
				{ParticipantID: uuid.New()},
				{ParticipantID: uuid.New()},
			},
			want: 24,
		},
		{
			// One untimed participant is enough to leave the window undetermined
			// on that side, which also collapses to a full day.
			name: "a mix of timed and untimed",
			participants: []models.ParticipantAvailabilitySummary{
				{ParticipantID: uuid.New(), StartTime: ptr("09:00")},
				{ParticipantID: uuid.New()},
			},
			want: 24,
		},
		{
			name: "a single timed availability",
			participants: []models.ParticipantAvailabilitySummary{
				{ParticipantID: uuid.New(), StartTime: ptr("09:00"), EndTime: ptr("17:00")},
			},
			want: 8,
		},
		{
			name: "two participants overlapping for two hours",
			participants: []models.ParticipantAvailabilitySummary{
				{ParticipantID: uuid.New(), StartTime: ptr("09:00"), EndTime: ptr("12:00")},
				{ParticipantID: uuid.New(), StartTime: ptr("10:00"), EndTime: ptr("14:00")},
			},
			want: 2,
		},
		{
			name: "two participants who never overlap",
			participants: []models.ParticipantAvailabilitySummary{
				{ParticipantID: uuid.New(), StartTime: ptr("09:00"), EndTime: ptr("10:00")},
				{ParticipantID: uuid.New(), StartTime: ptr("14:00"), EndTime: ptr("16:00")},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateDurationForDate(tt.participants); got != tt.want {
				t.Errorf("calculateDurationForDate = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// TestWeekdayAndDatedWindowsAgree covers a divergence that used to exist.
//
// For a weekday the calendar allowed but left unconfigured, the dated path treated the
// day as 00:00-23:59 while the weekday path — used for recurrences — returned an empty
// range meaning "do not clamp". An all-day one-off was therefore stored as 00:00-23:59
// while an all-day recurrence on the same weekday kept NULL times. Both render as "all
// day", which is why it went unnoticed, but they were not the same row.
func TestWeekdayAndDatedWindowsAgree(t *testing.T) {
	// Thursday (4) is allowed, but only Tuesday (2) has configured hours.
	calendar := calendarWith([]int{2, 4}, repository.AllowedHours{
		Weekdays: map[string]repository.TimeRange{"2": {Start: "09:00", End: "17:00"}},
	}, "ignore", false)

	cases := []struct {
		name    string
		iso     string
		weekday int
	}{
		// 2026-03-19 is a Thursday: allowed, no configured hours.
		{name: "an allowed weekday with no configured hours", iso: "2026-03-19", weekday: 4},
		// 2026-03-17 is a Tuesday: allowed, with hours.
		{name: "an allowed weekday with configured hours", iso: "2026-03-17", weekday: 2},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			datedStart, datedEnd := adjustTimesByAllowedHours(mustDate(t, tt.iso), nil, nil, calendar)
			weekdayStart, weekdayEnd := adjustTimesByAllowedHoursForWeekday(tt.weekday, nil, nil, calendar)

			if deref(datedStart) != deref(weekdayStart) || deref(datedEnd) != deref(weekdayEnd) {
				t.Errorf(
					"the dated and weekday clamps disagree: %s-%s vs %s-%s",
					deref(datedStart), deref(datedEnd), deref(weekdayStart), deref(weekdayEnd),
				)
			}
		})
	}

	t.Run("an untimed request on an unconfigured weekday becomes a full day", func(t *testing.T) {
		start, end := adjustTimesByAllowedHoursForWeekday(4, nil, nil, calendar)

		if deref(start) != "00:00" || deref(end) != "23:59" {
			t.Errorf("got %s-%s, want 00:00-23:59", deref(start), deref(end))
		}
	})

	t.Run("a weekday the calendar refuses is left unclamped", func(t *testing.T) {
		// Wednesday (3) is not allowed. Validation rejects it before this matters;
		// inventing a window here would only obscure that.
		start, end := adjustTimesByAllowedHoursForWeekday(3, ptr("06:00"), ptr("23:00"), calendar)

		if deref(start) != "06:00" || deref(end) != "23:00" {
			t.Errorf("got %s-%s, want the request unchanged", deref(start), deref(end))
		}
	})
}
