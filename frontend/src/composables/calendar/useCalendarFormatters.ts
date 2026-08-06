/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * The single point where vue-i18n meets the calendar model.
 *
 * Everything downstream receives preformatted strings, so no template calls a formatter
 * and no builder imports vue-i18n.
 */

import { computed, type ComputedRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatDate } from '@/utils/date/intlFormatters';
import { isFullDay } from '@/utils/date/timeRange';
import type { DayStatus } from '@/types/calendar';
import type { CalendarFormatters } from '@/utils/calendar/dayModel';

export interface UseCalendarFormatters {
  readonly formatters: ComputedRef<CalendarFormatters>;
  /** The active locale, for anything that still needs it directly. */
  readonly locale: ComputedRef<string>;
}

export function useCalendarFormatters(): UseCalendarFormatters {
  const { t, locale } = useI18n();

  // `locale.value` is 'fr' | 'en', which is already a valid BCP-47 tag and is passed
  // straight to Intl. The old code used three different conventions across three
  // files, one of which compared against a tag that could never match.
  const activeLocale = computed(() => locale.value);

  const formatters = computed<CalendarFormatters>(() => {
    const code = activeLocale.value;

    const timeRange = (startTime?: string, endTime?: string): string =>
      isFullDay(startTime, endTime)
        ? t('availability.allDay')
        : `${startTime ?? '00:00'}-${endTime ?? '23:59'}`;

    return {
      weekdayShort: date => formatDate(date, code, 'weekdayShort'),
      weekdayLong: date => formatDate(date, code, 'weekdayLong'),
      dayMonthShort: date => formatDate(date, code, 'dayMonthShort'),
      fullDate: date => formatDate(date, code, 'fullDate'),
      monthYear: date => formatDate(date, code, 'monthYear'),
      timeRange,
      dayAria: (fullDate, count, threshold, status: DayStatus) => {
        if (status === 'outside') return fullDate;
        if (status === 'disabled') {
          return `${fullDate}, ${t('calendar.dateNotAllowed')}`;
        }
        return `${fullDate}, ${t('calendar.participantsAvailable', { count, threshold })}`;
      },
    };
  });

  return { formatters, locale: activeLocale };
}
