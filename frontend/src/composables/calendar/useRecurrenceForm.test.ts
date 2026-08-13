/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, describe, expect, it } from 'vitest';
import { nextTick, ref } from 'vue';
import { withComposable } from '@/test/harness';
import { buildRecurrenceRequest, maxTime, minTime, useRecurrenceForm } from './useRecurrenceForm';
import type { PublicCalendar, RecurrenceWithExceptions } from '@/types';

/**
 * The recurrence form is where a calendar's rules meet what a participant is allowed to
 * type. Every case below is one of those rules, and each of them used to be enforced by
 * an anonymous computed buried three hundred lines into the participant view.
 */

interface CalendarShape {
  timezone?: string;
  allowed_weekdays?: number[];
  weekday_times?: Record<number, { min_time?: string; max_time?: string }>;
}

/** A calendar with only the fields this composable reads. */
function calendarOf(shape: CalendarShape): PublicCalendar {
  return shape as unknown as PublicCalendar;
}

function recurrenceOf(overrides: Partial<RecurrenceWithExceptions>): RecurrenceWithExceptions {
  return {
    id: 'r1',
    participant_id: 'p1',
    day_of_week: 2,
    start_date: '2026-03-02',
    created_at: '2026-01-01T00:00:00Z',
    exceptions: [],
    ...overrides,
  } as RecurrenceWithExceptions;
}

const harnesses: Array<{ unmount(): void }> = [];

function mountForm(calendar: PublicCalendar | undefined, locale: 'en' | 'fr' = 'en') {
  const calendarRef = ref<PublicCalendar | undefined>(calendar);
  const harness = withComposable(() => useRecurrenceForm(calendarRef), { locale });
  harnesses.push(harness);
  return { form: harness.result, calendarRef };
}

afterEach(() => {
  while (harnesses.length > 0) harnesses.pop()!.unmount();
  localStorage.clear();
});

describe('minTime / maxTime', () => {
  const cases: [string, string | undefined, string | undefined, string | undefined][] = [
    ['both set', '09:00', '17:00', '09:00'],
    ['first missing falls back to the second', undefined, '17:00', '17:00'],
    ['second missing falls back to the first', '09:00', undefined, '09:00'],
    ['neither set stays undefined', undefined, undefined, undefined],
  ];

  for (const [name, a, b, expected] of cases) {
    it(`minTime: ${name}`, () => {
      expect(minTime(a, b)).toBe(expected);
    });
  }

  it('maxTime picks the later of the two', () => {
    expect(maxTime('09:00', '17:00')).toBe('17:00');
    expect(maxTime(undefined, '17:00')).toBe('17:00');
    expect(maxTime('09:00', undefined)).toBe('09:00');
    expect(maxTime(undefined, undefined)).toBeUndefined();
  });
});

describe('buildRecurrenceRequest', () => {
  it('keeps only what was filled in', () => {
    const request = buildRecurrenceRequest({
      day_of_week: 3,
      start_date: '2026-03-04',
      start_time: '',
      end_time: '18:00',
      end_date: '',
      note: '',
    });

    // An empty string is not "no time": sending one would store a bound nobody set.
    expect(request).toEqual({ day_of_week: 3, start_date: '2026-03-04', end_time: '18:00' });
  });

  it('carries every optional field when they are all set', () => {
    expect(
      buildRecurrenceRequest({
        day_of_week: 1,
        start_date: '2026-03-02',
        start_time: '09:00',
        end_time: '17:00',
        end_date: '2026-06-01',
        note: 'standup',
      })
    ).toEqual({
      day_of_week: 1,
      start_date: '2026-03-02',
      start_time: '09:00',
      end_time: '17:00',
      end_date: '2026-06-01',
      note: 'standup',
    });
  });
});

describe('weekDaysOptions', () => {
  it('starts the week where the calendar timezone says it does', () => {
    const paris = mountForm(calendarOf({ timezone: 'Europe/Paris' }));
    expect(paris.form.weekDaysOptions.value.map(d => d.value)).toEqual([1, 2, 3, 4, 5, 6, 0]);

    const newYork = mountForm(calendarOf({ timezone: 'America/New_York' }));
    expect(newYork.form.weekDaysOptions.value.map(d => d.value)).toEqual([0, 1, 2, 3, 4, 5, 6]);
  });

  it('offers only the weekdays the calendar allows, in the rotated order', () => {
    const { form } = mountForm(
      calendarOf({ timezone: 'Europe/Paris', allowed_weekdays: [0, 2, 4] })
    );
    expect(form.weekDaysOptions.value.map(d => d.value)).toEqual([2, 4, 0]);
  });

  it('labels the days in the active locale', () => {
    const { form } = mountForm(calendarOf({ timezone: 'Europe/Paris' }), 'fr');
    expect(form.weekDaysOptions.value[0].label).toBe('Lundi');
  });

  /*
   * The default day is Monday. On a calendar that does not allow Mondays, leaving it
   * there would show a select whose value matches none of its options — which renders
   * blank and posts a day the backend rejects.
   */
  it('moves the default day into the allowed set', async () => {
    const { form } = mountForm(calendarOf({ timezone: 'Europe/Paris', allowed_weekdays: [3, 5] }));
    await nextTick();
    expect(form.newRecurrence.day_of_week).toBe(3);
  });
});

