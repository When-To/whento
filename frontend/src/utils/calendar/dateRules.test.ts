/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import type { HolidaysPolicy, TimeRange } from '@/types';
import {
  buildCalendarRules,
  isDayAllowed,
  isDayOpen,
  isInRange,
  isPast,
  isSlotAllowed,
  resolveTimeWindow,
  type CalendarRules,
  type DayContext,
} from './dateRules';

const BASE = buildCalendarRules({
  timeZone: 'Europe/Paris',
  todayISO: '2026-04-06',
  allowedWeekdays: [1, 2, 3, 4, 5],
  holidaysPolicy: 'ignore',
  allowHolidayEves: false,
});

function rules(overrides: Partial<CalendarRules>): CalendarRules {
  return { ...BASE, ...overrides };
}

function day(overrides: Partial<DayContext> = {}): DayContext {
  return {
    date: '2026-04-08',
    dayOfWeek: 3,
    isHoliday: false,
    isHolidayEve: false,
    ...overrides,
  };
}

describe('isInRange', () => {
  const bounded = rules({ startDate: '2026-04-05', endDate: '2026-04-30' });

  it('is inclusive on both boundary dates', () => {
    // The month grid used to disagree with the other two views here, because it
    // parsed the bounds as UTC midnight and then normalised to local midnight.
    expect(isInRange('2026-04-05', bounded)).toBe(true);
    expect(isInRange('2026-04-30', bounded)).toBe(true);
  });

  it('excludes dates outside the bounds', () => {
    expect(isInRange('2026-04-04', bounded)).toBe(false);
    expect(isInRange('2026-05-01', bounded)).toBe(false);
  });

  it('accepts everything when unbounded', () => {
    expect(isInRange('1999-01-01', BASE)).toBe(true);
    expect(isInRange('2099-01-01', BASE)).toBe(true);
  });

  it('honours a start bound with no end, and the reverse', () => {
    expect(isInRange('2026-04-04', rules({ startDate: '2026-04-05' }))).toBe(false);
    expect(isInRange('2099-01-01', rules({ startDate: '2026-04-05' }))).toBe(true);
    expect(isInRange('1999-01-01', rules({ endDate: '2026-04-30' }))).toBe(true);
    expect(isInRange('2026-05-01', rules({ endDate: '2026-04-30' }))).toBe(false);
  });
});

describe('isPast', () => {
  it('compares against today in the calendar timezone', () => {
    expect(isPast('2026-04-05', BASE)).toBe(true);
    expect(isPast('2026-04-06', BASE)).toBe(false);
    expect(isPast('2026-04-07', BASE)).toBe(false);
  });
});

describe('isDayAllowed', () => {
  it('accepts configured weekdays and rejects the others', () => {
    expect(isDayAllowed(day({ dayOfWeek: 3 }), BASE)).toBe(true);
    expect(isDayAllowed(day({ dayOfWeek: 0 }), BASE)).toBe(false);
    expect(isDayAllowed(day({ dayOfWeek: 6 }), BASE)).toBe(false);
  });

  it('rejects every weekday when the list is empty', () => {
    // Matches IsWeekdayAllowed in pkg/datevalidation. The shared frontend composable
    // used to treat an empty list as "no restriction", enabling days the backend
    // would then refuse to write.
    const none = rules({ allowedWeekdays: [] });
    for (let dow = 0; dow < 7; dow++) {
      expect(isDayAllowed(day({ dayOfWeek: dow }), none)).toBe(false);
    }
  });

  describe('holidays policy', () => {
    const holiday = day({ isHoliday: true });

    it("'block' rejects a holiday even on an allowed weekday", () => {
      expect(isDayAllowed(holiday, rules({ holidaysPolicy: 'block' }))).toBe(false);
      expect(isDayAllowed({ ...holiday, dayOfWeek: 0 }, rules({ holidaysPolicy: 'block' }))).toBe(
        false
      );
    });

    it("'allow' accepts a holiday even on a disallowed weekday", () => {
      const allow = rules({ holidaysPolicy: 'allow' });
      expect(isDayAllowed(holiday, allow)).toBe(true);
      expect(isDayAllowed({ ...holiday, dayOfWeek: 0 }, allow)).toBe(true);
    });

    it("'ignore' falls back to the weekday rule", () => {
      expect(isDayAllowed(holiday, BASE)).toBe(true);
      expect(isDayAllowed({ ...holiday, dayOfWeek: 0 }, BASE)).toBe(false);
    });
  });

  describe('holiday eves', () => {
    const eve = day({ isHolidayEve: true, dayOfWeek: 0 });

    it('rescues a disallowed weekday when eves are enabled', () => {
      expect(isDayAllowed(eve, BASE)).toBe(false);
      expect(isDayAllowed(eve, rules({ allowHolidayEves: true }))).toBe(true);
    });

    it('does not override a blocked holiday', () => {
      const both = day({ isHoliday: true, isHolidayEve: true, dayOfWeek: 0 });
      expect(isDayAllowed(both, rules({ holidaysPolicy: 'block', allowHolidayEves: true }))).toBe(
        false
      );
    });
  });
});

