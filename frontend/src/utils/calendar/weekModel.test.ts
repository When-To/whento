/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import type { Availability } from '@/types';
import { formatDate } from '@/utils/date/intlFormatters';
import { isFullDay, timeToMinutes } from '@/utils/date/timeRange';
import { buildCalendarRules } from './dateRules';
import { buildDayIndex } from './dayIndex';
import type { CalendarFormatters, ModelDeps } from './dayModel';
import type { HolidayIndex } from './holidays';
import { buildDayBands, type CoverageSegment } from './segments';
import { buildSlots, buildWeekModel, type WeekOptions } from './weekModel';

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
    isHolidayEve: () => false,
    getName: date => dates[date] ?? null,
  };
}

interface DepsOverrides {
  todayISO?: string;
  allowedWeekdays?: number[];
  weekStartDay?: number;
  availabilities?: Availability[];
  weekdayTimes?: Record<string, { min_time?: string; max_time?: string }>;
}

function makeDeps(overrides: DepsOverrides = {}): ModelDeps {
  return {
    rules: buildCalendarRules({
      timeZone: 'Europe/Paris',
      todayISO: overrides.todayISO ?? '2026-01-01',
      allowedWeekdays: overrides.allowedWeekdays ?? [0, 1, 2, 3, 4, 5, 6],
      holidaysPolicy: 'ignore',
      allowHolidayEves: false,
      threshold: 2,
      weekdayTimes: overrides.weekdayTimes,
    }),
    holidays: holidayIndex(),
    index: buildDayIndex({ availabilities: overrides.availabilities }),
    fmt,
    weekStartDay: overrides.weekStartDay ?? 1,
  };
}

function availability(date: string, start?: string, end?: string): Availability {
  return {
    id: `av-${date}-${start ?? 'all'}`,
    participant_id: 'p1',
    participant_name: 'Ada',
    participant_email_verified: false,
    date,
    start_time: start,
    end_time: end,
    created_at: '',
    updated_at: '',
  };
}

const OPTIONS: WeekOptions = { startHour: 8, endHour: 20, slotDurationMin: 30 };

describe('buildSlots', () => {
  it('divides the visible span into steps of the requested length', () => {
    const slots = buildSlots({ startHour: 8, endHour: 20, slotDurationMin: 30 });

    expect(slots).toHaveLength(24);
    expect(slots[0]).toEqual({ time: '08:00', startMin: 480, isHourStart: true });
    expect(slots[1]).toEqual({ time: '08:30', startMin: 510, isHourStart: false });
    expect(slots[slots.length - 1]).toEqual({ time: '19:30', startMin: 1170, isHourStart: false });
  });

  it('excludes the end hour itself, so the last slot ends on it', () => {
    const slots = buildSlots({ startHour: 8, endHour: 9, slotDurationMin: 15 });

    expect(slots.map(s => s.time)).toEqual(['08:00', '08:15', '08:30', '08:45']);
  });

  it('marks only the slots that begin an hour', () => {
    const slots = buildSlots({ startHour: 8, endHour: 10, slotDurationMin: 20 });

    expect(slots.filter(s => s.isHourStart).map(s => s.time)).toEqual(['08:00', '09:00']);
  });

  it('handles a step that does not divide the hour', () => {
    const slots = buildSlots({ startHour: 8, endHour: 9, slotDurationMin: 25 });

    // 08:00, 08:25, 08:50 — the last one runs past 09:00 and is still emitted,
    // because the loop tests the start of the slot rather than its end.
    expect(slots.map(s => s.time)).toEqual(['08:00', '08:25', '08:50']);
  });

  it('is empty when the window is inverted or degenerate', () => {
    expect(buildSlots({ startHour: 20, endHour: 8, slotDurationMin: 30 })).toEqual([]);
    expect(buildSlots({ startHour: 9, endHour: 9, slotDurationMin: 30 })).toEqual([]);
  });

  it('clamps a non-positive step to one minute rather than looping forever', () => {
    const slots = buildSlots({ startHour: 8, endHour: 9, slotDurationMin: 0 });

    expect(slots).toHaveLength(60);
    expect(slots[0].time).toBe('08:00');
    expect(slots[slots.length - 1].time).toBe('08:59');
  });
});

