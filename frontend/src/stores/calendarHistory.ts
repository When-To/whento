/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref, computed } from 'vue';

export interface CalendarHistoryItem {
  token: string;
  name: string;
  lastVisited: number;
  monthCount?: number; // Deprecated, use periodCount instead
  participantId?: string;
  // Display settings
  displayMode?: 'month' | 'week';
  periodCount?: number; // Number of periods (months or weeks)
  startHour?: number;
  endHour?: number;
  slotDuration?: number;
  /**
   * `grid` replaced `classic`/`compact` when the month grid started transposing itself
   * in CSS. Both older values are still readable and normalize to `grid` on load.
   */
  viewStyle?: 'grid' | 'list' | 'classic' | 'compact';
}

const STORAGE_KEY = 'whento_visited_calendars';
const MAX_HISTORY_ITEMS = 10;

/**
 * Get all visited calendars from localStorage
 */
function getVisitedCalendars(): CalendarHistoryItem[] {
  if (typeof window === 'undefined') return [];

  try {
    const data = localStorage.getItem(STORAGE_KEY);
    if (!data) return [];

    // JSON.parse succeeding says nothing about the shape. A stored `42` or `{}` parses
    // fine and then breaks the store on first access, because `sortedCalendars` spreads
    // the value and `[...42]` throws — and `init()` runs when the store is created, so
    // the failure takes the page down rather than degrading. Anything that is not an
    // array of entries carrying a token is discarded.
    const parsed: unknown = JSON.parse(data);
    if (!Array.isArray(parsed)) return [];

    return parsed.filter(
      (entry): entry is CalendarHistoryItem =>
        typeof entry === 'object' && entry !== null && typeof entry.token === 'string'
    );
  } catch {
    return [];
  }
}

/**
 * Save visited calendars to localStorage
 */
function saveVisitedCalendars(calendars: CalendarHistoryItem[]): void {
  if (typeof window === 'undefined') return;

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(calendars));
  } catch (error) {
    console.error('[CalendarHistory] Failed to save to localStorage:', error);
  }
}

export const useCalendarHistoryStore = defineStore('calendarHistory', () => {
  const calendars = ref<CalendarHistoryItem[]>([]);
  const isOpen = ref(false);
  const initialized = ref(false);

  const sortedCalendars = computed(() => {
    return [...calendars.value].sort((a, b) => b.lastVisited - a.lastVisited);
  });

  function init() {
    if (initialized.value) return;

    calendars.value = getVisitedCalendars();
    initialized.value = true;
  }

  // Auto-initialize when store is first accessed
  init();

  function addCalendar(token: string, name: string, participantId?: string) {
    // Find existing entry to preserve all settings
    const existing = calendars.value.find(c => c.token === token);

    // Remove existing entry with same token
    const filtered = calendars.value.filter(c => c.token !== token);

    // Add new entry at the beginning, preserving all existing settings
    const updated = [
      {
        token,
        name,
        lastVisited: Date.now(),
        // Preserve all display settings from existing entry
        ...(existing?.displayMode !== undefined && { displayMode: existing.displayMode }),
        ...(existing?.periodCount !== undefined && { periodCount: existing.periodCount }),
        ...(existing?.monthCount !== undefined && { monthCount: existing.monthCount }),
        ...(existing?.startHour !== undefined && { startHour: existing.startHour }),
        ...(existing?.endHour !== undefined && { endHour: existing.endHour }),
        ...(existing?.slotDuration !== undefined && { slotDuration: existing.slotDuration }),
        ...(existing?.viewStyle !== undefined && { viewStyle: existing.viewStyle }),
        // Update participantId if provided, otherwise preserve existing
        ...(participantId !== undefined
          ? { participantId }
          : existing?.participantId !== undefined && { participantId: existing.participantId }),
      },
      ...filtered,
    ];

    // Keep only MAX_HISTORY_ITEMS most recent
    calendars.value = updated.slice(0, MAX_HISTORY_ITEMS);

    // Save to localStorage
    saveVisitedCalendars(calendars.value);
  }

  function removeCalendar(token: string) {
    calendars.value = calendars.value.filter(c => c.token !== token);
    saveVisitedCalendars(calendars.value);
  }

  function clearHistory() {
    calendars.value = [];
    if (typeof window !== 'undefined') {
      localStorage.removeItem(STORAGE_KEY);
    }
  }

  function toggle() {
    isOpen.value = !isOpen.value;
  }

  function close() {
    isOpen.value = false;
  }

  function updateMonthCount(token: string, monthCount: number) {
    const calendar = calendars.value.find(c => c.token === token);
    if (calendar) {
      calendar.monthCount = monthCount;
      saveVisitedCalendars(calendars.value);
    }
  }

  function getMonthCount(token: string): number | undefined {
    const calendar = calendars.value.find(c => c.token === token);
    return calendar?.monthCount;
  }

  function updateParticipantId(token: string, participantId: string | undefined) {
    const calendar = calendars.value.find(c => c.token === token);
    if (calendar) {
      calendar.participantId = participantId;
      saveVisitedCalendars(calendars.value);
    }
  }

  function getParticipantId(token: string): string | undefined {
    const calendar = calendars.value.find(c => c.token === token);
    return calendar?.participantId;
  }

  // Display settings management
  function updateDisplaySettings(
    token: string,
    settings: {
      displayMode?: 'month' | 'week';
      periodCount?: number;
      startHour?: number;
      endHour?: number;
      slotDuration?: number;
      viewStyle?: 'grid' | 'list' | 'classic' | 'compact';
    }
  ) {
    const calendar = calendars.value.find(c => c.token === token);
    if (calendar) {
      if (settings.displayMode !== undefined) calendar.displayMode = settings.displayMode;
      if (settings.periodCount !== undefined) {
        calendar.periodCount = settings.periodCount;
        // Keep legacy monthCount for backward compatibility
        calendar.monthCount = settings.periodCount;
      }
      if (settings.startHour !== undefined) calendar.startHour = settings.startHour;
      if (settings.endHour !== undefined) calendar.endHour = settings.endHour;
      if (settings.slotDuration !== undefined) calendar.slotDuration = settings.slotDuration;
      if (settings.viewStyle !== undefined) calendar.viewStyle = settings.viewStyle;
      saveVisitedCalendars(calendars.value);
    }
  }

  function getDisplaySettings(token: string) {
    const calendar = calendars.value.find(c => c.token === token);
    if (!calendar) return undefined;

    return {
      displayMode: calendar.displayMode,
      periodCount: calendar.periodCount ?? calendar.monthCount, // Fallback to legacy monthCount
      startHour: calendar.startHour,
      endHour: calendar.endHour,
      slotDuration: calendar.slotDuration,
      viewStyle: calendar.viewStyle,
    };
  }

  return {
    calendars: sortedCalendars,
    isOpen,
    init,
    addCalendar,
    removeCalendar,
    clearHistory,
    toggle,
    close,
    updateMonthCount,
    getMonthCount,
    updateParticipantId,
    getParticipantId,
    updateDisplaySettings,
    getDisplaySettings,
  };
});
