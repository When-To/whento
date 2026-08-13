/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';
import { calendarsApi } from '@/api/calendars';
import { useAsyncActions } from '@/stores/asyncAction';
import type {
  CalendarWithParticipants,
  CreateCalendarRequest,
  PublicCalendar,
  UpdateCalendarRequest,
  CreateParticipantRequest,
  UpdateParticipantRequest,
} from '@/types';

export const useCalendarStore = defineStore('calendar', () => {
  // State
  const calendars = ref<CalendarWithParticipants[]>([]);
  /** The calendar being managed by its owner (or an admin). */
  const currentCalendar = ref<CalendarWithParticipants | null>(null);
  /**
   * The calendar being viewed through a public link. Kept apart from
   * `currentCalendar` because the backend answers those two routes with
   * different shapes: one slot for both made every owner-only field look
   * available to the public views, where it is in fact undefined.
   */
  const currentPublicCalendar = ref<PublicCalendar | null>(null);

  // `loading` is derived from a counter of in-flight actions rather than being a
  // flag each action sets and clears. See stores/asyncAction.ts.
  const { loading, error, run, clearError } = useAsyncActions();

  // Actions
  async function fetchCalendars() {
    return run('calendar.fetchError', async () => {
      try {
        const result = await calendarsApi.getAll();
        calendars.value = Array.isArray(result) ? result : [];
      } catch (err) {
        calendars.value = []; // Reset to empty array on error
        throw err;
      }
    });
  }

  async function fetchCalendar(id: string) {
    return run('calendar.fetchError', async () => {
      currentCalendar.value = await calendarsApi.getById(id);
    });
  }

  async function fetchPublicCalendar(token: string, participantId?: string) {
    return run('calendar.fetchError', async () => {
      currentPublicCalendar.value = await calendarsApi.getPublic(token, participantId);
    });
  }

  async function createCalendar(data: CreateCalendarRequest) {
    return run('calendar.createError', async () => {
      const calendar = await calendarsApi.create(data);
      // Ensure calendars is an array before pushing
      if (!Array.isArray(calendars.value)) {
        calendars.value = [];
      }
      // Add empty participants array since create doesn't return participants
      const calendarWithParticipants: CalendarWithParticipants = {
        ...calendar,
        participants: [],
      };
      calendars.value.push(calendarWithParticipants);
      return calendar;
    });
  }

  async function updateCalendar(id: string, data: UpdateCalendarRequest) {
    return run('calendar.updateError', async () => {
      const updated = await calendarsApi.update(id, data);
      const index = calendars.value.findIndex(c => c.id === id);
      if (index !== -1) {
        // Preserve existing participants since update doesn't return them
        const existingParticipants = calendars.value[index].participants || [];
        calendars.value[index] = { ...updated, participants: existingParticipants };
      }
      if (currentCalendar.value?.id === id) {
        currentCalendar.value = { ...currentCalendar.value, ...updated };
      }
      return updated;
    });
  }

  async function deleteCalendar(id: string) {
    return run('calendar.deleteError', async () => {
      await calendarsApi.delete(id);
      calendars.value = calendars.value.filter(c => c.id !== id);
      if (currentCalendar.value?.id === id) {
        currentCalendar.value = null;
      }
    });
  }

  async function addParticipant(calendarId: string, data: CreateParticipantRequest) {
    return run('calendar.addParticipantError', async () => {
      const participant = await calendarsApi.addParticipant(calendarId, data);
      if (currentCalendar.value?.id === calendarId) {
        currentCalendar.value.participants.push(participant);
      }
      return participant;
    });
  }

  async function updateParticipant(
    calendarId: string,
    participantId: string,
    data: UpdateParticipantRequest
  ) {
    return run('calendar.updateParticipantError', async () => {
      const participant = await calendarsApi.updateParticipant(calendarId, participantId, data);
      if (currentCalendar.value?.id === calendarId) {
        const index = currentCalendar.value.participants.findIndex(p => p.id === participantId);
        if (index !== -1) {
          currentCalendar.value.participants[index] = participant;
        }
      }
      return participant;
    });
  }

  async function deleteParticipant(calendarId: string, participantId: string) {
    return run('calendar.deleteParticipantError', async () => {
      await calendarsApi.deleteParticipant(calendarId, participantId);
      if (currentCalendar.value?.id === calendarId) {
        currentCalendar.value.participants = currentCalendar.value.participants.filter(
          p => p.id !== participantId
        );
      }
    });
  }

  async function addAnonymousParticipant(token: string, data: CreateParticipantRequest) {
    return run('calendar.addParticipantError', async () => {
      const participant = await calendarsApi.addAnonymousParticipant(token, data);
      if (currentPublicCalendar.value) {
        currentPublicCalendar.value.participants.push(participant);
      }
      return participant;
    });
  }

  async function regeneratePublicToken(id: string) {
    return run('calendar.regenerateError', async () => {
      const { public_token } = await calendarsApi.regeneratePublicToken(id);
      if (currentCalendar.value?.id === id) {
        currentCalendar.value.public_token = public_token;
      }
      const calendar = calendars.value.find(c => c.id === id);
      if (calendar) {
        calendar.public_token = public_token;
      }
      return public_token;
    });
  }

  async function regenerateICSToken(id: string) {
    return run('calendar.regenerateError', async () => {
      const { ics_token } = await calendarsApi.regenerateICSToken(id);
      if (currentCalendar.value?.id === id) {
        currentCalendar.value.ics_token = ics_token;
      }
      const calendar = calendars.value.find(c => c.id === id);
      if (calendar) {
        calendar.ics_token = ics_token;
      }
      return ics_token;
    });
  }

  function clearCurrentCalendar() {
    currentCalendar.value = null;
    currentPublicCalendar.value = null;
  }

  return {
    // State
    calendars,
    currentCalendar,
    currentPublicCalendar,
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
    addAnonymousParticipant,
    updateParticipant,
    deleteParticipant,
    regeneratePublicToken,
    regenerateICSToken,
    clearCurrentCalendar,
    clearError,
  };
});
