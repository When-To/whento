/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { getCountryForTimezone } from 'countries-and-timezones';
import { getWeekStartDay } from '@/i18n';

/**
 * First day of the week, using the same encoding as Date.prototype.getDay()
 * (0 = Sunday ... 6 = Saturday).
 */
export type WeekStartDay = 0 | 1 | 5 | 6;

/**
 * CLDR's world-wide default (region "001") is Monday: any region absent from
 * WEEK_START_BY_REGION inherits it.
 */
export const DEFAULT_WEEK_START: WeekStartDay = 1;

/** localStorage key holding an explicit user override (no UI writes it yet). */
const STORAGE_KEY = 'week-start-day';

/**
 * First day of the week per region, from Unicode CLDR supplemental weekData
 * (`firstDay`), CLDR 48 / Unicode 16.0.0.
 *
 * Only regions that differ from the Monday default are listed. Regenerate with:
 *   curl -sSLO https://raw.githubusercontent.com/unicode-org/cldr-json/main/cldr-json/cldr-core/supplemental/weekData.json
 * then map sun->0, mon->1, fri->5, sat->6, dropping "001" and "*-alt-*" keys.
 *
 * Beware of the counter-intuitive entries before "fixing" anything here: CLDR
 * puts Portugal and Saudi Arabia on Sunday, not Monday and Saturday.
 */
export const WEEK_START_BY_REGION: Readonly<Record<string, WeekStartDay>> = {
  // Sunday (0) — 56 regions
  AG: 0,
  AS: 0,
  BD: 0,
  BR: 0,
  BS: 0,
  BT: 0,
  BW: 0,
  BZ: 0,
  CA: 0,
  CO: 0,
  DM: 0,
  DO: 0,
  ET: 0,
  GT: 0,
  GU: 0,
  HK: 0,
  HN: 0,
  ID: 0,
  IL: 0,
  IN: 0,
  IS: 0,
  JM: 0,
  JP: 0,
  KE: 0,
  KH: 0,
  KR: 0,
  LA: 0,
  MH: 0,
  MM: 0,
  MO: 0,
  MT: 0,
  MX: 0,
  MZ: 0,
  NI: 0,
  NP: 0,
  PA: 0,
  PE: 0,
  PH: 0,
  PK: 0,
  PR: 0,
  PT: 0,
  PY: 0,
  SA: 0,
  SG: 0,
  SV: 0,
  TH: 0,
  TT: 0,
  TW: 0,
  UM: 0,
  US: 0,
  VE: 0,
  VI: 0,
  WS: 0,
  YE: 0,
  ZA: 0,
  ZW: 0,
  // Saturday (6) — 14 regions
  AF: 6,
  BH: 6,
  DJ: 6,
  DZ: 6,
  EG: 6,
  IQ: 6,
  IR: 6,
  JO: 6,
  KW: 6,
  LY: 6,
  OM: 6,
  QA: 6,
  SD: 6,
  SY: 6,
  // Friday (5) — 1 region
  MV: 5,
};

/**
 * Returns the first day of the week for a CLDR region code (e.g. "DE"),
 * or undefined when the region is empty/unknown so callers can fall through.
 * A known region absent from the table resolves to the Monday default.
 */
export function getWeekStartForRegion(region?: string | null): WeekStartDay | undefined {
  if (!region) return undefined;
  const upper = region.toUpperCase();
  if (!/^[A-Z]{2}$/.test(upper)) return undefined;
  return WEEK_START_BY_REGION[upper] ?? DEFAULT_WEEK_START;
}

/**
 * Returns the first day of the week for an IANA timezone (e.g. "Europe/Berlin"),
 * or undefined when the timezone maps to no country (e.g. "UTC").
 */
export function getWeekStartForTimezone(timezone?: string | null): WeekStartDay | undefined {
  if (!timezone) return undefined;
  try {
    return getWeekStartForRegion(getCountryForTimezone(timezone)?.id);
  } catch {
    // Unknown or malformed timezone identifier
    return undefined;
  }
}

/** Reads the explicit override, tolerating unavailable/blocked storage. */
function getStoredOverride(): WeekStartDay | undefined {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw === null) return undefined;
    const day = Number(raw);
    return day === 0 || day === 1 || day === 5 || day === 6 ? day : undefined;
  } catch {
    return undefined;
  }
}

/** Region the browser is configured for, expanded via CLDR likely subtags. */
function getBrowserRegion(): string | undefined {
  const tag = typeof navigator !== 'undefined' ? navigator.language : undefined;
  if (!tag) return undefined;
  try {
    // "de" -> "de-Latn-DE", so a language-only tag still yields a region
    return new Intl.Locale(tag).maximize().region ?? undefined;
  } catch {
    return tag.split('-')[1];
  }
}

/**
 * Number of grid cells a date sits after the start of its week — the count of
 * leading cells to pad a month grid with, and the offset to subtract to reach
 * a week's first day.
 *
 * @param date         any date
 * @param weekStartDay first day of the week (0 = Sunday ... 6 = Saturday)
 */
export function getWeekdayOffset(date: Date, weekStartDay: number): number {
  return (date.getDay() - weekStartDay + 7) % 7;
}

/**
 * Resolves the first day of the week, most specific source first:
 *   1. explicit user override in localStorage
 *   2. the calendar's timezone (shared by every participant, so everyone
 *      sees the same grid — and it works for anonymous participants, who
 *      have no user account to carry a preference)
 *   3. the browser's region, when no calendar is in context
 *   4. the application locale
 *
 * @param calendarTimezone IANA timezone of the calendar being displayed, if any
 * @param appLocale        active application locale, used as the last resort
 */
export function resolveWeekStart(calendarTimezone?: string | null, appLocale?: string): number {
  return (
    getStoredOverride() ??
    getWeekStartForTimezone(calendarTimezone) ??
    getWeekStartForRegion(getBrowserRegion()) ??
    getWeekStartDay(appLocale ?? '')
  );
}
