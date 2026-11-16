/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { onScopeDispose, ref, watch } from 'vue';
import { useAuthStore } from '@/stores/auth';
import type { DashboardSortDirection, DashboardSortMode } from '@/utils/dashboardOrdering';

/**
 * How the dashboard calendar list is ordered, viewed and rendered, per account.
 *
 * Every preference — sort mode/direction, the manual drag order, the pins and the
 * display view — lives under one per-account key (`dashboard:${userId}:prefs`),
 * never under one global key. A signed-out visitor gets their own `dashboard:anon:prefs`
 * slot, and switching accounts on a shared browser reloads that account's own stored
 * choices; nothing account A saves can bleed into account B, and a signed-out visitor
 * can never inherit a signed-in account's pins or order. Only preference metadata is
 * stored (ids, sort/view choices) — never calendar contents or tokens.
 *
 * Everything read back is defensive: a malformed value, a truncated blob or a denied
 * localStorage is treated as absent and the defaults are used, so the dashboard can
 * never crash on a foreign or corrupt value.
 */

export type DashboardViewMode = 'card' | 'list' | 'expanded' | 'compact';

const STORAGE_KEY_PREFIX = 'dashboard';
const PREFS_SUFFIX = 'prefs';

/** The per-account storage key; the signed-out session has its own anon slot. */
function prefsKey(userId: string | null): string {
  return `${STORAGE_KEY_PREFIX}:${userId ?? 'anon'}:${PREFS_SUFFIX}`;
}

interface StoredDashboardPrefs {
  sortMode?: unknown;
  sortDirection?: unknown;
  pinnedIds?: unknown;
  customOrder?: unknown;
  viewMode?: unknown;
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
  return [...new Set(ids.filter((id): id is string => typeof id === 'string' && id !== ''))];
}

function loadPrefs(prefsKey: string): StoredDashboardPrefs | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = localStorage.getItem(prefsKey);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    return typeof parsed === 'object' && parsed !== null ? (parsed as StoredDashboardPrefs) : {};
  } catch {
    // Malformed JSON or a denied storage read; treat as absent.
    return null;
  }
}

function savePrefs(prefs: StoredDashboardPrefs, prefsKey: string): void {
  try {
    localStorage.setItem(prefsKey, JSON.stringify(prefs));
  } catch {
    // Storage can be unavailable (private mode, quota); the preference simply does
    // not survive a reload, which is a graceful degradation.
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
   * Which account owns the refs. Account switching is synchronous so a mutator
   * called in the same tick always updates the incoming account, never the old one.
   */
  let preferencesForUser: string | null = authStore.user?.id ?? null;

  function applyPrefs(stored: StoredDashboardPrefs) {
    sortMode.value = isSortMode(stored.sortMode) ? stored.sortMode : 'alphabetical';
    sortDirection.value = isSortDirection(stored.sortDirection) ? stored.sortDirection : 'asc';
    pinnedIds.value = normalizeIds(isStringArray(stored.pinnedIds) ? stored.pinnedIds : []);
    customOrder.value = normalizeIds(isStringArray(stored.customOrder) ? stored.customOrder : []);
    viewMode.value = isViewMode(stored.viewMode) ? stored.viewMode : 'card';
  }

  /** Reload without persisting: switching accounts or tabs must never write stale data. */
  function applyUserPrefs(userId: string | null) {
    applyPrefs(loadPrefs(prefsKey(userId)) ?? {});
  }

  applyUserPrefs(preferencesForUser);

  /**
   * Persist the current preferences to the identity they belong to.
   *
   * Called from every mutator, synchronously, rather than from a watch on the refs:
   * a flush-time watcher re-fires after `applyUserPrefs` resets the refs during an
   * account switch, persisting the incoming account's freshly-loaded (untouched)
   * defaults into its key. Writing at mutation time ties each write to the identity
   * the change belongs to, so a switch that lands in the same tick cannot reroute
   * A's change into B's key nor materialise B's defaults for it.
   */
  function persistPrefs() {
    savePrefs(
      {
        sortMode: sortMode.value,
        sortDirection: sortDirection.value,
        pinnedIds: pinnedIds.value,
        customOrder: customOrder.value,
        viewMode: viewMode.value,
      },
      prefsKey(preferencesForUser)
    );
  }

  // Mutators already persist. Saving on account switch/sign-out would overwrite
  // newer changes from another tab. Retain stored metadata, but reset the visible
  // account's state immediately, without writing either account's key.
  watch(
    () => authStore.user?.id ?? null,
    (newId, oldId) => {
      if (newId === oldId) return;
      preferencesForUser = newId;
      applyUserPrefs(preferencesForUser);
    },
    { flush: 'sync' }
  );

  // Re-read immediately before changing a field, even if a storage event has not
  // arrived yet. Toggle/append/prune operations must also use the latest arrays.
  // If storage is missing or denied, keep usable in-memory preferences; a real
  // cross-tab removal/clear event explicitly resets them below.
  function mutatePrefs(change: () => void) {
    const stored = loadPrefs(prefsKey(preferencesForUser));
    if (stored !== null) applyPrefs(stored);
    change();
    persistPrefs();
  }

  function onStorage(event: StorageEvent) {
    try {
      if (event.storageArea !== window.localStorage) return;
    } catch {
      return;
    }
    if (event.key !== null && event.key !== prefsKey(preferencesForUser)) return;
    // Read current storage, not event.newValue: delayed events can describe an
    // older write than a subsequent write already made by this tab.
    applyUserPrefs(preferencesForUser);
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('storage', onStorage);
    onScopeDispose(() => window.removeEventListener('storage', onStorage));
  }

  function setSortMode(mode: DashboardSortMode) {
    mutatePrefs(() => {
      sortMode.value = mode;
    });
  }

  function setSortDirection(direction: DashboardSortDirection) {
    mutatePrefs(() => {
      sortDirection.value = direction;
    });
  }

  function togglePin(id: string) {
    mutatePrefs(() => {
      pinnedIds.value = pinnedIds.value.includes(id)
        ? pinnedIds.value.filter(pinned => pinned !== id)
        : normalizeIds([...pinnedIds.value, id]);
    });
  }

  function setCustomOrder(ids: readonly string[]) {
    mutatePrefs(() => {
      customOrder.value = normalizeIds(ids);
    });
  }

  /** Appends calendars that were added elsewhere (for example after a create). */
  function appendToCustomOrder(ids: readonly string[]) {
    mutatePrefs(() => {
      customOrder.value = normalizeIds([...customOrder.value, ...ids]);
    });
  }

  /** Drops ids of calendars that no longer exist, so the sequence stays clean. */
  function pruneCustomOrder(ids: readonly string[]) {
    const alive = new Set(ids);
    mutatePrefs(() => {
      customOrder.value = normalizeIds(customOrder.value.filter(id => alive.has(id)));
    });
  }

  /** Drops pins of calendars that no longer exist. */
  function prunePinned(ids: readonly string[]) {
    const alive = new Set(ids);
    mutatePrefs(() => {
      pinnedIds.value = normalizeIds(pinnedIds.value.filter(id => alive.has(id)));
    });
  }

  function setViewMode(mode: DashboardViewMode) {
    mutatePrefs(() => {
      viewMode.value = mode;
    });
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
    setCustomOrder,
    appendToCustomOrder,
    pruneCustomOrder,
    prunePinned,
  };
});
