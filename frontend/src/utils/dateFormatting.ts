/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * Licensed under the Business Source License 1.1
 * See LICENSE file for details
 */

/**
 * Format a Date as an ISO date string (YYYY-MM-DD) using local time.
 * Used when building API payloads or map keys keyed by calendar date.
 */
export function formatDateISO(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

/**
 * Format the weekday name for the given date in the given locale.
 * `style` controls the width ('long' for "Monday", 'short' for "Mon").
 */
export function formatWeekday(
  date: Date,
  locale: string,
  style: 'long' | 'short' = 'long'
): string {
  return new Intl.DateTimeFormat(locale, { weekday: style }).format(date);
}

/**
 * Day + short month (no year), e.g. "5 Apr".
 */
export function formatDayMonthShort(date: Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    day: 'numeric',
    month: 'short',
  }).format(date);
}

/**
 * Day + full month + year, e.g. "5 April 2026".
 */
export function formatFullDate(date: Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  }).format(date);
}
