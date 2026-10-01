<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="min-h-[calc(100vh-4rem)] bg-gray-50 py-8 dark:bg-gray-950">
    <div class="container-app">
      <!-- Header -->
      <div class="mb-8 flex items-center justify-between">
        <div>
          <h1 class="font-display text-3xl font-bold text-gray-900 dark:text-white">
            {{ t('calendar.myCalendars') }}
          </h1>
          <p class="mt-1 text-gray-600 dark:text-gray-400">
            {{ t('common.welcome', { name: user?.display_name }) }}
          </p>
        </div>
        <router-link to="/calendars/new" class="btn btn-primary">
          <svg class="mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 4v16m8-8H4"
            />
          </svg>
          {{ t('calendar.newCalendar') }}
        </router-link>
      </div>

      <!-- Quota Usage -->
      <div class="mb-6">
        <QuotaUsage />
      </div>

      <!-- Unified ICS Feed Section -->
      <div v-if="!loading && calendars.length > 0" class="mb-6">
        <div
          class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-900"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center">
              <svg
                class="mr-2 h-5 w-5 text-primary-600 dark:text-primary-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"
                />
              </svg>
              <div>
                <h3 class="font-display text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('unifiedFeed.title') }}
                </h3>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('unifiedFeed.description') }}
                </p>
              </div>
            </div>

            <!-- Enable button (if not configured) -->
            <button
              v-if="!unifiedFeedConfig?.configured"
              class="btn btn-primary btn-sm"
              :disabled="unifiedFeedLoading"
              @click="enableUnifiedFeed"
            >
              {{ t('unifiedFeed.enable') }}
            </button>
          </div>

          <!-- The store surfaces the translated failure here; it must not be lost. -->
          <p
            v-if="unifiedFeedStore.error"
            role="alert"
            class="mt-2 text-sm font-medium text-danger-600 dark:text-danger-400"
          >
            {{ unifiedFeedStore.error }}
          </p>

          <!-- Feed URL + Actions (if configured) -->
          <div v-if="unifiedFeedConfig?.configured" class="mt-3">
            <div class="flex items-center space-x-2">
              <input
                type="text"
                readonly
                :value="unifiedFeedUrl"
                :aria-label="t('a11y.unifiedFeedUrl')"
                class="input flex-1 text-xs"
                @click="($event.target as HTMLInputElement)?.select()"
              />
              <button
                class="btn btn-ghost btn-sm"
                :title="t('unifiedFeed.copyUrl')"
                @click="copyUnifiedFeedUrl"
              >
                <svg class="mr-1 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                  />
                </svg>
                {{ t('unifiedFeed.copyUrl') }}
              </button>
              <button
                class="btn btn-ghost btn-sm text-danger-600 dark:text-danger-400"
                :title="t('unifiedFeed.regenerateToken')"
                :disabled="unifiedFeedLoading"
                @click="regenerateUnifiedFeedToken"
              >
                <svg class="mr-1 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                  />
                </svg>
                {{ t('unifiedFeed.regenerateToken') }}
              </button>
            </div>
            <p
              v-if="unifiedFeedConfig.included_calendar_ids?.length === 0"
              class="mt-2 text-xs text-amber-600 dark:text-amber-400"
            >
              {{ t('unifiedFeed.noCalendarsSelected') }}
            </p>
          </div>
        </div>
      </div>

      <!-- Sort + view toolbar -->
      <div
        v-if="!loading && calendars.length > 0"
        class="mb-3 flex flex-wrap items-center gap-x-4 gap-y-3 border-b border-gray-200 pb-3 dark:border-gray-700"
      >
        <label
          for="dashboard-sort"
          class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
        >
          {{ t('dashboard.sortBy') }}
          <select id="dashboard-sort" v-model="sortKey" class="input w-44 py-1.5">
            <option value="name-asc">{{ t('dashboard.sortNameAsc') }}</option>
            <option value="name-desc">{{ t('dashboard.sortNameDesc') }}</option>
            <option value="custom">{{ t('dashboard.sortCustom') }}</option>
          </select>
        </label>

        <div class="flex items-center gap-2">
          <span class="text-sm text-gray-700 dark:text-gray-300">
            {{ t('dashboard.viewAs') }}
          </span>
          <div class="flex items-center gap-1" role="group" :aria-label="t('dashboard.viewAs')">
            <button
              v-for="mode in viewModes"
              :key="mode"
              type="button"
              class="card-action"
              :class="{ 'card-action-on': viewMode === mode }"
              :aria-label="t(viewModeAriaLabel(mode))"
              :aria-pressed="viewMode === mode"
              @click="setViewMode(mode)"
            >
              <svg
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  v-if="mode === 'card'"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zm10 0a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zm10 0a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"
                />
                <path
                  v-else-if="mode === 'list'"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 6h16M4 10h16M4 14h16M4 18h16"
                />
                <path
                  v-else-if="mode === 'expanded'"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 6h16M4 10h10M4 14h16M4 18h10"
                />
                <path
                  v-else
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 6h16M4 12h16M4 18h8"
                />
              </svg>
            </button>
          </div>
        </div>

        <p v-if="sortKey === 'custom'" class="w-full text-xs text-gray-500 dark:text-gray-400">
          {{ t('dashboard.reorderHint') }}
        </p>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <svg class="h-8 w-8 animate-spin text-primary-600" fill="none" viewBox="0 0 24 24">
          <circle
            class="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            stroke-width="4"
          />
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
      </div>

      <!-- Error State -->
      <div v-else-if="fetchError" class="flex flex-col items-center justify-center py-12">
        <div
          class="mb-6 flex h-24 w-24 items-center justify-center rounded-full bg-danger-100 dark:bg-danger-900"
        >
          <svg
            class="h-12 w-12 text-danger-600 dark:text-danger-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
        </div>
        <h2 class="mb-2 font-display text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('errors.generic') }}
        </h2>
        <p class="mb-6 text-gray-600 dark:text-gray-400">
          {{ fetchError }}
        </p>
        <button class="btn btn-primary" @click="loadCalendars()">
          {{ t('common.retry') }}
        </button>
      </div>

      <!-- Empty State -->
      <div
        v-else-if="calendars.length === 0"
        class="flex flex-col items-center justify-center py-12"
      >
        <div
          class="mb-6 flex h-24 w-24 items-center justify-center rounded-full bg-gray-100 dark:bg-gray-800"
        >
          <svg
            class="h-12 w-12 text-gray-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
            />
          </svg>
        </div>
        <h2 class="mb-2 font-display text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('calendar.noCalendars') }}
        </h2>
        <p class="mb-6 text-gray-600 dark:text-gray-400">
          {{ t('calendar.createFirstCalendar') }}
        </p>
        <router-link to="/calendars/new" class="btn btn-primary">
          <svg class="mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 4v16m8-8H4"
            />
          </svg>
          {{ t('calendar.newCalendar') }}
        </router-link>
      </div>

      <!-- Calendar list (pinned first, then the rest) -->
      <template v-else>
        <div v-if="pinnedCalendars.length > 0" class="mb-6">
          <h2
            class="mb-3 font-display text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"
          >
            {{ t('dashboard.pinned') }}
          </h2>
          <div :class="gridClass">
            <CalendarCard
              v-for="calendar in pinnedCalendars"
              :key="calendar.id"
              :calendar="calendar"
              :view="viewMode"
              :pinned="true"
              :draggable="isCustomOrder"
              :can-move-up="canMoveUp(calendar.id)"
              :can-move-down="canMoveDown(calendar.id)"
              :unified-feed-configured="feedConfigured"
              :feed-included="unifiedFeedStore.isCalendarIncluded(calendar.id)"
              @open="openCalendar(calendar.public_token)"
              @settings="router.push(`/calendars/${calendar.id}/settings`)"
              @copy-link="copyPublicLink(calendar.public_token)"
              @toggle-feed="toggleFeed(calendar.id)"
              @toggle-pin="dashboardStore.togglePin(calendar.id)"
              @move-up="moveCalendar(calendar.id, 'up')"
              @move-down="moveCalendar(calendar.id, 'down')"
              @start-drag="onDragStart"
              @end-drag="onDragEnd"
              @drop-on="onDropOn"
            />
          </div>
        </div>

        <div v-if="unpinnedCalendars.length > 0">
          <h2
            v-if="pinnedCalendars.length > 0"
            class="mb-3 font-display text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"
          >
            {{ t('dashboard.others') }}
          </h2>
          <div :class="gridClass">
            <CalendarCard
              v-for="calendar in unpinnedCalendars"
              :key="calendar.id"
              :calendar="calendar"
              :view="viewMode"
              :pinned="false"
              :draggable="isCustomOrder"
              :can-move-up="canMoveUp(calendar.id)"
              :can-move-down="canMoveDown(calendar.id)"
              :unified-feed-configured="feedConfigured"
              :feed-included="unifiedFeedStore.isCalendarIncluded(calendar.id)"
              @open="openCalendar(calendar.public_token)"
              @settings="router.push(`/calendars/${calendar.id}/settings`)"
              @copy-link="copyPublicLink(calendar.public_token)"
              @toggle-feed="toggleFeed(calendar.id)"
              @toggle-pin="dashboardStore.togglePin(calendar.id)"
              @move-up="moveCalendar(calendar.id, 'up')"
              @move-down="moveCalendar(calendar.id, 'down')"
              @start-drag="onDragStart"
              @end-drag="onDragEnd"
              @drop-on="onDropOn"
            />
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useAuthStore } from '@/stores/auth';
import { useCalendarStore } from '@/stores/calendar';
import { useDashboardStore } from '@/stores/dashboard';
import { useUnifiedFeedStore } from '@/stores/unifiedFeed';
import { useToastStore } from '@/stores/toast';
import { orderCalendars, orderedIds } from '@/utils/dashboardOrdering';
import type { DashboardViewMode } from '@/stores/dashboard';
import type { CalendarWithParticipants } from '@/types';
import QuotaUsage from '@/components/QuotaUsage.vue';
import CalendarCard from '@/components/dashboard/CalendarCard.vue';
import { translateErrorMessage } from '@/utils/errorTranslator';

