/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import type { Availability, RecurrenceWithExceptions } from '@/types';
import { formatDate } from '@/utils/date/intlFormatters';
import { isFullDay } from '@/utils/date/timeRange';
import { buildCalendarRules } from './dateRules';
import { buildDayIndex } from './dayIndex';
import {
  buildDayModel,
  buildListDays,
  buildMonthModel,
  buildWeekDays,
  buildWeekdayHeaders,
  type CalendarFormatters,
  type ModelDeps,
} from './dayModel';
import type { HolidayIndex } from './holidays';

const fmt: CalendarFormatters = {
  weekdayShort: date => formatDate(date, 'en', 'weekdayShort'),
  weekdayLong: date => formatDate(date, 'en', 'weekdayLong'),
  dayMonthShort: date => formatDate(date, 'en', 'dayMonthShort'),
  fullDate: date => formatDate(date, 'en', 'fullDate'),
  monthYear: date => formatDate(date, 'en', 'monthYear'),
  timeRange: (start, end) => (isFullDay(start, end) ? 'All day' : `${start}-${end}`),
  dayAria: (fullDate, count, threshold) => `${fullDate}, ${count}/${threshold}`,
};

function holidayIndex(dates: Record<string, string> = {}): HolidayIndex {
  return {
    countryCode: 'FR',
    isHoliday: date => date in dates,
    isHolidayEve: date => {
      const next = new Date(
        Number(date.slice(0, 4)),
        Number(date.slice(5, 7)) - 1,
        Number(date.slice(8, 10)) + 1
      );
      const iso = `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, '0')}-${String(
        next.getDate()
      ).padStart(2, '0')}`;
      return iso in dates;
    },
    getName: date => dates[date] ?? null,
  };
}

interface DepsOverrides {
  todayISO?: string;
  allowedWeekdays?: number[];
  weekStartDay?: number;
  threshold?: number;
  startDate?: string;
  endDate?: string;
  availabilities?: Availability[];
  recurrences?: RecurrenceWithExceptions[];
  participantCounts?: Record<string, number>;
  holidays?: Record<string, string>;
  highlighted?: Set<string>;
}

function makeDeps(overrides: DepsOverrides = {}): ModelDeps {
  return {
    rules: buildCalendarRules({
      timeZone: 'Europe/Paris',
      todayISO: overrides.todayISO ?? '2026-04-01',
      allowedWeekdays: overrides.allowedWeekdays ?? [0, 1, 2, 3, 4, 5, 6],
      holidaysPolicy: 'ignore',
      allowHolidayEves: false,
      startDate: overrides.startDate,
      endDate: overrides.endDate,
      threshold: overrides.threshold ?? 3,
    }),
    holidays: holidayIndex(overrides.holidays),
    index: buildDayIndex({
      availabilities: overrides.availabilities,
      recurrences: overrides.recurrences,
      participantCounts: overrides.participantCounts,
    }),
    fmt,
    weekStartDay: overrides.weekStartDay ?? 1,
    highlighted: overrides.highlighted,
  };
}

function availability(date: string, extra: Partial<Availability> = {}): Availability {
  return {
    id: `av-${date}`,
    participant_id: 'p1',
    participant_name: 'Ada',
    participant_email_verified: false,
    date,
    created_at: '',
    updated_at: '',
    ...extra,
  };
}

describe('density', () => {
  it('is the progress toward the threshold, capped at one', () => {
    const deps = makeDeps({ threshold: 4, participantCounts: { '2026-04-08': 2 } });
    expect(buildDayModel('2026-04-08', deps).density).toBe(0.5);
  });

  it('saturates once the threshold is met', () => {
    const deps = makeDeps({ threshold: 4, participantCounts: { '2026-04-08': 9 } });
    expect(buildDayModel('2026-04-08', deps).density).toBe(1);
  });

  it('is zero when nobody is available', () => {
    expect(buildDayModel('2026-04-08', makeDeps()).density).toBe(0);
  });
});

