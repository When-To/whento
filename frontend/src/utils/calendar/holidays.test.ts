/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { inTimezone, SAMPLE_TIMEZONES } from '@/test/timezone';
import {
  clearHolidayCache,
  getHolidayIndex,
  holidaysReady,
  holidaysVersion,
  resolveCountry,
} from './holidays';
import { holidaysApi } from '@/api/holidays';

/*
 * The holiday dataset lives on the backend now (pkg/datevalidation's offline
 * table, served from GET /api/v1/holidays), so this file exercises the module
 * against a mocked copy of that API. The fixtures mirror the 2026 dates the
 * backend tests use: Bastille Day (14 July) is French and not American,
 * Christmas is shared, and 24 December is the eve of it in both.
 *
 * Fetching is on demand and asynchronous: the module starts a fetch from
 * `getHolidayIndex` and answers "no holidays" until it lands, so each test
 * settles the microtask queue before asserting.
 */

const fr2026 = [
  { date: '2026-01-01', name: 'Nouvel an' },
  { date: '2026-04-06', name: 'Lundi de Pâques' },
  { date: '2026-05-01', name: 'Fête du Travail' },
  { date: '2026-05-08', name: 'Fête de la Victoire' },
  { date: '2026-05-14', name: 'Ascension' },
  { date: '2026-05-25', name: 'Lundi de Pentecôte' },
  { date: '2026-07-14', name: 'Fête Nationale' },
  { date: '2026-08-15', name: 'Assomption' },
  { date: '2026-11-01', name: 'Toussaint' },
  { date: '2026-11-11', name: 'Armistice de 1918' },
  { date: '2026-12-25', name: 'Noël' },
];

const us2026 = [
  { date: '2026-01-01', name: "New Year's Day" },
  { date: '2026-01-19', name: 'Martin Luther King Jr. Day' },
  { date: '2026-02-16', name: "Presidents' Day" },
  { date: '2026-05-25', name: 'Memorial Day' },
  { date: '2026-06-19', name: 'Juneteenth' },
  { date: '2026-07-04', name: 'Independence Day' },
  { date: '2026-09-07', name: 'Labor Day' },
  { date: '2026-10-12', name: 'Columbus Day' },
  { date: '2026-11-11', name: 'Veterans Day' },
  { date: '2026-11-26', name: 'Thanksgiving Day' },
  { date: '2026-12-25', name: 'Christmas Day' },
];

// A country with a genuinely multi-day holiday: the backend serves every day of
// the span, and the frontend must index each one, not just the first.
const ru2026 = [
  { date: '2026-01-01', name: 'New Year Holiday' },
  { date: '2026-01-02', name: 'New Year Holiday' },
  { date: '2026-01-03', name: 'New Year Holiday' },
  { date: '2026-01-04', name: 'New Year Holiday' },
  { date: '2026-01-05', name: 'New Year Holiday' },
  { date: '2026-01-06', name: 'New Year Holiday' },
];

const byTimezone: Record<
  string,
  { country: string; supported: boolean; holidays: { date: string; name: string }[] }
> = {
  'Europe/Paris': { country: 'FR', supported: true, holidays: fr2026 },
  'America/New_York': { country: 'US', supported: true, holidays: us2026 },
  'Europe/Moscow': { country: 'RU', supported: true, holidays: ru2026 },
};

vi.mock('@/api/holidays', () => ({
  holidaysApi: {
    supported: vi.fn().mockResolvedValue({ countries: ['FR', 'US', 'RU'] }),
    year: vi.fn((timezone: string, year: number) => {
      const entry = byTimezone[timezone];
      if (!entry || year !== 2026) {
        return Promise.resolve({
          country_code: entry?.country ?? null,
          supported: entry?.supported ?? false,
          holidays: [],
        });
      }
      return Promise.resolve({
        country_code: entry.country,
        supported: true,
        holidays: entry.holidays,
      });
    }),
  },
}));

/** Let the on-demand fetch that a lookup kicked off resolve. */
async function settle() {
  await Promise.resolve();
  await new Promise(resolve => setTimeout(resolve, 0));
}

beforeEach(() => {
  clearHolidayCache();
  vi.clearAllMocks();
});

describe('resolveCountry', () => {
  it('serves the timezone once the backend has answered', async () => {
    // Nothing has been fetched yet: the answer is "unknown", not a wrong guess.
    expect(resolveCountry('Europe/Paris')).toBeNull();

    getHolidayIndex('Europe/Paris', 'fr').isHoliday('2026-01-01'); // kicks the fetch
    await settle();

    expect(resolveCountry('Europe/Paris')).toBe('FR');
  });
});