const router = useRouter();
const { t } = useI18n();
const authStore = useAuthStore();
const calendarStore = useCalendarStore();
const dashboardStore = useDashboardStore();
const unifiedFeedStore = useUnifiedFeedStore();
const toastStore = useToastStore();

const user = computed(() => authStore.user);
const calendars = computed(() => {
  const cals = calendarStore.calendars;
  return Array.isArray(cals) ? cals.filter((c): c is CalendarWithParticipants => c != null) : [];
});
const loading = computed(() => calendarStore.loading);
const fetchError = ref<string | null>(null);

const viewModes: DashboardViewMode[] = ['card', 'list', 'expanded', 'compact'];

const viewMode = computed(() => dashboardStore.viewMode);
const isCustomOrder = computed(() => dashboardStore.sortMode === 'custom');

const feedConfigured = computed(() => unifiedFeedStore.config?.configured === true);

const unifiedFeedConfig = computed(() => unifiedFeedStore.config);
const unifiedFeedLoading = computed(() => unifiedFeedStore.loading);
const unifiedFeedUrl = computed(() => {
  if (!unifiedFeedConfig.value?.ics_token) return '';
  return `${window.location.origin}/api/v1/ics/unified/${unifiedFeedConfig.value.ics_token}.ics`;
});

// --- Sorting ------------------------------------------------------------------

