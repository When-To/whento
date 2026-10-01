/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Public-holiday lookup, indexed by calendar date.
 *
 * The previous implementation shipped a bundled 1.4 MB `date-holidays` database
 * and its own timezone→country table. Both duplicated (and could disagree with)
 * the backend's block/allow policy, so the data now lives in exactly one place:
 * the backend's offline table, served from GET /api/v1/holidays. This module
 * fetches a year at a time and answers from a hash, exactly like the former
 * local cache — only the source changed, and with it the 1.4 MB the browser
 * used to download.
 *
 * Reactivity model: the lookups are synchronous and run thousands of times per
 * render, so they read a plain cache. When a fetch lands, `holidaysVersion`
 * increments; consumers that track it (directly or through `holidaysReady`)
 * re-derive and pick up the populated data. The old module invalidated by
 * identity, this one by a version — same effect, one less thing to cache.
 */

import { computed, ref, type Ref } from 'vue';
import { addDaysISO, yearOf, type ISODate } from '@/utils/date/isoDate';
import { holidaysApi, type YearHoliday } from '@/api/holidays';

/** Public holidays for one timezone, answered by calendar date. */
export interface HolidayIndex {
  /** ISO country code resolved by the backend, or null when none is known yet. */
  readonly countryCode: string | null;
  /** Whether the offline table has national data for this country. */
  readonly supported: boolean;
  /** Whether the date is an official public holiday. */
  isHoliday(date: ISODate): boolean;
  /** Whether the *next* day is an official public holiday. */
  isHolidayEve(date: ISODate): boolean;
  /** Localized holiday name, or null when the date is not a public holiday. */
  getName(date: ISODate): string | null;
}

/** one cache cell: timezone -> year -> (date -> name) */
type YearMap = Map<number, Map<ISODate, string>>;

const yearCache = new Map<string, YearMap>();
/** timezone -> country code, once the backend has answered (null = no country). */
const countryByTimezone = new Map<string, string | null>();
/** timezone -> whether the backend's offline table covers it. */
const supportedByTimezone = new Map<string, boolean>();
const indexes = new Map<string, HolidayIndex>();
const yearLoads = new Map<string, Promise<void>>();

/** Increments whenever holiday data arrives; consumers track it to re-derive. */
export const holidaysVersion: Ref<number> = ref(0);

/**
 * True once at least one backend round trip has landed. Kept as a boolean for
 * callers that only want "engine ready"; it is derived from the version so both
 * stay in step.
 */
export const holidaysReady: Ref<boolean> = computed(() => holidaysVersion.value > 0);

/**
 * Warm-up hook kept for signature compatibility with callers that preload on
 * mount. Data is fetched per (timezone, year) on demand from `getHolidayIndex`,
 * so there is nothing to do here: it resolves immediately and is idempotent.
 */
export async function preloadHolidays(): Promise<void> {}

/**
 * Resolve an IANA timezone to an ISO country code.
 *
 * The backend owns this mapping (GET /api/v1/holidays answers `country_code`
 * for a timezone), so the frontend carries no timezone→country table. Before
 * the first fetch for a timezone the answer is null; the lookup functions start
 * that fetch, and `holidaysVersion` flips when it lands.
 */
export function resolveCountry(timeZone: string): string | null {
  if (!timeZone) return null;
  const cached = countryByTimezone.get(timeZone);
  if (cached !== undefined) return cached;
  return null;
}

function isoToday(): string {
  const now = new Date();
  const m = String(now.getMonth() + 1).padStart(2, '0');
  const d = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${m}-${d}`;
}

async function loadYear(timeZone: string, year: number): Promise<void> {
  // Already in the cache, or already being fetched: one fetch per (timezone,
  // year) no matter how many lookups or re-renders ask.
  if (yearCache.get(timeZone)?.has(year) || yearLoads.has(`${timeZone}|${year}`)) {
    return;
  }

  const key = `${timeZone}|${year}`;

  const load = (async () => {
    let holidays: YearHoliday[];
    try {
      const result = await holidaysApi.year(timeZone, year);
      countryByTimezone.set(timeZone, result.country_code);
      supportedByTimezone.set(timeZone, result.supported);
      holidays = result.holidays;
    } catch (error) {
      // The calendar keeps working without holiday shading; the failure is
      // visible rather than indistinguishably "not a holiday".
      // eslint-disable-next-line no-console
      console.warn(`[holidays] failed to load ${year} for ${timeZone}`, error);
      holidays = [];
    }

    const byDate = new Map<ISODate, string>();
    for (const h of holidays) {
      if (!byDate.has(h.date)) byDate.set(h.date, h.name);
    }
    let years = yearCache.get(timeZone);
    if (!years) {
      years = new Map();
      yearCache.set(timeZone, years);
    }
    years.set(year, byDate);

    holidaysVersion.value++;
  })();

  yearLoads.set(key, load);
  try {
    await load;
  } finally {
    yearLoads.delete(key);
  }
}

function lookup(timeZone: string, date: ISODate): string | undefined {
  const year = yearOf(date);
  const years = yearCache.get(timeZone);
  if (!years || !years.has(year)) {
    // Missing years are fetched on demand; the version bump makes consumers
    // re-derive once the data lands.
    void loadYear(timeZone, year);
    return undefined;
  }
  return years.get(year)!.get(date);
}

/**
 * Get the holiday index for a timezone and UI language.
 *
 * Synchronous by design: it is called from computeds, thousands of times per
 * render. The returned object is cached by (timezone, language) and always
 * reads the live year cache, so before a timezone's data has arrived it answers
 * "no holidays"; when a fetch lands, `holidaysVersion` ticks and consumers that
 * track it (through `holidaysReady`) re-derive — the same object then serves
 * the populated answers.
 */
export function getHolidayIndex(timeZone: string, language: string): HolidayIndex {
  const key = `${timeZone}|${language}`;
  const cached = indexes.get(key);
  if (cached) return cached;

  const index: HolidayIndex = {
    countryCode: resolveCountry(timeZone),
    supported: supportedByTimezone.get(timeZone) === true,
    isHoliday: date => lookup(timeZone, date) !== undefined,
    // Crosses years correctly: 31 December looks up the *next* year's map,
    // which `lookup` fills on demand.
    isHolidayEve: date => lookup(timeZone, addDaysISO(date, 1)) !== undefined,
    getName: date => lookup(timeZone, date) ?? null,
  };
  indexes.set(key, index);

  // Kick the first fetch (it resolves the country too). The version bump makes
  // dependent computeds re-derive once the data lands.
  void loadYear(timeZone, yearOf(isoToday()));
  return index;
}

/**
 * Drop every cached instance and year map.
 *
 * Only tests need this. Application code must never call it: the cache is keyed
 * by timezone and year and invalidated by the version counter, so it cannot go
 * stale.
 */
export function clearHolidayCache(): void {
  yearCache.clear();
  countryByTimezone.clear();
  supportedByTimezone.clear();
  indexes.clear();
  yearLoads.clear();
  holidaysVersion.value = 0;
}
