/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * The labels the two list panels of the participant view print: a date, a time range,
 * and a weekday name.
 *
 * They were three private functions of `ParticipantView`, which is why extracting the
 * recurrence editor and the availability list would otherwise have duplicated them. They
 * are kept apart from `useCalendarFormatters` on purpose: that one feeds the grid model
 * and formats a *cell*, this one formats a *row* and uses a different date style.
 */

import { useI18n } from 'vue-i18n';

const WEEKDAY_KEYS = [
  'availability.sunday',
  'availability.monday',
  'availability.tuesday',
  'availability.wednesday',
  'availability.thursday',
  'availability.friday',
  'availability.saturday',
];

/**
 * Whether a `YYYY-MM-DD` date is today or later, which is what decides if a stored
 * answer can still be edited.
 *
 * Pure, and exported separately so it can be tested without mounting vue-i18n.
 */
export function isDateInFuture(dateStr: string): boolean {
  const date = new Date(dateStr);
  date.setHours(0, 0, 0, 0);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return date >= today;
}

export interface AvailabilityLabels {
  formatDate(dateStr: string): string;
  formatTimeRange(startTime?: string, endTime?: string): string;
  getDayName(dayOfWeek: number): string;
  isDateInFuture(dateStr: string): boolean;
}

export function useAvailabilityLabels(): AvailabilityLabels {
  const { t, locale } = useI18n();

  /**
   * NOTE: `new Date('YYYY-MM-DD')` parses as UTC midnight, so this renders the previous
   * day west of Greenwich. That is pre-existing behaviour, shared by every caller, and
   * is left untouched here — changing it is a fix, not a move, and belongs in its own
   * change with its own test.
   */
  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    const localeCode = locale.value;
    return new Intl.DateTimeFormat(localeCode, {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      year: 'numeric',
    }).format(date);
  }

  function formatTimeRange(startTime?: string, endTime?: string): string {
    // Check if it's a full day range (00:00-23:59 or no times)
    const start = startTime || '00:00';
    const end = endTime || '23:59';

    if (start === '00:00' && end === '23:59') {
      return t('availability.allDay');
    }

    if (startTime && endTime) {
      return `${startTime} - ${endTime}`;
    } else if (startTime) {
      return `${t('availability.startTime')}: ${startTime}`;
    } else if (endTime) {
      return `${t('availability.endTime')}: ${endTime}`;
    }
    return t('availability.allDay');
  }

  function getDayName(dayOfWeek: number): string {
    return t(WEEKDAY_KEYS[dayOfWeek]);
  }

  return { formatDate, formatTimeRange, getDayName, isDateInFuture };
}
