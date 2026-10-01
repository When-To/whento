/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient } from './client';

/** One public holiday as served by the backend's offline dataset. */
export interface YearHoliday {
  date: string; // ISO 8601 (YYYY-MM-DD)
  name: string;
}

/** The payload of GET /api/v1/holidays. */
export interface HolidaysYear {
  country_code: string | null;
  supported: boolean;
  holidays: YearHoliday[];
}

export interface SupportedHolidays {
  countries: string[];
}

/**
 * The backend is the single holiday source (pkg/datevalidation's offline table).
 * The frontend used to ship its own 1.4 MB holiday library; this module fetches
 * the same dates the backend block/allow policy enforces instead.
 */
export const holidaysApi = {
  async year(timezone: string, year: number): Promise<HolidaysYear> {
    return apiClient.get<HolidaysYear>('/holidays', {
      params: { timezone, year },
    });
  },

  async supported(): Promise<SupportedHolidays> {
    return apiClient.get<SupportedHolidays>('/holidays/supported');
  },
};