describe('isDayOpen', () => {
  const bounded = rules({ startDate: '2026-04-05', endDate: '2026-04-30' });

  it('requires the date to be present, in range and allowed', () => {
    expect(isDayOpen(day({ date: '2026-04-08', dayOfWeek: 3 }), bounded)).toBe(true);
    expect(isDayOpen(day({ date: '2026-04-05', dayOfWeek: 0 }), bounded)).toBe(false); // past
    expect(isDayOpen(day({ date: '2026-05-06', dayOfWeek: 3 }), bounded)).toBe(false); // out of range
    expect(isDayOpen(day({ date: '2026-04-11', dayOfWeek: 6 }), bounded)).toBe(false); // weekday
  });
});

describe('resolveTimeWindow', () => {
  const nineToFive: TimeRange = { min_time: '09:00', max_time: '17:00' };

  it('is unrestricted when no windows are configured', () => {
    expect(resolveTimeWindow(day(), BASE)).toEqual({ allowed: true, minMin: 0, maxMin: 1440 });
  });

  it('applies the weekday window', () => {
    const withTimes = rules({ weekdayTimes: { 3: nineToFive } });
    expect(resolveTimeWindow(day({ dayOfWeek: 3 }), withTimes)).toEqual({
      allowed: true,
      minMin: 540,
      maxMin: 1020,
    });
  });

  it('denies past and out-of-range days', () => {
    expect(resolveTimeWindow(day({ date: '2026-04-05' }), BASE)).toEqual({ allowed: false });
    expect(
      resolveTimeWindow(day({ date: '2026-05-06' }), rules({ endDate: '2026-04-30' }))
    ).toEqual({ allowed: false });
  });

  it('uses the holiday window under the allow policy', () => {
    const allow = rules({
      holidaysPolicy: 'allow',
      holidayTimes: { min_time: '10:00', max_time: '14:00' },
    });
    expect(resolveTimeWindow(day({ isHoliday: true, dayOfWeek: 0 }), allow)).toEqual({
      allowed: true,
      minMin: 600,
      maxMin: 840,
    });
  });

  it('unions the holiday and weekday windows on an allowed weekday', () => {
    const allow = rules({
      holidaysPolicy: 'allow',
      holidayTimes: { min_time: '10:00', max_time: '14:00' },
      weekdayTimes: { 3: nineToFive },
    });
    // Earliest start (09:00) and latest end (17:00).
    expect(resolveTimeWindow(day({ isHoliday: true, dayOfWeek: 3 }), allow)).toEqual({
      allowed: true,
      minMin: 540,
      maxMin: 1020,
    });
  });

  it('uses the holiday-eve window when eves are enabled', () => {
    const eves = rules({
      allowHolidayEves: true,
      holidayEveTimes: { min_time: '08:00', max_time: '12:00' },
    });
    expect(resolveTimeWindow(day({ isHolidayEve: true, dayOfWeek: 0 }), eves)).toEqual({
      allowed: true,
      minMin: 480,
      maxMin: 720,
    });
  });

  it('falls back to the weekday window for an eve when eves are disabled', () => {
    const noEves = rules({ weekdayTimes: { 3: nineToFive } });
    expect(resolveTimeWindow(day({ isHolidayEve: true, dayOfWeek: 3 }), noEves)).toEqual({
      allowed: true,
      minMin: 540,
      maxMin: 1020,
    });
    expect(resolveTimeWindow(day({ isHolidayEve: true, dayOfWeek: 0 }), noEves)).toEqual({
      allowed: false,
    });
  });
});

