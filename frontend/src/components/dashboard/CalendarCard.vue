<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<!--
  One calendar on the dashboard, rendered in any of the four display views
  (`card`, `expanded`, `list`, `compact`). The component is deliberately dumb: it
  draws the calendar and forwards every interaction as an event, so the ordering,
  the unified-feed and the clipboard state all stay in the Dashboard.

  The calendar name is a *genuine link*: the one accessible "open" control on the
  card, with a real href so modified clicks (Ctrl/Cmd/Shift/middle) open the target in
  a new tab natively. A pseudo-element stretches it across the whole card (see
  `.calendar-card .card-open-area` in style.css), so pointer users can open a calendar
  by clicking anywhere on its surface. The card's own interleaved action controls are
  layered above that stretch (position/z-index) so activating them does not open the
  calendar; the card never nests interactive controls inside the link.

  Every control carries a stable `data-action` marker (and the card root a
  `data-calendar-id`), so keyboard tests and assistive tooling can target a calendar's
  controls without scraping translated labels.

  Focus after a reorder: the Dashboard moves a calendar (button or drag), then asks
  this component for the moved card via `focusMoveControl(direction)`. The component
  knows which view it is currently drawing, so it focuses only that view's visible
  control — never a hidden duplicate from another layout. When the requested direction
  is dead (the card is at the edge of its group) focus falls through to the opposite
  enabled control, and finally to the calendar's own open link, so keyboard focus
  never lands on BODY.
-->

