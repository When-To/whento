/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * The per-weekday and holiday time restrictions of a calendar, as the create and the
 * edit form both hold them.
 *
 * The two views carried byte-identical copies of `normalizeTime`, `prepareWeekdayTimes`
 * and `toggleWeekday`, which is how they came to disagree about nothing yet had to be
 * fixed twice. They are framework-free on purpose: nothing here touches Vue, so the
 * rules can be tested without mounting anything.
 */

export interface WeekdayTimeRange {
  min_time: string;
  max_time: string;
}

/** Sunday (0) through Saturday (6). Every day is always present, possibly empty. */
export type WeekdayTimes = Record<number, WeekdayTimeRange>;

/** The seven empty ranges a fresh form starts from. */
export function createEmptyWeekdayTimes(): WeekdayTimes {
  return {
    0: { min_time: '', max_time: '' },
    1: { min_time: '', max_time: '' },
    2: { min_time: '', max_time: '' },
    3: { min_time: '', max_time: '' },
    4: { min_time: '', max_time: '' },
    5: { min_time: '', max_time: '' },
    6: { min_time: '', max_time: '' },
  };
}

/**
 * Normalise `"00:00"` to the empty string.
 *
 * Midnight is what the time picker produces when it is cleared, and as a *restriction*
 * it means nothing — "not before 00:00" is not a constraint. Sending it would persist a
 * bound the user did not set.
 */
export function normalizeTime(time: string): string {
  return time === '00:00' ? '' : time;
}

/**
 * Shape the weekday ranges for the API: normalised, and with empty bounds omitted
 * rather than sent as empty strings.
 */
export function prepareWeekdayTimes(
  weekdayTimes: WeekdayTimes
): Record<number, { min_time?: string; max_time?: string }> {
  const result: Record<number, { min_time?: string; max_time?: string }> = {};
  for (const [day, times] of Object.entries(weekdayTimes)) {
    const minTime = normalizeTime(times.min_time);
    const maxTime = normalizeTime(times.max_time);
    result[Number(day)] = {
      ...(minTime ? { min_time: minTime } : {}),
      ...(maxTime ? { max_time: maxTime } : {}),
    };
  }
  return result;
}

/**
 * Add or remove `day` from the allowed weekdays, keeping the array sorted and never
 * emptying it — a calendar with no open day cannot be answered at all.
 *
 * Mutates in place rather than returning a new array, which is what the two forms have
 * always done: the edit view keeps a pristine copy of the form to detect unsaved
 * changes, and replacing the array here would change what that comparison sees.
 */
export function toggleWeekday(allowedWeekdays: number[], day: number): void {
  const index = allowedWeekdays.indexOf(day);
  if (index > -1) {
    // Remove if already selected (but keep at least one day)
    if (allowedWeekdays.length > 1) {
      allowedWeekdays.splice(index, 1);
    }
  } else {
    allowedWeekdays.push(day);
    allowedWeekdays.sort((a, b) => a - b);
  }
}
