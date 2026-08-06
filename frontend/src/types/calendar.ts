/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * View-model shapes for the calendar.
 *
 * `types/index.ts` stays the API contract and is untouched. These types describe what a
 * renderer needs, fully precomputed: every label is formatted once, every flag resolved
 * once, so templates contain property reads and no function calls.
 */

import type { Availability } from '@/types';
import type { ISODate } from '@/utils/date/isoDate';
import type { HHMM } from '@/utils/date/timeRange';

/**
 * How a day should read at a glance. Exactly one applies, in this precedence order:
 * threshold > available > recurring > free > disabled > outside.
 */
export type DayStatus =
  /** Filler cell from an adjacent month. Not interactive. */
  | 'outside'
  /** Past, out of the calendar's range, or refused by the weekday/holiday rules. */
  | 'disabled'
  /** Open, but the current participant has not answered. */
  | 'free'
  /** The current participant has an availability. */
  | 'available'
  /** Covered by one of the current participant's recurrences. */
  | 'recurring'
  /** Enough participants are available. */
  | 'threshold';

/** The current participant's own answer for a day, preformatted. */
export interface OwnAvailabilityView {
  readonly startTime?: HHMM;
  readonly endTime?: HHMM;
  readonly note?: string;
  readonly isFullDay: boolean;
  /** "All day" or "09:00-12:00", already localized. */
  readonly label: string;
}

/** A recurrence covering a day, preformatted. */
export interface RecurrenceView {
  readonly id: string;
  readonly startTime?: HHMM;
  readonly endTime?: HHMM;
  readonly note?: string;
  readonly label: string;
}

/** Number of heatmap steps, `0` (nobody) through {@link DENSITY_STEPS}. */
export const DENSITY_STEPS = 4;

/** Everything a day cell renders, in any of the views. */
export interface DayModel {
  readonly date: ISODate;
  readonly dayOfMonth: number;
  readonly dayOfWeek: number;

  /** Preformatted labels — the template never calls a formatter. */
  readonly weekdayShort: string;
  readonly weekdayLong: string;
  readonly dateShort: string;
  readonly dateLong: string;
  /** Full accessible description, e.g. "Wednesday 8 April 2026, 3 of 5 available". */
  readonly ariaLabel: string;

  readonly status: DayStatus;
  readonly isToday: boolean;
  readonly isPast: boolean;
  readonly isAllowed: boolean;
  readonly isCurrentMonth: boolean;
  readonly isHighlighted: boolean;
  readonly isHoliday: boolean;
  readonly isHolidayEve: boolean;
  readonly holidayName: string | null;

  readonly participantCount: number;
  readonly threshold: number;
  readonly meetsThreshold: boolean;
  /** Progress toward the threshold, 0..1. Drives the gauge width. */
  readonly density: number;
  /** Quantized density, 0..{@link DENSITY_STEPS}. Drives the heatmap background. */
  readonly densityStep: number;

  readonly own: OwnAvailabilityView | null;
  /** Every own availability for the day, when more than one needs listing. */
  readonly ownAll: readonly Availability[];
  readonly recurrence: RecurrenceView | null;
}

/** One rendered month. `days` is chronological and always a whole number of weeks. */
export interface MonthModel {
  readonly key: string;
  readonly year: number;
  /** 0-11. */
  readonly month: number;
  /** "April 2026", already localized. */
  readonly label: string;
  /**
   * Leading and trailing filler days included, so `days.length % 7 === 0`.
   *
   * Chronological order is what lets the grid transpose to the compact layout in pure
   * CSS: with `grid-auto-flow: column` over seven rows, item *i* lands on row `i % 7`
   * (its weekday) and column `floor(i / 7)` (its week).
   */
  readonly days: readonly DayModel[];
  /** Weekday headers in display order, honouring the locale's first day. */
  readonly weekdayHeaders: readonly string[];
}

/**
 * A pending change to the current participant's availabilities.
 *
 * Emitted as a batch by the week view and applied by `ParticipantView`. Empty strings
 * in `startTime`/`endTime` mean "all day" — the header-drag path relies on that.
 */
export interface AvailabilityOperation {
  type: 'create' | 'delete' | 'update';
  date: ISODate;
  startTime: string;
  endTime: string;
  oldStartTime?: string;
  oldEndTime?: string;
}