describe('isHoliday', () => {
  it('knows French public holidays', async () => {
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    await settle();
    expect(fr.isHoliday('2026-01-01')).toBe(true);
    expect(fr.isHoliday('2026-05-01')).toBe(true);
    expect(fr.isHoliday('2026-07-14')).toBe(true);
    expect(fr.isHoliday('2026-12-25')).toBe(true);
    expect(fr.isHoliday('2026-04-07')).toBe(false);
  });

  it('is country-specific', async () => {
    const fr = getHolidayIndex('Europe/Paris', 'en');
    const us = getHolidayIndex('America/New_York', 'en');
    await settle();
    // Labour Day is a French public holiday; the US observes it in September.
    expect(fr.isHoliday('2026-05-01')).toBe(true);
    expect(us.isHoliday('2026-05-01')).toBe(false);
    // Independence Day is on the 4th of July, not a French holiday.
    expect(us.isHoliday('2026-07-04')).toBe(true);
    expect(fr.isHoliday('2026-07-04')).toBe(false);
  });

  it('answers "no holidays" while the data is still loading', async () => {
    const index = getHolidayIndex('Europe/Paris', 'fr');
    expect(holidaysReady.value).toBe(false);
    expect(index.isHoliday('2026-01-01')).toBe(false);
    expect(index.getName('2026-01-01')).toBeNull();

    // Once the fetch lands, the version ticks and lookups come from the cache.
    await settle();
    expect(holidaysVersion.value).toBeGreaterThan(0);
    expect(index.isHoliday('2026-01-01')).toBe(true);
  });

  it('reports nothing for an unsupported timezone instead of throwing', async () => {
    const utc = getHolidayIndex('UTC', 'en');
    await settle();
    expect(utc.isHoliday('2026-01-01')).toBe(false);
    expect(utc.isHolidayEve('2025-12-31')).toBe(false);
    expect(utc.getName('2026-01-01')).toBeNull();
  });

  it('answers by the country calendar date, not the viewer clock', async () => {
    // The country comes from the timezone the caller passes; the browser's own
    // clock and timezone must not shift the answer.
    for (const viewerTimeZone of SAMPLE_TIMEZONES) {
      inTimezone(viewerTimeZone, async () => {
        clearHolidayCache();
        const fr = getHolidayIndex('Europe/Paris', 'fr');
        await settle();
        expect(fr.isHoliday('2026-01-01')).toBe(true);
        expect(fr.isHoliday('2026-01-02')).toBe(false);
      });
    }
    await settle();
  });
});

describe('multi-day holidays', () => {
  it('covers every day the backend serves for the Russian New Year week', async () => {
    const ru = getHolidayIndex('Europe/Moscow', 'en');
    await settle();
    for (const day of ['02', '03', '04', '05', '06']) {
      expect(ru.isHoliday(`2026-01-${day}`)).toBe(true);
    }
    expect(ru.isHoliday('2026-01-15')).toBe(false);
  });

  it('names every day of the span, not just the first', async () => {
    const ru = getHolidayIndex('Europe/Moscow', 'en');
    await settle();
    const name = ru.getName('2026-01-02');
    expect(name).toBe('New Year Holiday');
    expect(ru.getName('2026-01-05')).toBe(name);
    expect(ru.getName('2026-01-03')).toBe(name);
    // A day outside the span carries no name at all.
    expect(ru.getName('2026-01-15')).toBeNull();
  });
});

describe('isHolidayEve', () => {
  it('detects the day before a holiday', async () => {
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    await settle();
    expect(fr.isHolidayEve('2026-12-24')).toBe(true);
    expect(fr.isHolidayEve('2026-12-23')).toBe(false);
  });

  it('crosses the year boundary, loading the next year on demand', async () => {
    // 31 December 2025 is the eve of 1 January 2026: the lookup has to reach into
    // a year map that has not been loaded yet.
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    await settle();
    expect(fr.isHolidayEve('2025-12-31')).toBe(true);
  });
});

describe('getName', () => {
  it('serves the name the backend dataset attaches', async () => {
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    await settle();
    expect(fr.getName('2026-01-01')).toBe('Nouvel an');
    expect(getHolidayIndex('Europe/Paris', 'en').getName('2026-01-01')).toBe('Nouvel an');
  });

  it('returns null for ordinary days', async () => {
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    await settle();
    expect(fr.getName('2026-04-07')).toBeNull();
  });
});

describe('caching', () => {
  it('returns the same index instance for the same timezone and language', () => {
    expect(getHolidayIndex('Europe/Paris', 'fr')).toBe(getHolidayIndex('Europe/Paris', 'fr'));
    expect(getHolidayIndex('Europe/Paris', 'fr')).not.toBe(getHolidayIndex('Europe/Paris', 'en'));
  });

  it('loads each year exactly once, however many lookups are made', async () => {
    const fr = getHolidayIndex('Europe/Paris', 'fr');
    for (let day = 1; day <= 28; day++) {
      for (let month = 1; month <= 12; month++) {
        fr.isHoliday(`2026-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`);
      }
    }
    await settle();
    expect(holidaysApi.year).toHaveBeenCalledTimes(1);

    // A date in another year adds exactly one more load.
    fr.isHoliday('2027-01-01');
    await settle();
    expect(holidaysApi.year).toHaveBeenCalledTimes(2);
  });
});
