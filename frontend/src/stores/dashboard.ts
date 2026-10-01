/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref, watch } from 'vue';
import { useAuthStore } from '@/stores/auth';
import type { DashboardSortDirection, DashboardSortMode } from '@/utils/dashboardOrdering';

/**
 * How the dashboard calendar list is rendered, per browser/user.
 *
 * The ID-bearing preferences — pins and the custom drag order — are stored per
 * account (`whento_dashboard_prefs:<userId>`), so signing out and back in (or signing
 * in as a different account on a shared browser) cannot prune or cross-pollinate one
 * account's saved sequence with another's. The view mode is a browser-level display
 * preference and stays global (`whento_dashboard_prefs_view`). Everything stored is
 * defensive: a malformed value is discarded and the default used instead, and ids
 * that no longer exist are simply ignored by the ordering code rather than persisting
 * forever.
 */

export type DashboardViewMode = 'card' | 'list' | 'expanded' | 'compact';

const STORAGE_KEY = 'whento_dashboard_prefs';
const VIEW_KEY = 'whento_dashboard_prefs_view';

interface StoredDashboardPrefs {
  sortMode?: unknown;
  sortDirection?: unknown;
  pinnedIds?: unknown;
  customOrder?: unknown;
}

function isSortMode(value: unknown): value is DashboardSortMode {
  return value === 'alphabetical' || value === 'custom';
}

function isSortDirection(value: unknown): value is DashboardSortDirection {
  return value === 'asc' || value === 'desc';
}

function isViewMode(value: unknown): value is DashboardViewMode {
  return value === 'card' || value === 'list' || value === 'expanded' || value === 'compact';
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every(entry => typeof entry === 'string');
}

/**
 * Normalise an ID list from storage or a caller: keep non-empty strings and drop
 * duplicates, preserving first-occurrence order. A persisted `["a", "a"]` must render
 * one card, not two (Vue keys would clash), and empty ids are meaningless, so both are
 * discarded here rather than surviving into the ordering and the rendered cards.
 */
function normalizeIds(ids: readonly unknown[]): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const id of ids) {
    if (typeof id !== 'string' || id === '') continue;
    if (seen.has(id)) continue;
    seen.add(id);
    result.push(id);
  }
  return result;
}

function loadUser(prefsKey: string): StoredDashboardPrefs {
  if (typeof window === 'undefined') return {};
  try {
    const raw = localStorage.getItem(prefsKey);
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    return typeof parsed === 'object' && parsed !== null ? (parsed as StoredDashboardPrefs) : {};
  } catch {
    return {};
  }
}

function userKey(userId: string | null): string {
  return userId ? `${STORAGE_KEY}:${userId}` : `${STORAGE_KEY}:anon`;
}

function saveUser(prefs: StoredDashboardPrefs, userId: string | null): void {
  try {
    localStorage.setItem(userKey(userId), JSON.stringify(prefs));
  } catch {
    // Storage can be unavailable (private mode, quota); the preference simply does
    // not survive a reload, which is a graceful degradation.
  }
}

function loadView(): unknown {
  if (typeof window === 'undefined') return undefined;
  try {
    const raw = localStorage.getItem(VIEW_KEY);
    return raw ? JSON.parse(raw) : undefined;
  } catch {
    return undefined;
  }
}

function saveView(mode: DashboardViewMode): void {
  try {
    localStorage.setItem(VIEW_KEY, JSON.stringify(mode));
  } catch {
    // Graceful degradation as above.
  }
}