describe('buildWeekModel', () => {
  it('spans seven days from the start of the week', () => {
    // 2026-01-07 is a Wednesday; with weekStartDay 1 the week runs Mon 5 → Sun 11.
    const model = buildWeekModel('2026-01-07', makeDeps(), OPTIONS);

    expect(model.startISO).toBe('2026-01-05');
    expect(model.endISO).toBe('2026-01-11');
    expect(model.key).toBe('2026-01-05');
    expect(model.days).toHaveLength(7);
    expect(model.days.map(d => d.date)).toEqual([
      '2026-01-05',
      '2026-01-06',
      '2026-01-07',
      '2026-01-08',
      '2026-01-09',
      '2026-01-10',
      '2026-01-11',
    ]);
  });

  it('honours a Sunday week start', () => {
    const model = buildWeekModel('2026-01-07', makeDeps({ weekStartDay: 0 }), OPTIONS);

    expect(model.startISO).toBe('2026-01-04');
    expect(model.endISO).toBe('2026-01-10');
  });

  it('lays cells out row-major, seven days by the slot count', () => {
    const model = buildWeekModel('2026-01-07', makeDeps(), OPTIONS);

    expect(model.slots).toHaveLength(24);
    expect(model.cells).toHaveLength(7 * 24);

    // The first 24 cells are all of Monday, then Tuesday starts.
    expect(model.cells[0].date).toBe('2026-01-05');
    expect(model.cells[23].date).toBe('2026-01-05');
    expect(model.cells[24].date).toBe('2026-01-06');

    // Indices must agree with the position, since the views compute
    // `dayIndex * slotCount + slotIndex` to find a cell.
    model.cells.forEach((cell, position) => {
      expect(cell.dayIndex).toBe(Math.floor(position / 24));
      expect(cell.slotIndex).toBe(position % 24);
    });
  });

  it('keys each cell as date:time, the format the drag selection uses', () => {
    const model = buildWeekModel('2026-01-07', makeDeps(), OPTIONS);

    expect(model.cells[0].key).toBe('2026-01-05:08:00');
    expect(model.cells[1].key).toBe('2026-01-05:08:30');
    expect(new Set(model.cells.map(c => c.key)).size).toBe(model.cells.length);
  });

  it('disables every cell of a day the calendar does not allow', () => {
    // Wednesday (3) excluded. In a Monday-start week that is column index 2.
    const model = buildWeekModel(
      '2026-01-07',
      makeDeps({ allowedWeekdays: [1, 2, 4, 5] }),
      OPTIONS
    );

    const wednesday = model.columns[2];
    expect(wednesday.day.date).toBe('2026-01-07');
    expect(wednesday.enabled).toBe(false);

    const wednesdayCells = model.cells.filter(c => c.dayIndex === 2);
    expect(wednesdayCells.every(c => !c.enabled)).toBe(true);

    // A day that *is* allowed keeps its cells.
    expect(model.columns[0].enabled).toBe(true);
    expect(model.cells.filter(c => c.dayIndex === 0).every(c => c.enabled)).toBe(true);
  });

  it('disables the slots outside a day’s opening hours', () => {
    const model = buildWeekModel(
      '2026-01-07',
      makeDeps({ weekdayTimes: { '1': { min_time: '10:00', max_time: '12:00' } } }),
      OPTIONS
    );

    // Monday is dayIndex 0 and weekday 1, so it carries the restricted window.
    const monday = model.cells.filter(c => c.dayIndex === 0);
    const enabled = monday.filter(c => c.enabled).map(c => c.time);

    expect(enabled).toEqual(['10:00', '10:30', '11:00', '11:30']);
  });

  it('marks the slots the participant already covers', () => {
    const model = buildWeekModel(
      '2026-01-07',
      makeDeps({ availabilities: [availability('2026-01-06', '09:00', '11:00')] }),
      OPTIONS
    );

    const tuesday = model.cells.filter(c => c.dayIndex === 1);
    expect(tuesday.filter(c => c.hasOwn).map(c => c.time)).toEqual([
      '09:00',
      '09:30',
      '10:00',
      '10:30',
    ]);

    // No other day is touched.
    expect(model.cells.filter(c => c.dayIndex !== 1).some(c => c.hasOwn)).toBe(false);
  });

  it('treats an all-day availability as covering the whole column', () => {
    const model = buildWeekModel(
      '2026-01-07',
      makeDeps({ availabilities: [availability('2026-01-06')] }),
      OPTIONS
    );

    const tuesday = model.cells.filter(c => c.dayIndex === 1);
    expect(tuesday.every(c => c.hasOwn)).toBe(true);
    expect(model.columns[1].hasFullDayOwn).toBe(true);
    expect(model.columns[0].hasFullDayOwn).toBe(false);
  });

  it('does not report a timed availability as a full day', () => {
    const model = buildWeekModel(
      '2026-01-07',
      makeDeps({ availabilities: [availability('2026-01-06', '09:00', '11:00')] }),
      OPTIONS
    );

    expect(model.columns[1].hasFullDayOwn).toBe(false);
  });

  it('derives the geometry from the same options as the slots', () => {
    const model = buildWeekModel('2026-01-07', makeDeps(), OPTIONS);

    expect(model.geometry).toEqual({
      firstSlotMin: 480,
      lastSlotEndMin: 1200,
      slotDurationMin: 30,
      slotCount: 24,
    });
    expect(model.geometry.slotCount).toBe(model.slots.length);
  });
});

