/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * The composable reaches i18n, whose module body reads window.location to pick the
 * initial locale.
 *
 * @vitest-environment jsdom
 */

import { describe, expect, it } from 'vitest';
import { createApp, ref } from 'vue';

import { useParticipantCalendar } from './useParticipantCalendar';
import { i18n } from '@/i18n';
import type { CalendarWithParticipants } from '@/types';

/**
 * The composable calls useI18n(), which needs a component instance with the plugin
 * installed. A one-off app supplies both without pulling in a component-testing library.
 */
function withSetup<T>(composable: () => T): T {
  let result!: T;
  const app = createApp({
    setup() {
      result = composable();

      return () => null;
    },
  });
  app.use(i18n);
  app.mount(document.createElement('div'));

  return result;
}

/**
 * weekStart.test.ts covers the CLDR resolution itself. What is checked here is the
 * wiring: that the model derives its first day from the *calendar's* timezone rather
 * than from the reader's language. Without this, reverting the call site to
 * getWeekStartDay(locale) would leave every unit test green while putting German
 * calendars back on Sunday — which is the whole of issue #48.
 */

function modelFor(timezone: string) {
  const calendar = {
    id: 'calendar-1',
    name: 'Test',
    timezone,
    threshold: 1,
    allowed_weekdays: [0, 1, 2, 3, 4, 5, 6],
    holidays_policy: 'ignore',
    participants: [],
  } as unknown as CalendarWithParticipants;

  return withSetup(() =>
    useParticipantCalendar({
      calendar: ref(calendar),
      availabilities: ref([]),
      recurrences: ref([]),
      participantCounts: ref({}),
      dateSummaries: ref([]),
      months: ref([{ year: 2026, month: 8 }]),
      weekStarts: ref([]),
    })
  );
}

describe('the first day of the week', () => {
  it('follows the calendar timezone, not the reader language', () => {
    // The case from the issue: a German calendar must open on Monday.
    expect(modelFor('Europe/Berlin').weekStartDay.value).toBe(1);
    // And the convention genuinely differs by country, so a US calendar must not.
    expect(modelFor('America/New_York').weekStartDay.value).toBe(0);
  });

  it('handles regions whose week starts on neither Sunday nor Monday', () => {
    // Egypt starts on Saturday and the Maldives on Friday. A Sunday-or-Monday branch
    // cannot express either, which is why the grid rotates rather than special-casing.
    expect(modelFor('Africa/Cairo').weekStartDay.value).toBe(6);
    expect(modelFor('Indian/Maldives').weekStartDay.value).toBe(5);
  });

  it('reaches the month grid, not just the model', () => {
    // The offset is what pads the leading cells, so a first day that never reaches
    // buildMonthModel would be a setting with no effect on screen.
    const [berlin] = modelFor('Europe/Berlin').months.value;
    const [newYork] = modelFor('America/New_York').months.value;

    expect(berlin.weekdayHeaders[0]).not.toBe(newYork.weekdayHeaders[0]);
    // 1 September 2026 is a Tuesday: one cell after Monday, two after Sunday.
    expect(berlin.days.findIndex(day => day.date === '2026-09-01')).toBe(1);
    expect(newYork.days.findIndex(day => day.date === '2026-09-01')).toBe(2);
  });
});
