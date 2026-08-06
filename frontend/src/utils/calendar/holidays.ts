/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Public-holiday lookup, indexed by calendar date.
 *
 * The previous implementation answered every question with `Holidays#isHoliday(date)`,
 * which costs ~1.2 ms. The week grid asked it around 2700 times per render, so a single
 * grid blocked the main thread for roughly 1.8 seconds — times four mounted grids. It
 * also scanned all 206 supported countries (93 ms) to invert timezone -> country, and
 * built a throwaway `Holidays` instance for every holiday *name* it needed.
 *
 * This module instead pulls a whole year at once (~18 ms, names included) into a
 * `Map<ISODate, string>` and answers from the hash. The cache is keyed by country,
 * language and year, so it is always valid and never needs clearing.
 */

import Holidays, { type HolidaysTypes } from 'date-holidays';
import { getCountryForTimezone } from 'countries-and-timezones';
import { addDaysISO, yearOf, type ISODate } from '@/utils/date/isoDate';

/** Public holidays for one country, in one language, answered by calendar date. */
export interface HolidayIndex {
  /** ISO country code resolved from the timezone, or null when unsupported. */
  readonly countryCode: string | null;
  /** Whether the date is an official public holiday. */
  isHoliday(date: ISODate): boolean;
  /** Whether the *next* day is an official public holiday. */
  isHolidayEve(date: ISODate): boolean;
  /** Localized holiday name, or null when the date is not a public holiday. */
  getName(date: ISODate): string | null;
}

interface CountryCache {
  readonly instance: Holidays;
  readonly years: Map<number, Map<ISODate, string>>;
}

const countryCaches = new Map<string, CountryCache>();
const indexes = new Map<string, HolidayIndex>();

/**
 * Resolve an IANA timezone to an ISO country code.
 *
 * O(1) against the `countries-and-timezones` dataset, replacing a 93 ms scan that
 * instantiated `Holidays` once per supported country.
 */
export function resolveCountry(timeZone: string): string | null {
  if (!timeZone) return null;
  try {
    return getCountryForTimezone(timeZone)?.id ?? null;
  } catch {
    return null;
  }
}

function getCountryCache(countryCode: string, language: string): CountryCache {
  const key = `${countryCode}|${language}`;
  let cache = countryCaches.get(key);
  if (!cache) {
    cache = {
      instance: new Holidays(countryCode, { languages: [language] }),
      years: new Map(),
    };
    countryCaches.set(key, cache);
  }
  return cache;
}

const MS_PER_DAY = 24 * 60 * 60 * 1000;

/**
 * How many calendar days a holiday covers. Almost always one, but several countries
 * have genuine multi-day public holidays — Russia's New Year week, and the Eid
 * holidays in Turkey and the UAE, all span three to five days.
 */
function spanInDays(holiday: HolidaysTypes.Holiday): number {
  const start = holiday.start?.getTime();
  const end = holiday.end?.getTime();
  if (!start || !end || end <= start) return 1;
  return Math.max(1, Math.round((end - start) / MS_PER_DAY));
}

/**
 * Build (once) the date -> name map for a single year.
 *
 * Keys start from `holiday.date`, which is the date **in the country's own timezone**.
 * Deriving them from `holiday.start`/`end` instead would re-introduce the bug this
 * refactor removes: those are instants, so a viewer in Tokyo looking at a French
 * calendar would see 1 January's holiday land on 2 January. A public holiday is a
 * property of the country's calendar date, not of the viewer's clock.
 *
 * The instants are still used, but only for their *duration*: a multi-day holiday is
 * expanded over every day it covers, which is what `isHoliday(date)` did by testing
 * the instant against `[start, end)`.
 */
function loadYear(cache: CountryCache, year: number): Map<ISODate, string> {
  let index = cache.years.get(year);
  if (index) return index;

  index = new Map<ISODate, string>();
  try {
    const holidays: HolidaysTypes.Holiday[] = cache.instance.getHolidays(year) || [];
    for (const holiday of holidays) {
      if (holiday.type !== 'public') continue;
      let date = holiday.date.slice(0, 10);
      for (let day = spanInDays(holiday); day > 0; day--) {
        // First rule wins, matching the previous `find(h => h.type === 'public')`.
        if (!index.has(date)) index.set(date, holiday.name);
        date = addDaysISO(date, 1);
      }
    }
  } catch (error) {
    // A broken country ruleset must not take the calendar down, but it should be
    // visible: the previous code returned `false` indistinguishably from "not a
    // holiday", which made such failures impossible to notice.
    console.warn(`[holidays] failed to load ${year} for a country ruleset`, error);
  }

  cache.years.set(year, index);
  return index;
}

/**
 * Get the holiday index for a timezone and UI language.
 *
 * `language` is the vue-i18n locale (`'fr'` / `'en'`). It is part of the cache key, so
 * switching language yields correctly localized names — the previous `getHolidayName`
 * hard-coded French and every caller relied on that default.
 */
export function getHolidayIndex(timeZone: string, language: string): HolidayIndex {
  const key = `${timeZone}|${language}`;
  const existing = indexes.get(key);
  if (existing) return existing;

  const countryCode = resolveCountry(timeZone);

  const index: HolidayIndex = countryCode
    ? (() => {
        const cache = getCountryCache(countryCode, language);
        const lookup = (date: ISODate): string | undefined =>
          loadYear(cache, yearOf(date)).get(date);
        return {
          countryCode,
          isHoliday: date => lookup(date) !== undefined,
          // Crosses years correctly: 31 December looks up the *next* year's map,
          // which `loadYear` fills on demand.
          isHolidayEve: date => lookup(addDaysISO(date, 1)) !== undefined,
          getName: date => lookup(date) ?? null,
        };
      })()
    : {
        countryCode: null,
        isHoliday: () => false,
        isHolidayEve: () => false,
        getName: () => null,
      };

  indexes.set(key, index);
  return index;
}

/**
 * Drop every cached instance and year map.
 *
 * Only tests need this. Application code must never call it: the cache is keyed by
 * country, language and year, so it cannot go stale. The previous cache was keyed by
 * country alone and was cleared on component setup and even from inside a computed,
 * forcing a 93 ms rebuild on every recompute.
 */
export function clearHolidayCache(): void {
  countryCaches.clear();
  indexes.clear();
}