describe('buildDayModel status precedence', () => {
  it('marks a filler day as outside, whatever else is true', () => {
    const deps = makeDeps({ participantCounts: { '2026-04-08': 9 } });
    expect(buildDayModel('2026-04-08', deps, false).status).toBe('outside');
  });

  it('marks a past day as disabled', () => {
    const deps = makeDeps({ todayISO: '2026-04-10' });
    expect(buildDayModel('2026-04-08', deps).status).toBe('disabled');
  });

  it('marks an out-of-range day as disabled', () => {
    const deps = makeDeps({ endDate: '2026-04-05' });
    expect(buildDayModel('2026-04-08', deps).status).toBe('disabled');
  });

  it('marks a disallowed weekday as disabled', () => {
    // 2026-04-08 is a Wednesday (3).
    const deps = makeDeps({ allowedWeekdays: [1, 2, 4, 5] });
    expect(buildDayModel('2026-04-08', deps).status).toBe('disabled');
  });

  it('prefers threshold over the participant own answer', () => {
    const deps = makeDeps({
      threshold: 2,
      participantCounts: { '2026-04-08': 3 },
      availabilities: [availability('2026-04-08')],
    });
    expect(buildDayModel('2026-04-08', deps).status).toBe('threshold');
  });

  it('prefers an own availability over a recurrence', () => {
    const deps = makeDeps({
      availabilities: [availability('2026-04-08')],
      recurrences: [
        {
          id: 'r1',
          participant_id: 'p1',
          day_of_week: 3,
          start_date: '2026-01-01',
          created_at: '',
          exceptions: [],
        },
      ],
    });
    expect(buildDayModel('2026-04-08', deps).status).toBe('available');
  });

  it('falls back to recurring, then free', () => {
    const recurring = makeDeps({
      recurrences: [
        {
          id: 'r1',
          participant_id: 'p1',
          day_of_week: 3,
          start_date: '2026-01-01',
          created_at: '',
          exceptions: [],
        },
      ],
    });
    expect(buildDayModel('2026-04-08', recurring).status).toBe('recurring');
    expect(buildDayModel('2026-04-08', makeDeps()).status).toBe('free');
  });
});

describe('buildDayModel fields', () => {
  it('preformats every label exactly once', () => {
    const deps = makeDeps({
      availabilities: [availability('2026-04-08', { start_time: '09:00', end_time: '12:00' })],
    });
    const day = buildDayModel('2026-04-08', deps);

    expect(day.weekdayLong).toBe('Wednesday');
    expect(day.weekdayShort).toBe('Wed');
    expect(day.dateShort).toBe('Apr 8');
    expect(day.dateLong).toBe('April 8, 2026');
    expect(day.own?.label).toBe('09:00-12:00');
    expect(day.own?.isFullDay).toBe(false);
    expect(day.ariaLabel).toBe('April 8, 2026, 0/3');
  });

  it('labels an all-day availability', () => {
    const deps = makeDeps({ availabilities: [availability('2026-04-08')] });
    const day = buildDayModel('2026-04-08', deps);
    expect(day.own?.isFullDay).toBe(true);
    expect(day.own?.label).toBe('All day');
  });

  it('labels an explicit 00:00-23:59 as all day too', () => {
    // What the backend stores for an all-day answer. The month cell used to format
    // this itself and rendered the literal range.
    const deps = makeDeps({
      availabilities: [availability('2026-04-08', { start_time: '00:00', end_time: '23:59' })],
    });
    const day = buildDayModel('2026-04-08', deps);
    expect(day.own?.isFullDay).toBe(true);
    expect(day.own?.label).toBe('All day');
    expect(day.ownAll[0].label).toBe('All day');
  });

  it('exposes every own availability, not just the first', () => {
    const deps = makeDeps({
      availabilities: [
        availability('2026-04-08', { start_time: '09:00', end_time: '12:00' }),
        availability('2026-04-08', { start_time: '14:00', end_time: '18:00' }),
      ],
    });
    const day = buildDayModel('2026-04-08', deps);
    expect(day.ownAll).toHaveLength(2);
    expect(day.own?.startTime).toBe('09:00');
    // Every entry is preformatted, so no component ever formats a range itself.
    expect(day.ownAll.map(a => a.label)).toEqual(['09:00-12:00', '14:00-18:00']);
    expect(new Set(day.ownAll.map(a => a.key)).size).toBe(2);
  });

  it('marks today using the calendar timezone', () => {
    const deps = makeDeps({ todayISO: '2026-04-08' });
    expect(buildDayModel('2026-04-08', deps).isToday).toBe(true);
    expect(buildDayModel('2026-04-09', deps).isToday).toBe(false);
  });

  it('carries the highlight flag', () => {
    const deps = makeDeps({ highlighted: new Set(['2026-04-08']) });
    expect(buildDayModel('2026-04-08', deps).isHighlighted).toBe(true);
    expect(buildDayModel('2026-04-09', deps).isHighlighted).toBe(false);
  });

  it('carries the holiday name and eve flag', () => {
    const deps = makeDeps({ holidays: { '2026-05-01': 'Labour Day' } });
    expect(buildDayModel('2026-05-01', deps).holidayName).toBe('Labour Day');
    expect(buildDayModel('2026-05-01', deps).isHoliday).toBe(true);
    expect(buildDayModel('2026-04-30', deps).isHolidayEve).toBe(true);
    expect(buildDayModel('2026-04-29', deps).isHolidayEve).toBe(false);
  });
});

