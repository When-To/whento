/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Builders that turn calendar data into render-ready view models.
 *
 * `buildDayModel` is the single place a `DayModel` is born, so the month, week and list
 * views cannot drift apart again — they previously each rebuilt the same day state with
 * subtly different rules.
 *
 * The layer stays free of Vue and of vue-i18n: formatters arrive through
 * {@link CalendarFormatters}, which keeps it unit-testable in plain Node.
 */

import { DENSITY_STEPS, type DayModel, type DayStatus, type MonthModel } from '@/types/calendar';
import {
  addDaysISO,
  daysInMonth,
  formatDateISO,
  parseISODate,
  startOfWeekISO,
  type ISODate,
} from '@/utils/date/isoDate';
import { isFullDay } from '@/utils/date/timeRange';
import { isDayAllowed, isInRange, isPast, type CalendarRules } from './dateRules';
import type { DayIndex } from './dayIndex';
import type { HolidayIndex } from './holidays';

/** Localized formatting, injected so the pure layer never imports vue-i18n. */
export interface CalendarFormatters {
  weekdayShort(date: Date): string;
  weekdayLong(date: Date): string;
  dayMonthShort(date: Date): string;
  fullDate(date: Date): string;
  monthYear(date: Date): string;
  /** "All day" or "09:00-12:00". */
  timeRange(startTime?: string, endTime?: string): string;
  /** Accessible day description, given the formatted date and the counts. */
  dayAria(fullDate: string, count: number, threshold: number, status: DayStatus): string;
}

/** Everything the builders read. Built once per calendar, shared by every month. */
export interface ModelDeps {
  readonly rules: CalendarRules & { readonly threshold: number };
  readonly holidays: HolidayIndex;
  readonly index: DayIndex;
  readonly fmt: CalendarFormatters;
  /** 0 = Sunday, 1 = Monday. From `getWeekStartDay(locale)`. */
  readonly weekStartDay: number;
  readonly highlighted?: ReadonlySet<ISODate>;
}

/**
 * Quantize a participant count into heatmap steps.
 *
 * Scaled against the threshold so the background reads as "progress toward a workable
 * date", saturating once the threshold is met. A day with nobody is step 0; any day
 * with at least one participant is at least step 1, so "someone" is never invisible.
 */
export function densityStepFor(count: number, threshold: number): number {
  if (count <= 0) return 0;
  if (threshold <= 0) return DENSITY_STEPS;
  const ratio = Math.min(count / threshold, 1);
  return Math.max(1, Math.ceil(ratio * DENSITY_STEPS));
}

function statusFor(input: {
  isCurrentMonth: boolean;
  isOpen: boolean;
  meetsThreshold: boolean;
  hasOwn: boolean;
  hasRecurrence: boolean;
}): DayStatus {
  if (!input.isCurrentMonth) return 'outside';
  if (!input.isOpen) return 'disabled';
  if (input.meetsThreshold) return 'threshold';
  if (input.hasOwn) return 'available';
  if (input.hasRecurrence) return 'recurring';
  return 'free';
}

/**
 * Build the view model for one calendar date.
 *
 * `isCurrentMonth` is false for the filler cells an adjacent month contributes to a
 * month grid; the week and list views always pass true.
 */
