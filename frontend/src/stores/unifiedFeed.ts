/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';
import { unifiedFeedApi } from '@/api/unifiedFeed';
import { useAsyncActions } from '@/stores/asyncAction';
import type { UnifiedFeedConfig } from '@/types';

export const useUnifiedFeedStore = defineStore('unifiedFeed', () => {
  const config = ref<UnifiedFeedConfig | null>(null);
  const { loading, error, run, clearError } = useAsyncActions();

  async function fetchConfig() {
    return run('unifiedFeed.fetchError', async () => {
      config.value = await unifiedFeedApi.getConfig();
    });
  }

  async function createFeed() {
    return run('unifiedFeed.createError', async () => {
      config.value = await unifiedFeedApi.create();
    });
  }

  async function updateCalendars(calendarIds: string[]) {
    return run('unifiedFeed.updateError', async () => {
      await unifiedFeedApi.updateCalendars(calendarIds);
      if (config.value) {
        config.value.included_calendar_ids = calendarIds;
      }
    });
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
    return run('unifiedFeed.regenerateError', async () => {
      const { ics_token } = await unifiedFeedApi.regenerateToken();
      if (config.value) {
        config.value.ics_token = ics_token;
      }
    });
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
    clearError,
  };
});
