/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Single owner of how the calendar is displayed.
 *
 * The state used to live in three places at once: refs on the participant view, a
 * `viewMode` ref inside the month grid backed by its own global localStorage key, and a
 * `window` CustomEvent broadcasting changes to every mounted grid. Toggling the view
 * with twelve months on screen produced around 144 handler invocations and twelve
 * localStorage writes, each of which re-triggered the watcher that had dispatched it.
 */

import { computed, ref, type ComputedRef, type Ref } from 'vue';

export type DisplayMode = 'month' | 'week';

/**
 * `grid` covers what used to be `classic` and `compact`: the month grid transposes
 * itself in CSS below 768px, so there is no longer a mode to choose between them.
 */
export type ViewStyle = 'grid' | 'list';

/** What the calendar history store persists, per calendar. */
export interface DisplaySettings {
  displayMode: DisplayMode;
  periodCount: number;
  startHour: number;
  endHour: number;
  slotDuration: number;
  viewStyle: ViewStyle;
}

export interface CalendarViewState {
  readonly displayMode: Ref<DisplayMode>;
  readonly viewStyle: Ref<ViewStyle>;
  readonly periodCount: Ref<number>;
  readonly startHour: Ref<number>;
  readonly endHour: Ref<number>;
  readonly slotDuration: Ref<number>;
  /** Upper bound on `periodCount`, which differs between months and weeks. */
  readonly maxPeriods: ComputedRef<number>;
  /** Apply persisted settings, tolerating values written by older versions. */
  hydrate(settings: Record<string, unknown>): void;
  toSettings(): DisplaySettings;
}

const MAX_MONTHS = 12;
const MAX_WEEKS = 4;

/**
 * Normalize a persisted view style.
 *
 * Old entries hold `classic` or `compact`; both become `grid`. The storage key and
 * every field name are unchanged, so downgrading is not a data loss either.
 */
export function normalizeViewStyle(value: string | undefined): ViewStyle {
  return value === 'list' ? 'list' : 'grid';
}

export function useCalendarViewState(defaults: Partial<DisplaySettings> = {}): CalendarViewState {
  const displayMode = ref<DisplayMode>(defaults.displayMode ?? 'month');
  const viewStyle = ref<ViewStyle>(defaults.viewStyle ?? 'grid');
  const periodCount = ref(defaults.periodCount ?? 1);
  const startHour = ref(defaults.startHour ?? 8);
  const endHour = ref(defaults.endHour ?? 20);
  const slotDuration = ref(defaults.slotDuration ?? 15);

  const maxPeriods = computed(() => (displayMode.value === 'week' ? MAX_WEEKS : MAX_MONTHS));

  function hydrate(settings: Record<string, unknown>): void {
    if (settings.displayMode === 'month' || settings.displayMode === 'week') {
      displayMode.value = settings.displayMode;
    }
    if (typeof settings.viewStyle === 'string') {
      viewStyle.value = normalizeViewStyle(settings.viewStyle);
    }
    if (typeof settings.periodCount === 'number') {
      periodCount.value = Math.max(1, Math.min(settings.periodCount, maxPeriods.value));
    }
    if (typeof settings.startHour === 'number') startHour.value = settings.startHour;
    if (typeof settings.endHour === 'number') endHour.value = settings.endHour;
    if (typeof settings.slotDuration === 'number') slotDuration.value = settings.slotDuration;
  }

  function toSettings(): DisplaySettings {
    return {
      displayMode: displayMode.value,
      periodCount: periodCount.value,
      startHour: startHour.value,
      endHour: endHour.value,
      slotDuration: slotDuration.value,
      viewStyle: viewStyle.value,
    };
  }

  return {
    displayMode,
    viewStyle,
    periodCount,
    startHour,
    endHour,
    slotDuration,
    maxPeriods,
    hydrate,
    toSettings,
  };
}