export function buildDayModel(date: ISODate, deps: ModelDeps, isCurrentMonth = true): DayModel {
  const { rules, holidays, index, fmt } = deps;
  const dateObj = parseISODate(date);
  const dayOfWeek = dateObj.getDay();

  const isHoliday = holidays.isHoliday(date);
  const isHolidayEve = holidays.isHolidayEve(date);
  const past = isPast(date, rules);
  const allowed =
    isInRange(date, rules) && isDayAllowed({ date, dayOfWeek, isHoliday, isHolidayEve }, rules);
  const isOpen = !past && allowed;

  const ownAll = index.ownFor(date);
  const first = ownAll[0];
  const recurrenceHit = index.recurrenceFor(date, dayOfWeek);

  const participantCount = index.countFor(date);
  const threshold = rules.threshold || 1;
  const meetsThreshold = participantCount >= threshold;

  const status = statusFor({
    isCurrentMonth,
    isOpen,
    meetsThreshold,
    hasOwn: ownAll.length > 0,
    hasRecurrence: recurrenceHit !== null,
  });

  const dateLong = fmt.fullDate(dateObj);

  return {
    date,
    dayOfMonth: dateObj.getDate(),
    dayOfWeek,

    weekdayShort: fmt.weekdayShort(dateObj),
    weekdayLong: fmt.weekdayLong(dateObj),
    dateShort: fmt.dayMonthShort(dateObj),
    dateLong,
    ariaLabel: fmt.dayAria(dateLong, participantCount, threshold, status),

    status,
    isToday: date === rules.todayISO,
    isPast: past,
    isAllowed: allowed,
    isCurrentMonth,
    isHighlighted: deps.highlighted?.has(date) ?? false,
    isHoliday,
    isHolidayEve,
    holidayName: holidays.getName(date),

    participantCount,
    threshold,
    meetsThreshold,
    density: threshold > 0 ? Math.min(participantCount / threshold, 1) : 0,
    densityStep: densityStepFor(participantCount, threshold),

    own: first
      ? {
          startTime: first.start_time,
          endTime: first.end_time,
          note: first.note,
          isFullDay: isFullDay(first.start_time, first.end_time),
          label: fmt.timeRange(first.start_time, first.end_time),
        }
      : null,
    ownAll,
    recurrence: recurrenceHit
      ? {
          id: recurrenceHit.id,
          startTime: recurrenceHit.startTime,
          endTime: recurrenceHit.endTime,
          note: recurrenceHit.note,
          label: fmt.timeRange(recurrenceHit.startTime, recurrenceHit.endTime),
        }
      : null,
  };
}

/** Weekday header labels in display order, starting at the locale's first day. */
export function buildWeekdayHeaders(deps: ModelDeps): string[] {
  // Any week works; 2026-04-05 is a Sunday, so offsetting by weekStartDay lands on the
  // right first day whatever the locale.
  const headers: string[] = [];
  for (let i = 0; i < 7; i++) {
    const date = new Date(2026, 3, 5 + ((deps.weekStartDay + i) % 7));
    headers.push(deps.fmt.weekdayShort(date));
  }
  return headers;
}

/**
 * Build a whole month, padded with filler days to complete the first and last weeks.
 *
 * The result is always a multiple of seven days, which both grid layouts rely on.
 */
export function buildMonthModel(year: number, month: number, deps: ModelDeps): MonthModel {
  const firstOfMonth = new Date(year, month, 1);
  const leading = (firstOfMonth.getDay() - deps.weekStartDay + 7) % 7;
  const total = daysInMonth(year, month);

  const days: DayModel[] = [];

  let cursor = addDaysISO(formatDateISO(firstOfMonth), -leading);
  for (let i = 0; i < leading; i++) {
    days.push(buildDayModel(cursor, deps, false));
    cursor = addDaysISO(cursor, 1);
  }

  for (let i = 0; i < total; i++) {
    days.push(buildDayModel(cursor, deps, true));
    cursor = addDaysISO(cursor, 1);
  }

  const trailing = days.length % 7 === 0 ? 0 : 7 - (days.length % 7);
  for (let i = 0; i < trailing; i++) {
    days.push(buildDayModel(cursor, deps, false));
    cursor = addDaysISO(cursor, 1);
  }

  return {
    key: `${year}-${month}`,
    year,
    month,
    label: deps.fmt.monthYear(firstOfMonth),
    days,
    weekdayHeaders: buildWeekdayHeaders(deps),
  };
}

/** The seven days of the week containing `dateISO`, aligned to the locale's first day. */
export function buildWeekDays(dateISO: ISODate, deps: ModelDeps): DayModel[] {
  const days: DayModel[] = [];
  let cursor = startOfWeekISO(dateISO, deps.weekStartDay);
  for (let i = 0; i < 7; i++) {
    days.push(buildDayModel(cursor, deps, true));
    cursor = addDaysISO(cursor, 1);
  }
  return days;
}

/** A half-open span of dates to list. */
export interface DateRange {
  readonly startISO: ISODate;
  readonly endISO: ISODate;
}

/**
 * Flatten one or more ranges into the days worth acting on: deduplicated, open
 * (not past, in range, allowed), and chronological.
 */
export function buildListDays(ranges: readonly DateRange[], deps: ModelDeps): DayModel[] {
  const seen = new Set<ISODate>();
  const days: DayModel[] = [];

  for (const range of ranges) {
    let cursor = range.startISO;
    while (cursor <= range.endISO) {
      if (!seen.has(cursor)) {
        seen.add(cursor);
        const model = buildDayModel(cursor, deps, true);
        if (model.status !== 'disabled') days.push(model);
      }
      cursor = addDaysISO(cursor, 1);
    }
  }

  days.sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0));
  return days;
}
