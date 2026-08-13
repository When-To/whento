<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="card mb-6">
    <h2 class="mb-4 font-display text-xl font-semibold text-gray-900 dark:text-white">
      {{ t('calendar.participants') }}
    </h2>
    <div v-if="stats.length > 0" class="space-y-2">
      <button
        v-for="stat in stats"
        :key="stat.name"
        type="button"
        class="w-full flex items-center justify-between rounded-lg border p-3 transition-all cursor-pointer"
        :class="
          selected.has(stat.name)
            ? 'border-purple-500 bg-purple-50 dark:border-purple-400 dark:bg-purple-900/30'
            : 'border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800 hover:border-gray-300 dark:hover:border-gray-600'
        "
        @click="emit('toggle', stat.name)"
      >
        <span class="text-sm font-medium text-gray-900 dark:text-white">
          {{ stat.name }}
        </span>
        <span
          class="rounded-full px-3 py-1 text-sm font-semibold"
          :class="
            selected.has(stat.name)
              ? 'bg-purple-200 text-purple-900 dark:bg-purple-800 dark:text-purple-100'
              : 'bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200'
          "
        >
          {{ stat.count }}
          {{ stat.count > 1 ? t('availability.availabilities') : t('availability.availability') }}
        </span>
      </button>
    </div>
    <div v-else class="text-sm text-gray-600 dark:text-gray-400">
      {{ t('availability.noParticipant') }}
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Everyone on the calendar and how many days each has answered, selectable to highlight
 * the dates they all share.
 *
 * The selection is a plain set of names owned by the parent, which is what the calendar
 * model consumes; this component only reports the click.
 */
import { useI18n } from 'vue-i18n';

/** One participant's answer count over the visible range. */
export interface ParticipantStat {
  name: string;
  count: number;
}

defineProps<{
  stats: ParticipantStat[];
  /** Names currently highlighted. */
  selected: Set<string>;
}>();

const emit = defineEmits<{
  toggle: [name: string];
}>();

const { t } = useI18n();
</script>
