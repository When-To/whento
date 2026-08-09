// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"encoding/json"
	"testing"
)

func TestNormalizeTimeRange(t *testing.T) {
	tests := []struct {
		name string
		in   TimeRange
		want TimeRange
	}{
		{
			name: "already ordered",
			in:   TimeRange{MinTime: "09:00", MaxTime: "17:00"},
			want: TimeRange{MinTime: "09:00", MaxTime: "17:00"},
		},
		{
			name: "inverted, so swapped",
			in:   TimeRange{MinTime: "17:00", MaxTime: "09:00"},
			want: TimeRange{MinTime: "09:00", MaxTime: "17:00"},
		},
		{
			name: "equal bounds are left alone",
			in:   TimeRange{MinTime: "12:00", MaxTime: "12:00"},
			want: TimeRange{MinTime: "12:00", MaxTime: "12:00"},
		},
		{
			name: "one minute apart, inverted",
			in:   TimeRange{MinTime: "12:01", MaxTime: "12:00"},
			want: TimeRange{MinTime: "12:00", MaxTime: "12:01"},
		},
		{
			name: "midnight to end of day",
			in:   TimeRange{MinTime: "00:00", MaxTime: "23:59"},
			want: TimeRange{MinTime: "00:00", MaxTime: "23:59"},
		},
		{
			name: "end of day to midnight, so swapped",
			in:   TimeRange{MinTime: "23:59", MaxTime: "00:00"},
			want: TimeRange{MinTime: "00:00", MaxTime: "23:59"},
		},
		{
			name: "an empty min disables normalisation",
			in:   TimeRange{MinTime: "", MaxTime: "17:00"},
			want: TimeRange{MinTime: "", MaxTime: "17:00"},
		},
		{
			name: "an empty max disables normalisation",
			in:   TimeRange{MinTime: "09:00", MaxTime: ""},
			want: TimeRange{MinTime: "09:00", MaxTime: ""},
		},
		{
			name: "both empty",
			in:   TimeRange{},
			want: TimeRange{},
		},
		{
			// Unparseable values are passed through untouched so that request
			// validation, not this helper, is what reports them.
			name: "an unparseable min is passed through",
			in:   TimeRange{MinTime: "not-a-time", MaxTime: "09:00"},
			want: TimeRange{MinTime: "not-a-time", MaxTime: "09:00"},
		},
		{
			name: "an unparseable max is passed through",
			in:   TimeRange{MinTime: "17:00", MaxTime: "25:99"},
			want: TimeRange{MinTime: "17:00", MaxTime: "25:99"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeTimeRange(tt.in); got != tt.want {
				t.Errorf("NormalizeTimeRange(%+v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeWeekdayTimes(t *testing.T) {
	t.Run("normalises every entry independently", func(t *testing.T) {
		in := map[string]TimeRange{
			"1": {MinTime: "09:00", MaxTime: "17:00"},
			"2": {MinTime: "17:00", MaxTime: "09:00"},
			"3": {MinTime: "", MaxTime: "12:00"},
		}

		got := NormalizeWeekdayTimes(in)

		want := map[string]TimeRange{
			"1": {MinTime: "09:00", MaxTime: "17:00"},
			"2": {MinTime: "09:00", MaxTime: "17:00"},
			"3": {MinTime: "", MaxTime: "12:00"},
		}

		if len(got) != len(want) {
			t.Fatalf("got %d entries, want %d", len(got), len(want))
		}
		for day, wantRange := range want {
			if got[day] != wantRange {
				t.Errorf("day %s = %+v, want %+v", day, got[day], wantRange)
			}
		}
	})

	t.Run("does not mutate its input", func(t *testing.T) {
		in := map[string]TimeRange{"2": {MinTime: "17:00", MaxTime: "09:00"}}

		NormalizeWeekdayTimes(in)

		if in["2"].MinTime != "17:00" {
			t.Errorf("the input map was mutated: %+v", in["2"])
		}
	})

	t.Run("empty and nil are returned as-is", func(t *testing.T) {
		if got := NormalizeWeekdayTimes(nil); got != nil {
			t.Errorf("NormalizeWeekdayTimes(nil) = %+v, want nil", got)
		}
		if got := NormalizeWeekdayTimes(map[string]TimeRange{}); len(got) != 0 {
			t.Errorf("NormalizeWeekdayTimes(empty) = %+v, want empty", got)
		}
	})
}

func TestNormalizeHolidayTimes(t *testing.T) {
	tests := []struct {
		name             string
		min, max         string
		wantMin, wantMax string
	}{
		{name: "ordered", min: "10:00", max: "18:00", wantMin: "10:00", wantMax: "18:00"},
		{name: "inverted", min: "18:00", max: "10:00", wantMin: "10:00", wantMax: "18:00"},
		{name: "equal", min: "10:00", max: "10:00", wantMin: "10:00", wantMax: "10:00"},
		{name: "empty min", min: "", max: "18:00", wantMin: "", wantMax: "18:00"},
		{name: "empty max", min: "10:00", max: "", wantMin: "10:00", wantMax: ""},
		{name: "both empty", min: "", max: "", wantMin: "", wantMax: ""},
		{name: "unparseable", min: "noon", max: "10:00", wantMin: "noon", wantMax: "10:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin, gotMax := NormalizeHolidayTimes(tt.min, tt.max)
			if gotMin != tt.wantMin || gotMax != tt.wantMax {
				t.Errorf(
					"NormalizeHolidayTimes(%q, %q) = (%q, %q), want (%q, %q)",
					tt.min, tt.max, gotMin, gotMax, tt.wantMin, tt.wantMax,
				)
			}
		})
	}
}

// TestBuildAllowedHoursJSONDefaults pins the fallback that decides what a calendar
// created without explicit hours actually permits: every weekday fully open, and
// holidays left empty, which downstream reads as unrestricted rather than as closed.
func TestBuildAllowedHoursJSONDefaults(t *testing.T) {
	raw, err := BuildAllowedHoursJSON(nil, "", "", "", "")
	if err != nil {
		t.Fatalf("BuildAllowedHoursJSON: %v", err)
	}
	if raw == nil {
		t.Fatal("BuildAllowedHoursJSON returned nil without an error")
	}

	var got AllowedHours
	if err := json.Unmarshal([]byte(*raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Weekdays) != 7 {
		t.Errorf("got %d weekdays, want 7", len(got.Weekdays))
	}
	for _, day := range []string{"0", "1", "2", "3", "4", "5", "6"} {
		slot, ok := got.Weekdays[day]
		if !ok {
			t.Errorf("weekday %q is missing", day)
			continue
		}
		if slot.Start != "00:00" || slot.End != "23:59" {
			t.Errorf("weekday %q = %s-%s, want 00:00-23:59", day, slot.Start, slot.End)
		}
	}

	if got.Holidays.Start != "" || got.Holidays.End != "" {
		t.Errorf("holidays = %+v, want empty (unrestricted)", got.Holidays)
	}
	if got.HolidayEves.Start != "" || got.HolidayEves.End != "" {
		t.Errorf("holiday eves = %+v, want empty (unrestricted)", got.HolidayEves)
	}
}

// TestBuildAllowedHoursJSONPartialWeekdays covers the sharp edge in the defaulting: the
// fallback is all-or-nothing. Supplying a single weekday means the other six are absent
// from the stored JSON entirely, not defaulted to open.
func TestBuildAllowedHoursJSONPartialWeekdays(t *testing.T) {
	raw, err := BuildAllowedHoursJSON(
		map[string]TimeRange{"1": {MinTime: "09:00", MaxTime: "17:00"}},
		"", "", "", "",
	)
	if err != nil {
		t.Fatalf("BuildAllowedHoursJSON: %v", err)
	}

	var got AllowedHours
	if err := json.Unmarshal([]byte(*raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Weekdays) != 1 {
		t.Errorf("got %d weekdays, want 1 — the default only applies to a wholly empty map", len(got.Weekdays))
	}
	if slot := got.Weekdays["1"]; slot.Start != "09:00" || slot.End != "17:00" {
		t.Errorf("weekday 1 = %s-%s, want 09:00-17:00", slot.Start, slot.End)
	}
}

// TestAllowedHoursRoundTrip is the property that matters most: what CreateCalendar
// writes is what buildCalendarResponse reads back.
func TestAllowedHoursRoundTrip(t *testing.T) {
	weekdays := map[string]TimeRange{
		"1": {MinTime: "09:00", MaxTime: "17:00"},
		"5": {MinTime: "10:00", MaxTime: "12:30"},
	}

	raw, err := BuildAllowedHoursJSON(weekdays, "11:00", "15:00", "08:00", "20:00")
	if err != nil {
		t.Fatalf("BuildAllowedHoursJSON: %v", err)
	}

	gotWeekdays, holidayMin, holidayMax, eveMin, eveMax, err := ParseAllowedHoursJSON(raw)
	if err != nil {
		t.Fatalf("ParseAllowedHoursJSON: %v", err)
	}

	if len(gotWeekdays) != len(weekdays) {
		t.Fatalf("got %d weekdays, want %d", len(gotWeekdays), len(weekdays))
	}
	for day, want := range weekdays {
		if gotWeekdays[day] != want {
			t.Errorf("weekday %s = %+v, want %+v", day, gotWeekdays[day], want)
		}
	}

	if holidayMin != "11:00" || holidayMax != "15:00" {
		t.Errorf("holiday window = %s-%s, want 11:00-15:00", holidayMin, holidayMax)
	}
	if eveMin != "08:00" || eveMax != "20:00" {
		t.Errorf("holiday eve window = %s-%s, want 08:00-20:00", eveMin, eveMax)
	}
}

func TestParseAllowedHoursJSON(t *testing.T) {
	empty := ""
	garbage := "{not json"

	tests := []struct {
		name    string
		in      *string
		wantNil bool
		wantErr bool
	}{
		{name: "nil pointer", in: nil, wantNil: true},
		{name: "empty string", in: &empty, wantNil: true},
		{name: "malformed json", in: &garbage, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weekdays, _, _, _, _, err := ParseAllowedHoursJSON(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil && weekdays != nil {
				t.Errorf("weekdays = %+v, want nil", weekdays)
			}
		})
	}
}