<template>
  <!-- eslint-disable-next-line vuejs-accessibility/no-static-element-interactions -- Article is a pointer drop target; labeled move buttons provide keyboard reordering. -->
  <article
    :data-calendar-id="calendar.id"
    :class="rootClass"
    @dragover.prevent
    @drop.prevent="emit('drop-on', calendar.id)"
  >
    <!-- ------------------------------------------------------------------ -->
    <!-- Card view (the default grid tile)                                  -->
    <!-- ------------------------------------------------------------------ -->
    <template v-if="view === 'card'">
      <div class="mb-4 flex items-start justify-between gap-2">
        <div class="min-w-0 flex-1">
          <a
            ref="openArea"
            data-action="open"
            class="card-open-area"
            :class="[nameClass, 'text-left']"
            :href="openUrl"
            :aria-label="t('dashboard.openCalendar', { name: calendar.name })"
            @click="onNameOpen"
          >
            {{ calendar.name }}
          </a>
          <p v-if="calendar.description" :class="descriptionClass">
            {{ calendar.description }}
          </p>
        </div>
        <div class="flex shrink-0 items-center gap-1">
          <button
            v-if="draggable"
            type="button"
            data-action="drag"
            tabindex="-1"
            aria-hidden="true"
            class="drag-grip"
            draggable="true"
            :aria-label="t('dashboard.dragHandle')"
            :title="t('dashboard.dragHandle')"
            @click.stop.prevent
            @dragstart="handleDragStart"
            @dragend="emit('end-drag')"
          >
            <CardIcon name="grip" class="h-4 w-4" />
          </button>
          <button
            type="button"
            data-action="pin"
            class="card-action"
            :class="pinned ? 'card-action-on' : ''"
            :aria-label="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
            :title="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
            :aria-pressed="pinned"
            @click.stop.prevent="emit('toggle-pin')"
          >
            <CardIcon name="pin" class="h-4 w-4" />
          </button>
          <div
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400"
          >
            <CardIcon name="calendar" class="h-5 w-5" />
          </div>
        </div>
      </div>

      <div class="mb-4 flex items-center space-x-4 text-sm">
        <div class="flex items-center text-gray-600 dark:text-gray-400">
          <CardIcon name="people" class="mr-1 h-4 w-4" />
          <span>{{ participantCount }} {{ t('calendar.participantCount') }}</span>
        </div>
      </div>

      <div v-if="unifiedFeedConfigured" class="mb-2 flex items-center" @click.stop>
        <label :for="`unified-feed-${calendar.id}`" class="flex items-center">
          <input
            :id="`unified-feed-${calendar.id}`"
            type="checkbox"
            data-action="feed"
            :checked="feedIncluded"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
            @change="handleFeedToggle"
          />
          <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('unifiedFeed.includeInFeed') }}
          </span>
        </label>
      </div>

      <div class="flex items-center space-x-2">
        <template v-if="draggable">
          <button
            ref="moveUp"
            type="button"
            data-action="move-up"
            class="card-action"
            :aria-label="t('dashboard.moveUp')"
            :title="t('dashboard.moveUp')"
            :disabled="!canMoveUp"
            @click.stop.prevent="emit('move-up')"
          >
            <CardIcon name="up" class="h-4 w-4" />
          </button>
          <button
            ref="moveDown"
            type="button"
            data-action="move-down"
            class="card-action"
            :aria-label="t('dashboard.moveDown')"
            :title="t('dashboard.moveDown')"
            :disabled="!canMoveDown"
            @click.stop.prevent="emit('move-down')"
          >
            <CardIcon name="down" class="h-4 w-4" />
          </button>
        </template>
        <button
          type="button"
          data-action="copy"
          class="btn btn-ghost btn-sm flex-1"
          :title="t('calendar.copyLink')"
          @click.stop.prevent="emit('copy-link')"
        >
          <CardIcon name="copy" class="mr-1 h-4 w-4" />
          {{ t('common.copy') }}
        </button>
        <button
          type="button"
          data-action="settings"
          class="btn btn-ghost btn-sm"
          :title="t('common.settings')"
          @click.stop.prevent="emit('settings')"
        >
          <CardIcon name="settings" class="h-4 w-4" />
        </button>
      </div>
    </template>

    <!-- ------------------------------------------------------------------ -->
    <!-- Expanded view (rich row: description and feed included)             -->
    <!-- ------------------------------------------------------------------ -->
    <template v-else-if="view === 'expanded'">
      <div class="flex flex-wrap items-start gap-3">
        <button
          v-if="draggable"
          type="button"
          data-action="drag"
          tabindex="-1"
          aria-hidden="true"
          class="drag-grip mt-1"
          draggable="true"
          :aria-label="t('dashboard.dragHandle')"
          :title="t('dashboard.dragHandle')"
          @click.stop.prevent
          @dragstart="handleDragStart"
          @dragend="emit('end-drag')"
        >
          <CardIcon name="grip" class="h-4 w-4" />
        </button>

        <div
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400"
        >
          <CardIcon name="calendar" class="h-4 w-4" />
        </div>

        <div class="min-w-0 flex-1 basis-48">
          <div class="flex items-start justify-between gap-2">
            <a
              ref="openArea"
              data-action="open"
              class="card-open-area"
              :class="[nameClass, 'text-left']"
              :href="openUrl"
              :aria-label="t('dashboard.openCalendar', { name: calendar.name })"
              @click="onNameOpen"
            >
              {{ calendar.name }}
            </a>
            <button
              type="button"
              data-action="pin"
              class="card-action"
              :class="pinned ? 'card-action-on' : ''"
              :aria-label="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
              :title="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
              :aria-pressed="pinned"
              @click.stop.prevent="emit('toggle-pin')"
            >
              <CardIcon name="pin" class="h-4 w-4" />
            </button>
          </div>
          <p v-if="calendar.description" :class="descriptionClass">
            {{ calendar.description }}
          </p>
          <div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
            <span class="flex items-center text-gray-600 dark:text-gray-400">
              <CardIcon name="people" class="mr-1 h-4 w-4" />
              {{ participantCount }} {{ t('calendar.participantCount') }}
            </span>
            <label
              v-if="unifiedFeedConfigured"
              :for="`unified-feed-${calendar.id}`"
              class="flex cursor-pointer items-center"
              @click.stop
            >
              <input
                :id="`unified-feed-${calendar.id}`"
                type="checkbox"
                data-action="feed"
                :checked="feedIncluded"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
                @change="handleFeedToggle"
              />
              <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('unifiedFeed.includeInFeed') }}
              </span>
            </label>
          </div>
        </div>

        <div class="ml-auto flex shrink-0 items-center gap-1">
          <template v-if="draggable">
            <button
              ref="moveUp"
              type="button"
              data-action="move-up"
              class="card-action"
              :aria-label="t('dashboard.moveUp')"
              :title="t('dashboard.moveUp')"
              :disabled="!canMoveUp"
              @click.stop.prevent="emit('move-up')"
            >
              <CardIcon name="up" class="h-4 w-4" />
            </button>
            <button
              ref="moveDown"
              type="button"
              data-action="move-down"
              class="card-action"
              :aria-label="t('dashboard.moveDown')"
              :title="t('dashboard.moveDown')"
              :disabled="!canMoveDown"
              @click.stop.prevent="emit('move-down')"
            >
              <CardIcon name="down" class="h-4 w-4" />
            </button>
          </template>
          <button
            type="button"
            data-action="copy"
            class="card-action"
            :title="t('calendar.copyLink')"
            :aria-label="t('calendar.copyLink')"
            @click.stop.prevent="emit('copy-link')"
          >
            <CardIcon name="copy" class="h-4 w-4" />
          </button>
          <button
            type="button"
            data-action="settings"
            class="card-action"
            :title="t('common.settings')"
            :aria-label="t('common.settings')"
            @click.stop.prevent="emit('settings')"
          >
            <CardIcon name="settings" class="h-4 w-4" />
          </button>
        </div>
      </div>
    </template>

    <!-- ------------------------------------------------------------------ -->
    <!-- List and compact views (horizontal rows)                            -->
    <!-- ------------------------------------------------------------------ -->
    <template v-else>
      <div class="flex flex-wrap items-center gap-3">
        <button
          v-if="draggable"
          type="button"
          data-action="drag"
          tabindex="-1"
          aria-hidden="true"
          class="drag-grip"
          draggable="true"
          :aria-label="t('dashboard.dragHandle')"
          :title="t('dashboard.dragHandle')"
          @click.stop.prevent
          @dragstart="handleDragStart"
          @dragend="emit('end-drag')"
        >
          <CardIcon name="grip" class="h-4 w-4" />
        </button>

        <div :class="iconContainerClass">
          <CardIcon name="calendar" :class="iconClass" />
        </div>

        <div class="min-w-0 flex-1 basis-40">
          <a
            ref="openArea"
            data-action="open"
            class="card-open-area"
            :class="[nameClass, 'text-left']"
            :href="openUrl"
            :aria-label="t('dashboard.openCalendar', { name: calendar.name })"
            @click="onNameOpen"
          >
            {{ calendar.name }}
          </a>
          <p v-if="view === 'list' && calendar.description" :class="descriptionClass">
            {{ calendar.description }}
          </p>
        </div>

        <span class="shrink-0 text-sm text-gray-500 dark:text-gray-400">
          {{ participantCount }}
        </span>

        <div class="ml-auto flex shrink-0 items-center gap-1">
          <template v-if="draggable">
            <button
              ref="moveUp"
              type="button"
              data-action="move-up"
              class="card-action"
              :aria-label="t('dashboard.moveUp')"
              :title="t('dashboard.moveUp')"
              :disabled="!canMoveUp"
              @click.stop.prevent="emit('move-up')"
            >
              <CardIcon name="up" class="h-4 w-4" />
            </button>
            <button
              ref="moveDown"
              type="button"
              data-action="move-down"
              class="card-action"
              :aria-label="t('dashboard.moveDown')"
              :title="t('dashboard.moveDown')"
              :disabled="!canMoveDown"
              @click.stop.prevent="emit('move-down')"
            >
              <CardIcon name="down" class="h-4 w-4" />
            </button>
          </template>
          <button
            type="button"
            data-action="pin"
            class="card-action"
            :class="pinned ? 'card-action-on' : ''"
            :aria-label="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
            :title="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
            :aria-pressed="pinned"
            @click.stop.prevent="emit('toggle-pin')"
          >
            <CardIcon name="pin" class="h-4 w-4" />
          </button>
          <button
            type="button"
            data-action="copy"
            class="card-action"
            :title="t('calendar.copyLink')"
            :aria-label="t('calendar.copyLink')"
            @click.stop.prevent="emit('copy-link')"
          >
            <CardIcon name="copy" class="h-4 w-4" />
          </button>
          <button
            type="button"
            data-action="settings"
            class="card-action"
            :title="t('common.settings')"
            :aria-label="t('common.settings')"
            @click.stop.prevent="emit('settings')"
          >
            <CardIcon name="settings" class="h-4 w-4" />
          </button>
        </div>
      </div>
      <div v-if="unifiedFeedConfigured" class="mt-2 flex items-center" @click.stop>
        <label :for="`unified-feed-${calendar.id}`" class="flex items-center">
          <input
            :id="`unified-feed-${calendar.id}`"
            type="checkbox"
            data-action="feed"
            :checked="feedIncluded"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
            @change="handleFeedToggle"
          />
          <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('unifiedFeed.includeInFeed') }}
          </span>
        </label>
      </div>
    </template>
  </article>
