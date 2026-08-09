/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { beforeEach, describe, expect, it } from 'vitest';
import { inTimezone } from '@/test/timezone';
import { clearFormatterCache, formatDate, formatISODate, getFormatter } from './intlFormatters';

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
    expect(() => formatDate(date, 'fr', 'fullDate')).not.toThrow();
    expect(() => formatDate(date, 'en', 'fullDate')).not.toThrow();
    expect(formatDate(date, 'fr', 'fullDate')).toContain('2026');
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
