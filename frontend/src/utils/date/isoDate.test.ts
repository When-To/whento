/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import { inTimezone } from '@/test/timezone';
import {
  addDaysISO,
  addMonthsISO,
  dayOfWeekISO,
  daysInMonth,
  formatDateISO,
  isValidISODate,
  isoRange,
  monthOf,
  parseISODate,
  startOfMonthISO,
  startOfWeekISO,
  todayISO,
  yearOf,
} from './isoDate';

describe('parseISODate', () => {
  // Regression test for the UTC/local bug: `new Date('2026-04-05')` parses as UTC
  // midnight, so every negative-offset user saw the *previous* day's weekday in
  // every calendar cell.
  const timezones = ['UTC', 'America/New_York', 'America/Los_Angeles', 'Pacific/Kiritimati'];

  it.each(timezones)('parses to local midnight in %s', tz => {
    inTimezone(tz, () => {
      const date = parseISODate('2026-04-05');
      expect(date.getFullYear()).toBe(2026);
      expect(date.getMonth()).toBe(3);
      expect(date.getDate()).toBe(5);
      expect(date.getHours()).toBe(0);
      // 2026-04-05 is a Sunday, in every timezone.
      expect(date.getDay()).toBe(0);
    });
  });

  it.each(timezones)('round-trips through formatDateISO in %s', tz => {
    inTimezone(tz, () => {
      for (const iso of ['2026-01-01', '2026-02-28', '2026-03-29', '2026-10-25', '2026-12-31']) {
        expect(formatDateISO(parseISODate(iso))).toBe(iso);
      }
    });
  });

  it('disagrees with the naive `new Date(iso)` west of Greenwich', () => {
    inTimezone('America/New_York', () => {
      // This is exactly what the old code did, and why it was wrong.
      expect(new Date('2026-04-05').getDate()).toBe(4);
      expect(parseISODate('2026-04-05').getDate()).toBe(5);
    });
  });
});

describe('addDaysISO', () => {
  it('crosses month boundaries', () => {
    expect(addDaysISO('2026-01-31', 1)).toBe('2026-02-01');
    expect(addDaysISO('2026-02-01', -1)).toBe('2026-01-31');
  });

  it('crosses year boundaries', () => {
    expect(addDaysISO('2026-12-31', 1)).toBe('2027-01-01');
    expect(addDaysISO('2027-01-01', -1)).toBe('2026-12-31');
  });

  it('handles leap years', () => {
    expect(addDaysISO('2028-02-28', 1)).toBe('2028-02-29');
    expect(addDaysISO('2026-02-28', 1)).toBe('2026-03-01');
  });

  it('is DST-safe across spring-forward', () => {
    // 2026-03-29 is the European spring-forward; that local day is only 23 hours long,
    // so millisecond arithmetic would land back on the same date.
    inTimezone('Europe/Paris', () => {
      expect(addDaysISO('2026-03-28', 1)).toBe('2026-03-29');
      expect(addDaysISO('2026-03-29', 1)).toBe('2026-03-30');
      expect(addDaysISO('2026-03-28', 7)).toBe('2026-04-04');
    });
  });

  it('is DST-safe across fall-back', () => {
    // 2026-10-25 is 25 hours long in Europe/Paris.
    inTimezone('Europe/Paris', () => {
      expect(addDaysISO('2026-10-24', 1)).toBe('2026-10-25');
      expect(addDaysISO('2026-10-25', 1)).toBe('2026-10-26');
      expect(addDaysISO('2026-10-24', 7)).toBe('2026-10-31');
    });
  });

  it('is DST-safe in the southern hemisphere', () => {
    inTimezone('Australia/Sydney', () => {
      expect(addDaysISO('2026-10-03', 1)).toBe('2026-10-04');
      expect(addDaysISO('2026-04-04', 1)).toBe('2026-04-05');
    });
  });
});

describe('addMonthsISO', () => {
  it('clamps to the end of a shorter target month', () => {
    expect(addMonthsISO('2026-01-31', 1)).toBe('2026-02-28');
    expect(addMonthsISO('2028-01-31', 1)).toBe('2028-02-29');
    expect(addMonthsISO('2026-03-31', -1)).toBe('2026-02-28');
  });

  it('crosses years', () => {
    expect(addMonthsISO('2026-12-15', 1)).toBe('2027-01-15');
    expect(addMonthsISO('2026-01-15', -1)).toBe('2025-12-15');
    expect(addMonthsISO('2026-01-15', 12)).toBe('2027-01-15');
  });
});

