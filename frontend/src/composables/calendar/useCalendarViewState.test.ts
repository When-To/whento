/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import { normalizeViewStyle, useCalendarViewState } from './useCalendarViewState';

describe('normalizeViewStyle', () => {
  it('keeps list', () => {
    expect(normalizeViewStyle('list')).toBe('list');
  });

  it('maps the retired names onto grid', () => {
    // Entries written before the month grid started transposing itself in CSS hold
    // one of these; both mean the same thing now.
    expect(normalizeViewStyle('classic')).toBe('grid');
    expect(normalizeViewStyle('compact')).toBe('grid');
  });

  it('keeps grid', () => {
    expect(normalizeViewStyle('grid')).toBe('grid');
  });

  it('falls back to grid for anything unrecognised', () => {
    expect(normalizeViewStyle(undefined)).toBe('grid');
    expect(normalizeViewStyle('')).toBe('grid');
    expect(normalizeViewStyle('LIST')).toBe('grid');
    expect(normalizeViewStyle('something-else')).toBe('grid');
  });
});

describe('useCalendarViewState defaults', () => {
  it('starts on a single month with the standard working hours', () => {
    const state = useCalendarViewState();

    expect(state.displayMode.value).toBe('month');
    expect(state.viewStyle.value).toBe('grid');
    expect(state.periodCount.value).toBe(1);
    expect(state.startHour.value).toBe(8);
    expect(state.endHour.value).toBe(20);
    expect(state.slotDuration.value).toBe(15);
  });

  it('accepts partial overrides and defaults the rest', () => {
    const state = useCalendarViewState({ displayMode: 'week', startHour: 6 });

    expect(state.displayMode.value).toBe('week');
    expect(state.startHour.value).toBe(6);
    expect(state.endHour.value).toBe(20);
    expect(state.periodCount.value).toBe(1);
  });
});

describe('maxPeriods', () => {
  it('is twelve months and four weeks', () => {
    const state = useCalendarViewState();

    expect(state.maxPeriods.value).toBe(12);

    state.displayMode.value = 'week';
    expect(state.maxPeriods.value).toBe(4);

    state.displayMode.value = 'month';
    expect(state.maxPeriods.value).toBe(12);
  });
});

describe('hydrate', () => {
  it('applies every recognised setting', () => {
    const state = useCalendarViewState();

    state.hydrate({
      displayMode: 'week',
      viewStyle: 'list',
      periodCount: 3,
      startHour: 7,
      endHour: 19,
      slotDuration: 30,
    });

    expect(state.toSettings()).toEqual({
      displayMode: 'week',
      viewStyle: 'list',
      periodCount: 3,
      startHour: 7,
      endHour: 19,
      slotDuration: 30,
    });
  });

  it('leaves absent settings alone', () => {
    const state = useCalendarViewState({ startHour: 6, endHour: 22 });

    state.hydrate({ displayMode: 'week' });

    expect(state.startHour.value).toBe(6);
    expect(state.endHour.value).toBe(22);
  });

  it('ignores values of the wrong type rather than adopting them', () => {
    const state = useCalendarViewState();

    state.hydrate({
      displayMode: 'fortnight',
      periodCount: '3',
      startHour: null,
      endHour: undefined,
      slotDuration: {},
    });

    expect(state.displayMode.value).toBe('month');
    expect(state.periodCount.value).toBe(1);
    expect(state.startHour.value).toBe(8);
    expect(state.endHour.value).toBe(20);
    expect(state.slotDuration.value).toBe(15);
  });

  it('ignores an empty settings object', () => {
    const state = useCalendarViewState();

    state.hydrate({});

    expect(state.toSettings()).toEqual({
      displayMode: 'month',
      viewStyle: 'grid',
      periodCount: 1,
      startHour: 8,
      endHour: 20,
      slotDuration: 15,
    });
  });

  it('migrates a legacy view style on the way in', () => {
    const state = useCalendarViewState();

    state.hydrate({ viewStyle: 'compact' });

    expect(state.viewStyle.value).toBe('grid');
  });

  describe('the period clamp', () => {
    it('caps a month count at twelve', () => {
      const state = useCalendarViewState();

      state.hydrate({ periodCount: 99 });

      expect(state.periodCount.value).toBe(12);
    });

    it('caps a week count at four', () => {
      const state = useCalendarViewState();

      state.hydrate({ displayMode: 'week', periodCount: 12 });

      expect(state.periodCount.value).toBe(4);
    });

    it('raises a count below one', () => {
      const state = useCalendarViewState();

      state.hydrate({ periodCount: 0 });
      expect(state.periodCount.value).toBe(1);

      state.hydrate({ periodCount: -5 });
      expect(state.periodCount.value).toBe(1);
    });

    /**
     * The clamp reads maxPeriods, which depends on displayMode, and hydrate applies
     * displayMode first. So `{week, 12}` is correctly capped at 4 — but only because of
     * the statement order inside hydrate, which nothing else enforces.
     *
     * These two tests fix that ordering. Swapping the assignments in hydrate makes the
     * second one fail, since periodCount would then be clamped against the *previous*
     * mode's ceiling.
     */
    it('clamps against the mode being hydrated, not the one being replaced', () => {
      const state = useCalendarViewState();

      // Starts on month, whose ceiling is 12; hydrating to week must use 4.
      state.hydrate({ displayMode: 'week', periodCount: 12 });

      expect(state.displayMode.value).toBe('week');
      expect(state.periodCount.value).toBe(4);
    });

    it('clamps against the new mode when widening as well', () => {
      const state = useCalendarViewState({ displayMode: 'week' });

      // Starts on week, ceiling 4; hydrating to month must allow 12.
      state.hydrate({ displayMode: 'month', periodCount: 12 });

      expect(state.periodCount.value).toBe(12);
    });

    it('does not re-clamp when the mode is switched afterwards', () => {
      // Switching mode through the ref, rather than through hydrate, leaves an
      // out-of-range count in place. Pinned as documentation: the clamp only runs on
      // hydrate, so callers that flip displayMode directly must fix periodCount too.
      const state = useCalendarViewState();

      state.hydrate({ periodCount: 12 });
      state.displayMode.value = 'week';

      expect(state.maxPeriods.value).toBe(4);
      expect(state.periodCount.value).toBe(12);
    });
  });
});

describe('toSettings', () => {
  it('round-trips through hydrate', () => {
    const original = useCalendarViewState({
      displayMode: 'week',
      viewStyle: 'list',
      periodCount: 2,
      startHour: 9,
      endHour: 18,
      slotDuration: 30,
    });

    const restored = useCalendarViewState();
    restored.hydrate({ ...original.toSettings() });

    expect(restored.toSettings()).toEqual(original.toSettings());
  });

  it('reflects later mutations', () => {
    const state = useCalendarViewState();

    state.displayMode.value = 'week';
    state.slotDuration.value = 60;

    expect(state.toSettings()).toMatchObject({ displayMode: 'week', slotDuration: 60 });
  });
});
