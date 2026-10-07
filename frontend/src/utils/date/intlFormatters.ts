/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Cached `Intl.DateTimeFormat` instances.
 *
 * Constructing a formatter is expensive and the calendar formats thousands of dates per
 * render. The previous code built a fresh formatter inside a per-cell helper, so a month
 * grid allocated 42 of them on every render — and again on every drag frame.
 */

import { parseISODate, type ISODate } from './isoDate';

export type DateStyle =
  | 'weekdayShort'
  | 'weekdayLong'
  | 'dayMonthShort'
  | 'fullDate'
  | 'fullWithWeekday'
  | 'monthYear'
  | 'monthLong';

const OPTIONS: Record<DateStyle, Intl.DateTimeFormatOptions> = {
  weekdayShort: { weekday: 'short' },
  weekdayLong: { weekday: 'long' },
  dayMonthShort: { day: 'numeric', month: 'short' },
  fullDate: { day: 'numeric', month: 'long', year: 'numeric' },
  fullWithWeekday: { weekday: 'long', day: 'numeric', month: 'long' },
  monthYear: { month: 'long', year: 'numeric' },
  monthLong: { month: 'long' },
};

const cache = new Map<string, Intl.DateTimeFormat>();

/**
 * Get a memoized formatter for a (locale, style) pair.
 *
 * `locale` is the raw vue-i18n locale (`'fr'` / `'en'`), which is a valid BCP-47 tag.
 * Do not map it to `'fr-FR'` / `'en-US'`: the codebase previously used three different
 * conventions across three files, and one of the comparisons could never match.
 */
export function getFormatter(locale: string, style: DateStyle): Intl.DateTimeFormat {
  const key = `${locale}|${style}`;
  let formatter = cache.get(key);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(locale, OPTIONS[style]);
    cache.set(key, formatter);
  }
  return formatter;
}

/** Format a Date in the given locale and style. */
export function formatDate(date: Date, locale: string, style: DateStyle): string {
  return getFormatter(locale, style).format(date);
}

/** Format a `YYYY-MM-DD` string, parsed in local time so the day never shifts. */
export function formatISODate(iso: ISODate, locale: string, style: DateStyle): string {
  return getFormatter(locale, style).format(parseISODate(iso));
}

const relativeCache = new Map<string, Intl.RelativeTimeFormat>();

/** Get a memoized `Intl.RelativeTimeFormat` for a locale. */
function getRelativeFormatter(locale: string): Intl.RelativeTimeFormat {
  let formatter = relativeCache.get(locale);
  if (!formatter) {
    formatter = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
    relativeCache.set(locale, formatter);
  }
  return formatter;
}

/** Thresholds walked largest-unit-first, each with how many seconds it spans. */
const RELATIVE_UNITS: ReadonlyArray<readonly [Intl.RelativeTimeFormatUnit, number]> = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
];

/**
 * Format an instant relative to `now` — "2 days ago", "il y a 3 heures".
 *
 * Used by the activity journal, where an exact timestamp is noise: what the owner is
 * reading is how recently somebody joined or left, not the second they did it.
 *
 * Months and years are approximated at 30 and 365 days. The journal holds at most one
 * entry per date and the view asks for a few weeks at a time, so the error is never
 * reached in practice; anything older reads as "last year" either way.
 */
export function formatRelativeTime(date: Date, locale: string, now: Date = new Date()): string {
  const elapsedSeconds = (date.getTime() - now.getTime()) / 1000;

  for (const [unit, secondsPerUnit] of RELATIVE_UNITS) {
    if (Math.abs(elapsedSeconds) >= secondsPerUnit) {
      return getRelativeFormatter(locale).format(Math.round(elapsedSeconds / secondsPerUnit), unit);
    }
  }

  // Below a minute, `numeric: 'auto'` renders 0 seconds as "now" / "maintenant".
  return getRelativeFormatter(locale).format(Math.round(elapsedSeconds), 'second');
}

/** Drop every cached formatter. Exposed for tests that switch locale or timezone. */
export function clearFormatterCache(): void {
  cache.clear();
  relativeCache.clear();
}
