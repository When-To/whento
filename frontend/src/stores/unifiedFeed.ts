/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';
import { unifiedFeedApi } from '@/api/unifiedFeed';
import type { UnifiedFeedConfig } from '@/types';

export const useUnifiedFeedStore = defineStore('unifiedFeed', () => {
  const config = ref<UnifiedFeedConfig | null>(null);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function fetchConfig() {
    loading.value = true;
    error.value = null;

    try {
      config.value = await unifiedFeedApi.getConfig();
    } catch (err: any) {
      error.value = err.message || 'Failed to fetch unified feed config';
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function createFeed() {
    loading.value = true;
    error.value = null;

    try {
      config.value = await unifiedFeedApi.create();
    } catch (err: any) {
      error.value = err.message || 'Failed to create unified feed';
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function updateCalendars(calendarIds: string[]) {
    loading.value = true;
    error.value = null;

    try {
      await unifiedFeedApi.updateCalendars(calendarIds);
      if (config.value) {
        config.value.included_calendar_ids = calendarIds;
      }
    } catch (err: any) {
      error.value = err.message || 'Failed to update calendars';
      throw err;
    } finally {
      loading.value = false;
    }
  }

  async function toggleCalendar(calendarId: string) {
    if (!config.value) return;

    const current = config.value.included_calendar_ids || [];
    const newIds = current.includes(calendarId)
      ? current.filter(id => id !== calendarId)
      : [...current, calendarId];

    await updateCalendars(newIds);
  }

  async function regenerateToken() {
    loading.value = true;
    error.value = null;

    try {
      const { ics_token } = await unifiedFeedApi.regenerateToken();
      if (config.value) {
        config.value.ics_token = ics_token;
      }
    } catch (err: any) {
      error.value = err.message || 'Failed to regenerate token';
      throw err;
    } finally {
      loading.value = false;
    }
  }

  function isCalendarIncluded(calendarId: string): boolean {
    return config.value?.included_calendar_ids?.includes(calendarId) ?? false;
  }

  return {
    config,
    loading,
    error,
    fetchConfig,
    createFeed,
    updateCalendars,
    toggleCalendar,
    regenerateToken,
    isCalendarIncluded,
  };
});