const sortKey = computed({
  get() {
    if (dashboardStore.sortMode === 'custom') return 'custom';
    return dashboardStore.sortDirection === 'asc' ? 'name-asc' : 'name-desc';
  },
  set(value: string) {
    if (value === 'custom') {
      // First activation of custom order: freeze the currently visible order into the
      // manual sequence, so the list does not jump to whatever order the API returned.
      // The snapshot honours the active direction and pins — exactly what is on screen.
      if (dashboardStore.customOrder.length === 0) {
        dashboardStore.setCustomOrder(
          orderedIds(calendars.value, {
            sortMode: 'alphabetical',
            sortDirection: dashboardStore.sortDirection,
            pinnedIds: dashboardStore.pinnedIds,
            customOrder: [],
          })
        );
      }
      // Calendars created since the manual order was last touched join its tail, so a
      // real persisted sequence is preserved and only extended.
      dashboardStore.appendToCustomOrder(calendars.value.map(c => c.id));
      dashboardStore.setSortMode('custom');
    } else {
      dashboardStore.setSortMode('alphabetical');
      dashboardStore.setSortDirection(value === 'name-asc' ? 'asc' : 'desc');
    }
  },
});

const orderedCalendars = computed(() =>
  orderCalendars(calendars.value, {
    sortMode: dashboardStore.sortMode,
    sortDirection: dashboardStore.sortDirection,
    pinnedIds: dashboardStore.pinnedIds,
    customOrder: dashboardStore.customOrder,
  })
);

