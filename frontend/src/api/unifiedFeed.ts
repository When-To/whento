/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient } from './client';
import type { UnifiedFeedConfig } from '@/types';

export const unifiedFeedApi = {
  async getConfig(): Promise<UnifiedFeedConfig> {
    return apiClient.get<UnifiedFeedConfig>('/ics/unified-feed');
  },

  async create(): Promise<UnifiedFeedConfig> {
    return apiClient.post<UnifiedFeedConfig>('/ics/unified-feed', {});
  },

  async updateCalendars(calendarIds: string[]): Promise<void> {
    return apiClient.patch<void>('/ics/unified-feed/calendars', { calendar_ids: calendarIds });
  },

  async regenerateToken(): Promise<{ ics_token: string }> {
    return apiClient.post<{ ics_token: string }>('/ics/unified-feed/regenerate-token', {});
  },
};
