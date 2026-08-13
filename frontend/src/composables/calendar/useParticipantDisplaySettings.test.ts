/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { nextTick, ref } from 'vue';
import { withComposable } from '@/test/harness';
import { useCalendarHistoryStore } from '@/stores/calendarHistory';
import {
  defaultViewStyle,
  maxPeriodsFor,
  useParticipantDisplaySettings,
} from './useParticipantDisplaySettings';

/**
 * Two things go wrong with view settings, and both are invisible until a user complains.
 *
 * Either a change that moves the visible date range does not refetch, so the calendar
 * shows last month's answers under this month's header; or every change refetches, and
 * dragging the week's start hour fires a request per pixel.
 *
 * The tests below pin the split: mode and period count reload, hours and slot size do
 * not, and nothing at all is written before the calendar has loaded.
 */

const TOKEN = 'tok-1';

const harnesses: Array<{ unmount(): void }> = [];

interface HarnessOptions {
  ready?: boolean;
}

function mountSettings(options: HarnessOptions = {}) {
  const ready = ref(options.ready ?? true);
  const onRangeChange = vi.fn(async () => {});

  const harness = withComposable(() =>
    useParticipantDisplaySettings({
      token: ref(TOKEN),
      isReady: () => ready.value,
      onRangeChange,
    })
  );
  harnesses.push(harness);

  return { settings: harness.result, onRangeChange, ready };
}

beforeEach(() => {
  localStorage.clear();
  setActivePinia(createPinia());
  // The history entry has to exist, or every write is a silent no-op.
  useCalendarHistoryStore().addCalendar(TOKEN, 'Team offsite', 'p1');
});

afterEach(() => {
  while (harnesses.length > 0) harnesses.pop()!.unmount();
});

describe('maxPeriodsFor', () => {
  it('allows a year of months but only a month of weeks', () => {
    expect(maxPeriodsFor('month')).toBe(12);
    expect(maxPeriodsFor('week')).toBe(4);
  });
});

describe('defaultViewStyle', () => {
  const cases: [number, string][] = [
    [375, 'list'],
    [767, 'list'],
    [768, 'grid'],
    [1440, 'grid'],
  ];

  for (const [width, expected] of cases) {
    it(`is ${expected} at ${width}px`, () => {
      // A month grid is unreadable on a phone, so narrow viewports open on the list.
      vi.stubGlobal('innerWidth', width);
      expect(defaultViewStyle()).toBe(expected);
      vi.unstubAllGlobals();
    });
  }
});

describe('restore', () => {
  it('applies every field a stored entry carries', () => {
    const { settings } = mountSettings();

    settings.restore({
      displayMode: 'week',
      periodCount: 3,
      startHour: 7,
      endHour: 22,
      slotDuration: 15,
      viewStyle: 'list',
    });

    expect(settings.displayMode.value).toBe('week');
    expect(settings.numberOfPeriods.value).toBe(3);
    expect(settings.startHour.value).toBe(7);
    expect(settings.endHour.value).toBe(22);
    expect(settings.slotDuration.value).toBe(15);
    expect(settings.viewStyle.value).toBe('list');
  });

  it('leaves untouched whatever the entry does not carry', () => {
    const { settings } = mountSettings();

    settings.restore({ displayMode: 'week' });

    expect(settings.displayMode.value).toBe('week');
    expect(settings.startHour.value).toBe(8);
    expect(settings.slotDuration.value).toBe(30);
  });

  /*
   * Entries written before the grid and the compact grid were merged still say
   * 'classic' or 'compact'. Both mean the grid now; a stray value must not leave the
   * select showing nothing.
   */
  it('folds the retired view styles into the grid', () => {
    const { settings } = mountSettings();

    settings.restore({ viewStyle: 'classic' });
    expect(settings.viewStyle.value).toBe('grid');

    settings.restore({ viewStyle: 'compact' });
    expect(settings.viewStyle.value).toBe('grid');

    settings.restore({ viewStyle: 'list' });
    expect(settings.viewStyle.value).toBe('list');
  });

  it('ignores a missing entry', () => {
    const { settings } = mountSettings();
    settings.restore(undefined);
    expect(settings.displayMode.value).toBe('month');
  });
});

describe('period clamping', () => {
  /*
   * Twelve months is a valid choice; twelve weeks is not an option the selector offers.
   * Carrying the count across without clamping asked the server for three months of
   * data and rendered a select whose value matched no option.
   */
  it('caps the period count when switching to weeks', async () => {
    const { settings } = mountSettings();
    settings.numberOfPeriods.value = 12;
    await nextTick();

    settings.displayMode.value = 'week';
    await nextTick();

    expect(settings.numberOfPeriods.value).toBe(4);
  });

  it('leaves the count alone when switching back to months', async () => {
    const { settings } = mountSettings();
    settings.displayMode.value = 'week';
    settings.numberOfPeriods.value = 3;
    await nextTick();

    settings.displayMode.value = 'month';
    await nextTick();

    expect(settings.numberOfPeriods.value).toBe(3);
  });
});

describe('reloading', () => {
  it('refetches when the mode changes', async () => {
    const { settings, onRangeChange } = mountSettings();

    settings.displayMode.value = 'week';
    await nextTick();

    expect(onRangeChange).toHaveBeenCalledTimes(1);
  });

  it('refetches when the number of periods changes', async () => {
    const { settings, onRangeChange } = mountSettings();

    settings.numberOfPeriods.value = 4;
    await nextTick();

    expect(onRangeChange).toHaveBeenCalledTimes(1);
  });

  /*
   * These three only change how one week is drawn — the same days are already loaded.
   * Refetching here would turn a drag of the hour range into a burst of requests.
   */
  it('does not refetch for the week hours or the slot size', async () => {
    const { settings, onRangeChange } = mountSettings();

    settings.startHour.value = 6;
    settings.endHour.value = 23;
    settings.slotDuration.value = 15;
    await nextTick();

    expect(onRangeChange).not.toHaveBeenCalled();
  });

  it('does not refetch for the view style either', async () => {
    const { settings, onRangeChange } = mountSettings();

    settings.viewStyle.value = 'list';
    await nextTick();

    expect(onRangeChange).not.toHaveBeenCalled();
  });
});

describe('persistence', () => {
  it('writes each change back to the calendar history entry', async () => {
    const { settings } = mountSettings();
    const history = useCalendarHistoryStore();

    settings.displayMode.value = 'week';
    settings.viewStyle.value = 'list';
    settings.startHour.value = 6;
    await nextTick();

    expect(history.getDisplaySettings(TOKEN)).toMatchObject({
      displayMode: 'week',
      viewStyle: 'list',
      startHour: 6,
    });
  });

  /*
   * The history entry is created by the calendar load. Writing before then targets a
   * token the store does not know, which is dropped — and the user's choice with it.
   */
  it('holds off until the calendar has loaded', async () => {
    const { settings, ready } = mountSettings({ ready: false });
    const history = useCalendarHistoryStore();

    settings.displayMode.value = 'week';
    await nextTick();
    expect(history.getDisplaySettings(TOKEN)?.displayMode).toBeUndefined();

    ready.value = true;
    settings.numberOfPeriods.value = 2;
    await nextTick();
    expect(history.getDisplaySettings(TOKEN)?.periodCount).toBe(2);
  });
});