const pinnedCalendars = computed(() =>
  orderedCalendars.value.filter(c => dashboardStore.pinnedIds.includes(c.id))
);
const unpinnedCalendars = computed(() =>
  orderedCalendars.value.filter(c => !dashboardStore.pinnedIds.includes(c.id))
);

// Keep the manual order and pins consistent with the actual calendar set: a calendar
// created after the order was set gets appended, a deleted one is pruned. The manual
// sequence is only maintained while custom order is active — seeding it during
// alphabetical sorting is what made the first switch to "custom order" snap to the API
// order instead of the visible one. Pins are maintained either way.
//
// Pruning only ever runs against an *authoritative* list for the current user. A
// logout or a failed fetch empties the list *and* clears the calendarsForUser marker;
// treating that as a deletion set would erase the user's saved pins and custom order
// on a transient error. Preferences are also stored per account (see stores/dashboard
// .ts), so loading another account prunes that account's own stored ids, never A's.
watch(
  () => calendars.value.map(c => c.id),
  ids => {
    const currentUserId = authStore.user?.id ?? null;
    if (calendarStore.calendarsForUser === null) return;
    if (calendarStore.calendarsForUser !== currentUserId) return;
    if (dashboardStore.sortMode === 'custom') {
      dashboardStore.appendToCustomOrder(ids);
      dashboardStore.pruneCustomOrder(ids);
    }
    dashboardStore.prunePinned(ids);
  }
);

// --- View switching -------------------------------------------------------------

function setViewMode(mode: DashboardViewMode) {
  dashboardStore.setViewMode(mode);
}

function viewModeAriaLabel(mode: DashboardViewMode): string {
  switch (mode) {
    case 'card':
      return 'dashboard.viewCards';
    case 'list':
      return 'dashboard.viewList';
    case 'expanded':
      return 'dashboard.viewExpanded';
    case 'compact':
      return 'dashboard.viewCompact';
  }
}

/** Grid/stack class for the current view. */
const gridClass = computed(() => {
  if (viewMode.value === 'card') {
    return 'grid gap-6 sm:grid-cols-2 lg:grid-cols-3';
  }
  return 'space-y-2';
});

// --- Drag and drop reordering ---------------------------------------------------

const draggingId = ref<string | null>(null);

function onDragStart(id: string) {
  draggingId.value = id;
}

function onDragEnd() {
  draggingId.value = null;
}

function onDropOn(targetId: string) {
  const sourceId = draggingId.value;
  draggingId.value = null;
  if (!sourceId || sourceId === targetId) return;

  // The pinned/unpinned boundary is not traversable. Pinned calendars are always
  // regrouped above the rest, so dropping across the boundary would change the
  // persisted `customOrder` without producing any on-screen move — and would leave
  // the sequence scrambled after a later pin toggle. Reject it before mutating.
  const sourcePinned = dashboardStore.pinnedIds.includes(sourceId);
  const targetPinned = dashboardStore.pinnedIds.includes(targetId);
  if (sourcePinned !== targetPinned) return;

  // Reorder within the calendar's own group, keeping the other group's relative
  // order, exactly like the move buttons below.
  const group = groupOrderIds(sourceId);
  const from = group.indexOf(sourceId);
  const to = group.indexOf(targetId);
  if (from === -1 || to === -1) return;

  const nextGroup = [...group];
  const [moved] = nextGroup.splice(from, 1);
  const targetIndex = nextGroup.indexOf(targetId);
  nextGroup.splice(targetIndex, 0, moved);

  const other = orderedCalendars.value
    .filter(c => dashboardStore.pinnedIds.includes(c.id) !== sourcePinned)
    .map(c => c.id);
  const next = sourcePinned ? [...nextGroup, ...other] : [...other, ...nextGroup];
  dashboardStore.setCustomOrder(next);
}

/**
 * The ids of one display group (pinned or unpinned) in their on-screen order.
 *
 * The move buttons and the drag-and-drop both reorder within the calendar's own
 * group; the pinned/unpinned boundary is not traversable by either, because pinned
 * calendars are always regrouped above the rest.
 */
