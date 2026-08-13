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

import { ref, type Ref } from 'vue';
import { getCountryForTimezone } from 'countries-and-timezones';
import { addDaysISO, yearOf, type ISODate } from '@/utils/date/isoDate';

// Type-only, so nothing of `date-holidays` survives into this module's static
// imports — the whole point of the split below.
import type Holidays from 'date-holidays';
import type { HolidaysTypes } from 'date-holidays';

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
 * `date-holidays` is 1.4 MB — by a wide margin the largest thing the app ships, and
 * only the participant calendar ever needs it. A static `import` here made it part
 * of the entry bundle for every visitor, including anyone who only ever sees the
 * login page, which is what the "lazy loaded chunk" comment in vite.config.ts
 * claimed was already happening and was not.
 *
 * So the library is fetched on demand. Until it arrives, `getHolidayIndex` answers
 * "no holidays" rather than blocking, and `holidaysReady` flips when the module
 * lands so the UI can recompute. The window is one network round trip on a route
 * that is already fetching its calendar data.
 */
type HolidaysConstructor = typeof import('date-holidays').default;

let HolidaysCtor: HolidaysConstructor | null = null;
let enginePromise: Promise<void> | null = null;

/**
 * Flips to true once the engine is in memory.
 *
 * Reactive so callers can re-derive when it changes; deliberately *not* read inside
 * `isHoliday`, which runs thousands of times per grid render. Instead the index
 * cache is dropped on load, so `getHolidayIndex` hands back a new object and any
 * computed built on it invalidates by identity.
 */
export const holidaysReady: Ref<boolean> = ref(false);

/**
 * Fetch the holiday engine. Idempotent, and safe to call from a render path: repeat
 * callers share the one in-flight import.
 */
export function preloadHolidays(): Promise<void> {
  enginePromise ??= import('date-holidays')
    .then(module => {
      HolidaysCtor = module.default;
      // Drop the placeholder indexes handed out while the engine was loading, so the
      // next `getHolidayIndex` builds a real one *and returns a different object*.
      indexes.clear();
      holidaysReady.value = true;
    })
    .catch(error => {
      // Same contract as a broken country ruleset below: the calendar keeps working
      // without holiday shading, and the failure is visible rather than silent.
      // eslint-disable-next-line no-console
      console.warn('[holidays] failed to load the holiday engine', error);
    });
  return enginePromise;
}

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
    // Only ever reached from `getHolidayIndex` after `HolidaysCtor` is set.
    const Ctor = HolidaysCtor as HolidaysConstructor;
    cache = {
      instance: new Ctor(countryCode, { languages: [language] }),
      years: new Map(),
    };
    countryCaches.set(key, cache);
  }
  return cache;
}

/** The answer for an unsupported country, and for the window before the engine loads. */
function emptyIndex(countryCode: string | null): HolidayIndex {
  return {
    countryCode,
    isHoliday: () => false,
    isHolidayEve: () => false,
    getName: () => null,
  };
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
 *
 * Synchronous by design: it is called from computeds, thousands of times per render.
 * Before the engine has loaded it answers "no holidays" and starts the fetch; when
 * that lands, the cache is dropped and the next call returns a fully populated index
 * under a new identity, which is what makes dependent computeds re-run.
 */
export function getHolidayIndex(timeZone: string, language: string): HolidayIndex {
  const key = `${timeZone}|${language}`;
  const existing = indexes.get(key);
  if (existing) return existing;

  const countryCode = resolveCountry(timeZone);

  let index: HolidayIndex;
  if (!countryCode) {
    index = emptyIndex(null);
  } else if (!HolidaysCtor) {
    void preloadHolidays();
    // Not cached under `indexes`: caching it would be correct (the load clears the
    // map) but leaves a stale object reachable if the import fails outright.
    return emptyIndex(countryCode);
  } else {
    const cache = getCountryCache(countryCode, language);
    const lookup = (date: ISODate): string | undefined => loadYear(cache, yearOf(date)).get(date);
    index = {
      countryCode,
      isHoliday: date => lookup(date) !== undefined,
      // Crosses years correctly: 31 December looks up the *next* year's map,
      // which `loadYear` fills on demand.
      isHolidayEve: date => lookup(addDaysISO(date, 1)) !== undefined,
      getName: date => lookup(date) ?? null,
    };
  }

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
