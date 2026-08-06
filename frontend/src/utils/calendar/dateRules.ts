/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Which dates and time slots a calendar accepts.
 *
 * This replaces four implementations that disagreed with one another: the shared
 * `isDateAllowed` composable, `checkDateAllowed` in the month grid, `isDateEnabled` in
 * the week grid, and an inline predicate in the list view. The reference is
 * `pkg/datevalidation/datevalidation.go` — the frontend must not enable a day the
 * backend will reject.
 */

import { DAY_END_MIN, timeToMinutes } from '@/utils/date/timeRange';
import type { ISODate } from '@/utils/date/isoDate';
import type { HolidaysPolicy, TimeRange } from '@/types';

/** Every date/time constraint a calendar imposes, resolved once per calendar. */
export interface CalendarRules {
  /** IANA timezone the calendar is expressed in. */
  readonly timeZone: string;
  /** Today, *in the calendar's timezone*, so "past" does not depend on the viewer. */
  readonly todayISO: ISODate;
  /** Weekdays (0 = Sunday) that accept availabilities. Empty means none. */
  readonly allowedWeekdays: readonly number[];
  readonly holidaysPolicy: HolidaysPolicy;
  readonly allowHolidayEves: boolean;
  /** Inclusive bounds of the calendar, `YYYY-MM-DD`. */
  readonly startDate?: ISODate;
  readonly endDate?: ISODate;
  /** Per-weekday opening hours, keyed by day of week. */
  readonly weekdayTimes?: Readonly<Record<string, TimeRange>>;
  readonly holidayTimes?: TimeRange;
  readonly holidayEveTimes?: TimeRange;
}

/** What a builder already knows about a day before asking the rules. */
export interface DayContext {
  readonly date: ISODate;
  readonly dayOfWeek: number;
  readonly isHoliday: boolean;
  readonly isHolidayEve: boolean;
}

/** Resolved opening hours for one day, or a refusal. */
export type DayTimeWindow =
  | { readonly allowed: false }
  | { readonly allowed: true; readonly minMin: number; readonly maxMin: number };

const DENIED: DayTimeWindow = { allowed: false };
const UNRESTRICTED: DayTimeWindow = { allowed: true, minMin: 0, maxMin: DAY_END_MIN };

/**
 * Whether the date falls inside the calendar's `start_date`/`end_date`, inclusive.
 *
 * Compares ISO strings. The month grid used to parse the bounds with `new Date(iso)`
 * (UTC midnight) and then call `setHours(0,0,0,0)` (local midnight), which lands on the
 * *previous* local day for negative offsets — so it disagreed with the week and list
 * views about which boundary days were selectable.
 */
export function isInRange(date: ISODate, rules: CalendarRules): boolean {
  if (rules.startDate && date < rules.startDate) return false;
  if (rules.endDate && date > rules.endDate) return false;
  return true;
}

/** Whether the date is before today in the calendar's timezone. */
export function isPast(date: ISODate, rules: CalendarRules): boolean {
  return date < rules.todayISO;
}

/**
 * Whether the weekday itself is accepted.
 *
 * An empty list allows nothing, matching `IsWeekdayAllowed` in the backend. The shared
 * frontend composable used to treat an empty list as "no restriction", which enabled
 * days the backend then refused to write.
 */
function isWeekdayAllowed(dayOfWeek: number, rules: CalendarRules): boolean {
  return rules.allowedWeekdays.includes(dayOfWeek);
}

/**
 * Whether a day accepts availabilities at all, ignoring opening hours.
 *
 * Does *not* consider the past or the calendar bounds — compose with {@link isPast}
 * and {@link isInRange} for the full day-level gate.
 */
export function isDayAllowed(ctx: DayContext, rules: CalendarRules): boolean {
  if (ctx.isHoliday) {
    if (rules.holidaysPolicy === 'allow') return true;
    if (rules.holidaysPolicy === 'block') return false;
    // 'ignore': a holiday is just another day.
    return isWeekdayAllowed(ctx.dayOfWeek, rules);
  }

  if (isWeekdayAllowed(ctx.dayOfWeek, rules)) return true;

  // A disallowed weekday is still accepted when it precedes a holiday and the
  // calendar opts into eves.
  return rules.allowHolidayEves && ctx.isHolidayEve;
}

/** Full day-level gate: in range, not past, and accepted by the weekday/holiday rules. */
export function isDayOpen(ctx: DayContext, rules: CalendarRules): boolean {
  return !isPast(ctx.date, rules) && isInRange(ctx.date, rules) && isDayAllowed(ctx, rules);
}