describe('isSlotAllowed', () => {
  const window = { allowed: true, minMin: 540, maxMin: 1020 } as const;

  it('accepts any slot that overlaps the window', () => {
    expect(isSlotAllowed(window, 540, 555)).toBe(true); // exactly at the start
    expect(isSlotAllowed(window, 525, 540)).toBe(false); // ends exactly at the start
    expect(isSlotAllowed(window, 530, 545)).toBe(true); // straddles the start
    expect(isSlotAllowed(window, 1005, 1020)).toBe(true); // ends exactly at the end
    expect(isSlotAllowed(window, 1020, 1035)).toBe(false); // starts exactly at the end
    expect(isSlotAllowed(window, 1015, 1030)).toBe(true); // straddles the end
  });

  it('rejects everything on a denied day', () => {
    expect(isSlotAllowed({ allowed: false }, 540, 555)).toBe(false);
  });
});

/**
 * Verbatim transcription of `isTimeSlotAllowed` from the previous WeeklyCalendarGrid,
 * used as the oracle for the port. It is 120 lines with three branches that each
 * duplicate the same "widest range" merge, which is exactly why it needed a
 * differential test rather than a reading.
 */
function legacyIsTimeSlotAllowed(input: {
  dayOfWeek: number;
  time: string;
  slotDuration: number;
  isHoliday: boolean;
  isHolidayEve: boolean;
  allowedWeekdays: number[];
  holidaysPolicy: string;
  allowHolidayEves: boolean;
  weekdayTimes?: Record<string, TimeRange>;
  holidayMinTime?: string;
  holidayMaxTime?: string;
  holidayEveMinTime?: string;
  holidayEveMaxTime?: string;
}): boolean {
  const toMin = (t: string) => {
    const [h, m] = t.split(':').map(Number);
    return h * 60 + m;
  };
  const dayOfWeek = input.dayOfWeek;
  const isHoliday = input.isHoliday;
  const isHolidayEve = input.isHolidayEve;
  const isDayAllowedLegacy = input.allowedWeekdays && input.allowedWeekdays.includes(dayOfWeek);
  const wt = input.weekdayTimes;

  let dayIsEnabled: boolean;
  let minTime: string | undefined;
  let maxTime: string | undefined;

  if (isHoliday) {
    if (input.holidaysPolicy === 'allow') {
      minTime = input.holidayMinTime;
      maxTime = input.holidayMaxTime;
      dayIsEnabled = true;
    } else if (input.holidaysPolicy === 'block') {
      return false;
    } else {
      if (!isDayAllowedLegacy) return false;
      if (wt && wt[dayOfWeek]) {
        minTime = wt[dayOfWeek].min_time;
        maxTime = wt[dayOfWeek].max_time;
      }
      dayIsEnabled = true;
    }

    if (dayIsEnabled && isDayAllowedLegacy && wt && wt[dayOfWeek]) {
      const weekdayMin = wt[dayOfWeek].min_time;
      const weekdayMax = wt[dayOfWeek].max_time;
      if (minTime && weekdayMin) minTime = minTime < weekdayMin ? minTime : weekdayMin;
      else minTime = minTime || weekdayMin;
      if (maxTime && weekdayMax) maxTime = maxTime > weekdayMax ? maxTime : weekdayMax;
      else maxTime = maxTime || weekdayMax;
    }
  } else if (isHolidayEve) {
    if (input.allowHolidayEves) {
      minTime = input.holidayEveMinTime;
      maxTime = input.holidayEveMaxTime;
      dayIsEnabled = true;
    } else {
      if (!isDayAllowedLegacy) return false;
      if (wt && wt[dayOfWeek]) {
        minTime = wt[dayOfWeek].min_time;
        maxTime = wt[dayOfWeek].max_time;
      }
      dayIsEnabled = true;
    }

    if (dayIsEnabled && isDayAllowedLegacy && wt && wt[dayOfWeek]) {
      const weekdayMin = wt[dayOfWeek].min_time;
      const weekdayMax = wt[dayOfWeek].max_time;
      if (minTime && weekdayMin) minTime = minTime < weekdayMin ? minTime : weekdayMin;
      else minTime = minTime || weekdayMin;
      if (maxTime && weekdayMax) maxTime = maxTime > weekdayMax ? maxTime : weekdayMax;
      else maxTime = maxTime || weekdayMax;
    }
  } else {
    if (!isDayAllowedLegacy) return false;
    if (wt && wt[dayOfWeek]) {
      minTime = wt[dayOfWeek].min_time;
      maxTime = wt[dayOfWeek].max_time;
    }
    dayIsEnabled = true;
  }

  if (!dayIsEnabled) return false;
  if (!minTime && !maxTime) return true;

  const slotStartMin = toMin(input.time);
  const slotEndMin = slotStartMin + input.slotDuration;
  const minTimeMin = minTime ? toMin(minTime) : 0;
  const maxTimeMin = maxTime ? toMin(maxTime) : 24 * 60;

  return slotStartMin < maxTimeMin && slotEndMin > minTimeMin;
}

