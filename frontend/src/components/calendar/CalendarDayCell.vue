/* * WhenTo - Collaborative event calendar for self-hosted environments * Copyright (C) 2025 WhenTo
Contributors * SPDX-License-Identifier: BSL-1.1 */

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

const { t } = useI18n();

const isInteractive = computed(
  () => props.day.status !== 'outside' && props.day.status !== 'disabled'
);

/** Gauge width, capped at 100%. Repeats the density without relying on hue. */
const gaugeWidth = computed(() => `${Math.round(props.day.density * 100)}%`);

/**
 * A day can carry a one-off availability *and* be covered by a recurrence at the
 * same time. The previous grid collapsed the two into one exclusive status and the
 * recurrence simply disappeared; both are shown here.
 */
const ownLabels = computed(() =>
  props.day.ownAll.map(availability => ({
    key: `${props.day.date}-${availability.start_time ?? 'all'}-${availability.end_time ?? 'day'}`,
    label:
      availability.start_time && availability.end_time
        ? `${availability.start_time}-${availability.end_time}`
        : t('availability.allDay'),
    note: availability.note,
  }))
);

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
    :data-density="day.densityStep || undefined"
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
    <span v-for="own in ownLabels" :key="own.key" class="cal-tag" :title="own.note || own.label">
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
      {{ day.recurrence.label }}
    </span>

    <div
      v-if="day.status !== 'outside' && day.status !== 'disabled'"
      class="cal-gauge"
      role="presentation"
    >
      <div class="cal-gauge-fill" :style="{ width: gaugeWidth }" />
    </div>

    <span v-if="day.status !== 'outside' && day.status !== 'disabled'" class="cal-count">
      {{ day.participantCount }}/{{ day.threshold }}
    </span>
  </div>
</template>
