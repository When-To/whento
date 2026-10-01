/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * How "My calendars" is ordered on the dashboard.
 *
 * Kept as a pure module so the ordering rule has exactly one home and the tests can
 * pin it down without a component or a store in the way. The rule:
 *
 *   - pinned calendars always form a group at the top;
 *   - each calendar is either in `pinnedIds` (pinned) or not (the rest);
 *   - within each group the calendars are sorted by the active mode —
 *     alphabetically (ascending or descending by name), or by `customOrder`,
 *     the drag-and-drop sequence;
 *   - calendars `customOrder` does not know about keep their relative order at
 *     the end of the unpinned group, so a freshly created calendar never vanishes
 *     just because the user has not dragged it yet.
 */

export type DashboardSortMode = 'alphabetical' | 'custom';
export type DashboardSortDirection = 'asc' | 'desc';

export interface DashboardOrderingPrefs {
  sortMode: DashboardSortMode;
  sortDirection: DashboardSortDirection;
  /** The calendars at the top, in pinned order (top-most first). */
  readonly pinnedIds: readonly string[];
  /** The user's drag-and-drop sequence; ids not listed here sort last, stable. */
  readonly customOrder: readonly string[];
}

export interface DashboardViewableCard {
  readonly id: string;
  readonly name: string;
}

function nameComparator(
  direction: DashboardSortDirection
): (a: DashboardViewableCard, b: DashboardViewableCard) => number {
  return (a, b) => {
    const result = a.name.localeCompare(b.name);
    return direction === 'asc' ? result : -result;
  };
}

function customComparator(
  order: readonly string[]
): (a: DashboardViewableCard, b: DashboardViewableCard) => number {
  const index = new Map(order.map((id, i) => [id, i]));
  return (a, b) => {
    const ai = index.get(a.id);
    const bi = index.get(b.id);
    if (ai === undefined && bi === undefined) return 0;
    // Unknown calendars sort after every known one, so a not-yet-dragged calendar
    // lands at the end of its group instead of jumping around.
    if (ai === undefined) return 1;
    if (bi === undefined) return -1;
    return ai - bi;
  };
}

/**
 * Puts the calendars in dashboard display order: pinned first, then everyone else,
 * each group ordered by the active mode.
 */
export function orderCalendars<T extends DashboardViewableCard>(
  calendars: readonly T[],
  prefs: DashboardOrderingPrefs
): T[] {
  const available = calendars.filter((c): c is T => c != null && typeof c.id === 'string');
  const byId = new Map(available.map(c => [c.id, c]));

  // Drop pinned ids that no longer exist so they cannot sort to the front forever
  // after a calendar has been deleted.
  const presentPinned = prefs.pinnedIds.filter(id => byId.has(id));
  const pinnedSet = new Set(presentPinned);

  const pinned = presentPinned.map(id => byId.get(id)!);
  const unpinned = available.filter(c => !pinnedSet.has(c.id));

  const compare =
    prefs.sortMode === 'custom'
      ? customComparator(prefs.customOrder)
      : nameComparator(prefs.sortDirection);

  return [...pinned].sort(compare).concat([...unpinned].sort(compare));
}

/** The visible id order, for persisting the drag sequence. */
export function orderedIds<T extends DashboardViewableCard>(
  calendars: readonly T[],
  prefs: DashboardOrderingPrefs
): string[] {
  return orderCalendars(calendars, prefs).map(c => c.id);
}
