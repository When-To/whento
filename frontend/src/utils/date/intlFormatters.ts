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

/** Drop every cached formatter. Exposed for tests that switch locale or timezone. */
export function clearFormatterCache(): void {
  cache.clear();
}
