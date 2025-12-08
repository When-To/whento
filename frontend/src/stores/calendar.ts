/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia'
import { ref } from 'vue'
import { calendarsApi } from '@/api/calendars'
import type {
  CalendarWithParticipants,
  CreateCalendarRequest,
  UpdateCalendarRequest,
  CreateParticipantRequest,
  UpdateParticipantRequest,
} from '@/types'

export const useCalendarStore = defineStore('calendar', () => {
  // State
  const calendars = ref<CalendarWithParticipants[]>([])
  const currentCalendar = ref<CalendarWithParticipants | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Actions
  async function fetchCalendars() {
    loading.value = true
    error.value = null

    try {
      const result = await calendarsApi.getAll()
      calendars.value = Array.isArray(result) ? result : []
    } catch (err: any) {
      error.value = err.message || 'Failed to fetch calendars'
      calendars.value = [] // Reset to empty array on error
      throw err
    } finally {
      loading.value = false
    }
  }

  async function fetchCalendar(id: string) {
    loading.value = true
    error.value = null

    try {
      currentCalendar.value = await calendarsApi.getById(id)
    } catch (err: any) {
      error.value = err.message || 'Failed to fetch calendar'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function fetchPublicCalendar(token: string, participantId?: string) {
    loading.value = true
    error.value = null

    try {
      currentCalendar.value = await calendarsApi.getPublic(token, participantId)
    } catch (err: any) {
      error.value = err.message || 'Failed to fetch calendar'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function createCalendar(data: CreateCalendarRequest) {
    loading.value = true
    error.value = null

    try {
      const calendar = await calendarsApi.create(data)
      // Ensure calendars is an array before pushing
      if (!Array.isArray(calendars.value)) {
        calendars.value = []
      }
      // Add empty participants array since create doesn't return participants
      const calendarWithParticipants: CalendarWithParticipants = {
        ...calendar,
        participants: [],
      }
      calendars.value.push(calendarWithParticipants)
      return calendar
    } catch (err: any) {
      error.value = err.message || 'Failed to create calendar'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function updateCalendar(id: string, data: UpdateCalendarRequest) {
    loading.value = true
    error.value = null

    try {
      const updated = await calendarsApi.update(id, data)
      const index = calendars.value.findIndex(c => c.id === id)
      if (index !== -1) {
        // Preserve existing participants since update doesn't return them
        const existingParticipants = calendars.value[index].participants || []
        calendars.value[index] = { ...updated, participants: existingParticipants }
      }
      if (currentCalendar.value?.id === id) {
        currentCalendar.value = { ...currentCalendar.value, ...updated }
      }
      return updated
    } catch (err: any) {
      error.value = err.message || 'Failed to update calendar'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function deleteCalendar(id: string) {
    loading.value = true
    error.value = null

    try {
      await calendarsApi.delete(id)
      calendars.value = calendars.value.filter(c => c.id !== id)
      if (currentCalendar.value?.id === id) {
        currentCalendar.value = null
      }
    } catch (err: any) {
      error.value = err.message || 'Failed to delete calendar'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function addParticipant(calendarId: string, data: CreateParticipantRequest) {
    loading.value = true
    error.value = null

    try {
      const participant = await calendarsApi.addParticipant(calendarId, data)
      if (currentCalendar.value?.id === calendarId) {
        currentCalendar.value.participants.push(participant)
      }
      return participant
    } catch (err: any) {
      error.value = err.message || 'Failed to add participant'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function updateParticipant(
    calendarId: string,
    participantId: string,
    data: UpdateParticipantRequest
  ) {
    loading.value = true
    error.value = null

    try {
      const participant = await calendarsApi.updateParticipant(calendarId, participantId, data)
      if (currentCalendar.value?.id === calendarId) {
        const index = currentCalendar.value.participants.findIndex(p => p.id === participantId)
        if (index !== -1) {
          currentCalendar.value.participants[index] = participant
        }
      }
      return participant
    } catch (err: any) {
      error.value = err.message || 'Failed to update participant'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function deleteParticipant(calendarId: string, participantId: string) {
    loading.value = true
    error.value = null

    try {
      await calendarsApi.deleteParticipant(calendarId, participantId)
      if (currentCalendar.value?.id === calendarId) {
        currentCalendar.value.participants = currentCalendar.value.participants.filter(
          p => p.id !== participantId
        )
      }
    } catch (err: any) {
      error.value = err.message || 'Failed to delete participant'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function regeneratePublicToken(id: string) {
    loading.value = true
    error.value = null

    try {
      const { public_token } = await calendarsApi.regeneratePublicToken(id)
      if (currentCalendar.value?.id === id) {
        currentCalendar.value.public_token = public_token
      }
      const calendar = calendars.value.find(c => c.id === id)
      if (calendar) {
        calendar.public_token = public_token
      }
      return public_token
    } catch (err: any) {
      error.value = err.message || 'Failed to regenerate token'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function regenerateICSToken(id: string) {
    loading.value = true
    error.value = null

    try {
      const { ics_token } = await calendarsApi.regenerateICSToken(id)
      if (currentCalendar.value?.id === id) {
        currentCalendar.value.ics_token = ics_token
      }
      const calendar = calendars.value.find(c => c.id === id)
      if (calendar) {
        calendar.ics_token = ics_token
      }
      return ics_token
    } catch (err: any) {
      error.value = err.message || 'Failed to regenerate ICS token'
      throw err
    } finally {
      loading.value = false
    }
  }

  function clearCurrentCalendar() {
    currentCalendar.value = null
  }

  return {
    // State
    calendars,
    currentCalendar,
    loading,
    error,

    // Actions
    fetchCalendars,
    fetchCalendar,
    fetchPublicCalendar,
    createCalendar,
    updateCalendar,
    deleteCalendar,
    addParticipant,
    updateParticipant,
    deleteParticipant,
    regeneratePublicToken,
    regenerateICSToken,
    clearCurrentCalendar,
  }
})
