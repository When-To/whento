<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { DayModel } from '@/types/calendar';

interface Props {
  day: DayModel;
  /** Selection state while a drag is in progress. */
  drag?: 'add' | 'remove' | null;
  /** The one cell in the grid's tab order, for roving tabindex. */
  focused?: boolean;
}

const props = withDefaults(defineProps<Props>(), { drag: null, focused: false });

const emit = defineEmits<{
  (e: 'remove-recurrence', recurrenceId: string, date: string): void;
  (e: 'show-details', date: string): void;
}>();

const { t } = useI18n();

const isInteractive = computed(
  () => props.day.status !== 'outside' && props.day.status !== 'disabled'
);

/** Gauge width, capped at 100%. Repeats the density without relying on hue. */
const gaugeWidth = computed(() => `${Math.round(props.day.density * 100)}%`);

const holidayTitle = computed(() => {
  if (props.day.isHoliday) return props.day.holidayName ?? t('calendar.publicHoliday');
  if (props.day.isHolidayEve) return t('calendar.holidayEve');
  return undefined;
});
</script>

<template>
  <div
    class="cal-cell"
    role="gridcell"
    :data-date="day.date"
    :data-status="day.status"
    :data-own="day.own !== null || undefined"
    :data-recurring="day.recurrence !== null || undefined"
    :data-threshold="day.meetsThreshold || undefined"
    :data-today="day.isToday || undefined"
    :data-holiday="day.isHoliday || undefined"
    :data-holiday-eve="(!day.isHoliday && day.isHolidayEve) || undefined"
    :data-highlight="day.isHighlighted || undefined"
    :data-drag="drag || undefined"
    :aria-label="day.ariaLabel"
    :aria-disabled="!isInteractive || undefined"
    :aria-selected="day.own !== null || undefined"
    :tabindex="isInteractive ? (focused ? 0 : -1) : -1"
  >
    <div class="cal-cell-head">
      <span class="cal-day-num">{{ day.dayOfMonth }}</span>
      <span
        v-if="day.meetsThreshold"
        class="cal-badge"
        :title="t('calendar.thresholdMet')"
        aria-hidden="true"
        >✓</span
      >
      <span v-else-if="holidayTitle" class="cal-badge" :title="holidayTitle" aria-hidden="true"
        >★</span
      >
    </div>

    <!-- One-off availabilities: every time range for the day, not just the first. -->
    <!-- A day can carry a one-off availability *and* a recurrence at the same time;
         the previous grid collapsed the two and the recurrence disappeared. -->
    <span v-for="own in day.ownAll" :key="own.key" class="cal-tag" :title="own.note || own.label">
      <span class="cal-dot" />
      {{ own.label }}
    </span>

    <!-- A recurrence stays visible even when a one-off availability also covers the day. -->
    <span
      v-if="day.recurrence"
      class="cal-tag"
      :title="`${t('availability.recurring')} · ${day.recurrence.label}`"
    >
      <span class="cal-dot cal-dot--recurring" />
      <span class="truncate">{{ day.recurrence.label }}</span>
      <!-- Skip this one occurrence of the recurrence. -->
      <button
        v-if="!day.isPast"
        type="button"
        data-no-drag
        class="cal-tag-action"
        :title="t('availability.addException')"
        :aria-label="t('availability.addException')"
        @click.stop="emit('remove-recurrence', day.recurrence!.id, day.date)"
        @pointerdown.stop
      >
        &times;
      </button>
    </span>

    <div
      v-if="day.status !== 'outside' && day.status !== 'disabled'"
      class="cal-gauge"
      role="presentation"
    >
      <div class="cal-gauge-fill" :style="{ width: gaugeWidth }" />
    </div>

    <!-- Opening the per-participant breakdown needs its own affordance: the cell
         itself belongs to the drag surface. -->
    <button
      v-if="day.status !== 'outside' && day.status !== 'disabled'"
      type="button"
      data-no-drag
      class="cal-count"
      :title="t('participant.participantsForDate')"
      @click.stop="emit('show-details', day.date)"
      @pointerdown.stop
    >
      {{ day.participantCount }}/{{ day.threshold }}
      <!-- A small glyph so the control reads as "open details" rather than a label. -->
      <svg
        class="size-2.5 shrink-0 opacity-70"
        viewBox="0 0 20 20"
        fill="currentColor"
        aria-hidden="true"
      >
        <path d="M10 3a7 7 0 100 14 7 7 0 000-14zm.75 10.5h-1.5v-5h1.5v5zm0-6.5h-1.5V5.5h1.5V7z" />
      </svg>
    </button>
  </div>
</template>
