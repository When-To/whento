/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import {
  createEmptyWeekdayTimes,
  normalizeTime,
  prepareWeekdayTimes,
  toggleWeekday,
  type WeekdayTimes,
} from './weekdayTimes';

describe('normalizeTime', () => {
  const cases: [string, string, string][] = [
    ['midnight is not a restriction', '00:00', ''],
    ['an empty value stays empty', '', ''],
    ['a real bound is kept', '08:30', '08:30'],
    ['end of day is a real bound', '23:59', '23:59'],
    // 00:00 is what a cleared picker emits; 0:00 is not a value the picker can produce,
    // and treating it as one would silently accept a shape the API rejects.
    ['an unpadded midnight is left alone', '0:00', '0:00'],
  ];

  for (const [name, input, expected] of cases) {
    it(name, () => {
      expect(normalizeTime(input)).toBe(expected);
    });
  }
});

describe('createEmptyWeekdayTimes', () => {
  it('covers all seven days', () => {
    const times = createEmptyWeekdayTimes();
    expect(Object.keys(times)).toEqual(['0', '1', '2', '3', '4', '5', '6']);
    for (const day of Object.values(times)) {
      expect(day).toEqual({ min_time: '', max_time: '' });
    }
  });

  it('hands out an independent object each call', () => {
    const first = createEmptyWeekdayTimes();
    const second = createEmptyWeekdayTimes();
    first[0].min_time = '09:00';
    expect(second[0].min_time).toBe('');
  });
});

describe('prepareWeekdayTimes', () => {
  it('omits empty and midnight bounds rather than sending them', () => {
    const times: WeekdayTimes = {
      ...createEmptyWeekdayTimes(),
      1: { min_time: '09:00', max_time: '18:00' },
      2: { min_time: '00:00', max_time: '18:00' },
      3: { min_time: '', max_time: '' },
    };

    const prepared = prepareWeekdayTimes(times);

    expect(prepared[1]).toEqual({ min_time: '09:00', max_time: '18:00' });
    // Midnight normalises away, but the day's real upper bound survives.
    expect(prepared[2]).toEqual({ max_time: '18:00' });
    expect(prepared[3]).toEqual({});
  });

  it('keys the result by number, not by the string Object.entries yields', () => {
    const prepared = prepareWeekdayTimes({ 5: { min_time: '10:00', max_time: '' } });
    expect(Object.keys(prepared)).toEqual(['5']);
    expect(prepared[5]).toEqual({ min_time: '10:00' });
  });
});

describe('toggleWeekday', () => {
  it('adds a day and keeps the list sorted', () => {
    const allowed = [1, 5];
    toggleWeekday(allowed, 3);
    expect(allowed).toEqual([1, 3, 5]);
  });

  it('removes a day that is already allowed', () => {
    const allowed = [1, 3, 5];
    toggleWeekday(allowed, 3);
    expect(allowed).toEqual([1, 5]);
  });

  /*
   * A calendar with no open weekday cannot be answered at all, and the form offers no
   * way back: the last remaining button would simply stop responding. Refusing the
   * removal is what keeps the calendar usable.
   */
  it('refuses to empty the list', () => {
    const allowed = [2];
    toggleWeekday(allowed, 2);
    expect(allowed).toEqual([2]);
  });

  /*
   * The edit form keeps a pristine copy of the calendar to detect unsaved changes, and
   * that copy is compared by value against this array. Replacing the array instead of
   * mutating it would change which object the comparison sees.
   */
  it('mutates in place instead of returning a new array', () => {
    const allowed = [1];
    const same = allowed;
    toggleWeekday(allowed, 4);
    expect(same).toBe(allowed);
    expect(same).toEqual([1, 4]);
  });
});
