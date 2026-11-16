/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { beforeEach, describe, expect, it } from 'vitest';
import { inTimezone } from '@/test/timezone';
import {
  clearFormatterCache,
  formatDate,
  formatISODate,
  formatRelativeTime,
  getFormatter,
} from './intlFormatters';

beforeEach(() => {
  clearFormatterCache();
});

describe('getFormatter', () => {
  it('returns the same instance for the same locale and style', () => {
    // The whole point of this module: the previous code allocated one
    // Intl.DateTimeFormat per calendar cell, on every render and every drag frame.
    expect(getFormatter('fr', 'weekdayShort')).toBe(getFormatter('fr', 'weekdayShort'));
  });

  it('returns distinct instances per locale and per style', () => {
    expect(getFormatter('fr', 'weekdayShort')).not.toBe(getFormatter('en', 'weekdayShort'));
    expect(getFormatter('fr', 'weekdayShort')).not.toBe(getFormatter('fr', 'weekdayLong'));
  });
});

describe('formatDate', () => {
  // 2026-04-05 is a Sunday.
  const date = new Date(2026, 3, 5);

  it('formats weekdays per locale', () => {
    expect(formatDate(date, 'en', 'weekdayLong')).toBe('Sunday');
    expect(formatDate(date, 'fr', 'weekdayLong')).toBe('dimanche');
  });

  it('accepts the bare vue-i18n locale codes', () => {
    // `locale.value` is 'fr' | 'en', never 'fr-FR'. The old code compared against
    // 'fr-FR' in one place, a branch that could never be taken.
    //
    // Asserting the exact strings rather than not.toThrow(): Intl never throws on a
    // well-formed tag, so the old form passed even if both locales silently produced
    // the same output — which is precisely the bug it was meant to catch. `2026` is
    // no better, since it appears in both.
    expect(formatDate(date, 'fr', 'fullDate')).toBe('5 avril 2026');
    expect(formatDate(date, 'en', 'fullDate')).toBe('April 5, 2026');
    expect(formatDate(date, 'fr', 'fullDate')).not.toBe(formatDate(date, 'en', 'fullDate'));
  });

  it('formats month and year', () => {
    expect(formatDate(date, 'en', 'monthYear')).toBe('April 2026');
  });
});

describe('formatISODate', () => {
  it('parses in local time, so the weekday never shifts', () => {
    inTimezone('America/New_York', () => {
      clearFormatterCache();
      expect(formatISODate('2026-04-05', 'en', 'weekdayLong')).toBe('Sunday');
    });
  });
});

describe('formatRelativeTime', () => {
  // A fixed "now", so these do not drift with the clock the suite runs on.
  const now = new Date('2026-04-05T12:00:00Z');

  const cases: ReadonlyArray<[string, string, string, string]> = [
    ['minutes', '2026-04-05T11:30:00Z', 'en', '30 minutes ago'],
    ['hours', '2026-04-05T09:00:00Z', 'en', '3 hours ago'],
    ['days', '2026-04-02T12:00:00Z', 'en', '3 days ago'],
    ['days, in French', '2026-04-02T12:00:00Z', 'fr', 'il y a 3 jours'],
    // numeric: 'auto' is what turns -1 day into a word rather than "1 day ago".
    ['yesterday reads as a word', '2026-04-04T12:00:00Z', 'en', 'yesterday'],
    ['hier', '2026-04-04T12:00:00Z', 'fr', 'hier'],
  ];

  it.each(cases)('formats %s', (_name, iso, locale, expected) => {
    expect(formatRelativeTime(new Date(iso), locale, now)).toBe(expected);
  });

  it('picks the largest unit that fits, not the first that does', () => {
    // 90 minutes is an hour and a half: "an hour ago", never "90 minutes ago".
    expect(formatRelativeTime(new Date('2026-04-05T10:30:00Z'), 'en', now)).toBe('1 hour ago');
  });

  it('handles an instant within the last minute', () => {
    expect(formatRelativeTime(new Date('2026-04-05T11:59:40Z'), 'en', now)).toBe('20 seconds ago');
  });
});