describe('buildMonthModel', () => {
  it('always returns whole weeks', () => {
    for (let month = 0; month < 12; month++) {
      for (const weekStartDay of [0, 1]) {
        const model = buildMonthModel(2026, month, makeDeps({ weekStartDay }));
        expect(model.days.length % 7).toBe(0);
      }
    }
  });

  it('starts the grid on the locale first day', () => {
    // 2026-03-01 is a Sunday.
    const sundayFirst = buildMonthModel(2026, 2, makeDeps({ weekStartDay: 0 }));
    expect(sundayFirst.days[0].date).toBe('2026-03-01');
    expect(sundayFirst.days[0].isCurrentMonth).toBe(true);

    const mondayFirst = buildMonthModel(2026, 2, makeDeps({ weekStartDay: 1 }));
    // A Monday-first grid must back up to the previous Monday, 2026-02-23.
    expect(mondayFirst.days[0].date).toBe('2026-02-23');
    expect(mondayFirst.days[0].isCurrentMonth).toBe(false);
  });

  it('is chronological with no gaps or duplicates', () => {
    const model = buildMonthModel(2026, 3, makeDeps());
    const dates = model.days.map(d => d.date);
    expect(new Set(dates).size).toBe(dates.length);
    for (let i = 1; i < dates.length; i++) {
      expect(dates[i] > dates[i - 1]).toBe(true);
    }
  });

  it('flags exactly the days of the month as current', () => {
    const model = buildMonthModel(2026, 3, makeDeps());
    const current = model.days.filter(d => d.isCurrentMonth);
    expect(current).toHaveLength(30);
    expect(current[0].date).toBe('2026-04-01');
    expect(current[29].date).toBe('2026-04-30');
  });

  it('spans a year boundary correctly', () => {
    const december = buildMonthModel(2026, 11, makeDeps());
    expect(december.days.some(d => d.date.startsWith('2027-01'))).toBe(true);
    const january = buildMonthModel(2027, 0, makeDeps({ weekStartDay: 1 }));
    expect(january.days.some(d => d.date.startsWith('2026-12'))).toBe(true);
  });

  it('lays out for the CSS transposition: index % 7 is a stable weekday', () => {
    // The compact layout relies on `grid-auto-flow: column` over seven rows, which
    // only lines up if every seventh day shares a weekday.
    const model = buildMonthModel(2026, 3, makeDeps({ weekStartDay: 1 }));
    for (let i = 7; i < model.days.length; i++) {
      expect(model.days[i].dayOfWeek).toBe(model.days[i - 7].dayOfWeek);
    }
  });

  it('labels the month', () => {
    expect(buildMonthModel(2026, 3, makeDeps()).label).toBe('April 2026');
    expect(buildMonthModel(2026, 3, makeDeps()).key).toBe('2026-3');
  });
});

