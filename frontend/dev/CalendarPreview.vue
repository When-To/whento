<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import CalendarMonthView from '../src/components/calendar/CalendarMonthView.vue';
import { useCalendarFormatters } from '../src/composables/calendar/useCalendarFormatters';
import { buildCalendarRules } from '../src/utils/calendar/dateRules';
import { buildDayIndex } from '../src/utils/calendar/dayIndex';
import { buildMonthModel, type ModelDeps } from '../src/utils/calendar/dayModel';
import { getHolidayIndex } from '../src/utils/calendar/holidays';
import { getWeekStartDay } from '../src/i18n';
import type { Availability, RecurrenceWithExceptions } from '../src/types';

const { locale } = useI18n();
const { formatters } = useCalendarFormatters();

/** Fixed so screenshots are stable across runs. */
const YEAR = 2026;
const MONTH = 3; // April
const THRESHOLD = 5;
const TODAY = '2026-04-06';

function iso(day: number): string {
  return `2026-04-${String(day).padStart(2, '0')}`;
}

function availability(day: number, start?: string, end?: string, note?: string): Availability {
  return {
    id: `av-${day}`,
    participant_id: 'p1',
    participant_name: 'Ada',
    participant_email_verified: false,
    date: iso(day),
    start_time: start,
    end_time: end,
    note,
    created_at: '',
    updated_at: '',
  };
}

/** One of every state the cell can be in, laid out across a real month. */
const availabilities: Availability[] = [
  availability(8, '09:00', '12:00'),
  availability(9),
  availability(15, '14:00', '18:00', 'after the standup'),
  availability(16, '09:00', '12:00'),
  availability(16, '14:00', '18:00'),
  availability(23, '08:00', '10:00'),
];

const recurrences: RecurrenceWithExceptions[] = [
  {
    id: 'r1',
    participant_id: 'p1',
    day_of_week: 4, // Thursday
    start_time: '10:00',
    end_time: '11:00',
    start_date: '2026-04-01',
    created_at: '',
    exceptions: [{ id: 'e1', recurrence_id: 'r1', excluded_date: iso(23), created_at: '' }],
  },
];

/** A spread of counts so every density step is represented. */
const participantCounts: Record<string, number> = {
  [iso(7)]: 1,
  [iso(8)]: 2,
  [iso(9)]: 3,
  [iso(10)]: 4,
  [iso(13)]: 5,
  [iso(14)]: 1,
  [iso(15)]: 3,
  [iso(16)]: 5,
  [iso(17)]: 6,
  [iso(20)]: 2,
  [iso(21)]: 4,
  [iso(22)]: 5,
  [iso(23)]: 2,
  [iso(24)]: 1,
  [iso(27)]: 3,
  [iso(28)]: 5,
  [iso(29)]: 4,
  [iso(30)]: 2,
};

const deps = computed<ModelDeps>(() => ({
  rules: buildCalendarRules({
    timeZone: 'Europe/Paris',
    todayISO: TODAY,
    allowedWeekdays: [1, 2, 3, 4, 5],
    holidaysPolicy: 'ignore',
    allowHolidayEves: true,
    threshold: THRESHOLD,
  }),
  holidays: getHolidayIndex('Europe/Paris', locale.value),
  index: buildDayIndex({ availabilities, recurrences, participantCounts }),
  fmt: formatters.value,
  weekStartDay: getWeekStartDay(locale.value),
  highlighted: new Set([iso(22)]),
}));

const model = computed(() => buildMonthModel(YEAR, MONTH, deps.value));

const dark = ref(false);
function applyTheme() {
  document.documentElement.classList.toggle('dark', dark.value);
}
function toggleTheme() {
  dark.value = !dark.value;
  applyTheme();
}

onMounted(() => {
  const params = new URLSearchParams(window.location.search);
  dark.value = params.get('theme') === 'dark';
  applyTheme();
});

/** The two candidate density ramps, shown side by side for comparison. */
const RAMPS = [
  {
    name: 'sand → green (current)',
    light: ['#d4a843', '#65a30d', '#15803d', '#166534'],
    dark: ['#b45309', '#65a30d', '#22c55e', '#4ade80'],
  },
  {
    name: 'single-hue green (alternative)',
    light: ['#22c55e', '#16a34a', '#15803d', '#166534'],
    dark: ['#166534', '#15803d', '#16a34a', '#22c55e'],
  },
];
</script>

<template>
  <main class="min-h-screen bg-gray-50 p-6 dark:bg-gray-950">
    <div class="mx-auto max-w-5xl space-y-6">
      <header class="flex items-center justify-between">
        <div>
          <h1 class="text-xl">Calendar preview</h1>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            April 2026 · threshold {{ THRESHOLD }} · today {{ TODAY }} · weekdays Mon–Fri
          </p>
        </div>
        <div class="flex gap-2">
          <button class="btn btn-secondary" @click="locale = locale === 'en' ? 'fr' : 'en'">
            {{ locale }}
          </button>
          <button class="btn btn-secondary" @click="toggleTheme">
            {{ dark ? 'dark' : 'light' }}
          </button>
        </div>
      </header>

      <section class="card space-y-3 p-4" data-testid="ramps">
        <h2 class="text-base">Density ramps</h2>
        <div v-for="ramp in RAMPS" :key="ramp.name" class="flex items-center gap-3 text-sm">
          <span class="w-56 shrink-0 text-gray-600 dark:text-gray-400">{{ ramp.name }}</span>
          <span class="flex overflow-hidden rounded border border-gray-300 dark:border-gray-600">
            <span
              v-for="(hex, i) in dark ? ramp.dark : ramp.light"
              :key="i"
              class="flex h-8 w-16 items-center justify-center text-xs"
              :style="{
                backgroundColor: hex,
                color: i < 2 && !dark ? '#111827' : i === 0 && dark ? '#f3f4f6' : '#111827',
              }"
            >
              {{ i + 1 }}
            </span>
          </span>
          <span class="text-xs text-gray-500">0 = empty cell surface</span>
        </div>
      </section>

      <CalendarMonthView :model="model" show-navigation data-testid="month" />

      <section class="card p-4 text-sm text-gray-600 dark:text-gray-400">
        <h2 class="mb-2 text-base text-gray-900 dark:text-gray-100">What to look for</h2>
        <ul class="list-inside list-disc space-y-1">
          <li>Weekends and the first week are disabled — dashed border, no gauge.</li>
          <li>8 and 9 April carry a one-off availability; 16 April carries two.</li>
          <li>Every Thursday carries a recurrence, except 23 April which is an exception.</li>
          <li>16 April also carries a recurrence — both chips must be visible.</li>
          <li>13, 16, 22 and 28 April meet the threshold — check mark and top ramp step.</li>
          <li>22 April is highlighted; 6 April is today.</li>
          <li>1 May is a public holiday and 30 April its eve — check the next month.</li>
          <li>Narrow the window below 768px: the grid transposes with no JavaScript.</li>
        </ul>
      </section>
    </div>
  </main>
</template>
