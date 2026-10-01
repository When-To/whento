/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';
import { calendarsApi } from '@/api/calendars';
import { useAuthStore } from '@/stores/auth';
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
  /**
   * The user id that the `calendars` list was loaded for, or null when it has never
   * been loaded (or the user is signed out).
   *
   * The dashboard list answers "which calendars does *this* visitor own?" and carries
   * full participant ids. It must never be reused for a different account: on a shared
   * browser a stale list from account A would otherwise make account B look like the
   * owner of A's calendars when they open a public link. Callers that make access
   * decisions (the public calendar view) therefore compare this against the signed-in
   * user before trusting the cache.
   */
  const calendarsForUser = ref<string | null>(null);
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

  /**
   * The load generation. `clearCalendars()` bumps it so that any `fetchCalendars` or
   * `fetchCalendar` still in flight when the account is cleared cannot commit its
   * (possibly account-A) response afterwards — even if account A signs straight back
   * in before it settles. A response is only committed when its generation is still
   * the current one and it still belongs to the user that started it.
   */
  let loadGeneration = 0;

  /**
   * The most recent list-load. `fetchCalendars` requests capture a sequence number and
   * only the latest may commit: two overlapping loads for the same account otherwise
   * race last-response-wins, and — worse — an older *failure* would clear a newer
   * successful result, because its catch path sees the same generation as current.
   * This can happen when a dashboard load is still in flight while navigation to a
   * public calendar starts a second ownership-cache load.
   */
  let listRequestSeq = 0;

  /**
   * The most recent public-calendar load. `fetchPublicCalendar` requests capture a
   * sequence number and only the latest may commit, so a slow response for one token
   * cannot overwrite the calendar the visitor is now actually on (the router reuses
   * the same component instance when navigating between public calendar links).
   */
  let publicLoadSeq = 0;

  /**
   * The most recent owner-detail load. `fetchCalendar` requests capture a sequence
   * number and only the latest may commit: CalendarSettings reuses one component when
   * navigating /calendars/A/settings → /calendars/B/settings, and a slow A response
   * must not overwrite the calendar B is now on screen for.
   */
  let detailLoadSeq = 0;

  // Actions
  async function fetchCalendars() {
    return run('calendar.fetchError', async () => {
      // Bind the response to whoever is signed in *now that the request starts*, and
      // to this load generation. `clearCalendars` on logout must invalidate an
      // in-flight request; otherwise an account-A answer could arrive after account B
      // signed in and be labelled as B's cache (and used to lift A's participant lock).
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      const seq = ++listRequestSeq;
      try {
        const result = await calendarsApi.getAll();
        if (generation !== loadGeneration) return;
        if (seq !== listRequestSeq) return;
        // Normalise the same way as `requestingUserId` (absent user -> null), so an
        // anonymous load compares clean and only a genuinely different (or missing)
        // signed-in user between request and response discards the result.
        if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
        calendars.value = Array.isArray(result) ? result : [];
        calendarsForUser.value = requestingUserId;
      } catch (err) {
        // A stale failure must not wipe the state a newer load produced — neither one
        // invalidated by a clear, nor a superseded overlapping request.
        if (generation !== loadGeneration) return;
        if (seq !== listRequestSeq) return;
        calendars.value = []; // Reset to empty array on error
        calendarsForUser.value = null;
        throw err;
      }
    });
  }

  /**
   * Drop the *account-owned* calendar state: the owner list (with full participant
   * ids), the user stamp on it, and the calendar being managed in settings. Also
   * invalidate in-flight requests via the epoch and sequence counters.
   *
   * Deliberately *not* cleared here: `currentPublicCalendar`. A session reset on a
   * public route — another tab signs out, or the local refresh fails — must leave the
   * public payload renderable: there is no navigation to reload it from, and it is
   * not authenticated account data. Ownership privileges disappear with `authStore`
   * (the signed-in user is gone), but the public calendar stays usable.
   */
  function clearCalendars() {
    loadGeneration += 1;
    listRequestSeq += 1;
    calendars.value = [];
    calendarsForUser.value = null;
    currentCalendar.value = null;
  }

  async function fetchCalendar(id: string) {
    return run('calendar.fetchError', async () => {
      // The same account/epoch guard as `fetchCalendars`: the response carries the
      // owner's full settings — participant ids and the public/ICS capability tokens —
      // and must not be resurrected into a cleared store by a request that was started
      // before the account was.
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      const seq = ++detailLoadSeq;
      const calendar = await calendarsApi.getById(id);
      // Only the newest owner-detail load may commit: navigating between settings pages
      // reuses the component, so a slow A response must never override B.
      if (seq !== detailLoadSeq) return;
      if (generation !== loadGeneration) return;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
      currentCalendar.value = calendar;
    });
  }

  async function fetchPublicCalendar(token: string, participantId?: string) {
    return run('calendar.fetchError', async () => {
      const seq = ++publicLoadSeq;
      const result = await calendarsApi.getPublic(token, participantId);
      // Only the newest load may commit: a slow response for one calendar must not
      // overwrite the calendar the visitor is now actually on (router reuses this
      // component instance across /c/<token-a> → /c/<token-b>).
      if (seq !== publicLoadSeq) return;
      currentPublicCalendar.value = result;
    });
  }

  async function createCalendar(data: CreateCalendarRequest) {
    return run('calendar.createError', async () => {
      // A create started under a previous account must not inject a capability-bearing
      // calendar into the current account's trusted list, nor let the old view's
      // success continuation navigate after logout. Commit only while the epoch and
      // initiating user are still current.
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      const calendar = await calendarsApi.create(data);
      if (generation !== loadGeneration) return null;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return null;
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
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      const updated = await calendarsApi.update(id, data);
      // Mutations are account-scoped too: a response from a previous account's request
      // must not rewrite the current user's list or settings object.
      if (generation !== loadGeneration) return updated;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return updated;
      const index = calendars.value.findIndex(c => c.id === id);
      if (index !== -1) {
        // Merge the PATCH response onto the existing entry. The backend now answers
        // with the complete post-write row, but a partial answer must not be able to
        // blank fields it did not touch — participants are the obvious casualty, and
        // a stale partial body must likewise not revert an unrelated field changed by
        // someone else in the meantime. Merge, do not replace.
        const existingParticipants = calendars.value[index].participants || [];
        calendars.value[index] = {
          ...calendars.value[index],
          ...updated,
          participants: existingParticipants,
        };
      }
      if (currentCalendar.value?.id === id) {
        currentCalendar.value = { ...currentCalendar.value, ...updated };
      }
      return updated;
    });
  }

  async function deleteCalendar(id: string) {
    return run('calendar.deleteError', async () => {
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      await calendarsApi.delete(id);
      if (generation !== loadGeneration) return;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
      calendars.value = calendars.value.filter(c => c.id !== id);
      if (currentCalendar.value?.id === id) {
        currentCalendar.value = null;
      }
    });
  }

  async function addParticipant(calendarId: string, data: CreateParticipantRequest) {
    return run('calendar.addParticipantError', async () => {
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      const participant = await calendarsApi.addParticipant(calendarId, data);
      if (generation !== loadGeneration) return participant;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return participant;
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
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      const participant = await calendarsApi.updateParticipant(calendarId, participantId, data);
      if (generation !== loadGeneration) return participant;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return participant;
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
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      await calendarsApi.deleteParticipant(calendarId, participantId);
      if (generation !== loadGeneration) return;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return;
      if (currentCalendar.value?.id === calendarId) {
        currentCalendar.value.participants = currentCalendar.value.participants.filter(
          p => p.id !== participantId
        );
      }
    });
  }

  async function addAnonymousParticipant(token: string, data: CreateParticipantRequest) {
    return run('calendar.addParticipantError', async () => {
      const generation = loadGeneration;
      // Bind the optimistic roster append to the calendar on screen when the request
      // started: a join that settles after the visitor navigated to another public
      // calendar must not push its participant into the *new* roster. Object identity
      // (PublicCalendar does not carry its public token) also drops the append cleanly
      // when the same calendar is reloaded while the create is still in flight.
      const onScreenCalendar = currentPublicCalendar.value;
      const participant = await calendarsApi.addAnonymousParticipant(token, data);
      // Public joins are allowed to settle in the old epoch; only the commit is
      // dropped if the calendar was cleared in the meantime.
      if (generation !== loadGeneration) return participant;
      if (currentPublicCalendar.value === onScreenCalendar && currentPublicCalendar.value) {
        currentPublicCalendar.value.participants.push(participant);
      }
      return participant;
    });
  }

  async function regeneratePublicToken(id: string) {
    return run('calendar.regenerateError', async () => {
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      const { public_token } = await calendarsApi.regeneratePublicToken(id);
      if (generation !== loadGeneration) return public_token;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return public_token;
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
      const requestingUserId = useAuthStore().user?.id ?? null;
      const generation = loadGeneration;
      const { ics_token } = await calendarsApi.regenerateICSToken(id);
      if (generation !== loadGeneration) return ics_token;
      if ((useAuthStore().user?.id ?? null) !== requestingUserId) return ics_token;
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
    /** The user id the `calendars` list belongs to; null while signed out. */
    calendarsForUser,
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
    /** Drops the account-owned calendars and current settings object (used on logout); keeps the current *public* calendar. */
    clearCalendars,
    clearError,
  };
});