</template>

<script setup lang="ts">
import { computed, h, ref, type FunctionalComponent } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CalendarWithParticipants } from '@/types';
import type { DashboardViewMode } from '@/stores/dashboard';

const props = defineProps<{
  calendar: CalendarWithParticipants;
  view: DashboardViewMode;
  pinned: boolean;
  draggable: boolean;
  /** Whether this card is not at the top of its pinned/unpinned group. */
  canMoveUp: boolean;
  /** Whether this card is not at the bottom of its pinned/unpinned group. */
  canMoveDown: boolean;
  unifiedFeedConfigured: boolean;
  feedIncluded: boolean;
}>();

const emit = defineEmits<{
  (event: 'open'): void;
  (event: 'settings'): void;
  (event: 'copy-link'): void;
  (event: 'toggle-feed'): void;
  (event: 'toggle-pin'): void;
  // Keyboard/touch reordering, available whenever the custom order is active.
  (event: 'move-up'): void;
  (event: 'move-down'): void;
  (event: 'start-drag', id: string): void;
  (event: 'end-drag'): void;
  (event: 'drop-on', id: string): void;
}>();

const { t } = useI18n();

/** The card's icons, drawn once here instead of inline in every view. */
const ICONS = {
  grip: {
    solid: true,
    paths: [
      'M7 2a2 2 0 10.001 4.001A2 2 0 007 2zm0 6a2 2 0 10.001 4.001A2 2 0 007 8zm0 6a2 2 0 10.001 4.001A2 2 0 007 14zm6-8a2 2 0 10-.001-4.001A2 2 0 0013 6zm0 2a2 2 0 10.001 4.001A2 2 0 0013 8zm0 6a2 2 0 10.001 4.001A2 2 0 0013 14z',
    ],
  },
  pin: {
    solid: true,
    paths: [
      'M9.05 2.915a1 1 0 011.9 0l1 3a1 1 0 01-.13.82L11 8.5V12l2 2.5V6.695a1 1 0 01-.13-.82l1-3a1 1 0 011.9 0l.87 2.61a2 2 0 01.36 1.1V8l2.826 3.533a1 1 0 01-.784 1.6H13.2l-1.7 5.1a1 1 0 01-1.9 0l-1.7-5.1H2.934a1 1 0 01-.784-1.6L5 8V6.695a2 2 0 01.36-1.1l.87-2.61a1 1 0 011.9 0l.87 2.61a1 1 0 01.13.82V4.985a1 1 0 01-.13-.82l1-3z',
    ],
  },
  calendar: {
    solid: false,
    paths: [
      'M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z',
    ],
  },
  people: {
    solid: false,
    paths: [
      'M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z',
    ],
  },
  up: { solid: false, paths: ['M5 15l7-7 7 7'] },
  down: { solid: false, paths: ['M19 9l-7 7-7-7'] },
  copy: {
    solid: false,
    paths: [
      'M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z',
    ],
  },
  settings: {
    solid: false,
    paths: [
      'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z',
      'M15 12a3 3 0 11-6 0 3 3 0 016 0z',
    ],
  },
} satisfies Record<string, { solid: boolean; paths: string[] }>;