describe('time restrictions', () => {
  const calendar = calendarOf({
    timezone: 'Europe/Paris',
    weekday_times: { 1: { min_time: '09:00', max_time: '18:00' } },
  });

  it('seeds the times from the weekday window once the calendar arrives', async () => {
    const calendarRef = ref<PublicCalendar | undefined>(undefined);
    const harness = withComposable(() => useRecurrenceForm(calendarRef));
    harnesses.push(harness);

    expect(harness.result.newRecurrence.start_time).toBe('');

    calendarRef.value = calendar;
    await nextTick();

    expect(harness.result.newRecurrence.start_time).toBe('09:00');
    expect(harness.result.newRecurrence.end_time).toBe('18:00');
  });

  it('bounds the start picker by both the day window and the chosen end', async () => {
    const { form } = mountForm(calendar);
    await nextTick();

    expect(form.newRecurrenceMinTime.value).toBe('09:00');
    // The end has been pulled in to 12:00, so the start may no longer reach 18:00.
    form.newRecurrence.end_time = '12:00';
    expect(form.newRecurrenceStartTimeMax.value).toBe('12:00');
  });

  it('bounds the end picker by both the day window and the chosen start', async () => {
    const { form } = mountForm(calendar);
    await nextTick();

    form.newRecurrence.start_time = '14:00';
    expect(form.newRecurrenceEndTimeMin.value).toBe('14:00');
    expect(form.newRecurrenceMaxTime.value).toBe('18:00');
  });

  it('leaves the pickers unbounded on a calendar with no window for that day', async () => {
    const { form } = mountForm(calendarOf({ timezone: 'Europe/Paris', weekday_times: {} }));
    await nextTick();

    expect(form.newRecurrenceMinTime.value).toBeUndefined();
    expect(form.newRecurrenceMaxTime.value).toBeUndefined();
  });

  /*
   * Changing the weekday replaces the times outright rather than keeping them: the
   * previous day's window says nothing about the new one, and keeping a 09:00 start on
   * a day that opens at 14:00 produced a request the backend refused.
   */
  it('replaces the times when the new-recurrence weekday changes', async () => {
    // Mounted empty, then filled: the calendar always arrives after the form exists,
    // and the seeding watcher is what puts the first day's window into the pickers.
    const { form, calendarRef } = mountForm(undefined);
    calendarRef.value = calendarOf({
      timezone: 'Europe/Paris',
      weekday_times: {
        1: { min_time: '09:00', max_time: '18:00' },
        3: { min_time: '14:00', max_time: '16:00' },
      },
    });
    await nextTick();
    expect(form.newRecurrence.start_time).toBe('09:00');

    form.newRecurrence.day_of_week = 3;
    await nextTick();

    expect(form.newRecurrence.start_time).toBe('14:00');
    expect(form.newRecurrence.end_time).toBe('16:00');
  });

  /*
   * Editing is the opposite: an existing rule already carries times worth keeping, so
   * the window only fills what is blank.
   */
  it('only fills the blanks when editing', async () => {
    const { form } = mountForm(
      calendarOf({
        timezone: 'Europe/Paris',
        weekday_times: { 4: { min_time: '08:00', max_time: '20:00' } },
      })
    );

    form.startEditing(recurrenceOf({ id: 'r9', day_of_week: 2, start_time: '10:00' }));
    form.editingRecurrence.day_of_week = 4;
    await nextTick();

    expect(form.editingRecurrence.start_time).toBe('10:00');
    expect(form.editingRecurrence.end_time).toBe('20:00');
  });
});

describe('validation', () => {
  it('rejects a zero-length range but accepts a half-open one', async () => {
    const { form } = mountForm(calendarOf({ timezone: 'Europe/Paris' }));
    await nextTick();

    form.newRecurrence.start_time = '10:00';
    form.newRecurrence.end_time = '10:00';
    expect(form.hasEqualTimesNewRecurrence.value).toBe(true);

    form.newRecurrence.end_time = '';
    expect(form.hasEqualTimesNewRecurrence.value).toBe(false);

    form.newRecurrence.start_time = '';
    form.newRecurrence.end_time = '';
    expect(form.hasEqualTimesNewRecurrence.value).toBe(false);
  });
});

describe('editing lifecycle', () => {
  it('loads a recurrence into the edit form and clears it again', () => {
    const { form } = mountForm(calendarOf({ timezone: 'Europe/Paris' }));

    form.startEditing(
      recurrenceOf({
        id: 'r7',
        day_of_week: 5,
        start_time: '09:30',
        end_time: '11:00',
        start_date: '2026-04-03',
        end_date: '2026-05-01',
        note: 'sprint review',
      })
    );

    expect(form.editingRecurrenceId.value).toBe('r7');
    expect(form.editingRecurrence).toMatchObject({
      day_of_week: 5,
      start_time: '09:30',
      end_time: '11:00',
      start_date: '2026-04-03',
      end_date: '2026-05-01',
      note: 'sprint review',
    });

    form.resetEditing();

    expect(form.editingRecurrenceId.value).toBeNull();
    expect(form.editingRecurrence).toMatchObject({
      day_of_week: 1,
      start_time: '',
      end_time: '',
      start_date: '',
      end_date: '',
      note: '',
    });
  });

  it('normalises a recurrence whose optional fields are absent', () => {
    const { form } = mountForm(calendarOf({ timezone: 'Europe/Paris' }));

    form.startEditing(recurrenceOf({ start_time: undefined, note: undefined }));

    // Undefined would leave the inputs uncontrolled; the form uses empty strings.
    expect(form.editingRecurrence.start_time).toBe('');
    expect(form.editingRecurrence.note).toBe('');
  });

  it('resets the create form back to its defaults', async () => {
    const { form } = mountForm(calendarOf({ timezone: 'Europe/Paris' }));
    await nextTick();

    form.newRecurrence.start_date = '2026-03-02';
    form.newRecurrence.note = 'weekly';
    form.resetNewRecurrence();

    expect(form.newRecurrence).toMatchObject({
      day_of_week: 1,
      start_date: '',
      end_date: '',
      note: '',
    });
  });
});