function groupOrderIds(id: string): string[] {
  const pinned = dashboardStore.pinnedIds.includes(id);
  return orderedCalendars.value
    .filter(c => dashboardStore.pinnedIds.includes(c.id) === pinned)
    .map(c => c.id);
}

/**
 * Keyboard/touch reorder within the calendar's own pinned/unpinned group, one slot at
 * a time. The first item of a group cannot move up and the last cannot move down — a
 * move across the pinned boundary would not be visible (pinned calendars are always
 * regrouped at the top), so boundary moves leave the sequence untouched. The buttons
 * are disabled at those edges too, so the user is told before clicking.
 */
function moveCalendar(id: string, direction: 'up' | 'down') {
  const pinned = dashboardStore.pinnedIds.includes(id);
  const group = groupOrderIds(id);
  const from = group.indexOf(id);
  if (from === -1) return;
  const to = direction === 'up' ? from - 1 : from + 1;
  // Boundary: nothing within this group to swap with.
  if (to < 0 || to >= group.length) return;

  const nextGroup = [...group];
  const [moved] = nextGroup.splice(from, 1);
  nextGroup.splice(to, 0, moved);

  // Rebuild the global sequence with this group's segment replaced in place; the
  // other group keeps its relative order.
  const other = orderedCalendars.value
    .filter(c => dashboardStore.pinnedIds.includes(c.id) !== pinned)
    .map(c => c.id);
  const next = pinned ? [...nextGroup, ...other] : [...other, ...nextGroup];
  dashboardStore.setCustomOrder(next);
}

/** Whether `id` has a same-group calendar above it to swap with. */
function canMoveUp(id: string): boolean {
  return groupOrderIds(id).indexOf(id) > 0;
}

/** Whether `id` has a same-group calendar below it to swap with. */
function canMoveDown(id: string): boolean {
  const group = groupOrderIds(id);
  const index = group.indexOf(id);
  return index !== -1 && index < group.length - 1;
}

// --- Calendar actions -------------------------------------------------------------

function openCalendar(publicToken: string) {
  router.push(`/c/${publicToken}`);
}

function copyPublicLink(token: string) {
  const url = `${window.location.origin}/c/${token}`;
  navigator.clipboard.writeText(url);
  toastStore.success(t('common.linkCopied'));
}

function copyUnifiedFeedUrl() {
  navigator.clipboard.writeText(unifiedFeedUrl.value);
  toastStore.success(t('unifiedFeed.urlCopied'));
}

async function enableUnifiedFeed() {
  try {
    await unifiedFeedStore.createFeed();
  } catch {
    // The store remembers the translated failure (rendered above); spell it out too,
    // because the enable button only appears when there is no feed to show.
    toastStore.error(unifiedFeedStore.error ?? t('errors.unexpected'));
  }
}

async function regenerateUnifiedFeedToken() {
  if (!confirm(t('unifiedFeed.regenerateConfirm'))) return;
  try {
    await unifiedFeedStore.regenerateToken();
    toastStore.success(t('unifiedFeed.tokenRegenerated'));
  } catch {
    toastStore.error(unifiedFeedStore.error ?? t('errors.unexpected'));
  }
}

/** Toggle a calendar's feed membership, surfacing a failure instead of dropping it. */
async function toggleFeed(calendarId: string) {
  try {
    await unifiedFeedStore.toggleCalendar(calendarId);
  } catch {
    // The inline error above shows the translated message; this catches what would
    // otherwise be an unhandled rejection from the fire-and-forget checkbox.
    toastStore.error(unifiedFeedStore.error ?? t('errors.unexpected'));
  }
}

async function loadCalendars() {
  fetchError.value = null;
  try {
    await calendarStore.fetchCalendars();
  } catch (err) {
    fetchError.value = t(translateErrorMessage(err, { fallback: 'calendar.fetchError' }));
  }
}

onMounted(() => {
  loadCalendars();
  // The mount-time config read feeds the inline error; it must not also end up as an
  // unhandled rejection in the global handler.
  unifiedFeedStore.fetchConfig().catch(() => {
    toastStore.error(unifiedFeedStore.error ?? t('errors.unexpected'));
  });
});
</script>
