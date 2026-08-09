/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Availability, recurrence and participant-count lookup, indexed by calendar date.
 *
 * The three calendar views each scanned the *entire* availability list once per day
 * cell (`availabilities.filter(a => a.date === dateString)`), and the recurrence list
 * once more with a nested exception scan. Over a twelve-month range that is O(days x
 * availabilities), redone on every recompute. Building the indexes once turns each
 * cell into a hash lookup.
 */

import type { Availability, DateAvailabilitySummary, RecurrenceWithExceptions } from '@/types';
import type { ISODate } from '@/utils/date/isoDate';

/** A recurrence that applies to a given date. */
export interface RecurrenceHit {
  readonly id: string;
  readonly startTime?: string;
  readonly endTime?: string;
  readonly note?: string;
}

/** Everything the day builders need to know about a single date. */
export interface DayIndex {
  /** The current participant's availabilities on that date, in input order. */
  ownFor(date: ISODate): readonly Availability[];
  /** The first recurrence covering that date, exceptions excluded. */
  recurrenceFor(date: ISODate, dayOfWeek: number): RecurrenceHit | null;
  /** How many participants are available on that date. */
  countFor(date: ISODate): number;
  /** The per-participant breakdown for that date, when loaded. */
  summaryFor(date: ISODate): DateAvailabilitySummary | undefined;
  /** Highest participant count across the indexed range, for density scaling. */
  readonly maxCount: number;
}

interface IndexedRecurrence {
  readonly recurrence: RecurrenceWithExceptions;
  readonly excluded: ReadonlySet<ISODate>;
}

const NO_AVAILABILITIES: readonly Availability[] = [];

export interface BuildDayIndexInput {
  readonly availabilities?: readonly Availability[] | null;
  readonly recurrences?: readonly RecurrenceWithExceptions[] | null;
  readonly participantCounts?: Readonly<Record<string, number>> | null;
  readonly dateSummaries?: readonly DateAvailabilitySummary[] | null;
}

/** Build the per-date indexes. Cost is linear in the input, once per data change. */
export function buildDayIndex(input: BuildDayIndexInput): DayIndex {
  const byDate = new Map<ISODate, Availability[]>();
  for (const availability of input.availabilities ?? []) {
    const existing = byDate.get(availability.date);
    if (existing) existing.push(availability);
    else byDate.set(availability.date, [availability]);
  }

  // Recurrences are bucketed by weekday, so a date only ever examines the handful
  // that could possibly match — typically zero or one.
  const byWeekday: IndexedRecurrence[][] = [[], [], [], [], [], [], []];
  for (const recurrence of input.recurrences ?? []) {
    const bucket = byWeekday[recurrence.day_of_week];
    if (!bucket) continue;
    bucket.push({
      recurrence,
      excluded: new Set((recurrence.exceptions ?? []).map(e => e.excluded_date)),
    });
  }

  const counts = input.participantCounts ?? {};

  const summaries = new Map<ISODate, DateAvailabilitySummary>();
  for (const summary of input.dateSummaries ?? []) {
    summaries.set(summary.date, summary);
  }

  let maxCount = 0;
  for (const value of Object.values(counts)) {
    if (value > maxCount) maxCount = value;
  }

  return {
    maxCount,

    ownFor: date => byDate.get(date) ?? NO_AVAILABILITIES,

    recurrenceFor: (date, dayOfWeek) => {
      for (const { recurrence, excluded } of byWeekday[dayOfWeek] ?? []) {
        // Dates are compared as ISO strings throughout, never as Date objects.
        if (date < recurrence.start_date) continue;
        if (recurrence.end_date && date > recurrence.end_date) continue;
        if (excluded.has(date)) continue;
        return {
          id: recurrence.id,
          startTime: recurrence.start_time,
          endTime: recurrence.end_time,
          note: recurrence.note,
        };
      }
      return null;
    },

    countFor: date => counts[date] ?? 0,

    summaryFor: date => summaries.get(date),
  };
}
