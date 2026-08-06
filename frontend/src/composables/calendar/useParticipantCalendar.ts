/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Owner of the calendar view model.
 *
 * Called once by the orchestrating view. Holiday lookup, date rules, the per-date index
 * and the formatters are built once here and shared by every rendered month, week and
 * list — previously each of the twelve mounted month grids and four week grids rebuilt
 * all of it independently from the same props.
 */

import { computed, type ComputedRef, type Ref } from 'vue';
import { getWeekStartDay } from '@/i18n';
import type {
  Availability,
  CalendarWithParticipants,
  DateAvailabilitySummary,
  RecurrenceWithExceptions,
} from '@/types';
import type { DayModel, MonthModel } from '@/types/calendar';
import { todayISO, type ISODate } from '@/utils/date/isoDate';
import { getHolidayIndex } from '@/utils/calendar/holidays';
import { buildCalendarRules } from '@/utils/calendar/dateRules';
import { buildDayIndex } from '@/utils/calendar/dayIndex';
import {
  buildListDays,
  buildMonthModel,
  buildWeekDays,
  type DateRange,
  type ModelDeps,
} from '@/utils/calendar/dayModel';
import { useCalendarFormatters } from './useCalendarFormatters';

/** A month to render, as `{ year, month }` with a 0-indexed month. */
export interface MonthRef {
  readonly year: number;
  readonly month: number;
}

export interface UseParticipantCalendarOptions {
  readonly calendar: Ref<CalendarWithParticipants | null>;
  readonly availabilities: Ref<readonly Availability[]>;
  readonly recurrences: Ref<readonly RecurrenceWithExceptions[]>;
  readonly participantCounts: Ref<Readonly<Record<string, number>>>;
  readonly dateSummaries: Ref<readonly DateAvailabilitySummary[]>;
  /** Dates to highlight, e.g. the selected participants' common dates. */
  readonly highlighted?: Ref<ReadonlySet<ISODate>>;
  /** Months currently rendered, in display order. */
  readonly months: Ref<readonly MonthRef[]>;
  /** Week start dates currently rendered, in display order. */
  readonly weekStarts: Ref<readonly ISODate[]>;
}

export interface ParticipantCalendarModel {
  readonly deps: ComputedRef<ModelDeps>;
  readonly weekStartDay: ComputedRef<number>;
  /** Today, in the calendar's timezone. */
  readonly today: ComputedRef<ISODate>;
  readonly months: ComputedRef<readonly MonthModel[]>;
  /** Seven days per rendered week, in display order. */
  readonly weeks: ComputedRef<readonly (readonly DayModel[])[]>;
  /** Actionable days across whichever periods are currently rendered. */
  readonly listDays: ComputedRef<readonly DayModel[]>;
}

export function useParticipantCalendar(
  options: UseParticipantCalendarOptions
): ParticipantCalendarModel {
  const { formatters, locale } = useCalendarFormatters();

  const weekStartDay = computed(() => getWeekStartDay(locale.value));
  const timeZone = computed(() => options.calendar.value?.timezone || 'Europe/Paris');
  const today = computed(() => todayISO(timeZone.value));

  const rules = computed(() => {
    const calendar = options.calendar.value;
    return buildCalendarRules({
      timeZone: timeZone.value,
      todayISO: today.value,
      allowedWeekdays: calendar?.allowed_weekdays,
      holidaysPolicy: calendar?.holidays_policy,
      allowHolidayEves: calendar?.allow_holiday_eves,
      startDate: calendar?.start_date,
      endDate: calendar?.end_date,
      weekdayTimes: calendar?.weekday_times,
      holidayTimes: {
        min_time: calendar?.holiday_min_time,
        max_time: calendar?.holiday_max_time,
      },
      holidayEveTimes: {
        min_time: calendar?.holiday_eve_min_time,
        max_time: calendar?.holiday_eve_max_time,
      },
      threshold: calendar?.threshold,
    });
  });

  const holidays = computed(() => getHolidayIndex(timeZone.value, locale.value));

  const index = computed(() =>
    buildDayIndex({
      availabilities: options.availabilities.value,
      recurrences: options.recurrences.value,
      participantCounts: options.participantCounts.value,
      dateSummaries: options.dateSummaries.value,
    })
  );

  const deps = computed<ModelDeps>(() => ({
    rules: rules.value,
    holidays: holidays.value,
    index: index.value,
    fmt: formatters.value,
    weekStartDay: weekStartDay.value,
    highlighted: options.highlighted?.value,
  }));

  const months = computed(() =>
    options.months.value.map(ref => buildMonthModel(ref.year, ref.month, deps.value))
  );

  const weeks = computed(() =>
    options.weekStarts.value.map(start => buildWeekDays(start, deps.value))
  );

  const listDays = computed(() => {
    const ranges: DateRange[] = [];

    for (const month of months.value) {
      const days = month.days.filter(day => day.isCurrentMonth);
      if (days.length > 0) {
        ranges.push({ startISO: days[0].date, endISO: days[days.length - 1].date });
      }
    }

    for (const week of weeks.value) {
      if (week.length > 0) {
        ranges.push({ startISO: week[0].date, endISO: week[week.length - 1].date });
      }
    }

    return buildListDays(ranges, deps.value);
  });

  return { deps, weekStartDay, today, months, weeks, listDays };
}