describe('buildWeekdayHeaders', () => {
  it('starts on Monday for a Monday-first locale', () => {
    expect(buildWeekdayHeaders(makeDeps({ weekStartDay: 1 }))).toEqual([
      'Mon',
      'Tue',
      'Wed',
      'Thu',
      'Fri',
      'Sat',
      'Sun',
    ]);
  });

  it('starts on Sunday for a Sunday-first locale', () => {
    expect(buildWeekdayHeaders(makeDeps({ weekStartDay: 0 }))).toEqual([
      'Sun',
      'Mon',
      'Tue',
      'Wed',
      'Thu',
      'Fri',
      'Sat',
    ]);
  });
});

describe('buildWeekDays', () => {
  it('returns the seven days of the containing week', () => {
    // 2026-04-08 is a Wednesday.
    const monday = buildWeekDays('2026-04-08', makeDeps({ weekStartDay: 1 }));
    expect(monday.map(d => d.date)).toEqual([
      '2026-04-06',
      '2026-04-07',
      '2026-04-08',
      '2026-04-09',
      '2026-04-10',
      '2026-04-11',
      '2026-04-12',
    ]);

    const sunday = buildWeekDays('2026-04-08', makeDeps({ weekStartDay: 0 }));
    expect(sunday[0].date).toBe('2026-04-05');
  });

  it('keeps seven distinct days across a DST boundary', () => {
    // 2026-03-29 is the European spring-forward.
    const week = buildWeekDays('2026-03-29', makeDeps({ weekStartDay: 1 }));
    expect(week).toHaveLength(7);
    expect(new Set(week.map(d => d.date)).size).toBe(7);
    expect(week.map(d => d.dayOfWeek)).toEqual([1, 2, 3, 4, 5, 6, 0]);
  });
});

describe('buildListDays', () => {
  it('drops disabled days and keeps the rest in order', () => {
    const deps = makeDeps({ todayISO: '2026-04-08', allowedWeekdays: [1, 2, 3, 4, 5] });
    const days = buildListDays([{ startISO: '2026-04-06', endISO: '2026-04-13' }], deps);

    // 06 and 07 are past; 11 (Sat) and 12 (Sun) are disallowed weekdays.
    expect(days.map(d => d.date)).toEqual(['2026-04-08', '2026-04-09', '2026-04-10', '2026-04-13']);
  });

  it('deduplicates overlapping ranges', () => {
    const deps = makeDeps();
    const days = buildListDays(
      [
        { startISO: '2026-04-06', endISO: '2026-04-10' },
        { startISO: '2026-04-08', endISO: '2026-04-12' },
      ],
      deps
    );
    expect(new Set(days.map(d => d.date)).size).toBe(days.length);
    expect(days).toHaveLength(7);
  });

  it('sorts across ranges given out of order', () => {
    const deps = makeDeps();
    const days = buildListDays(
      [
        { startISO: '2026-05-01', endISO: '2026-05-02' },
        { startISO: '2026-04-06', endISO: '2026-04-07' },
      ],
      deps
    );
    expect(days.map(d => d.date)).toEqual(['2026-04-06', '2026-04-07', '2026-05-01', '2026-05-02']);
  });

  it('returns nothing for an inverted range', () => {
    expect(buildListDays([{ startISO: '2026-04-10', endISO: '2026-04-06' }], makeDeps())).toEqual(
      []
    );
  });
});
