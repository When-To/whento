/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * @deprecated Compatibility shim. Import from `@/utils/date/isoDate` and
 * `@/utils/date/intlFormatters` instead. This file is deleted once the calendar
 * components stop importing it.
 */

import { formatDate } from './date/intlFormatters';

export { formatDateISO } from './date/isoDate';

/**
 * Format the weekday name for the given date in the given locale.
 * `style` controls the width ('long' for "Monday", 'short' for "Mon").
 */
export function formatWeekday(
  date: Date,
  locale: string,
  style: 'long' | 'short' = 'long'
): string {
  return formatDate(date, locale, style === 'long' ? 'weekdayLong' : 'weekdayShort');
}

/**
 * Day + short month (no year), e.g. "5 Apr".
 */
export function formatDayMonthShort(date: Date, locale: string): string {
  return formatDate(date, locale, 'dayMonthShort');
}

/**
 * Day + full month + year, e.g. "5 April 2026".
 */
export function formatFullDate(date: Date, locale: string): string {
  return formatDate(date, locale, 'fullDate');
}
