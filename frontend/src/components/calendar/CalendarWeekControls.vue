<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

/**
 * Visible time range and slot size for the week grids.
 *
 * These lived inside the week grid, which meant every mounted grid carried its own
 * copy and a watcher chain kept them in sync — and with more than one week on screen
 * the controls rendered on the grid that was configured to follow, not lead. The
 * parent owns the values now and renders one set.
 */

const startHour = defineModel<number>('startHour', { required: true });
const endHour = defineModel<number>('endHour', { required: true });
const slotDuration = defineModel<number>('slotDuration', { required: true });

const { t } = useI18n();

const HOURS = Array.from({ length: 25 }, (_, hour) => hour);
const DURATIONS = [15, 30, 60];

/** The end must stay after the start, so the two lists constrain each other. */
const startOptions = computed(() => HOURS.filter(hour => hour < endHour.value));
const endOptions = computed(() => HOURS.filter(hour => hour > startHour.value));

function label(hour: number): string {
  return `${String(hour).padStart(2, '0')}:00`;
}
</script>

<template>
  <div class="flex flex-wrap items-end gap-3">
    <label class="flex flex-col gap-1 text-xs text-gray-600 dark:text-gray-400">
      {{ t('calendar.startHour') }}
      <select v-model.number="startHour" class="input py-1 text-sm">
        <option v-for="hour in startOptions" :key="hour" :value="hour">{{ label(hour) }}</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-xs text-gray-600 dark:text-gray-400">
      {{ t('calendar.endHour') }}
      <select v-model.number="endHour" class="input py-1 text-sm">
        <option v-for="hour in endOptions" :key="hour" :value="hour">{{ label(hour) }}</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-xs text-gray-600 dark:text-gray-400">
      {{ t('calendar.slotDuration') }}
      <select v-model.number="slotDuration" class="input py-1 text-sm">
        <option v-for="minutes in DURATIONS" :key="minutes" :value="minutes">
          {{ minutes }} min
        </option>
      </select>
    </label>
  </div>
</template>
