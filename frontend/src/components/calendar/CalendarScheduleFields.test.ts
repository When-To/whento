/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, describe, expect, it } from 'vitest';
import { lastEmit, mountWithI18n } from '@/test/harness';
import CalendarScheduleFields from './CalendarScheduleFields.vue';
import { createEmptyWeekdayTimes } from '@/utils/calendar/weekdayTimes';

/**
 * The "allow/block days and hours" panel, shared by calendar creation and calendar
 * settings after two hundred identical lines were maintained in each.
 *
 * What is worth testing is what the two copies had to agree on: the weekday row follows
 * the calendar's timezone, an unselected day cannot be given opening hours, and the
 * holiday windows only open when the policy actually allows holidays.
 */

const BASE_PROPS = {
  timezone: 'Europe/Paris',
  startDate: '',
  endDate: '',
  allowedWeekdays: [0, 1, 2, 3, 4, 5, 6],
  weekdayTimes: createEmptyWeekdayTimes(),
  holidaysPolicy: 'ignore' as const,
  holidayMinTime: '',
  holidayMaxTime: '',
  allowHolidayEves: false,
  holidayEveMinTime: '',
  holidayEveMaxTime: '',
};

function mountFields(props: Record<string, unknown> = {}) {
  return mountWithI18n(CalendarScheduleFields, { props: { ...BASE_PROPS, ...props } });
}

/** The seven weekday buttons, in rendered order. */
function weekdayButtons(wrapper: ReturnType<typeof mountFields>) {
  return wrapper.findAll('button[type="button"]');
}

afterEach(() => {
  localStorage.clear();
});

describe('CalendarScheduleFields', () => {
  it('starts the weekday row where the calendar timezone does', () => {
    const paris = weekdayButtons(mountFields()).map(b => b.text());
    expect(paris[0]).toBe('Mon');
    expect(paris[6]).toBe('Sun');

    const newYork = weekdayButtons(mountFields({ timezone: 'America/New_York' })).map(b =>
      b.text()
    );
    expect(newYork[0]).toBe('Sun');
  });

  it('adds a weekday that was blocked', async () => {
    const allowedWeekdays = [1, 2];
    const wrapper = mountFields({ allowedWeekdays });

    // Third button in Monday-first order is Wednesday (3).
    await weekdayButtons(wrapper)[2].trigger('click');

    expect(allowedWeekdays).toEqual([1, 2, 3]);
  });

  it('removes a weekday that was allowed', async () => {
    const allowedWeekdays = [1, 2, 3];
    const wrapper = mountFields({ allowedWeekdays });

    await weekdayButtons(wrapper)[1].trigger('click');

    expect(allowedWeekdays).toEqual([1, 3]);
  });

  /*
   * A calendar with no open weekday cannot be answered at all, and there is no way back
   * through the UI: the last button would simply stop responding.
   */
  it('will not let the last weekday be removed', async () => {
    const allowedWeekdays = [4];
    const wrapper = mountFields({ allowedWeekdays });

    // Thursday is the fourth button in Monday-first order.
    await weekdayButtons(wrapper)[3].trigger('click');

    expect(allowedWeekdays).toEqual([4]);
  });

  it('disables the opening hours of days that are blocked', () => {
    const wrapper = mountFields({ allowedWeekdays: [1] });
    const times = wrapper.findAllComponents({ name: 'TimeSelect' });

    // Fourteen weekday pickers (a start and an end for each of seven days), then the
    // four holiday ones. Monday is first in this timezone, so only its two are live.
    expect(times[0].props('disabled')).toBe(false);
    expect(times[1].props('disabled')).toBe(true);
    expect(times[7].props('disabled')).toBe(false);
    expect(times[8].props('disabled')).toBe(true);
  });

  /*
   * A holiday window is only meaningful when holidays are answerable at all: "ignore"
   * and "block" both mean there is nothing to bound.
   */
  it('opens the holiday window only when the policy allows holidays', () => {
    const ignored = mountFields().findAllComponents({ name: 'TimeSelect' });
    expect(ignored[14].props('disabled')).toBe(true);

    const allowed = mountFields({ holidaysPolicy: 'allow' }).findAllComponents({
      name: 'TimeSelect',
    });
    expect(allowed[14].props('disabled')).toBe(false);
    expect(allowed[15].props('disabled')).toBe(false);
  });

  /*
   * "Holiday eve" means the working day before a holiday. With every weekday already
   * open there is no such thing to unblock, so the option is turned off entirely.
   */
  it('disables holiday eves when every weekday is already allowed', () => {
    const wrapper = mountFields();
    expect(wrapper.find('#allow-holiday-eves').attributes('disabled')).toBeDefined();

    const partial = mountFields({ allowedWeekdays: [1, 2, 3, 4, 5] });
    expect(partial.find('#allow-holiday-eves').attributes('disabled')).toBeUndefined();
  });

  it('opens the holiday-eve window only when eves are allowed and selectable', () => {
    const off = mountFields({ allowedWeekdays: [1, 2, 3, 4, 5] }).findAllComponents({
      name: 'TimeSelect',
    });
    expect(off[16].props('disabled')).toBe(true);

    const on = mountFields({
      allowedWeekdays: [1, 2, 3, 4, 5],
      allowHolidayEves: true,
    }).findAllComponents({ name: 'TimeSelect' });
    expect(on[16].props('disabled')).toBe(false);
    expect(on[17].props('disabled')).toBe(false);
  });

  it('emits the date range through its models', async () => {
    const wrapper = mountFields();
    const dates = wrapper.findAll('input[type="date"]');

    await dates[0].setValue('2026-03-01');
    await dates[1].setValue('2026-06-30');

    expect(lastEmit(wrapper, 'update:startDate')).toEqual(['2026-03-01']);
    expect(lastEmit(wrapper, 'update:endDate')).toEqual(['2026-06-30']);
  });
});
