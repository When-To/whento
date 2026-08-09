/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import {
  addMinutesCapped,
  contains,
  coversTime,
  DAY_END_MIN,
  isFullDay,
  LAST_MINUTE,
  mergeIntervals,
  minutesToTime,
  overlaps,
  timeToMinutes,
  toInterval,
  type Interval,
} from './timeRange';

describe('timeToMinutes / minutesToTime', () => {
  it.each([
    ['00:00', 0],
    ['00:15', 15],
    ['09:30', 570],
    ['12:00', 720],
    ['23:59', 1439],
  ])('%s <-> %i', (time, minutes) => {
    expect(timeToMinutes(time)).toBe(minutes);
    expect(minutesToTime(minutes)).toBe(time);
  });

  it('zero-pads single digits', () => {
    expect(minutesToTime(65)).toBe('01:05');
  });
});

describe('addMinutesCapped', () => {
  it('adds within the day', () => {
    expect(addMinutesCapped('09:00', 30)).toBe('09:30');
    expect(addMinutesCapped('09:45', 30)).toBe('10:15');
  });

  it('caps at 23:59 instead of wrapping past midnight', () => {
    expect(addMinutesCapped('23:50', 30)).toBe('23:59');
    expect(addMinutesCapped('23:00', 120)).toBe('23:59');
    expect(addMinutesCapped('23:59', 1)).toBe('23:59');
  });

  it('reaches exactly 23:59 from the last slot', () => {
    expect(addMinutesCapped('23:45', 14)).toBe('23:59');
    expect(addMinutesCapped('23:45', 15)).toBe('23:59');
  });
});

describe('isFullDay', () => {
  // This is the exact matrix that diverged into four implementations. The outlier
  // returned false for (undefined, undefined), rendering all-day availabilities as
  // the literal string "00:00-23:59" in one view and "All day" in the others.
  it.each([
    [undefined, undefined, true],
    [null, null, true],
    ['', '', true],
    ['00:00', '23:59', true],
    ['00:00', undefined, true],
    [undefined, '23:59', true],
    ['00:00', '12:00', false],
    ['09:00', '23:59', false],
    ['09:00', '17:00', false],
    ['00:00', '00:00', false],
  ])('isFullDay(%s, %s) === %s', (start, end, expected) => {
    expect(isFullDay(start, end)).toBe(expected);
  });
});

describe('toInterval', () => {
  it('maps absent times to the whole day, ending at midnight', () => {
    expect(toInterval({})).toEqual({ startMin: 0, endMin: DAY_END_MIN });
  });

  it('converts explicit times', () => {
    expect(toInterval({ start_time: '09:00', end_time: '17:30' })).toEqual({
      startMin: 540,
      endMin: 1050,
    });
  });

  it('deliberately ends at 1440, not 1439, unlike coversTime', () => {
    // Documented asymmetry, ported verbatim: coverage maths runs to midnight while
    // slot painting stops at 23:59. See the TODO in timeRange.ts.
    expect(toInterval({ start_time: '09:00' }).endMin).toBe(DAY_END_MIN);
    expect(DAY_END_MIN).toBe(LAST_MINUTE + 1);
  });
});

describe('coversTime', () => {
  it('covers everything when both times are absent', () => {
    expect(coversTime({}, '00:00')).toBe(true);
    expect(coversTime({}, '23:45')).toBe(true);
  });

  it('is inclusive of the start and exclusive of the end', () => {
    const range = { start_time: '09:00', end_time: '12:00' };
    expect(coversTime(range, '08:45')).toBe(false);
    expect(coversTime(range, '09:00')).toBe(true);
    expect(coversTime(range, '11:45')).toBe(true);
    expect(coversTime(range, '12:00')).toBe(false);
  });

  it('treats an absent end as 23:59', () => {
    const range = { start_time: '09:00' };
    expect(coversTime(range, '23:45')).toBe(true);
    expect(coversTime(range, '23:59')).toBe(false);
  });
});

describe('overlaps / contains', () => {
  const nine = { startMin: 540, endMin: 720 };

  it('detects genuine overlap', () => {
    expect(overlaps(nine, { startMin: 600, endMin: 800 })).toBe(true);
    expect(overlaps(nine, { startMin: 400, endMin: 600 })).toBe(true);
  });

  it('does not treat touching intervals as overlapping', () => {
    expect(overlaps(nine, { startMin: 720, endMin: 800 })).toBe(false);
    expect(overlaps(nine, { startMin: 400, endMin: 540 })).toBe(false);
  });

  it('detects containment', () => {
    expect(contains(nine, { startMin: 600, endMin: 700 })).toBe(true);
    expect(contains(nine, nine)).toBe(true);
    expect(contains(nine, { startMin: 600, endMin: 800 })).toBe(false);
  });
});

describe('mergeIntervals', () => {
  it('returns empty for empty input', () => {
    expect(mergeIntervals([])).toEqual([]);
  });

  it('merges overlapping intervals', () => {
    expect(
      mergeIntervals([
        { startMin: 540, endMin: 720 },
        { startMin: 600, endMin: 800 },
      ])
    ).toEqual([{ startMin: 540, endMin: 800 }]);
  });

  it('merges intervals that merely touch', () => {
    expect(
      mergeIntervals([
        { startMin: 540, endMin: 720 },
        { startMin: 720, endMin: 800 },
      ])
    ).toEqual([{ startMin: 540, endMin: 800 }]);
  });

  it('keeps disjoint intervals separate and sorted', () => {
    expect(
      mergeIntervals([
        { startMin: 900, endMin: 1000 },
        { startMin: 540, endMin: 600 },
      ])
    ).toEqual([
      { startMin: 540, endMin: 600 },
      { startMin: 900, endMin: 1000 },
    ]);
  });

  it('absorbs a fully nested interval', () => {
    expect(
      mergeIntervals([
        { startMin: 540, endMin: 900 },
        { startMin: 600, endMin: 700 },
      ])
    ).toEqual([{ startMin: 540, endMin: 900 }]);
  });

  it('never mutates its input', () => {
    // The previous implementation extended `sorted[0]`, which is a reference into the
    // caller's array — harmless only because every caller passed a freshly built list.
    const input: Interval[] = [
      { startMin: 540, endMin: 720 },
      { startMin: 600, endMin: 800 },
    ];
    const snapshot = JSON.parse(JSON.stringify(input));
    mergeIntervals(input);
    expect(input).toEqual(snapshot);
  });
});
