/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * How the participant view is currently being displayed, and the persistence of that
 * choice.
 *
 * Six refs, four watchers and a restore step used to sit inline in `ParticipantView`,
 * interleaved with availability loading and e-mail enrolment. Nothing else in the view
 * reads them except to pass them down, so they belong together: the invariants here are
 * that a week never shows more than four periods, that every change is written back to
 * the per-calendar history entry, and that the two settings which change the *visible
 * date range* — the mode and the number of periods — trigger a reload while the ones
 * that only change the drawing of a week do not.
 *
 * The last point is the reason this is not simply `useCalendarViewState` plus a watcher:
 * the reload is asymmetric, and turning it into a blanket "persist and refetch" would
 * fire a request every time somebody drags the week's start hour.
 */

import { ref, watch, type ComputedRef, type Ref } from 'vue';
import { useCalendarHistoryStore } from '@/stores/calendarHistory';
import { normalizeViewStyle, type DisplayMode, type ViewStyle } from './useCalendarViewState';

/** Upper bound on the number of displayed periods, which differs per mode. */
export function maxPeriodsFor(mode: DisplayMode): number {
  return mode === 'month' ? 12 : 4;
}

/**
 * The default view style.
 *
 * A month grid is unreadable on a phone, so narrow viewports start on the list. Guarded
 * for the absence of `window` because the module is imported by tests that run in plain
 * Node.
 */
export function defaultViewStyle(): ViewStyle {
  if (typeof window === 'undefined') return 'grid';
  return window.innerWidth < 768 ? 'list' : 'grid';
}

/** What `calendarHistory.getDisplaySettings()` hands back — every field optional. */
export interface StoredDisplaySettings {
  displayMode?: DisplayMode;
  periodCount?: number;
  startHour?: number;
  endHour?: number;
  slotDuration?: number;
  viewStyle?: string;
}

export interface ParticipantDisplaySettings {
  readonly displayMode: Ref<DisplayMode>;
  readonly viewStyle: Ref<ViewStyle>;
  readonly numberOfPeriods: Ref<number>;
  readonly startHour: Ref<number>;
  readonly endHour: Ref<number>;
  readonly slotDuration: Ref<number>;
  /** Apply a persisted entry, ignoring fields it does not carry. */
  restore(saved: StoredDisplaySettings | undefined): void;
}

export interface UseParticipantDisplaySettingsOptions {
  /** The public token the settings are persisted under. */
  token: Ref<string> | ComputedRef<string>;
  /**
   * Whether the calendar has loaded. Nothing is persisted before it has: the history
   * entry is created by the load, and writing to a token that is not in it is a no-op
   * that would silently drop the user's choice.
   */
  isReady: () => boolean;
  /** Refetch the visible range. Called only for changes that move that range. */
  onRangeChange: () => Promise<void>;
}

export function useParticipantDisplaySettings(
  options: UseParticipantDisplaySettingsOptions
): ParticipantDisplaySettings {
  const { token, isReady, onRangeChange } = options;
  const historyStore = useCalendarHistoryStore();

  const displayMode = ref<DisplayMode>('month');
  const viewStyle = ref<ViewStyle>(defaultViewStyle());
  const numberOfPeriods = ref(1);
  const startHour = ref(8);
  const endHour = ref(20);
  const slotDuration = ref(30);

  function restore(saved: StoredDisplaySettings | undefined): void {
    if (!saved) return;
    if (saved.displayMode !== undefined) displayMode.value = saved.displayMode;
    if (saved.periodCount !== undefined) numberOfPeriods.value = saved.periodCount;
    if (saved.startHour !== undefined) startHour.value = saved.startHour;
    if (saved.endHour !== undefined) endHour.value = saved.endHour;
    if (saved.slotDuration !== undefined) slotDuration.value = saved.slotDuration;
    if (saved.viewStyle !== undefined) {
      // Older entries hold 'classic' or 'compact'; both mean the grid now.
      viewStyle.value = normalizeViewStyle(saved.viewStyle);
    }
  }

  watch(displayMode, async newMode => {
    // Clamp before anything else: twelve weeks is not an option, and a stored month
    // count carried into week mode would otherwise request three months of data.
    const maxPeriods = maxPeriodsFor(newMode);
    if (numberOfPeriods.value > maxPeriods) {
      numberOfPeriods.value = maxPeriods;
    }

    if (isReady()) {
      historyStore.updateDisplaySettings(token.value, { displayMode: newMode });
      await onRangeChange();
    }
  });

  watch(numberOfPeriods, async newCount => {
    if (isReady()) {
      historyStore.updateDisplaySettings(token.value, { periodCount: newCount });
      await onRangeChange();
    }
  });

  // The week grid used to emit these and the parent persisted them; now that the parent
  // owns the values it has to save them itself, or they reset on every visit.
  watch([startHour, endHour, slotDuration], ([newStart, newEnd, newDuration]) => {
    if (token.value) {
      historyStore.updateDisplaySettings(token.value, {
        startHour: newStart,
        endHour: newEnd,
        slotDuration: newDuration,
      });
    }
  });

  watch(viewStyle, newStyle => {
    if (isReady()) {
      historyStore.updateDisplaySettings(token.value, { viewStyle: newStyle });
    }
  });

  return {
    displayMode,
    viewStyle,
    numberOfPeriods,
    startHour,
    endHour,
    slotDuration,
    restore,
  };
}
