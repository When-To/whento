/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Time-of-day arithmetic for availabilities.
 *
 * An availability carries optional `start_time` / `end_time` as `HH:MM`. Absent times
 * mean "all day"; see {@link coversTime} for how an absent end is treated.
 */

/** A time of day, `HH:MM`, 24-hour. */
export type HHMM = string;

/** Half-open minute interval `[startMin, endMin)` since local midnight. */
export interface Interval {
  readonly startMin: number;
  readonly endMin: number;
}

/** Minutes in a day. Used as the exclusive end of an all-day interval. */
export const DAY_END_MIN = 24 * 60;

/** Last representable minute as a wall-clock time, i.e. `23:59`. */
export const LAST_MINUTE = 23 * 60 + 59;

/** `'09:30'` -> `570`. */
export function timeToMinutes(time: HHMM): number {
  const [hours, minutes] = time.split(':').map(Number);
  return hours * 60 + minutes;
}

/** `570` -> `'09:30'`. */
export function minutesToTime(totalMinutes: number): HHMM {
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`;
}

/**
 * Add minutes to a time, capping at `23:59` rather than wrapping past midnight.
 * An availability that would run to `24:00` is stored as ending at `23:59`.
 */
export function addMinutesCapped(time: HHMM, minutes: number): HHMM {
  return minutesToTime(Math.min(timeToMinutes(time) + minutes, LAST_MINUTE));
}

/**
 * Whether an availability covers the whole day.
 *
 * This is the single canonical definition. It previously existed in four divergent
 * copies; the outlier returned `false` when both times were absent, which rendered an
 * all-day availability as the literal string `"00:00-23:59"` in the list view's popup
 * while the grids showed "All day" for the same record.
 */
export function isFullDay(startTime?: string | null, endTime?: string | null): boolean {
  if (!startTime && !endTime) return true;
  const start = startTime || '00:00';
  const end = endTime || '23:59';
  return start === '00:00' && end === '23:59';
}

/**
 * Whether an availability covers the given time slot start.
 *
 * Uses string comparison on `HH:MM`, which is lexicographically ordered, and treats an
 * absent end as `23:59`, so the trailing 23:59 slot is never painted.
 */
export function coversTime(
  range: { start_time?: string | null; end_time?: string | null },
  time: HHMM
): boolean {
  if (!range.start_time && !range.end_time) return true;
  const start = range.start_time || '00:00';
  const end = range.end_time || '23:59';
  return time >= start && time < end;
}

/**
 * Merge overlapping or touching intervals into a minimal sorted set.
 *
 * Unlike the previous in-place implementation, this never mutates its input: the old
 * version extended `sorted[0]`, which is a reference into the caller's array.
 */
export function mergeIntervals(intervals: readonly Interval[]): Interval[] {
  if (intervals.length === 0) return [];

  const sorted = [...intervals].sort((a, b) => a.startMin - b.startMin);
  const merged: Interval[] = [{ startMin: sorted[0].startMin, endMin: sorted[0].endMin }];

  for (let i = 1; i < sorted.length; i++) {
    const current = sorted[i];
    const last = merged[merged.length - 1];
    if (current.startMin <= last.endMin) {
      merged[merged.length - 1] = {
        startMin: last.startMin,
        endMin: Math.max(last.endMin, current.endMin),
      };
    } else {
      merged.push({ startMin: current.startMin, endMin: current.endMin });
    }
  }

  return merged;
}