describe('isoRange', () => {
  it('is inclusive on both ends', () => {
    expect(isoRange('2026-04-05', '2026-04-08')).toEqual([
      '2026-04-05',
      '2026-04-06',
      '2026-04-07',
      '2026-04-08',
    ]);
  });

  it('returns a single date when start equals end', () => {
    expect(isoRange('2026-04-05', '2026-04-05')).toEqual(['2026-04-05']);
  });

  it('returns empty for an inverted range', () => {
    expect(isoRange('2026-04-08', '2026-04-05')).toEqual([]);
  });

  it('spans a DST boundary without duplicating or losing a day', () => {
    inTimezone('Europe/Paris', () => {
      const range = isoRange('2026-03-27', '2026-03-31');
      expect(range).toEqual(['2026-03-27', '2026-03-28', '2026-03-29', '2026-03-30', '2026-03-31']);
      expect(new Set(range).size).toBe(range.length);
    });
  });
});

describe('startOfWeekISO', () => {
  // 2026-04-08 is a Wednesday.
  it('honours a Monday-first locale', () => {
    expect(startOfWeekISO('2026-04-08', 1)).toBe('2026-04-06');
  });

  it('honours a Sunday-first locale', () => {
    expect(startOfWeekISO('2026-04-08', 0)).toBe('2026-04-05');
  });

  it('is idempotent on a week start', () => {
    expect(startOfWeekISO('2026-04-06', 1)).toBe('2026-04-06');
    expect(startOfWeekISO('2026-04-05', 0)).toBe('2026-04-05');
  });

  it('walks back across a month boundary', () => {
    // 2026-04-01 is a Wednesday.
    expect(startOfWeekISO('2026-04-01', 1)).toBe('2026-03-30');
  });
});

describe('todayISO', () => {
  it('returns a well-formed date for a valid timezone', () => {
    expect(isValidISODate(todayISO('Europe/Paris'))).toBe(true);
    expect(isValidISODate(todayISO('Pacific/Auckland'))).toBe(true);
  });

  it('differs between far-apart timezones at some point in the day', () => {
    // Kiritimati (+14) and Niue (-11) are 25 hours apart, so their calendar dates
    // never match. This is what makes the calendar `timezone` setting meaningful.
    expect(todayISO('Pacific/Kiritimati')).not.toBe(todayISO('Pacific/Niue'));
  });

  it('falls back to the browser date for an unknown timezone', () => {
    expect(todayISO('Not/AZone')).toBe(formatDateISO(new Date()));
  });

  it('falls back to the browser date when no timezone is given', () => {
    expect(todayISO()).toBe(formatDateISO(new Date()));
  });
});

describe('string accessors', () => {
  it('reads components without allocating a Date', () => {
    expect(yearOf('2026-04-05')).toBe(2026);
    expect(monthOf('2026-04-05')).toBe(3);
    expect(startOfMonthISO('2026-04-05')).toBe('2026-04-01');
  });

  it('computes the weekday in local time', () => {
    inTimezone('America/Los_Angeles', () => {
      expect(dayOfWeekISO('2026-04-05')).toBe(0);
      expect(dayOfWeekISO('2026-04-06')).toBe(1);
    });
  });

  it('counts days in a month', () => {
    expect(daysInMonth(2026, 0)).toBe(31);
    expect(daysInMonth(2026, 1)).toBe(28);
    expect(daysInMonth(2028, 1)).toBe(29);
    expect(daysInMonth(2026, 3)).toBe(30);
  });
});

describe('isValidISODate', () => {
  it('accepts real dates', () => {
    expect(isValidISODate('2026-04-05')).toBe(true);
    expect(isValidISODate('2028-02-29')).toBe(true);
  });

  it('rejects malformed or impossible dates', () => {
    expect(isValidISODate('2026-4-5')).toBe(false);
    expect(isValidISODate('2026-13-01')).toBe(false);
    expect(isValidISODate('2026-02-30')).toBe(false);
    expect(isValidISODate('not-a-date')).toBe(false);
    expect(isValidISODate('')).toBe(false);
  });
});