/**
 * Widen a time range with another, keeping the earliest start and the latest end.
 *
 * Note the deliberate quirk, ported verbatim: when one side is absent the other is
 * adopted rather than treated as unbounded. An absent bound semantically means "no
 * restriction", which is *wider*, so this narrows instead of widening. Preserved as-is
 * because changing it moves which slots are selectable on holidays that also fall on a
 * configured weekday; it deserves its own reviewed change.
 */
function widen(base: TimeRange | undefined, extra: TimeRange | undefined): TimeRange | undefined {
  if (!base) return extra;
  if (!extra) return base;

  const min =
    base.min_time && extra.min_time
      ? base.min_time < extra.min_time
        ? base.min_time
        : extra.min_time
      : base.min_time || extra.min_time;

  const max =
    base.max_time && extra.max_time
      ? base.max_time > extra.max_time
        ? base.max_time
        : extra.max_time
      : base.max_time || extra.max_time;

  return { min_time: min, max_time: max };
}

function toWindow(range: TimeRange | undefined): DayTimeWindow {
  if (!range || (!range.min_time && !range.max_time)) return UNRESTRICTED;
  return {
    allowed: true,
    minMin: range.min_time ? timeToMinutes(range.min_time) : 0,
    maxMin: range.max_time ? timeToMinutes(range.max_time) : DAY_END_MIN,
  };
}

/**
 * Opening hours for one day, merging the holiday, holiday-eve and weekday windows.
 *
 * Resolved once per day. The previous `isTimeSlotAllowed` redid all of this per *cell*,
 * two to four times, which is where the week grid's render cost came from.
 */
export function resolveTimeWindow(ctx: DayContext, rules: CalendarRules): DayTimeWindow {
  if (isPast(ctx.date, rules) || !isInRange(ctx.date, rules)) return DENIED;

  const weekdayAllowed = isWeekdayAllowed(ctx.dayOfWeek, rules);
  const weekdayRange = rules.weekdayTimes?.[String(ctx.dayOfWeek)];

  let range: TimeRange | undefined;

  if (ctx.isHoliday) {
    if (rules.holidaysPolicy === 'block') return DENIED;
    if (rules.holidaysPolicy === 'allow') {
      range = rules.holidayTimes;
    } else {
      // 'ignore': a holiday is just another day.
      if (!weekdayAllowed) return DENIED;
      range = weekdayRange;
    }
  } else if (ctx.isHolidayEve) {
    if (rules.allowHolidayEves) {
      range = rules.holidayEveTimes;
    } else {
      if (!weekdayAllowed) return DENIED;
      range = weekdayRange;
    }
  } else {
    if (!weekdayAllowed) return DENIED;
    range = weekdayRange;
  }

  // A day that is both special and a configured weekday gets the union of the two
  // windows. This is a no-op on the branches that already took `weekdayRange`.
  if (weekdayAllowed && weekdayRange) {
    range = widen(range, weekdayRange);
  }

  return toWindow(range);
}

/**
 * Whether a slot `[startMin, endMin)` is selectable within a day's opening hours.
 * A slot counts as allowed as soon as it *overlaps* the window.
 */
export function isSlotAllowed(window: DayTimeWindow, startMin: number, endMin: number): boolean {
  if (!window.allowed) return false;
  return startMin < window.maxMin && endMin > window.minMin;
}

/** Sensible weekday list when the calendar does not restrict them at all. */
export const ALL_WEEKDAYS: readonly number[] = [0, 1, 2, 3, 4, 5, 6];

/** Build {@link CalendarRules} from a calendar record plus the resolved "today". */
export function buildCalendarRules(input: {
  timeZone?: string | null;
  todayISO: ISODate;
  allowedWeekdays?: readonly number[] | null;
  holidaysPolicy?: HolidaysPolicy | string | null;
  allowHolidayEves?: boolean | null;
  startDate?: string | null;
  endDate?: string | null;
  weekdayTimes?: Readonly<Record<string, TimeRange>> | null;
  holidayTimes?: TimeRange | null;
  holidayEveTimes?: TimeRange | null;
  threshold?: number | null;
}): CalendarRules & { readonly threshold: number } {
  const policy = input.holidaysPolicy;
  return {
    timeZone: input.timeZone || 'UTC',
    todayISO: input.todayISO,
    // An absent list means "unrestricted"; an explicitly empty one means "none".
    allowedWeekdays: input.allowedWeekdays ?? ALL_WEEKDAYS,
    holidaysPolicy:
      policy === 'allow' || policy === 'block' || policy === 'ignore' ? policy : 'ignore',
    allowHolidayEves: input.allowHolidayEves ?? false,
    startDate: input.startDate || undefined,
    endDate: input.endDate || undefined,
    weekdayTimes: input.weekdayTimes || undefined,
    holidayTimes: input.holidayTimes || undefined,
    holidayEveTimes: input.holidayEveTimes || undefined,
    threshold: input.threshold ?? 0,
  };
}
