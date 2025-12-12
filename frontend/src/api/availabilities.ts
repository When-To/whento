/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient } from './client'
import type {
  Availability,
  CreateAvailabilityRequest,
  RecurrenceWithExceptions,
  CreateRecurrenceRequest,
  DateAvailabilitySummary,
  ParticipantAvailabilitiesResponse,
} from '@/types'

export const availabilitiesApi = {
  // Availability operations (public with token)
  async getByParticipant(
    token: string,
    participantId: string,
    startDate?: string,
    endDate?: string
  ): Promise<ParticipantAvailabilitiesResponse> {
    const params = new URLSearchParams()
    if (startDate) params.append('start', startDate)
    if (endDate) params.append('end', endDate)

    const queryString = params.toString()
    const url = `/availabilities/calendar/${token}/participant/${participantId}${
      queryString ? `?${queryString}` : ''
    }`

    return apiClient.get<ParticipantAvailabilitiesResponse>(url)
  },

  async create(
    token: string,
    participantId: string,
    data: CreateAvailabilityRequest
  ): Promise<Availability> {
    return apiClient.post<Availability>(
      `/availabilities/calendar/${token}/participant/${participantId}`,
      data
    )
  },

  async update(
    token: string,
    participantId: string,
    date: string,
    data: Partial<CreateAvailabilityRequest>
  ): Promise<Availability> {
    return apiClient.patch<Availability>(
      `/availabilities/calendar/${token}/participant/${participantId}/${date}`,
      data
    )
  },

  async delete(token: string, participantId: string, date: string): Promise<void> {
    return apiClient.delete<void>(
      `/availabilities/calendar/${token}/participant/${participantId}/${date}`
    )
  },

  // Recurrence operations
  async getRecurrences(token: string, participantId: string): Promise<RecurrenceWithExceptions[]> {
    return apiClient.get<RecurrenceWithExceptions[]>(
      `/availabilities/calendar/${token}/participant/${participantId}/recurrences`
    )
  },

  async createRecurrence(
    token: string,
    participantId: string,
    data: CreateRecurrenceRequest
  ): Promise<RecurrenceWithExceptions> {
    return apiClient.post<RecurrenceWithExceptions>(
      `/availabilities/calendar/${token}/participant/${participantId}/recurrence`,
      data
    )
  },

  async updateRecurrence(
    token: string,
    participantId: string,
    recurrenceId: string,
    data: CreateRecurrenceRequest
  ): Promise<RecurrenceWithExceptions> {
    return apiClient.patch<RecurrenceWithExceptions>(
      `/availabilities/calendar/${token}/participant/${participantId}/recurrence/${recurrenceId}`,
      data
    )
  },

  async deleteRecurrence(
    token: string,
    participantId: string,
    recurrenceId: string
  ): Promise<void> {
    return apiClient.delete<void>(
      `/availabilities/calendar/${token}/participant/${participantId}/recurrence/${recurrenceId}`
    )
  },

  async createException(
    token: string,
    participantId: string,
    recurrenceId: string,
    date: string
  ): Promise<void> {
    return apiClient.post<void>(
      `/availabilities/calendar/${token}/participant/${participantId}/recurrence/${recurrenceId}/exception`,
      {
        excluded_date: date,
      }
    )
  },

  async deleteException(
    token: string,
    participantId: string,
    recurrenceId: string,
    date: string
  ): Promise<void> {
    return apiClient.delete<void>(
      `/availabilities/calendar/${token}/participant/${participantId}/recurrence/${recurrenceId}/exception/${date}`
    )
  },

  // Aggregated data
  async getDateSummary(token: string, date: string): Promise<DateAvailabilitySummary> {
    return apiClient.get<DateAvailabilitySummary>(`/availabilities/calendar/${token}/dates/${date}`)
  },

  async getRangeSummary(
    token: string,
    startDate: string,
    endDate: string
  ): Promise<DateAvailabilitySummary[]> {
    return apiClient.get<DateAvailabilitySummary[]>(
      `/availabilities/calendar/${token}/range?start=${startDate}&end=${endDate}`
    )
  },
}