const SOLID = { fill: 'currentColor', viewBox: '0 0 20 20', 'aria-hidden': 'true' };
const OUTLINE = {
  fill: 'none',
  viewBox: '0 0 24 24',
  stroke: 'currentColor',
  'aria-hidden': 'true',
};
const STROKE = { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'stroke-width': '2' };

/** Decorative icon; the size comes from the caller's `class`. */
const CardIcon: FunctionalComponent<{ name: keyof typeof ICONS }> = ({ name }) => {
  const { solid, paths } = ICONS[name];
  return h(
    'svg',
    solid ? SOLID : OUTLINE,
    paths.map(d => h('path', solid ? { d } : { ...STROKE, d }))
  );
};

const participantCount = computed(() => props.calendar.participants?.length || 0);

function handleFeedToggle(event: Event) {
  // Restore the controlled value until the store confirms the asynchronous update.
  // A failed update must not leave the DOM checkbox disagreeing with membership.
  (event.target as HTMLInputElement).checked = props.feedIncluded;
  emit('toggle-feed');
}

/** The real destination of the card's open link, so modified clicks open it natively. */
const openUrl = computed(() => `/c/${props.calendar.public_token}`);

/**
 * Focus an element and keep the focus ring visible.
 *
 * Browsers do not reliably paint `:focus-visible` for a programmatic `.focus()`, so a
 * keyboard user's focus indicator can silently vanish across a reorder. A fallback
 * class mirrors the ring unconditionally (same visual as `:focus-visible`) and is
 * removed the moment the element loses focus, so the ring is deterministic in every
 * engine and never stays painted on a blurred control.
 */
