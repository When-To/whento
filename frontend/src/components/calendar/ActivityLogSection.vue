<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<template>
  <div class="space-y-4">
    <p class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('calendar.activity.description') }}
    </p>

    <p v-if="loading" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </p>

    <p v-else-if="error" class="text-sm text-danger-600 dark:text-danger-400">
      {{ t('calendar.activity.loadError') }}
    </p>

    <p v-else-if="entries.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
      {{ t('calendar.activity.empty') }}
    </p>

    <ul v-else class="space-y-3">
      <li
        v-for="entry in entries"
        :key="entry.date"
        class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800"
      >
        <div class="mb-2 flex flex-wrap items-baseline justify-between gap-2">
          <h4 class="font-semibold text-gray-900 dark:text-white">
            {{ formatISODate(entry.date as ISODate, locale, 'fullWithWeekday') }}
          </h4>
          <span
            class="text-sm"
            :class="
              entry.count >= entry.threshold
                ? 'font-medium text-primary-600 dark:text-primary-400'
                : 'text-gray-500 dark:text-gray-400'
            "
          >
            {{ t('calendar.activity.count', { count: entry.count, threshold: entry.threshold }) }}
          </span>
        </div>

        <dl class="space-y-1 text-sm">
          <div class="flex flex-wrap gap-x-2">
            <dt class="text-gray-500 dark:text-gray-400">
              {{ t('calendar.activity.lastJoined') }}
            </dt>
            <dd v-if="entry.last_joined" class="text-gray-900 dark:text-gray-100">
              <span class="font-medium">{{ entry.last_joined.name }}</span>
              <span class="ml-1 text-gray-500 dark:text-gray-400">
                {{ relative(entry.last_joined.at) }}
              </span>
            </dd>
            <dd v-else class="text-gray-400 dark:text-gray-500">
              {{ t('calendar.activity.noEntry') }}
            </dd>
          </div>

          <!--
            The withdrawal is the line the owner opened this section for, so it is the
            only one that carries colour. It is absent far more often than it is
            present: it is only ever recorded when the date had already reached its
            threshold.
          -->
          <div v-if="entry.last_withdrawn" class="flex flex-wrap gap-x-2">
            <dt class="text-danger-600 dark:text-danger-400">
              {{ t('calendar.activity.lastWithdrawn') }}
            </dt>
            <dd class="text-danger-600 dark:text-danger-400">
              <span class="font-medium">{{ entry.last_withdrawn.name }}</span>
              <span class="ml-1 opacity-80">{{ relative(entry.last_withdrawn.at) }}</span>
            </dd>
          </div>

          <div class="flex flex-wrap gap-x-2">
            <dt class="text-gray-500 dark:text-gray-400">
              {{ t('calendar.activity.available') }}
            </dt>
            <dd v-if="entry.available.length > 0" class="text-gray-900 dark:text-gray-100">
              {{ entry.available.map(p => p.name).join(', ') }}
            </dd>
            <dd v-else class="text-gray-400 dark:text-gray-500">
              {{ t('calendar.activity.noneAvailable') }}
            </dd>
          </div>
        </dl>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
/**
 * The owner-facing activity journal, one row per date.
 *
 * A component rather than markup inlined in CalendarSettings.vue so that "renders with
 * entries" and "renders without entries" are testable without mounting a view that
 * needs a router, a store and a calendar.
 *
 * It fetches nothing itself: the parent owns the range and the request, which keeps the
 * lazy load on section open where it belongs and leaves this purely a renderer.
 */
import { useI18n } from 'vue-i18n';
import type { DateActivityEntry } from '@/types';
import { formatISODate, formatRelativeTime } from '@/utils/date/intlFormatters';
import type { ISODate } from '@/utils/date/isoDate';

defineProps<{
  entries: DateActivityEntry[];
  loading: boolean;
  error: boolean;
}>();

const { t, locale } = useI18n();

/** An RFC 3339 instant from the API, as "3 days ago" in the active locale. */
function relative(at: string): string {
  return formatRelativeTime(new Date(at), locale.value);
}
</script>