/**
 * The seam the existing tests miss.
 *
 * `dayModel.test.ts` and `segments.test.ts` both construct a `GridGeometry` by hand, so
 * nothing checks that the geometry `buildWeekModel` actually produces lines up with the
 * slot rows it also produces. A one-slot offset between the two would put every band
 * half a row out and no unit test would notice.
 */
describe('the geometry agrees with the slot rows it is built alongside', () => {
  const cases: { name: string; options: WeekOptions }[] = [
    { name: '30-minute slots over a working day', options: OPTIONS },
    { name: '15-minute slots', options: { startHour: 8, endHour: 20, slotDurationMin: 15 } },
    { name: '60-minute slots', options: { startHour: 0, endHour: 24, slotDurationMin: 60 } },
    { name: 'a narrow window', options: { startHour: 12, endHour: 14, slotDurationMin: 30 } },
  ];

  for (const { name, options } of cases) {
    it(`positions a band on its slot boundary — ${name}`, () => {
      const model = buildWeekModel('2026-01-07', makeDeps(), options);
      const { geometry, slots } = model;

      // A band covering exactly one slot, taken from the middle of the grid.
      const index = Math.floor(slots.length / 2);
      const slot = slots[index];

      const segment: CoverageSegment = {
        startMin: slot.startMin,
        endMin: slot.startMin + options.slotDurationMin,
        count: 1,
        includesCurrent: false,
      };

      const [band] = buildDayBands([segment], [], geometry);

      const span = geometry.lastSlotEndMin - geometry.firstSlotMin;
      const expectedTop = ((slot.startMin - geometry.firstSlotMin) / span) * 100;
      const expectedHeight = (options.slotDurationMin / span) * 100;

      expect(parseFloat(band.top)).toBeCloseTo(expectedTop, 1);
      expect(parseFloat(band.height)).toBeCloseTo(expectedHeight, 1);

      // And the same thing said the other way round: the band's top, as a fraction of
      // the grid, must land on row `index` of `slots.length`.
      expect(Math.round((parseFloat(band.top) / 100) * slots.length)).toBe(index);
    });
  }

  it('places the first slot flush with the top and the last flush with the bottom', () => {
    const model = buildWeekModel('2026-01-07', makeDeps(), OPTIONS);
    const { geometry, slots } = model;

    const first = slots[0];
    const last = slots[slots.length - 1];

    const [firstBand] = buildDayBands(
      [
        {
          startMin: first.startMin,
          endMin: first.startMin + OPTIONS.slotDurationMin,
          count: 1,
          includesCurrent: false,
        },
      ],
      [],
      geometry
    );
    const [lastBand] = buildDayBands(
      [
        {
          startMin: last.startMin,
          endMin: last.startMin + OPTIONS.slotDurationMin,
          count: 1,
          includesCurrent: false,
        },
      ],
      [],
      geometry
    );

    expect(parseFloat(firstBand.top)).toBe(0);
    expect(parseFloat(lastBand.top) + parseFloat(lastBand.height)).toBeCloseTo(100, 1);
  });

  it('clips a band that starts before the visible window', () => {
    const model = buildWeekModel('2026-01-07', makeDeps(), OPTIONS);

    const [band] = buildDayBands(
      [{ startMin: 0, endMin: 9 * 60, count: 1, includesCurrent: false }],
      [],
      model.geometry
    );

    expect(parseFloat(band.top)).toBe(0);
    expect(band.startMin).toBe(model.geometry.firstSlotMin);
  });

  it('drops a band that falls entirely outside the visible window', () => {
    const model = buildWeekModel('2026-01-07', makeDeps(), OPTIONS);

    const bands = buildDayBands(
      [{ startMin: 0, endMin: 7 * 60, count: 1, includesCurrent: false }],
      [],
      model.geometry
    );

    expect(bands).toEqual([]);
  });

  it('agrees with the cell a band sits over', () => {
    // The strongest form of the check: take a real availability, find the cells the
    // model marked as covered, and confirm the band built from the same interval spans
    // exactly those rows.
    const model = buildWeekModel(
      '2026-01-07',
      makeDeps({ availabilities: [availability('2026-01-06', '10:00', '12:00')] }),
      OPTIONS
    );

    const covered = model.cells.filter(c => c.dayIndex === 1 && c.hasOwn);
    expect(covered).toHaveLength(4);

    const [band] = buildDayBands(
      [
        {
          startMin: timeToMinutes('10:00'),
          endMin: timeToMinutes('12:00'),
          count: 1,
          includesCurrent: true,
        },
      ],
      [],
      model.geometry
    );

    const firstRow = Math.round((parseFloat(band.top) / 100) * model.slots.length);
    const rowCount = Math.round((parseFloat(band.height) / 100) * model.slots.length);

    expect(firstRow).toBe(covered[0].slotIndex);
    expect(rowCount).toBe(covered.length);
  });
});
