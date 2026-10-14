/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import { inTimezone } from '@/test/timezone';
import {
  addDaysISO,
  dayOfWeekISO,
  daysInMonth,
  formatDateISO,
  parseISODate,
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
    expect(todayISO('Europe/Paris')).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(todayISO('Pacific/Auckland')).toMatch(/^\d{4}-\d{2}-\d{2}$/);
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
