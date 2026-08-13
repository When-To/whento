/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { withComposable } from '@/test/harness';
import { isDateInFuture, useAvailabilityLabels } from './useAvailabilityLabels';

const harnesses: Array<{ unmount(): void }> = [];

function mountLabels(locale: 'en' | 'fr' = 'en') {
  const harness = withComposable(() => useAvailabilityLabels(), { locale });
  harnesses.push(harness);
  return harness.result;
}

afterEach(() => {
  while (harnesses.length > 0) harnesses.pop()!.unmount();
  vi.useRealTimers();
});

describe('formatTimeRange', () => {
  /*
   * "All day" is not a separate flag on the wire: an availability with no times, or one
   * spanning 00:00-23:59, means the whole day. Printing "00:00 - 23:59" instead was the
   * bug this collapses.
   */
  const allDayCases: [string, string | undefined, string | undefined][] = [
    ['no times at all', undefined, undefined],
    ['an explicit full span', '00:00', '23:59'],
    ['only a midnight start', '00:00', undefined],
    ['only an end-of-day end', undefined, '23:59'],
  ];

  for (const [name, start, end] of allDayCases) {
    it(`reads ${name} as all day`, () => {
      expect(mountLabels().formatTimeRange(start, end)).toBe('All day');
    });
  }

  it('prints a closed range as-is', () => {
    expect(mountLabels().formatTimeRange('09:00', '17:30')).toBe('09:00 - 17:30');
  });

  it('names the bound when only one side is set', () => {
    const labels = mountLabels();
    expect(labels.formatTimeRange('09:00', undefined)).toBe('Start: 09:00');
    expect(labels.formatTimeRange(undefined, '17:30')).toBe('End: 17:30');
  });

  it('translates', () => {
    expect(mountLabels('fr').formatTimeRange(undefined, undefined)).toBe('Jour complet');
  });
});

describe('getDayName', () => {
  it('maps the ISO weekday numbers, Sunday first', () => {
    const labels = mountLabels();
    expect([0, 1, 2, 3, 4, 5, 6].map(labels.getDayName)).toEqual([
      'Sunday',
      'Monday',
      'Tuesday',
      'Wednesday',
      'Thursday',
      'Friday',
      'Saturday',
    ]);
  });

  it('translates', () => {
    expect(mountLabels('fr').getDayName(3)).toBe('Mercredi');
  });
});

describe('formatDate', () => {
  it('follows the active locale', () => {
    const english = mountLabels().formatDate('2026-03-02');
    const french = mountLabels('fr').formatDate('2026-03-02');

    expect(english).toContain('2026');
    expect(french).toContain('2026');
    expect(english).not.toBe(french);
  });
});

describe('isDateInFuture', () => {
  /*
   * The cut-off is midnight, not "now": an answer for today is still editable at 23:00,
   * because the day it refers to has not passed.
   */
  it('counts today as editable', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 2, 2, 23, 0, 0));

    expect(isDateInFuture('2026-03-02')).toBe(true);
    expect(isDateInFuture('2026-03-03')).toBe(true);
    expect(isDateInFuture('2026-03-01')).toBe(false);
  });

  it('is reachable through the composable too', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 2, 2, 9, 0, 0));

    expect(mountLabels().isDateInFuture('2026-03-01')).toBe(false);
  });
});
