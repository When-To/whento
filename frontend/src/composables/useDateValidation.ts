/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * @deprecated Compatibility shim over `@/utils/calendar/holidays`. Kept only so the
 * existing calendar components keep working while they are rewritten; new code should
 * use `getHolidayIndex` directly and pass ISO date strings.
 */

import { i18n } from '@/i18n';
import { formatDateISO } from '@/utils/date/isoDate';
import { getHolidayIndex } from '@/utils/calendar/holidays';

function indexFor(timezone: string, locale?: string) {
  return getHolidayIndex(timezone, locale ?? i18n.global.locale.value);
}

/**
 * Composable to validate if a date is allowed according to calendar parameters.
 */
export function useDateValidation() {
  /**
   * Checks if a date is allowed for adding availability.
   * Mirrors the backend: weekday, holidays policy, and holiday eves.
   */
  const isDateAllowed = (
    date: Date,
    timezone: string,
    allowedWeekdays: number[],
    holidaysPolicy: 'ignore' | 'allow' | 'block',
    allowHolidayEves: boolean
  ): boolean => {
    const holidays = indexFor(timezone);
    const iso = formatDateISO(date);
    const isHolidayDate = holidays.isHoliday(iso);

    if (holidaysPolicy === 'block' && isHolidayDate) return false;
    if (holidaysPolicy === 'allow' && isHolidayDate) return true;

    const weekday = date.getDay();
    if (!allowedWeekdays || allowedWeekdays.length === 0 || allowedWeekdays.includes(weekday)) {
      return true;
    }

    return allowHolidayEves && holidays.isHolidayEve(iso);
  };

  /** Checks if a date is a public holiday. */
  const checkIsHoliday = (date: Date, timezone: string): boolean =>
    indexFor(timezone).isHoliday(formatDateISO(date));

  /** Checks if a date is the day before a public holiday. */
  const checkIsHolidayEve = (date: Date, timezone: string): boolean =>
    indexFor(timezone).isHolidayEve(formatDateISO(date));

  /**
   * Localized name of a public holiday, or null.
   * Defaults to the active UI locale — it previously defaulted to French, and every
   * caller omitted the argument, so English users saw French holiday names.
   */
  const getHolidayName = (date: Date, timezone: string, locale?: string): string | null =>
    indexFor(timezone, locale).getName(formatDateISO(date));

  return {
    isDateAllowed,
    checkIsHoliday,
    checkIsHolidayEve,
    getHolidayName,
  };
}
