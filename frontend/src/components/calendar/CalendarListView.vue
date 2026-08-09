<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { DayModel } from '@/types/calendar';

/**
 * The actionable days of the visible period, as a flat list.
 *
 * The mobile default, and the accessible fallback for the grids. It reads the same
 * `DayModel` they do, so a day cannot look available here and blocked there — which is
 * exactly what happened when this view carried its own copy of the date rules.
 */

interface Props {
  days: readonly DayModel[];
  label: string;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'day-click', date: string): void;
  (e: 'day-details', date: string, anchor: DOMRect): void;
  (e: 'previous'): void;
  (e: 'next'): void;
}>();

const { t } = useI18n();

function openDetails(date: string, event: Event) {
  const row =
    (event.currentTarget as HTMLElement | null)?.closest<HTMLElement>('[data-date]') ??
    (event.target as HTMLElement | null)?.closest<HTMLElement>('[data-date]');
  if (row) emit('day-details', date, row.getBoundingClientRect());
}
</script>

<template>
  <section class="card p-3 md:p-4">
    <header class="mb-3 flex items-center justify-between gap-2">
      <button
        type="button"
        class="btn btn-ghost px-2 py-1"
        :aria-label="t('calendar.previousMonth')"
        @click="emit('previous')"
      >
        &lsaquo;
      </button>
      <h3 class="text-base font-semibold capitalize">{{ label }}</h3>
      <button
        type="button"
        class="btn btn-ghost px-2 py-1"
        :aria-label="t('calendar.nextMonth')"
        @click="emit('next')"
      >
        &rsaquo;
      </button>
    </header>

    <p v-if="days.length === 0" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('calendar.noActionableDays') }}
    </p>

    <ul v-else class="space-y-2">
      <li v-for="day in days" :key="day.date">
        <div
          class="cal-row"
          :data-date="day.date"
          :data-status="day.status"
          :data-own="day.own !== null || undefined"
          :data-recurring="day.recurrence !== null || undefined"
          :data-threshold="day.meetsThreshold || undefined"
          :data-today="day.isToday || undefined"
          :data-highlight="day.isHighlighted || undefined"
          @contextmenu.prevent="openDetails(day.date, $event)"
        >
          <button
            type="button"
            class="cal-row-main"
            :aria-label="day.ariaLabel"
            @click="emit('day-click', day.date)"
          >
            <span class="flex min-w-0 flex-col gap-0.5">
              <span class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-semibold">{{ day.weekdayLong }}</span>
                <span class="text-sm opacity-80">{{ day.dateLong }}</span>
                <span v-if="day.isToday" class="badge badge-primary">{{
                  t('calendar.today')
                }}</span>
                <span v-if="day.isHoliday" class="cal-row-badge" data-kind="holiday">
                  {{ day.holidayName || t('calendar.publicHoliday') }}
                </span>
                <span v-else-if="day.isHolidayEve" class="cal-row-badge" data-kind="eve">
                  {{ t('calendar.holidayEve') }}
                </span>
              </span>

              <span class="flex flex-wrap items-center gap-1.5">
                <!-- One-off availabilities and recurrences are both shown; a day can
                     carry either, or both at once. -->
                <span v-for="own in day.ownAll" :key="own.key" class="cal-tag">
                  <span class="cal-dot" />
                  {{ own.label }}
                </span>
                <span v-if="day.recurrence" class="cal-tag">
                  <span class="cal-dot cal-dot--recurring" />
                  {{ day.recurrence.label }}
                </span>
              </span>
            </span>
          </button>

          <div class="flex shrink-0 items-center gap-2">
            <button
              type="button"
              class="cal-count"
              :title="t('calendar.viewParticipantsFor', { date: day.dateLong })"
              @click="openDetails(day.date, $event)"
            >
              {{ day.participantCount }}/{{ day.threshold }}
              <span v-if="day.meetsThreshold" aria-hidden="true"> ✓</span>
            </button>
          </div>
        </div>
      </li>
    </ul>
  </section>
</template>
