<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { DayModel, MonthModel } from '@/types/calendar';
import { useDragSelection, type DragTarget } from '@/composables/calendar/useDragSelection';
import { useCalendarGridNav } from '@/composables/calendar/useCalendarGridNav';
import CalendarDayCell from './CalendarDayCell.vue';

interface Props {
  model: MonthModel;
  showNavigation?: boolean;
}

const props = withDefaults(defineProps<Props>(), { showNavigation: false });

const emit = defineEmits<{
  (e: 'day-click', date: string): void;
  (e: 'days-select', dates: string[]): void;
  (e: 'days-deselect', dates: string[]): void;
  (e: 'day-details', date: string, anchor: DOMRect): void;
  (e: 'month-change', year: number, month: number): void;
}>();

const { t } = useI18n();

const gridRef = ref<HTMLElement | null>(null);

/** Only the days of the month itself take part in selection. */
const selectableDays = computed(() => props.model.days.filter(day => day.isCurrentMonth));

const byDate = computed(() => new Map(props.model.days.map(day => [day.date, day])));

interface DayTargetRef extends DragTarget {
  readonly index: number;
}

/**
 * Position in the grid, so a rectangle is the same shape in both layouts: the classic
 * layout reads week down / weekday across, the compact one transposes it, and both
 * derive from the same chronological index.
 */
function positionOf(index: number) {
  return { week: Math.floor(index / 7), weekday: index % 7 };
}

const drag = useDragSelection<DayTargetRef>({
  container: gridRef,
  resolve: element => {
    const cell = element.closest<HTMLElement>('[data-date]');
    const date = cell?.dataset.date;
    if (!date) return null;
    const index = props.model.days.findIndex(day => day.date === date);
    return index >= 0 ? { key: date, index } : null;
  },
  canStart: target => {
    const day = props.model.days[target.index];
    return !!day && day.isCurrentMonth && day.status !== 'disabled';
  },
  initialMode: target => (props.model.days[target.index]?.own ? 'remove' : 'add'),
  rectangle: (anchor, focus) => {
    const a = positionOf(anchor.index);
    const b = positionOf(focus.index);
    const minWeek = Math.min(a.week, b.week);
    const maxWeek = Math.max(a.week, b.week);
    const minDay = Math.min(a.weekday, b.weekday);
    const maxDay = Math.max(a.weekday, b.weekday);

    const targets: DayTargetRef[] = [];
    for (let week = minWeek; week <= maxWeek; week++) {
      for (let weekday = minDay; weekday <= maxDay; weekday++) {
        const index = week * 7 + weekday;
        const day = props.model.days[index];
        if (day) targets.push({ key: day.date, index });
      }
    }
    return targets;
  },
  commit: result => {
    if (result.targets.length === 0) return;

    if (result.isTap || result.targets.length === 1) {
      emit('day-click', result.targets[0].key);
      return;
    }

    const dates = result.targets.map(target => target.key);
    if (result.mode === 'remove') {
      // Only days that actually carry an availability can be removed.
      const removable = dates.filter(date => byDate.value.get(date)?.own);
      if (removable.length > 0) emit('days-deselect', removable);
    } else {
      emit('days-select', dates);
    }
  },
});

// Keyboard navigation shares the drag's commit path, so arrows and pointer cannot
// disagree about what a selection does.
const nav = useCalendarGridNav({
  container: gridRef,
  count: computed(() => props.model.days.length),
  isFocusable: index => {
    const day = props.model.days[index];
    return !!day && day.isCurrentMonth && day.status !== 'disabled';
  },
  activate: index => {
    const day = props.model.days[index];
    if (day) emit('day-click', day.date);
  },
  // The per-participant button inside each cell is out of the tab order, as an
  // ARIA grid requires, so Alt+Enter is how a keyboard reaches it.
  showDetails: index => {
    const day = props.model.days[index];
    if (day) openDetails(day.date);
  },
  commitRange: (anchor, focus) => {
    const from = Math.min(anchor, focus);
    const to = Math.max(anchor, focus);
    const dates: string[] = [];
    let anyWithoutOwn = false;
    for (let index = from; index <= to; index++) {
      const day = props.model.days[index];
      if (!day || !day.isCurrentMonth || day.status === 'disabled') continue;
      dates.push(day.date);
      if (!day.own) anyWithoutOwn = true;
    }
    if (dates.length === 0) return;
    // Mirrors the pointer drag: a range that is not already fully covered adds.
    if (anyWithoutOwn) emit('days-select', dates);
    else emit('days-deselect', dates);
  },
  shiftPeriod: delta => (delta > 0 ? goToNextMonth() : goToPreviousMonth()),
});

function dragStateFor(day: DayModel, index: number): 'add' | 'remove' | null {
  if (drag.selected.value.has(day.date)) return drag.mode.value;
  return nav.rangeIndices.value.has(index) ? 'add' : null;
}

function goToPreviousMonth() {
  const month = props.model.month === 0 ? 11 : props.model.month - 1;
  const year = props.model.month === 0 ? props.model.year - 1 : props.model.year;
  emit('month-change', year, month);
}

function goToNextMonth() {
  const month = props.model.month === 11 ? 0 : props.model.month + 1;
  const year = props.model.month === 11 ? props.model.year + 1 : props.model.year;
  emit('month-change', year, month);
}

/** Open the participant details for a day, anchored to its cell. */
function openDetails(date: string) {
  const day = byDate.value.get(date);
  if (!day || !day.isCurrentMonth || day.status === 'disabled') return;
  const cell = gridRef.value?.querySelector<HTMLElement>(`[data-date="${date}"]`);
  if (!cell) return;
  emit('day-details', date, cell.getBoundingClientRect());
}

/** Right-click anywhere on a cell is a second, mouse-friendly way in. */
function openDetailsFromEvent(event: Event) {
  const cell = (event.target as Element | null)?.closest<HTMLElement>('[data-date]');
  if (cell?.dataset.date) openDetails(cell.dataset.date);
}

defineExpose({ selectableDays });
</script>

<template>
  <section class="card p-3 md:p-4">
    <header v-if="showNavigation" class="mb-3 flex items-center justify-between gap-2">
      <button
        type="button"
        class="btn btn-ghost px-2 py-1"
        data-testid="month-prev"
        :aria-label="t('calendar.previousMonth')"
        @click="goToPreviousMonth"
      >
        &lsaquo;
      </button>
      <h3 class="text-base font-semibold capitalize">{{ model.label }}</h3>
      <button
        type="button"
        class="btn btn-ghost px-2 py-1"
        data-testid="month-next"
        :aria-label="t('calendar.nextMonth')"
        @click="goToNextMonth"
      >
        &rsaquo;
      </button>
    </header>
    <h3 v-else class="mb-3 text-base font-semibold capitalize">{{ model.label }}</h3>

    <div class="cal-month">
      <div class="cal-weekdays" aria-hidden="true">
        <span v-for="(label, i) in model.weekdayHeaders" :key="i" class="cal-weekday">
          {{ label }}
        </span>
      </div>

      <div
        ref="gridRef"
        class="cal-grid"
        role="grid"
        :aria-label="model.label"
        @pointerdown="drag.onPointerDown"
        @keydown="nav.onKeydown"
        @contextmenu.prevent="openDetailsFromEvent"
      >
        <CalendarDayCell
          v-for="(day, index) in model.days"
          :key="day.date"
          :day="day"
          :drag="dragStateFor(day, index)"
          :focused="index === nav.tabStopIndex.value"
          @show-details="openDetails"
        />
      </div>
    </div>
  </section>
</template>
