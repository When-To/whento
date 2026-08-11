/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * The last-resort fallback goes through src/i18n.ts, whose module body reads
 * window.location to pick the initial locale, so this file needs a DOM even though the
 * unit under test is pure.
 *
 * @vitest-environment jsdom
 */

import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  DEFAULT_WEEK_START,
  WEEK_START_BY_REGION,
  getWeekStartForRegion,
  getWeekStartForTimezone,
  getWeekdayOffset,
  resolveWeekStart,
} from './weekStart';

/** Pins navigator.language for the duration of a test. */
function withBrowserLanguage(tag: string) {
  vi.spyOn(navigator, 'language', 'get').mockReturnValue(tag);
}

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe('getWeekStartForRegion', () => {
  it('returns the CLDR value for regions that differ from the default', () => {
    expect(getWeekStartForRegion('US')).toBe(0);
    expect(getWeekStartForRegion('EG')).toBe(6);
    expect(getWeekStartForRegion('MV')).toBe(5);
  });

  it('falls back to Monday for regions CLDR leaves at the world default', () => {
    expect(getWeekStartForRegion('DE')).toBe(1);
    expect(getWeekStartForRegion('FR')).toBe(1);
    // Not in CLDR's table at all, so it inherits "001"
    expect(getWeekStartForRegion('ZZ')).toBe(DEFAULT_WEEK_START);
  });

  it('accepts lowercase region codes', () => {
    expect(getWeekStartForRegion('us')).toBe(0);
  });

  it('returns undefined for absent or malformed input so callers fall through', () => {
    expect(getWeekStartForRegion(undefined)).toBeUndefined();
    expect(getWeekStartForRegion(null)).toBeUndefined();
    expect(getWeekStartForRegion('')).toBeUndefined();
    expect(getWeekStartForRegion('DEU')).toBeUndefined();
    expect(getWeekStartForRegion('001')).toBeUndefined();
  });
});

describe('getWeekStartForTimezone', () => {
  it('maps an IANA timezone to its country then to the CLDR value', () => {
    expect(getWeekStartForTimezone('Europe/Berlin')).toBe(1);
    expect(getWeekStartForTimezone('America/New_York')).toBe(0);
    expect(getWeekStartForTimezone('Asia/Riyadh')).toBe(0); // CLDR puts SA on Sunday
    expect(getWeekStartForTimezone('Africa/Cairo')).toBe(6);
    expect(getWeekStartForTimezone('Indian/Maldives')).toBe(5);
  });

  it('returns undefined for timezones that map to no country', () => {
    expect(getWeekStartForTimezone('UTC')).toBeUndefined();
    expect(getWeekStartForTimezone('Not/AZone')).toBeUndefined();
    expect(getWeekStartForTimezone(undefined)).toBeUndefined();
    expect(getWeekStartForTimezone('')).toBeUndefined();
  });
});

describe('resolveWeekStart', () => {
  it('prefers an explicit override over everything else', () => {
    localStorage.setItem('week-start-day', '6');
    expect(resolveWeekStart('Europe/Berlin', 'en')).toBe(6);
  });

  it('ignores an override that is not a valid first day', () => {
    localStorage.setItem('week-start-day', '3');
    expect(resolveWeekStart('Europe/Berlin', 'en')).toBe(1);
  });

  it('uses the calendar timezone ahead of the browser and the locale', () => {
    withBrowserLanguage('en-US');
    // Timezone says Monday, browser and locale would both say Sunday
    expect(resolveWeekStart('Europe/Berlin', 'en')).toBe(1);
  });

  it('falls back to the browser region when no calendar is in context', () => {
    withBrowserLanguage('de-DE');
    expect(resolveWeekStart(undefined, 'en')).toBe(1);

    withBrowserLanguage('en-US');
    expect(resolveWeekStart(undefined, 'fr')).toBe(0);
  });

  it('expands a language-only browser tag to a region', () => {
    withBrowserLanguage('de');
    expect(resolveWeekStart(undefined, 'en')).toBe(1);
  });

  it('falls back to the app locale when the browser exposes no usable region', () => {
    withBrowserLanguage('');
    expect(resolveWeekStart(undefined, 'en')).toBe(0);
    expect(resolveWeekStart(undefined, 'fr')).toBe(1);
  });

  it('resolves the reported bug: a German calendar starts on Monday', () => {
    // The reporter's browser is German, but the app only ships en/fr so the
    // locale alone would resolve to Sunday.
    withBrowserLanguage('de-DE');
    expect(resolveWeekStart('Europe/Berlin', 'en')).toBe(1);
  });
});

describe('getWeekdayOffset', () => {
  // 2026-08-01 is a Saturday (getDay() === 6)
  const saturday = new Date(2026, 7, 1);

  it('counts days elapsed since the start of the week', () => {
    expect(getWeekdayOffset(saturday, 0)).toBe(6); // Sunday-first
    expect(getWeekdayOffset(saturday, 1)).toBe(5); // Monday-first
    expect(getWeekdayOffset(saturday, 6)).toBe(0); // Saturday-first
  });

  it('is zero exactly on the first day of the week', () => {
    for (let day = 0; day < 7; day++) {
      // 2026-02-01 is a Sunday, so adding `day` lands on weekday `day`
      const date = new Date(2026, 1, 1 + day);
      expect(getWeekdayOffset(date, day)).toBe(0);
    }
  });

  it('pads a month grid with the right number of leading cells', () => {
    // 2026-03-01 is a Sunday
    const march = new Date(2026, 2, 1);
    expect(getWeekdayOffset(march, 0)).toBe(0); // Sunday-first: no padding
    expect(getWeekdayOffset(march, 1)).toBe(6); // Monday-first: a full week of padding
  });
});

describe('WEEK_START_BY_REGION', () => {
  it('only lists regions that differ from the Monday default', () => {
    for (const [region, day] of Object.entries(WEEK_START_BY_REGION)) {
      expect(day, `${region} should not be listed`).not.toBe(DEFAULT_WEEK_START);
      expect(region).toMatch(/^[A-Z]{2}$/);
    }
  });

  it('matches the CLDR data the runtime was built with', () => {
    const weekInfoOf = (region: string) => {
      const locale = new Intl.Locale(`und-${region}`) as Intl.Locale & {
        getWeekInfo?: () => { firstDay: number };
        weekInfo?: { firstDay: number };
      };
      return locale.getWeekInfo?.() ?? locale.weekInfo;
    };

    // Intl exposes CLDR week data only on newer runtimes; skip rather than
    // make the table depend on it.
    if (!weekInfoOf('DE')) return;

    for (const region of Object.keys(WEEK_START_BY_REGION)) {
      // Intl encodes 1 = Monday ... 7 = Sunday; the app uses getDay()'s 0 = Sunday
      const expected = weekInfoOf(region)!.firstDay % 7;
      expect(WEEK_START_BY_REGION[region], `CLDR firstDay for ${region}`).toBe(expected);
    }
  });

  it('agrees with the runtime that unlisted regions start on Monday', () => {
    const locale = new Intl.Locale('und-DE') as Intl.Locale & { weekInfo?: { firstDay: number } };
    if (!locale.weekInfo) return;
    for (const region of ['DE', 'FR', 'GB', 'RU', 'CN', 'AU']) {
      expect(WEEK_START_BY_REGION[region]).toBeUndefined();
      const info = new Intl.Locale(`und-${region}`) as Intl.Locale & {
        weekInfo?: { firstDay: number };
      };
      expect(info.weekInfo!.firstDay % 7).toBe(DEFAULT_WEEK_START);
    }
  });
});
