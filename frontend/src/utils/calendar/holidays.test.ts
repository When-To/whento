/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { inTimezone, SAMPLE_TIMEZONES } from '@/test/timezone';
import { clearHolidayCache, getHolidayIndex, resolveCountry } from './holidays';

beforeEach(() => {
  clearHolidayCache();
});

describe('resolveCountry', () => {
  it.each([
    ['Europe/Paris', 'FR'],
    ['America/New_York', 'US'],
    ['Asia/Tokyo', 'JP'],
    ['Australia/Sydney', 'AU'],
  ])('%s -> %s', (timeZone, expected) => {
    expect(resolveCountry(timeZone)).toBe(expected);
  });

  it.each(['UTC', 'Etc/GMT+5', 'Not/AZone', ''])('returns null for %s', timeZone => {
    expect(resolveCountry(timeZone)).toBeNull();
  });
});

describe('isHoliday', () => {
  it('knows French public holidays', () => {
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    expect(fr.isHoliday('2026-01-01')).toBe(true);
    expect(fr.isHoliday('2026-05-01')).toBe(true);
    expect(fr.isHoliday('2026-07-14')).toBe(true);
    expect(fr.isHoliday('2026-12-25')).toBe(true);
    expect(fr.isHoliday('2026-04-07')).toBe(false);
  });

  it('is country-specific', () => {
    const fr = getHolidayIndex('Europe/Paris', 'en');
    const us = getHolidayIndex('America/New_York', 'en');
    // Labour Day is a French public holiday; the US observes it in September.
    expect(fr.isHoliday('2026-05-01')).toBe(true);
    expect(us.isHoliday('2026-05-01')).toBe(false);
    // Independence Day is not a French holiday.
    expect(us.isHoliday('2026-07-03')).toBe(true);
    expect(fr.isHoliday('2026-07-04')).toBe(false);
  });

  it('reports nothing for an unsupported timezone instead of throwing', () => {
    const utc = getHolidayIndex('UTC', 'en');
    expect(utc.countryCode).toBeNull();
    expect(utc.isHoliday('2026-01-01')).toBe(false);
    expect(utc.isHolidayEve('2025-12-31')).toBe(false);
    expect(utc.getName('2026-01-01')).toBeNull();
  });

  it('answers by the country calendar date, not the viewer clock', () => {
    // The previous implementation compared instants, so a viewer in Tokyo saw a
    // French 1 January holiday land on 2 January. The answer must not depend on
    // where the browser is.
    for (const timeZone of SAMPLE_TIMEZONES) {
      inTimezone(timeZone, () => {
        clearHolidayCache();
        const fr = getHolidayIndex('Europe/Paris', 'fr');
        expect(fr.isHoliday('2026-01-01')).toBe(true);
        expect(fr.isHoliday('2026-01-02')).toBe(false);
      });
    }
  });
});

describe('multi-day holidays', () => {
  // A handful of countries have genuine multi-day public holidays. `isHoliday(date)`
  // matched any instant inside [start, end), so indexing by the first day alone
  // would silently unblock the rest of the span.
  it('covers every day of the Russian New Year week', () => {
    const ru = getHolidayIndex('Europe/Moscow', 'en');
    for (const day of ['02', '03', '04', '05', '06']) {
      expect(ru.isHoliday(`2026-01-${day}`)).toBe(true);
    }
    expect(ru.isHoliday('2026-01-15')).toBe(false);
  });

  it('covers every day of the Turkish Eid holidays', () => {
    const tr = getHolidayIndex('Europe/Istanbul', 'en');
    for (const day of ['20', '21', '22', '23']) {
      expect(tr.isHoliday(`2026-03-${day}`)).toBe(true);
    }
    expect(tr.isHoliday('2026-03-24')).toBe(false);
  });

  it('names every day of the span, not just the first', () => {
    const ru = getHolidayIndex('Europe/Moscow', 'en');
    expect(ru.getName('2026-01-05')).toBe(ru.getName('2026-01-02'));
    expect(ru.getName('2026-01-05')).toBeTruthy();
  });

  it('treats the day before a multi-day holiday as an eve', () => {
    const tr = getHolidayIndex('Europe/Istanbul', 'en');
    expect(tr.isHolidayEve('2026-03-19')).toBe(true);
    // Inside the span the next day is also a holiday, so it is an eve too.
    expect(tr.isHolidayEve('2026-03-22')).toBe(true);
    expect(tr.isHolidayEve('2026-03-23')).toBe(false);
  });
});

describe('isHolidayEve', () => {
  it('detects the day before a holiday', () => {
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    expect(fr.isHolidayEve('2026-12-24')).toBe(true);
    expect(fr.isHolidayEve('2026-12-23')).toBe(false);
  });

  it('crosses the year boundary, loading the next year on demand', () => {
    // 31 December 2025 is the eve of 1 January 2026: the lookup has to reach into a
    // year map that has not been loaded yet.
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    expect(fr.isHolidayEve('2025-12-31')).toBe(true);
  });
});

describe('getName', () => {
  it('localizes names to the requested language', () => {
    // The previous getHolidayName defaulted to French and every caller omitted the
    // argument, so English users saw French holiday names.
    expect(getHolidayIndex('Europe/Paris', 'fr').getName('2026-01-01')).toBe('Nouvel An');
    expect(getHolidayIndex('Europe/Paris', 'en').getName('2026-01-01')).toBe("New Year's Day");
  });

  it('returns null for ordinary days', () => {
    expect(getHolidayIndex('Europe/Paris', 'fr').getName('2026-04-07')).toBeNull();
  });
});

describe('caching', () => {
  it('returns the same index instance for the same timezone and language', () => {
    expect(getHolidayIndex('Europe/Paris', 'fr')).toBe(getHolidayIndex('Europe/Paris', 'fr'));
    expect(getHolidayIndex('Europe/Paris', 'fr')).not.toBe(getHolidayIndex('Europe/Paris', 'en'));
  });

  it('loads each year exactly once, however many lookups are made', async () => {
    // This is the whole point of the module: the week grid used to issue ~2700
    // per-date lookups per render at ~1.2 ms each.
    const holidays = await import('date-holidays');
    const spy = vi.spyOn(holidays.default.prototype, 'getHolidays');
    clearHolidayCache();

    const fr = getHolidayIndex('Europe/Paris', 'fr');
    for (let day = 1; day <= 28; day++) {
      for (let month = 1; month <= 12; month++) {
        fr.isHoliday(`2026-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`);
      }
    }
    expect(spy).toHaveBeenCalledTimes(1);

    // A date in another year adds exactly one more load.
    fr.isHoliday('2027-01-01');
    expect(spy).toHaveBeenCalledTimes(2);

    spy.mockRestore();
  });

  it('constructs one Holidays instance per country and language', async () => {
    // `init` is what the constructor calls, so counting it counts instantiations.
    // The previous getHolidayName built a fresh instance on *every* call, once per
    // day cell, explicitly bypassing the cache to get a localized name.
    const holidays = await import('date-holidays');
    const spy = vi.spyOn(holidays.default.prototype, 'init');
    clearHolidayCache();

    for (let i = 0; i < 20; i++) {
      const fr = getHolidayIndex('Europe/Paris', 'fr');
      fr.isHoliday('2026-01-01');
      fr.getName('2026-05-01');
    }
    expect(spy).toHaveBeenCalledTimes(1);

    // A different language is a different instance, and only one more.
    getHolidayIndex('Europe/Paris', 'en').getName('2026-01-01');
    expect(spy).toHaveBeenCalledTimes(2);

    spy.mockRestore();
  });
});
