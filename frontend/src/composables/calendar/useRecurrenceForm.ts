/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * The form state behind the recurrence editor: which weekdays may be picked, what the
 * calendar's per-weekday opening hours allow, and whether the current values are
 * submittable.
 *
 * All of it was inline in `ParticipantView` — two reactive forms, six computed bounds,
 * three watchers and a `watchEffect`, spread across three hundred lines that also
 * contained e-mail enrolment and clipboard handling. None of it touches the network, so
 * pulling it out makes the part with the actual rules testable on its own.
 *
 * The rules worth knowing:
 *
 *   - a calendar may restrict which weekdays can be answered at all, and the day
 *     selector must show only those;
 *   - each allowed weekday may carry a `min_time`/`max_time` window, and the two time
 *     pickers have to combine that window with each other — the start cannot exceed the
 *     chosen end, nor the day's maximum;
 *   - equal start and end are rejected, because a zero-length recurrence is not a
 *     refusal, it is a mistake.
 */

import { computed, reactive, ref, watch, watchEffect, type ComputedRef, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { resolveWeekStart } from '@/utils/weekStart';
import type { CreateRecurrenceRequest, PublicCalendar, RecurrenceWithExceptions } from '@/types';

const WEEKDAY_KEYS = [
  'availability.sunday',
  'availability.monday',
  'availability.tuesday',
  'availability.wednesday',
  'availability.thursday',
  'availability.friday',
  'availability.saturday',
];

/** The earlier of two optional `HH:MM` strings; either may be missing. */
export function minTime(a: string | undefined, b: string | undefined): string | undefined {
  if (!a) return b;
  if (!b) return a;
  return a < b ? a : b;
}

/** The later of two optional `HH:MM` strings; either may be missing. */
export function maxTime(a: string | undefined, b: string | undefined): string | undefined {
  if (!a) return b;
  if (!b) return a;
  return a > b ? a : b;
}

/**
 * Turn a filled form into a request, dropping every empty optional field.
 *
 * An empty string is not "no time"; sending one would store a bound the participant did
 * not set, and the backend would reject it as malformed.
 */
export function buildRecurrenceRequest(form: CreateRecurrenceRequest): CreateRecurrenceRequest {
  const data: CreateRecurrenceRequest = {
    day_of_week: form.day_of_week,
    start_date: form.start_date,
  };

  if (form.start_time) data.start_time = form.start_time;
  if (form.end_time) data.end_time = form.end_time;
  if (form.end_date) data.end_date = form.end_date;
  if (form.note) data.note = form.note;

  return data;
}

/** A weekday the calendar accepts, already labelled in the active locale. */
export interface WeekDayOption {
  value: number;
  label: string;
}

/** `models.TimeRange`, as the weekday restriction map holds it. */
interface TimeRestrictions {
  min_time?: string;
  max_time?: string;
}

export interface RecurrenceForm {
  readonly weekDaysOptions: ComputedRef<WeekDayOption[]>;
  readonly newRecurrence: CreateRecurrenceRequest;
  readonly editingRecurrenceId: Ref<string | null>;
  readonly editingRecurrence: CreateRecurrenceRequest;
  readonly newRecurrenceStartTimeMax: ComputedRef<string | undefined>;
  readonly newRecurrenceEndTimeMin: ComputedRef<string | undefined>;
  readonly newRecurrenceMinTime: ComputedRef<string | undefined>;
  readonly newRecurrenceMaxTime: ComputedRef<string | undefined>;
  readonly editingRecurrenceStartTimeMax: ComputedRef<string | undefined>;
  readonly editingRecurrenceEndTimeMin: ComputedRef<string | undefined>;
  readonly editingRecurrenceMinTime: ComputedRef<string | undefined>;
  readonly editingRecurrenceMaxTime: ComputedRef<string | undefined>;
  readonly hasEqualTimesNewRecurrence: ComputedRef<boolean>;
  readonly hasEqualTimesEditingRecurrence: ComputedRef<boolean>;
  resetNewRecurrence(): void;
  startEditing(recurrence: RecurrenceWithExceptions): void;
  resetEditing(): void;
}

const EMPTY_FORM: CreateRecurrenceRequest = {
  day_of_week: 1, // Monday by default
  start_time: '',
  end_time: '',
  note: '',
  start_date: '',
  end_date: '',
};

export function useRecurrenceForm(
  calendar: Ref<PublicCalendar | undefined> | ComputedRef<PublicCalendar | undefined>
): RecurrenceForm {
  const { t, locale } = useI18n();

  const newRecurrence = reactive<CreateRecurrenceRequest>({ ...EMPTY_FORM });
  const editingRecurrenceId = ref<string | null>(null);
  const editingRecurrence = reactive<CreateRecurrenceRequest>({ ...EMPTY_FORM });

  const weekDaysOptions = computed<WeekDayOption[]>(() => {
    const firstDayOfWeek = resolveWeekStart(calendar.value?.timezone, locale.value);

    const daysOrder: WeekDayOption[] = [];
    for (let i = 0; i < 7; i++) {
      const dayValue = (firstDayOfWeek + i) % 7;
      daysOrder.push({ value: dayValue, label: t(WEEKDAY_KEYS[dayValue]) });
    }

    // Filter days based on calendar's allowed weekdays
    const allowedWeekdays = calendar.value?.allowed_weekdays;
    if (allowedWeekdays && allowedWeekdays.length > 0) {
      return daysOrder.filter(day => allowedWeekdays.includes(day.value));
    }

    return daysOrder;
  });

  function restrictionsFor(dayOfWeek: number | null | undefined): TimeRestrictions {
    if (!calendar.value?.weekday_times || dayOfWeek === null || dayOfWeek === undefined) {
      return {};
    }
    return calendar.value.weekday_times[dayOfWeek] || {};
  }

  const newRecurrenceTimeRestrictions = computed(() => restrictionsFor(newRecurrence.day_of_week));
  const editingRecurrenceTimeRestrictions = computed(() =>
    restrictionsFor(editingRecurrence.day_of_week)
  );

  const newRecurrenceStartTimeMax = computed(() =>
    minTime(newRecurrenceTimeRestrictions.value.max_time, newRecurrence.end_time || undefined)
  );
  const newRecurrenceEndTimeMin = computed(() =>
    maxTime(newRecurrence.start_time || undefined, newRecurrenceTimeRestrictions.value.min_time)
  );
  const newRecurrenceMinTime = computed(
    () => newRecurrenceTimeRestrictions.value.min_time || undefined
  );
  const newRecurrenceMaxTime = computed(
    () => newRecurrenceTimeRestrictions.value.max_time || undefined
  );

  const editingRecurrenceStartTimeMax = computed(() =>
    minTime(
      editingRecurrenceTimeRestrictions.value.max_time,
      editingRecurrence.end_time || undefined
    )
  );
  const editingRecurrenceEndTimeMin = computed(() =>
    maxTime(
      editingRecurrence.start_time || undefined,
      editingRecurrenceTimeRestrictions.value.min_time
    )
  );
  const editingRecurrenceMinTime = computed(
    () => editingRecurrenceTimeRestrictions.value.min_time || undefined
  );
  const editingRecurrenceMaxTime = computed(
    () => editingRecurrenceTimeRestrictions.value.max_time || undefined
  );

  // Validation: start and end must not be the same instant (both set and equal)
  const hasEqualTimesNewRecurrence = computed(() =>
    Boolean(
      newRecurrence.start_time &&
      newRecurrence.end_time &&
      newRecurrence.start_time === newRecurrence.end_time
    )
  );
  const hasEqualTimesEditingRecurrence = computed(() =>
    Boolean(
      editingRecurrence.start_time &&
      editingRecurrence.end_time &&
      editingRecurrence.start_time === editingRecurrence.end_time
    )
  );

  // Automatically set the default day_of_week to the first allowed weekday
  watchEffect(() => {
    if (weekDaysOptions.value.length > 0 && newRecurrence.day_of_week !== null) {
      // Only update if the current value is not in the allowed list
      const isCurrentAllowed = weekDaysOptions.value.some(
        day => day.value === newRecurrence.day_of_week
      );
      if (!isCurrentAllowed) {
        newRecurrence.day_of_week = weekDaysOptions.value[0].value;
      }
    } else if (weekDaysOptions.value.length > 0) {
      // Initialize with first value if day_of_week is null
      newRecurrence.day_of_week = weekDaysOptions.value[0].value;
    }
  });

  // Picking another weekday replaces the times outright: the previous day's window is
  // meaningless on the new one, and keeping it silently produced out-of-range requests.
  watch(
    () => newRecurrence.day_of_week,
    newDay => {
      if (newDay !== null && calendar.value?.weekday_times) {
        const restrictions = calendar.value.weekday_times[newDay] || {};
        newRecurrence.start_time = restrictions.min_time || '';
        newRecurrence.end_time = restrictions.max_time || '';
      }
    }
  );

  // The calendar arrives after the form is created, so seed the empty fields once it
  // does — without overriding anything the participant has already typed.
  watch(
    () => calendar.value?.weekday_times,
    weekdayTimes => {
      if (weekdayTimes && newRecurrence.day_of_week !== null) {
        const restrictions = weekdayTimes[newRecurrence.day_of_week] || {};
        if (!newRecurrence.start_time) {
          newRecurrence.start_time = restrictions.min_time || '';
        }
        if (!newRecurrence.end_time) {
          newRecurrence.end_time = restrictions.max_time || '';
        }
      }
    }
  );

  // Editing is more conservative: an existing recurrence already has times worth
  // keeping, so a weekday's window only fills the blanks.
  watch(
    () => editingRecurrence.day_of_week,
    newDay => {
      if (newDay !== null && calendar.value?.weekday_times && editingRecurrenceId.value) {
        const restrictions = calendar.value.weekday_times[newDay] || {};

        if (restrictions.min_time && !editingRecurrence.start_time) {
          editingRecurrence.start_time = restrictions.min_time;
        }

        if (restrictions.max_time && !editingRecurrence.end_time) {
          editingRecurrence.end_time = restrictions.max_time;
        }
      }
    }
  );

  function resetNewRecurrence(): void {
    Object.assign(newRecurrence, EMPTY_FORM);
  }

  function startEditing(recurrence: RecurrenceWithExceptions): void {
    editingRecurrenceId.value = recurrence.id;
    editingRecurrence.day_of_week = recurrence.day_of_week;
    editingRecurrence.start_time = recurrence.start_time || '';
    editingRecurrence.end_time = recurrence.end_time || '';
    editingRecurrence.note = recurrence.note || '';
    editingRecurrence.start_date = recurrence.start_date;
    editingRecurrence.end_date = recurrence.end_date || '';
  }

  function resetEditing(): void {
    editingRecurrenceId.value = null;
    Object.assign(editingRecurrence, EMPTY_FORM);
  }

  return {
    weekDaysOptions,
    newRecurrence,
    editingRecurrenceId,
    editingRecurrence,
    newRecurrenceStartTimeMax,
    newRecurrenceEndTimeMin,
    newRecurrenceMinTime,
    newRecurrenceMaxTime,
    editingRecurrenceStartTimeMax,
    editingRecurrenceEndTimeMin,
    editingRecurrenceMinTime,
    editingRecurrenceMaxTime,
    hasEqualTimesNewRecurrence,
    hasEqualTimesEditingRecurrence,
    resetNewRecurrence,
    startEditing,
    resetEditing,
  };
}