describe('differential: resolveTimeWindow + isSlotAllowed vs the legacy implementation', () => {
  const POLICIES: HolidaysPolicy[] = ['ignore', 'allow', 'block'];
  const WEEKDAY_SETS: number[][] = [[], [1, 2, 3, 4, 5], [0, 1, 2, 3, 4, 5, 6], [3]];
  const TIME_RANGES: (TimeRange | undefined)[] = [
    undefined,
    { min_time: '09:00', max_time: '17:00' },
    { min_time: '10:00', max_time: '14:00' },
    { min_time: '08:00' },
    { max_time: '12:00' },
  ];
  const SLOT_TIMES = ['00:00', '07:45', '08:00', '09:00', '13:30', '16:45', '17:00', '23:45'];
  const SLOT_DURATION = 15;

  it('agrees on every combination of day kind, policy, weekday set and time windows', () => {
    let compared = 0;

    for (const policy of POLICIES) {
      for (const allowHolidayEves of [false, true]) {
        for (const allowedWeekdays of WEEKDAY_SETS) {
          for (const weekdayRange of TIME_RANGES) {
            for (const specialRange of TIME_RANGES) {
              for (const [isHoliday, isHolidayEve] of [
                [false, false],
                [true, false],
                [false, true],
                [true, true],
              ] as const) {
                for (let dayOfWeek = 0; dayOfWeek < 7; dayOfWeek++) {
                  const weekdayTimes = weekdayRange
                    ? { [String(dayOfWeek)]: weekdayRange }
                    : undefined;

                  const ported = rules({
                    // Neutralise the past/range gates: the legacy function did not
                    // apply them, they lived in isDateEnabled.
                    todayISO: '2000-01-01',
                    startDate: undefined,
                    endDate: undefined,
                    allowedWeekdays,
                    holidaysPolicy: policy,
                    allowHolidayEves,
                    weekdayTimes,
                    holidayTimes: specialRange,
                    holidayEveTimes: specialRange,
                  });

                  const window = resolveTimeWindow(
                    { date: '2026-04-08', dayOfWeek, isHoliday, isHolidayEve },
                    ported
                  );

                  for (const time of SLOT_TIMES) {
                    const [h, m] = time.split(':').map(Number);
                    const startMin = h * 60 + m;
                    const actual = isSlotAllowed(window, startMin, startMin + SLOT_DURATION);
                    const expected = legacyIsTimeSlotAllowed({
                      dayOfWeek,
                      time,
                      slotDuration: SLOT_DURATION,
                      isHoliday,
                      isHolidayEve,
                      allowedWeekdays,
                      holidaysPolicy: policy,
                      allowHolidayEves,
                      weekdayTimes,
                      holidayMinTime: specialRange?.min_time,
                      holidayMaxTime: specialRange?.max_time,
                      holidayEveMinTime: specialRange?.min_time,
                      holidayEveMaxTime: specialRange?.max_time,
                    });

                    compared++;
                    if (actual !== expected) {
                      throw new Error(
                        `mismatch: policy=${policy} eves=${allowHolidayEves} ` +
                          `weekdays=[${allowedWeekdays}] dow=${dayOfWeek} ` +
                          `holiday=${isHoliday} eve=${isHolidayEve} time=${time} ` +
                          `weekday=${JSON.stringify(weekdayRange)} ` +
                          `special=${JSON.stringify(specialRange)} ` +
                          `-> got ${actual}, legacy ${expected}`
                      );
                    }
                  }
                }
              }
            }
          }
        }
      }
    }

    // Guard against the loops silently collapsing to nothing.
    expect(compared).toBeGreaterThan(100_000);
  });
});
