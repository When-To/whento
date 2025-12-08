/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { apiClient } from './client'
import type {
  Calendar,
  CalendarWithParticipants,
  CreateCalendarRequest,
  UpdateCalendarRequest,
  Participant,
  CreateParticipantRequest,
  UpdateParticipantRequest,
} from '@/types'

export const calendarsApi = {
  // Calendar operations (authenticated)
  async getAll(): Promise<CalendarWithParticipants[]> {
    return apiClient.get<CalendarWithParticipants[]>('/calendars')
  },

  async getById(id: string): Promise<CalendarWithParticipants> {
    return apiClient.get<CalendarWithParticipants>(`/calendars/${id}`)
  },

  async create(data: CreateCalendarRequest): Promise<Calendar> {
    return apiClient.post<Calendar>('/calendars', data)
  },

  async update(id: string, data: UpdateCalendarRequest): Promise<Calendar> {
    return apiClient.patch<Calendar>(`/calendars/${id}`, data)
  },

  async delete(id: string): Promise<void> {
    return apiClient.delete<void>(`/calendars/${id}`)
  },

  async regeneratePublicToken(id: string): Promise<{ public_token: string }> {
    return apiClient.post<{ public_token: string }>(`/calendars/${id}/regenerate-token`, {
      token_type: 'public',
    })
  },

  async regenerateICSToken(id: string): Promise<{ ics_token: string }> {
    return apiClient.post<{ ics_token: string }>(`/calendars/${id}/regenerate-token`, {
      token_type: 'ics',
    })
  },

  // Participant operations (authenticated)
  async addParticipant(calendarId: string, data: CreateParticipantRequest): Promise<Participant> {
    return apiClient.post<Participant>(`/calendars/${calendarId}/participants`, data)
  },

  async updateParticipant(
    calendarId: string,
    participantId: string,
    data: UpdateParticipantRequest
  ): Promise<Participant> {
    return apiClient.patch<Participant>(
      `/calendars/${calendarId}/participants/${participantId}`,
      data
    )
  },

  async deleteParticipant(calendarId: string, participantId: string): Promise<void> {
    return apiClient.delete<void>(`/calendars/${calendarId}/participants/${participantId}`)
  },

  // Public calendar view (no auth required)
  async getPublic(token: string, participantId?: string): Promise<CalendarWithParticipants> {
    const params = participantId ? { participant_id: participantId } : {}
    return apiClient.get<CalendarWithParticipants>(`/calendars/public/${token}`, { params })
  },

  async getSummary(token: string): Promise<any> {
    return apiClient.get<any>(`/calendars/public/${token}/summary`)
  },
}
