/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Local-time ISO date handling.
 *
 * The whole calendar keys availabilities, participant counts and API payloads by
 * `YYYY-MM-DD` strings. Those strings denote a *calendar date*, never an instant, so
 * they must never round-trip through UTC: `new Date('2026-04-05')` parses as UTC
 * midnight, and formatting it back in a negative-offset timezone yields April 4.
 * Every helper here stays in local time.
 */

/** A calendar date, `YYYY-MM-DD`. Always produced by {@link formatDateISO}. */
export type ISODate = string;

const ISO_DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

/**
 * Format a Date as an ISO date string (YYYY-MM-DD) using local time.
 * Used when building API payloads or map keys keyed by calendar date.
 */
export function formatDateISO(date: Date): ISODate {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

/**
 * Parse a `YYYY-MM-DD` string into a Date at *local* midnight.
 *
 * This is the inverse of {@link formatDateISO}. Never use `new Date(iso)` instead:
 * that parses as UTC midnight and shifts the day for anyone west of Greenwich.
 */
export function parseISODate(iso: ISODate): Date {
  const year = Number(iso.slice(0, 4));
  const month = Number(iso.slice(5, 7));
  const day = Number(iso.slice(8, 10));
  return new Date(year, month - 1, day);
}

/** True when the string is a well-formed, real calendar date. */
export function isValidISODate(value: string): boolean {
  if (!ISO_DATE_PATTERN.test(value)) return false;
  return formatDateISO(parseISODate(value)) === value;
}

/** The year component, without allocating a Date. */
export function yearOf(iso: ISODate): number {
  return Number(iso.slice(0, 4));
}

/** The month component (0-11), without allocating a Date. */
export function monthOf(iso: ISODate): number {
  return Number(iso.slice(5, 7)) - 1;
}

/**
 * Shift a calendar date by whole days.
 *
 * Goes through `setDate`, which is DST-safe — adding 1 to a spring-forward day
 * yields the next calendar day, not a 23-hour offset.
 */
export function addDaysISO(iso: ISODate, days: number): ISODate {
  const date = parseISODate(iso);
  date.setDate(date.getDate() + days);
  return formatDateISO(date);
}

/** Shift a calendar date by whole months, clamping to the end of the target month. */
export function addMonthsISO(iso: ISODate, months: number): ISODate {
  const date = parseISODate(iso);
  const targetMonth = date.getMonth() + months;
  const dayOfMonth = date.getDate();
  date.setDate(1);
  date.setMonth(targetMonth);
  date.setDate(Math.min(dayOfMonth, daysInMonth(date.getFullYear(), date.getMonth())));
  return formatDateISO(date);
}

/** Number of days in the given month (0-indexed month). */
export function daysInMonth(year: number, month: number): number {
  return new Date(year, month + 1, 0).getDate();
}

/** Day of week (0 = Sunday) for a calendar date, in local time. */
export function dayOfWeekISO(iso: ISODate): number {
  return parseISODate(iso).getDay();
}

/**
 * Every calendar date from `startISO` to `endISO`, inclusive.
 * Returns an empty array when the range is inverted.
 */
export function isoRange(startISO: ISODate, endISO: ISODate): ISODate[] {
  if (startISO > endISO) return [];
  const dates: ISODate[] = [];
  const cursor = parseISODate(startISO);
  const last = parseISODate(endISO);
  while (cursor <= last) {
    dates.push(formatDateISO(cursor));
    cursor.setDate(cursor.getDate() + 1);
  }
  return dates;
}

/**
 * Today's calendar date *in the given IANA timezone*.
 *
 * A participant in Sydney looking at a `Europe/Paris` calendar must see the same
 * "today" and the same set of past days as a participant in Paris. `en-CA` is used
 * because it is the locale Intl formats as `YYYY-MM-DD`.
 */
export function todayISO(timeZone?: string): ISODate {
  if (!timeZone) return formatDateISO(new Date());
  try {
    return new Intl.DateTimeFormat('en-CA', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).format(new Date());
  } catch {
    // Unknown timezone: fall back to the browser's own date rather than throwing.
    return formatDateISO(new Date());
  }
}

/**
 * Start of the week containing `iso`, honouring the locale's first day.
 * `weekStartDay` comes from `getWeekStartDay(locale)` in `src/i18n.ts`.
 */
export function startOfWeekISO(iso: ISODate, weekStartDay: number): ISODate {
  const offset = (dayOfWeekISO(iso) - weekStartDay + 7) % 7;
  return addDaysISO(iso, -offset);
}

/** First day of the month containing `iso`. */
export function startOfMonthISO(iso: ISODate): ISODate {
  return `${iso.slice(0, 8)}01`;
}
