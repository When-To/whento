<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <!-- Mobile: Stacked layout with collapsible controls -->
  <div class="mb-4 p-6">
    <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <!-- Title and description -->
      <div class="flex-1">
        <h2 class="font-display text-xl font-semibold text-gray-900 dark:text-white">
          {{ t('calendar.calendar') }}
          <span
            v-if="dateRangeText"
            class="text-base font-normal text-gray-600 dark:text-gray-400 block md:inline mt-1 md:mt-0"
          >
            {{ dateRangeText }}
          </span>
        </h2>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-400 hidden md:block">
          {{ instructions }}
        </p>
      </div>

      <!-- Controls: Responsive layout -->
      <div class="flex flex-col gap-3 md:flex-row md:items-center md:gap-4">
        <!-- Display mode selector -->
        <div class="flex items-center gap-2">
          <label for="displayMode" class="text-sm text-gray-700 dark:text-gray-300 shrink-0">
            {{ t('calendar.displayMode') }}
          </label>
          <select
            id="displayMode"
            v-model="displayMode"
            class="input text-sm flex-1 md:w-32 min-h-11 md:min-h-0"
          >
            <option value="month">
              {{ t('calendar.monthView') }}
            </option>
            <option value="week">
              {{ t('calendar.weekView') }}
            </option>
          </select>
        </div>

        <!-- Grid or list -->
        <div class="flex items-center gap-2">
          <label for="viewStyle" class="text-sm text-gray-700 dark:text-gray-300 shrink-0">
            {{ t('calendar.viewStyle') }}
          </label>
          <select
            id="viewStyle"
            v-model="viewStyle"
            class="input text-sm flex-1 md:w-32 min-h-11 md:min-h-0"
          >
            <option value="grid">{{ t('calendar.viewClassic') }}</option>
            <option value="list">{{ t('calendar.listView') }}</option>
          </select>
        </div>

        <!-- Period count selector -->
        <div class="flex items-center gap-2">
          <label for="periodCount" class="text-sm text-gray-700 dark:text-gray-300 shrink-0">
            {{ periodCountLabel }}
          </label>
          <select
            id="periodCount"
            v-model.number="numberOfPeriods"
            class="input text-sm flex-1 md:w-20 min-h-11 md:min-h-0"
          >
            <option v-for="n in maxPeriods" :key="n" :value="n">
              {{ n }}
            </option>
          </select>
        </div>
      </div>
    </div>

    <!-- Mobile instruction text (below controls) -->
    <p class="mt-3 text-sm text-gray-600 dark:text-gray-400 md:hidden">
      {{ instructions }}
    </p>
  </div>
</template>

<script setup lang="ts">
/**
 * The header of the calendar card: what the calendar is called, over what range, and the
 * three selectors that decide how it is drawn.
 *
 * The ids `displayMode`, `viewStyle` and `periodCount` are load-bearing — the labels
 * point at them and the backend end-to-end suite drives `#displayMode` directly — so
 * they are kept exactly as they were.
 *
 * The three ternaries the audit flagged (the period-count label, the option count, and
 * the two navigation handlers next door) are named computeds here; the template no
 * longer decides anything.
 */
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { maxPeriodsFor } from '@/composables/calendar/useParticipantDisplaySettings';
import type { DisplayMode, ViewStyle } from '@/composables/calendar/useCalendarViewState';

defineProps<{
  /** Preformatted "from … to …" for the calendar's own date bounds; may be empty. */
  dateRangeText: string;
}>();

const displayMode = defineModel<DisplayMode>('displayMode', { required: true });
const viewStyle = defineModel<ViewStyle>('viewStyle', { required: true });
const numberOfPeriods = defineModel<number>('numberOfPeriods', { required: true });

const { t } = useI18n();

const maxPeriods = computed(() => maxPeriodsFor(displayMode.value));

const periodCountLabel = computed(() =>
  displayMode.value === 'week' ? t('calendar.numberOfWeeks') : t('calendar.numberOfMonths')
);

const instructions = computed(() => t('availability.clickOrDragToAdd'));
</script>