function focusWithRing(el: HTMLElement | null | undefined): boolean {
  if (!el) return false;
  el.focus({ preventScroll: true });
  el.classList.remove('focus-visible-fallback');
  el.classList.add('focus-visible-fallback');
  el.addEventListener('blur', () => el.classList.remove('focus-visible-fallback'), { once: true });
  return true;
}

function isEnabled(el: HTMLElement | null | undefined): el is HTMLElement {
  return !!el && !(el instanceof HTMLButtonElement && el.disabled);
}

// Exactly one view is mounted at a time, so each role needs a single ref: a move
// focuses only the controls that are actually visible, never a hidden sibling.
const moveUp = ref<HTMLButtonElement | null>(null);
const moveDown = ref<HTMLButtonElement | null>(null);
const openArea = ref<HTMLAnchorElement | null>(null);

/**
 * Focus this card's move control for `direction`.
 *
 * The narrow abstraction the Dashboard calls after a move: preferred direction first,
 * the opposite enabled control when the requested one is dead (the card sits at the
 * edge of its group), and the calendar's own open link as the last resort. The
 * Dashboard also uses `'auto'` after a drag, where there is no direction to speak of.
 */
function focusMoveControl(direction: 'up' | 'down' | 'auto'): boolean {
  const up = moveUp.value;
  const down = moveDown.value;
  if (direction === 'up') {
    if (isEnabled(up)) return focusWithRing(up);
    if (isEnabled(down)) return focusWithRing(down);
    return focusWithRing(openArea.value);
  }
  if (direction === 'down') {
    if (isEnabled(down)) return focusWithRing(down);
    if (isEnabled(up)) return focusWithRing(up);
    return focusWithRing(openArea.value);
  }
  if (isEnabled(up)) return focusWithRing(up);
  if (isEnabled(down)) return focusWithRing(down);
  return focusWithRing(openArea.value);
}

/** Focus the calendar's own open link (name). */
function focusOpen(): boolean {
  return focusWithRing(openArea.value);
}

defineExpose({ focusMoveControl, focusOpen });

/**
 * Open the calendar from the name link.
 *
 * A plain click (left button, no modifiers) is routed through the `open` emit so the
 * Dashboard navigates without a full page load. A modified click — Ctrl/Cmd/Shift, or
 * a middle click — is the browser's "open in a new tab/window" gesture against the
 * real `href`, so it is deliberately left to the browser instead of being prevented.
 */
function onNameOpen(event: MouseEvent) {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
    return;
  }
  event.preventDefault();
  emit('open');
}

const rootClass = computed(() => {
  if (props.view === 'card') {
    return 'calendar-card card card-hover group cursor-pointer';
  }
  if (props.view === 'expanded') {
    return 'calendar-card card card-hover group cursor-pointer p-4';
  }
  if (props.view === 'compact') {
    return 'calendar-card card card-hover group cursor-pointer px-3 py-2';
  }
  return 'calendar-card card card-hover group cursor-pointer p-3';
});

const nameClass = computed(() => {
  if (props.view === 'card') {
    return 'mb-1 font-display text-xl font-semibold text-gray-900 group-hover:text-primary-600 dark:text-white dark:group-hover:text-primary-400';
  }
  if (props.view === 'expanded') {
    return 'font-display text-lg font-semibold text-gray-900 group-hover:text-primary-600 dark:text-white dark:group-hover:text-primary-400';
  }
  if (props.view === 'compact') {
    return 'truncate text-sm font-medium text-gray-900 dark:text-white';
  }
  return 'truncate font-medium text-gray-900 dark:text-white';
});

const descriptionClass = computed(() =>
  props.view === 'list'
    ? 'mt-0.5 truncate text-sm text-gray-500 dark:text-gray-400'
    : 'mt-1 flex-1 whitespace-pre-wrap wrap-anywhere text-sm text-gray-600 dark:text-gray-400'
);

const iconContainerClass = computed(() => {
  const base =
    'flex shrink-0 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400';
  return props.view === 'compact' ? `${base} h-7 w-7` : `${base} h-8 w-8`;
});

const iconClass = computed(() => (props.view === 'compact' ? 'h-3.5 w-3.5' : 'h-4 w-4'));

function handleDragStart(event: DragEvent) {
  // The id travels through the emitted event, not through the data transfer: the
  // DataTransfer API is loader-gated in tests, so it is populated best-effort only.
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move';
    try {
      event.dataTransfer.setData('text/plain', props.calendar.id);
    } catch {
      // Some test environments lack DataTransfer; the drag still works via events.
    }
  }
  emit('start-drag', props.calendar.id);
}
</script>