export const useDashboardStore = defineStore('dashboard', () => {
  const authStore = useAuthStore();

  const sortMode = ref<DashboardSortMode>('alphabetical');
  const sortDirection = ref<DashboardSortDirection>('asc');
  const pinnedIds = ref<string[]>([]);
  const customOrder = ref<string[]>([]);
  const viewMode = ref<DashboardViewMode>('card');

  /**
   * Which account's preferences currently live in the refs. The persistence watcher
   * writes to this identity, captured when a change happens, rather than to
   * `authStore.user.id` at flush time: without it, an account change that lands in
   * the same tick would make A's in-memory value get saved into B's key.
   */
  let preferencesForUser: string | null = authStore.user?.id ?? null;

  /** Load the stored ID-bearing preferences for the given account into the store. */
  function applyUserPrefs(userId: string | null) {
    const stored = loadUser(userKey(userId));
    sortMode.value = isSortMode(stored.sortMode) ? stored.sortMode : 'alphabetical';
    sortDirection.value = isSortDirection(stored.sortDirection) ? stored.sortDirection : 'asc';
    pinnedIds.value = normalizeIds(isStringArray(stored.pinnedIds) ? stored.pinnedIds : []);
    customOrder.value = normalizeIds(isStringArray(stored.customOrder) ? stored.customOrder : []);
  }

  const initialView = loadView();
  viewMode.value = isViewMode(initialView) ? initialView : 'card';
  applyUserPrefs(preferencesForUser);

  /**
   * Persist the current ID-bearing preferences to the identity they belong to.
   *
   * Called from every mutator, synchronously, rather than from a watch on the refs:
   * a flush-time watcher re-fires after `applyUserPrefs` resets the refs during an
   * account switch, persisting the incoming account's freshly-loaded (untouched)
   * defaults into its key. Writing at mutation time ties each write to the identity
   * the change belongs to, so a switch that lands in the same tick cannot reroute
   * A's change into B's key nor materialise B's defaults for it.
   */
  function persistPrefs() {
    saveUser(
      {
        sortMode: sortMode.value,
        sortDirection: sortDirection.value,
        pinnedIds: pinnedIds.value,
        customOrder: customOrder.value,
      },
      preferencesForUser
    );
  }

  watch(viewMode, () => saveView(viewMode.value));

  // Account switch: save the outgoing account's in-memory prefs to ITS OWN key first
  // (the mutators already do this, but doing it here makes it atomic with the switch),
  // then reload the incoming account's stored preferences. The view mode is a
  // browser-level display preference and intentionally stays put.
  watch(
    () => authStore.user?.id ?? null,
    (newId, oldId) => {
      if (newId === oldId) return;
      saveUser(
        {
          sortMode: sortMode.value,
          sortDirection: sortDirection.value,
          pinnedIds: pinnedIds.value,
          customOrder: customOrder.value,
        },
        // Capture the outgoing identity explicitly: `newId` has already been written,
        // and `preferencesForUser` is what the in-memory values belong to.
        oldId
      );
      preferencesForUser = newId;
      applyUserPrefs(preferencesForUser);
    }
  );

  function setSortMode(mode: DashboardSortMode) {
    sortMode.value = mode;
    persistPrefs();
  }

  function setSortDirection(direction: DashboardSortDirection) {
    sortDirection.value = direction;
    persistPrefs();
  }

  function togglePin(id: string) {
    pinnedIds.value = pinnedIds.value.includes(id)
      ? pinnedIds.value.filter(pinned => pinned !== id)
      : [...pinnedIds.value, id];
    persistPrefs();
  }

  function isPinned(id: string): boolean {
    return pinnedIds.value.includes(id);
  }

  function setCustomOrder(ids: readonly string[]) {
    customOrder.value = normalizeIds(ids);
    persistPrefs();
  }

  /** Appends calendars that were added elsewhere (for example after a create). */
  function appendToCustomOrder(ids: readonly string[]) {
    customOrder.value = normalizeIds([...customOrder.value, ...ids]);
    persistPrefs();
  }

  /** Drops ids of calendars that no longer exist, so the sequence stays clean. */
  function pruneCustomOrder(ids: readonly string[]) {
    const alive = new Set(ids);
    customOrder.value = normalizeIds(customOrder.value.filter(id => alive.has(id)));
    persistPrefs();
  }

  /** Drops pins of calendars that no longer exist. */
  function prunePinned(ids: readonly string[]) {
    const alive = new Set(ids);
    pinnedIds.value = normalizeIds(pinnedIds.value.filter(id => alive.has(id)));
    persistPrefs();
  }

  function setViewMode(mode: DashboardViewMode) {
    viewMode.value = mode;
  }

  return {
    sortMode,
    sortDirection,
    pinnedIds,
    customOrder,
    viewMode,
    setSortMode,
    setSortDirection,
    setViewMode,
    togglePin,
    isPinned,
    setCustomOrder,
    appendToCustomOrder,
    pruneCustomOrder,
    prunePinned,
  };
});
