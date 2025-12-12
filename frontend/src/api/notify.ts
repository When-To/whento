/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * Licensed under the Business Source License 1.1
 * See LICENSE file for details
 */

import { apiClient } from './client'
import type {
  NotifyConfig,
  NotifyConfigResponse,
  ParticipantEmailResponse,
} from '@/types'

// Re-export types for convenience
export type { NotifyConfig }

/**
 * Get notification configuration for a calendar
 */
export const getNotifyConfig = async (calendarId: string): Promise<NotifyConfig> => {
  const response = await apiClient.get<NotifyConfigResponse>(
    `/calendars/${calendarId}/notify-config`
  );
  return response.config;
};

/**
 * Update notification configuration for a calendar
 */
export const updateNotifyConfig = async (
  calendarId: string,
  config: NotifyConfig
): Promise<NotifyConfig> => {
  const response = await apiClient.patch<NotifyConfigResponse>(
    `/calendars/${calendarId}/notify-config`,
    { config }
  );
  return response.config;
};

/**
 * Add email address to a participant for notifications
 */
export const addParticipantEmail = async (
  token: string,
  participantId: string,
  email: string
): Promise<ParticipantEmailResponse> => {
  return await apiClient.post<ParticipantEmailResponse>(
    `/calendars/${token}/participants/${participantId}/email`,
    { email }
  );
};

/**
 * Verify participant email with verification token
 */
export const verifyParticipantEmail = async (token: string): Promise<{ message: string }> => {
  return await apiClient.get<{ message: string }>(
    `/calendars/participants/verify-email/${token}`
  );
};

/**
 * Resend verification email to a participant
 */
export const resendVerificationEmail = async (
  token: string,
  participantId: string
): Promise<{ message: string }> => {
  return await apiClient.post<{ message: string }>(
    `/calendars/${token}/participants/${participantId}/resend-verification`
  );
};

/**
 * Get default notification configuration
 */
export const getDefaultNotifyConfig = (): NotifyConfig => {
  return {
    enabled: false,
    notify_owner: true,
    notify_participants: false,
    channels: {
      email: { enabled: true },
      discord: { enabled: false },
      slack: { enabled: false },
      telegram: { enabled: false },
    },
    reminders: {
      enabled: false,
      hours_before: 24,
    },
  };
};
