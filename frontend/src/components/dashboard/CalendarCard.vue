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

  Drag-and-drop is broken out on purpose: only the small grip initiates a drag (so
  clicking, text selection and the inner controls keep working), the whole card is a
  drop target, and the target id is emitted as `drop-on`.
-->

<template>
  <article :class="rootClass" @dragover.prevent @drop.prevent="emit('drop-on', calendar.id)">
    <!-- ------------------------------------------------------------------ -->
    <!-- Card view (the default grid tile)                                  -->
    <!-- ------------------------------------------------------------------ -->
    <template v-if="view === 'card'">
      <div class="mb-4 flex items-start justify-between gap-2">
        <div class="min-w-0 flex-1">
          <a
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
            class="drag-grip"
            draggable="true"
            :aria-label="t('dashboard.dragHandle')"
            :title="t('dashboard.dragHandle')"
            @click.stop.prevent
            @dragstart="handleDragStart"
            @dragend="emit('end-drag')"
          >
            <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
              <path
                d="M7 2a2 2 0 10.001 4.001A2 2 0 007 2zm0 6a2 2 0 10.001 4.001A2 2 0 007 8zm0 6a2 2 0 10.001 4.001A2 2 0 007 14zm6-8a2 2 0 10-.001-4.001A2 2 0 0013 6zm0 2a2 2 0 10.001 4.001A2 2 0 0013 8zm0 6a2 2 0 10.001 4.001A2 2 0 0013 14z"
              />
            </svg>
          </button>
          <button
            type="button"
            class="card-action"
            :class="pinned ? 'card-action-on' : ''"
            :aria-label="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
            :title="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
            :aria-pressed="pinned"
            @click.stop.prevent="emit('toggle-pin')"
          >
            <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
              <path
                d="M9.05 2.915a1 1 0 011.9 0l1 3a1 1 0 01-.13.82L11 8.5V12l2 2.5V6.695a1 1 0 01-.13-.82l1-3a1 1 0 011.9 0l.87 2.61a2 2 0 01.36 1.1V8l2.826 3.533a1 1 0 01-.784 1.6H13.2l-1.7 5.1a1 1 0 01-1.9 0l-1.7-5.1H2.934a1 1 0 01-.784-1.6L5 8V6.695a2 2 0 01.36-1.1l.87-2.61a1 1 0 011.9 0l.87 2.61a1 1 0 01.13.82V4.985a1 1 0 01-.13-.82l1-3z"
              />
            </svg>
          </button>
          <div
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400"
          >
            <svg
              class="h-5 w-5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
              />
            </svg>
          </div>
        </div>
      </div>

      <div class="mb-4 flex items-center space-x-4 text-sm">
        <div class="flex items-center text-gray-600 dark:text-gray-400">
          <svg
            class="mr-1 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
            />
          </svg>
          <span>{{ participantCount }} {{ t('calendar.participantCount') }}</span>
        </div>
      </div>

      <div v-if="unifiedFeedConfigured" class="mb-2 flex items-center" @click.stop>
        <label :for="`unified-feed-${calendar.id}`" class="flex items-center">
          <input
            :id="`unified-feed-${calendar.id}`"
            type="checkbox"
            :checked="feedIncluded"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
            @change="emit('toggle-feed')"
          />
          <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('unifiedFeed.includeInFeed') }}
          </span>
        </label>
      </div>

      <div class="flex items-center space-x-2">
        <template v-if="draggable">
          <button
            type="button"
            class="card-action"
            :aria-label="t('dashboard.moveUp')"
            :title="t('dashboard.moveUp')"
            :disabled="!canMoveUp"
            @click.stop.prevent="emit('move-up')"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M5 15l7-7 7 7"
              />
            </svg>
          </button>
          <button
            type="button"
            class="card-action"
            :aria-label="t('dashboard.moveDown')"
            :title="t('dashboard.moveDown')"
            :disabled="!canMoveDown"
            @click.stop.prevent="emit('move-down')"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M19 9l-7 7-7-7"
              />
            </svg>
          </button>
        </template>
        <button
          type="button"
          class="btn btn-ghost btn-sm flex-1"
          :title="t('calendar.copyLink')"
          @click.stop.prevent="emit('copy-link')"
        >
          <svg
            class="mr-1 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
            />
          </svg>
          {{ t('common.copy') }}
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-sm"
          :title="t('common.settings')"
          @click.stop.prevent="emit('settings')"
        >
          <svg
            class="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
            />
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
            />
          </svg>
        </button>
      </div>
    </template>

    <!-- ------------------------------------------------------------------ -->
    <!-- Expanded view (rich row: description and feed included)             -->
    <!-- ------------------------------------------------------------------ -->
    <template v-else-if="view === 'expanded'">
      <div class="flex items-start gap-3">
        <button
          v-if="draggable"
          type="button"
          class="drag-grip mt-1"
          draggable="true"
          :aria-label="t('dashboard.dragHandle')"
          :title="t('dashboard.dragHandle')"
          @click.stop.prevent
          @dragstart="handleDragStart"
          @dragend="emit('end-drag')"
        >
          <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
            <path
              d="M7 2a2 2 0 10.001 4.001A2 2 0 007 2zm0 6a2 2 0 10.001 4.001A2 2 0 007 8zm0 6a2 2 0 10.001 4.001A2 2 0 007 14zm6-8a2 2 0 10-.001-4.001A2 2 0 0013 6zm0 2a2 2 0 10.001 4.001A2 2 0 0013 8zm0 6a2 2 0 10.001 4.001A2 2 0 0013 14z"
            />
          </svg>
        </button>

        <div
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary-100 text-primary-600 dark:bg-primary-900 dark:text-primary-400"
        >
          <svg
            class="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
            />
          </svg>
        </div>

        <div class="min-w-0 flex-1">
          <div class="flex items-start justify-between gap-2">
            <a
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
              class="card-action"
              :class="pinned ? 'card-action-on' : ''"
              :aria-label="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
              :title="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
              :aria-pressed="pinned"
              @click.stop.prevent="emit('toggle-pin')"
            >
              <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
                <path
                  d="M9.05 2.915a1 1 0 011.9 0l1 3a1 1 0 01-.13.82L11 8.5V12l2 2.5V6.695a1 1 0 01-.13-.82l1-3a1 1 0 011.9 0l.87 2.61a2 2 0 01.36 1.1V8l2.826 3.533a1 1 0 01-.784 1.6H13.2l-1.7 5.1a1 1 0 01-1.9 0l-1.7-5.1H2.934a1 1 0 01-.784-1.6L5 8V6.695a2 2 0 01.36-1.1l.87-2.61a1 1 0 011.9 0l.87 2.61a1 1 0 01.13.82V4.985a1 1 0 01-.13-.82l1-3z"
                />
              </svg>
            </button>
          </div>
          <p v-if="calendar.description" :class="descriptionClass">
            {{ calendar.description }}
          </p>
          <div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
            <span class="flex items-center text-gray-600 dark:text-gray-400">
              <svg
                class="mr-1 h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"
                />
              </svg>
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
                :checked="feedIncluded"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600 dark:bg-gray-700"
                @change="emit('toggle-feed')"
              />
              <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('unifiedFeed.includeInFeed') }}
              </span>
            </label>
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-1">
          <template v-if="draggable">
            <button
              type="button"
              class="card-action"
              :aria-label="t('dashboard.moveUp')"
              :title="t('dashboard.moveUp')"
              :disabled="!canMoveUp"
              @click.stop.prevent="emit('move-up')"
            >
              <svg
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M5 15l7-7 7 7"
                />
              </svg>
            </button>
            <button
              type="button"
              class="card-action"
              :aria-label="t('dashboard.moveDown')"
              :title="t('dashboard.moveDown')"
              :disabled="!canMoveDown"
              @click.stop.prevent="emit('move-down')"
            >
              <svg
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </button>
          </template>
          <button
            type="button"
            class="card-action"
            :title="t('calendar.copyLink')"
            :aria-label="t('calendar.copyLink')"
            @click.stop.prevent="emit('copy-link')"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
              />
            </svg>
          </button>
          <button
            type="button"
            class="card-action"
            :title="t('common.settings')"
            :aria-label="t('common.settings')"
            @click.stop.prevent="emit('settings')"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
              />
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
              />
            </svg>
          </button>
        </div>
      </div>
    </template>

    <!-- ------------------------------------------------------------------ -->
    <!-- List and compact views (horizontal rows)                            -->
    <!-- ------------------------------------------------------------------ -->
    <template v-else>
      <div class="flex items-center gap-3">
        <button
          v-if="draggable"
          type="button"
          class="drag-grip"
          draggable="true"
          :aria-label="t('dashboard.dragHandle')"
          :title="t('dashboard.dragHandle')"
          @click.stop.prevent
          @dragstart="handleDragStart"
          @dragend="emit('end-drag')"
        >
          <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
            <path
              d="M7 2a2 2 0 10.001 4.001A2 2 0 007 2zm0 6a2 2 0 10.001 4.001A2 2 0 007 8zm0 6a2 2 0 10.001 4.001A2 2 0 007 14zm6-8a2 2 0 10-.001-4.001A2 2 0 0013 6zm0 2a2 2 0 10.001 4.001A2 2 0 0013 8zm0 6a2 2 0 10.001 4.001A2 2 0 0013 14z"
            />
          </svg>
        </button>

        <div :class="iconContainerClass">
          <svg
            :class="iconClass"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
            />
          </svg>
        </div>

        <div class="min-w-0 flex-1">
          <a
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

        <div class="flex shrink-0 items-center gap-1">
          <template v-if="draggable">
            <button
              type="button"
              class="card-action"
              :aria-label="t('dashboard.moveUp')"
              :title="t('dashboard.moveUp')"
              :disabled="!canMoveUp"
              @click.stop.prevent="emit('move-up')"
            >
              <svg
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M5 15l7-7 7 7"
                />
              </svg>
            </button>
            <button
              type="button"
              class="card-action"
              :aria-label="t('dashboard.moveDown')"
              :title="t('dashboard.moveDown')"
              :disabled="!canMoveDown"
              @click.stop.prevent="emit('move-down')"
            >
              <svg
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            </button>
          </template>
          <button
            type="button"
            class="card-action"
            :class="pinned ? 'card-action-on' : ''"
            :aria-label="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
            :title="pinned ? t('dashboard.unpinCalendar') : t('dashboard.pinCalendar')"
            :aria-pressed="pinned"
            @click.stop.prevent="emit('toggle-pin')"
          >
            <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
              <path
                d="M9.05 2.915a1 1 0 011.9 0l1 3a1 1 0 01-.13.82L11 8.5V12l2 2.5V6.695a1 1 0 01-.13-.82l1-3a1 1 0 011.9 0l.87 2.61a2 2 0 01.36 1.1V8l2.826 3.533a1 1 0 01-.784 1.6H13.2l-1.7 5.1a1 1 0 01-1.9 0l-1.7-5.1H2.934a1 1 0 01-.784-1.6L5 8V6.695a2 2 0 01.36-1.1l.87-2.61a1 1 0 011.9 0l.87 2.61a1 1 0 01.13.82V4.985a1 1 0 01-.13-.82l1-3z"
              />
            </svg>
          </button>
          <button
            type="button"
            class="card-action"
            :title="t('calendar.copyLink')"
            :aria-label="t('calendar.copyLink')"
            @click.stop.prevent="emit('copy-link')"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
              />
            </svg>
          </button>
          <button
            type="button"
            class="card-action"
            :title="t('common.settings')"
            :aria-label="t('common.settings')"
            @click.stop.prevent="emit('settings')"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
              />
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
              />
            </svg>
          </button>
        </div>
      </div>
    </template>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
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

const participantCount = computed(() => props.calendar.participants?.length || 0);

/** The real destination of the card's open link, so modified clicks open it natively. */
const openUrl = computed(() => `/c/${props.calendar.public_token}`);

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
    return 'calendar-card card card-hover group cursor-pointer flex items-center gap-3 px-3 py-2';
  }
  return 'calendar-card card card-hover group cursor-pointer flex items-center gap-3 p-3';
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
    : 'mt-1 flex-1 whitespace-pre-wrap text-sm text-gray-600 dark:text-gray-400'
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
