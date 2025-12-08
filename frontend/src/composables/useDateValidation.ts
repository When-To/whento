/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import Holidays from 'date-holidays'

// Type for holidays returned by date-holidays
interface Holiday {
  date: string
  start: Date
  end: Date
  name: string
  type: string
  rule?: string
}

// Cache for Holidays instances by country
const holidaysCache = new Map<string, Holidays>()

// Cache for timezone → country code mapping (generated from date-holidays)
let timezoneToCountryMap: Record<string, string> | null = null

// Function to invalidate cache (useful for forcing a reload)
export function clearHolidaysCache() {
  holidaysCache.clear()
  timezoneToCountryMap = null
}

/**
 * Builds the timezone → country mapping from date-holidays data
 * This function scans all countries and their zones to create the reverse mapping
 */
function buildTimezoneToCountryMap(): Record<string, string> {
  if (timezoneToCountryMap !== null) {
    return timezoneToCountryMap
  }

  const hd = new Holidays()
  const mapping: Record<string, string> = {}

  // Get all countries from date-holidays
  const countries = hd.getCountries()

  // For each country, get its zones and build the reverse mapping
  for (const countryCode of Object.keys(countries)) {
    try {
      const countryHd = new Holidays(countryCode)
      const zones = countryHd.getTimezones()

      // Associate each zone with this country
      if (zones && Array.isArray(zones)) {
        for (const zone of zones) {
          // Don't overwrite if already defined (priority to first country found)
          if (!mapping[zone]) {
            mapping[zone] = countryCode
          }
        }
      }
    } catch (_error) {
      // Ignore unsupported countries
      continue
    }
  }

  timezoneToCountryMap = mapping
  return mapping
}

/**
 * Converts an IANA timezone to ISO country code
 * Uses internal date-holidays data (zones defined in YAML files)
 */
function getCountryFromTimezone(timezone: string): string | null {
  const mapping = buildTimezoneToCountryMap()
  return mapping[timezone] || null
}

/**
 * Gets or creates a Holidays instance for a given country
 */
function getHolidaysInstance(countryCode: string): Holidays {
  if (!holidaysCache.has(countryCode)) {
    holidaysCache.set(countryCode, new Holidays(countryCode))
  }
  return holidaysCache.get(countryCode)!
}

/**
 * Checks if a date is an official holiday (type "public" only)
 * Does NOT return celebrations like Mother's Day, Father's Day, etc.
 */
function isHoliday(date: Date, countryCode: string): boolean {
  try {
    const hd = getHolidaysInstance(countryCode)
    const holidays = hd.isHoliday(date) as Holiday[] | Holiday | false
    if (!holidays) return false

    // Filter only public holidays
    if (Array.isArray(holidays)) {
      return holidays.some(h => h.type === 'public')
    }

    // If it's a single object, check its type
    return holidays.type === 'public'
  } catch (_error) {
    // If country is not supported, return false
    return false
  }
}

/**
 * Checks if a date is the day before a holiday
 */
function isHolidayEve(date: Date, countryCode: string): boolean {
  const nextDay = new Date(date)
  nextDay.setDate(nextDay.getDate() + 1)
  return isHoliday(nextDay, countryCode)
}

/**
 * Composable to validate if a date is allowed according to calendar parameters
 */
export function useDateValidation() {
  /**
   * Checks if a date is allowed for adding availability
   * Similar logic to backend: checks weekday, holidays policy, and holiday eves
   */
  const isDateAllowed = (
    date: Date,
    timezone: string,
    allowedWeekdays: number[],
    holidaysPolicy: 'ignore' | 'allow' | 'block',
    allowHolidayEves: boolean
  ): boolean => {
    // Get country code to check holidays
    const countryCode = getCountryFromTimezone(timezone)

    // Check if it's a holiday (if we have the country code)
    const isHolidayDate = countryCode ? isHoliday(date, countryCode) : false

    // Apply holiday policy
    if (holidaysPolicy === 'block' && isHolidayDate) {
      // If it's a holiday and policy is "block", reject
      return false
    }

    if (holidaysPolicy === 'allow' && isHolidayDate) {
      // If it's a holiday and policy is "allow", accept
      return true
    }

    // For "ignore" or non-holidays, check day of week
    const weekday = date.getDay()
    if (!allowedWeekdays || allowedWeekdays.length === 0 || allowedWeekdays.includes(weekday)) {
      return true
    }

    // If day of week is not allowed, check holiday eve exception
    if (countryCode && allowHolidayEves && isHolidayEve(date, countryCode)) {
      return true
    }

    return false
  }

  /**
   * Checks if a date is a holiday
   * Useful for visual display
   */
  const checkIsHoliday = (date: Date, timezone: string): boolean => {
    const countryCode = getCountryFromTimezone(timezone)
    if (!countryCode) return false
    return isHoliday(date, countryCode)
  }

  /**
   * Checks if a date is a holiday eve
   * Useful for visual display
   */
  const checkIsHolidayEve = (date: Date, timezone: string): boolean => {
    const countryCode = getCountryFromTimezone(timezone)
    if (!countryCode) return false
    return isHolidayEve(date, countryCode)
  }

  /**
   * Gets the name of an official holiday (type "public" only)
   */
  const getHolidayName = (date: Date, timezone: string, locale: string = 'fr'): string | null => {
    const countryCode = getCountryFromTimezone(timezone)
    if (!countryCode) return null

    try {
      // For getHolidayName, we can't use cache because we need the locale
      // Create a temporary instance with the locale
      const hd = new Holidays(countryCode, { languages: [locale] })
      const holidays = hd.isHoliday(date) as Holiday[] | Holiday | false
      if (holidays && Array.isArray(holidays) && holidays.length > 0) {
        // Return only public holidays
        const publicHoliday = holidays.find(h => h.type === 'public')
        return publicHoliday ? publicHoliday.name : null
      }
      // If it's a single object, check its type
      if (
        holidays &&
        typeof holidays === 'object' &&
        !Array.isArray(holidays) &&
        holidays.type === 'public'
      ) {
        return holidays.name
      }
    } catch (_error) {
      return null
    }

    return null
  }

  return {
    isDateAllowed,
    checkIsHoliday,
    checkIsHolidayEve,
    getHolidayName,
  }
}
