<!--
  WhenTo - Collaborative event calendar for self-hosted environments
  Copyright (C) 2025 WhenTo Contributors
  SPDX-License-Identifier: BSL-1.1
-->

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Availability } from '@/types';
import type { AvailabilityOperation } from '@/types/calendar';
import type { SlotCell, WeekModel } from '@/utils/calendar/weekModel';
import { addRange, removeRange, toggleFullDay, toggleSlot } from '@/utils/calendar/availabilityOps';
import { addMinutesCapped } from '@/utils/date/timeRange';
import { useDragSelection, type DragTarget } from '@/composables/calendar/useDragSelection';

interface Props {
  model: WeekModel;
  slotDurationMin: number;
  /** Participants needed for a slot to count as workable. */
  threshold: number;
  /** Look up the current participant's availabilities for a date. */
  availabilitiesFor: (date: string) => readonly Availability[];
  showNavigation?: boolean;
}

const props = withDefaults(defineProps<Props>(), { showNavigation: false });

const emit = defineEmits<{
  (e: 'batch-operations', operations: AvailabilityOperation[]): void;
  (e: 'split-refused'): void;
  (e: 'no-op'): void;
  (e: 'day-details', date: string, anchor: DOMRect): void;
  (e: 'week-change', startISO: string): void;
}>();

const { t } = useI18n();

const gridRef = ref<HTMLElement | null>(null);
const headerRef = ref<HTMLElement | null>(null);

const cellByKey = computed(() => new Map(props.model.cells.map(cell => [cell.key, cell])));

/** Column height in rows, so the band overlay can span the whole day. */
const slotCount = computed(() => props.model.slots.length);

/** Closing time of the last row, which has no row of its own to label. */
const endLabel = computed(() => {
  const last = props.model.slots[props.model.slots.length - 1];
  if (!last) return '';
  const end = last.startMin + props.slotDurationMin;
  return `${String(Math.floor(end / 60)).padStart(2, '0')}:${String(end % 60).padStart(2, '0')}`;
});

/** Spacing of the hour separators, expressed in slot heights. */
const hourHeight = computed(
  () => `calc(var(--cal-slot-h) * ${60 / Math.max(1, props.slotDurationMin)})`
);

function dispatch(result: { operations: AvailabilityOperation[]; splitRefused: boolean }) {
  if (result.splitRefused) {
    emit('split-refused');
    return;
  }
  if (result.operations.length === 0) {
    emit('no-op');
    return;
  }
  emit('batch-operations', result.operations);
}

// ---------------------------------------------------------------- slot drag

interface SlotTarget extends DragTarget {
  readonly dayIndex: number;
  readonly slotIndex: number;
}

const slotDrag = useDragSelection<SlotTarget>({
  container: gridRef,
  resolve: element => {
    const node = element.closest<HTMLElement>('[data-slot-key]');
    const cell = node?.dataset.slotKey ? cellByKey.value.get(node.dataset.slotKey) : undefined;
    return cell ? { key: cell.key, dayIndex: cell.dayIndex, slotIndex: cell.slotIndex } : null;
  },
  canStart: target => cellByKey.value.get(target.key)?.enabled ?? false,
  initialMode: target => (cellByKey.value.get(target.key)?.hasOwn ? 'remove' : 'add'),
  rectangle: (anchor, focus) => {
    const minDay = Math.min(anchor.dayIndex, focus.dayIndex);
    const maxDay = Math.max(anchor.dayIndex, focus.dayIndex);
    const minSlot = Math.min(anchor.slotIndex, focus.slotIndex);
    const maxSlot = Math.max(anchor.slotIndex, focus.slotIndex);

    const targets: SlotTarget[] = [];
    for (let dayIndex = minDay; dayIndex <= maxDay; dayIndex++) {
      for (let slotIndex = minSlot; slotIndex <= maxSlot; slotIndex++) {
        const cell = props.model.cells[dayIndex * slotCount.value + slotIndex];
        if (cell) targets.push({ key: cell.key, dayIndex, slotIndex });
      }
    }
    return targets;
  },
  commit: result => {
    if (result.targets.length === 0) return;

    const anchorCell = cellByKey.value.get(result.anchor.key);
    if (!anchorCell) return;

    // A tap, or a gesture that never left its cell, toggles that one slot.
    if (result.isTap || result.targets.length === 1) {
      dispatch(
        toggleSlot({
          date: anchorCell.date,
          time: anchorCell.time,
          slotDurationMin: props.slotDurationMin,
          dayAvailabilities: props.availabilitiesFor(anchorCell.date),
        })
      );
      return;
    }

    const cells = result.targets
      .map(target => cellByKey.value.get(target.key))
      .filter((cell): cell is SlotCell => !!cell);

    const dates = [...new Set(cells.map(cell => cell.date))];
    const times = cells.map(cell => cell.time).sort();
    const selection = {
      dates,
      startTime: times[0],
      // The last slot is included, so the range runs to the end of it.
      endTime: addMinutesCapped(times[times.length - 1], props.slotDurationMin),
    };

    dispatch(
      result.mode === 'remove'
        ? removeRange(selection, props.availabilitiesFor)
        : addRange(selection, props.availabilitiesFor)
    );
  },
});

// -------------------------------------------------------------- header drag

interface DayTarget extends DragTarget {
  readonly dayIndex: number;
}

const headerDrag = useDragSelection<DayTarget>({
  container: headerRef,
  resolve: element => {
    if (isDetailsControl(element)) return null;
    const node = element.closest<HTMLElement>('[data-day-index]');
    const index = node?.dataset.dayIndex;
    if (index === undefined) return null;
    const column = props.model.columns[Number(index)];
    return column ? { key: column.day.date, dayIndex: Number(index) } : null;
  },
  canStart: target => props.model.columns[target.dayIndex]?.enabled ?? false,
  initialMode: target => (props.model.columns[target.dayIndex]?.hasFullDayOwn ? 'remove' : 'add'),
  rectangle: (anchor, focus) => {
    const min = Math.min(anchor.dayIndex, focus.dayIndex);
    const max = Math.max(anchor.dayIndex, focus.dayIndex);
    const targets: DayTarget[] = [];
    for (let dayIndex = min; dayIndex <= max; dayIndex++) {
      const column = props.model.columns[dayIndex];
      if (column) targets.push({ key: column.day.date, dayIndex });
    }
    return targets;
  },
  commit: result => {
    const operations: AvailabilityOperation[] = [];
    for (const target of result.targets) {
      const column = props.model.columns[target.dayIndex];
      if (!column) continue;
      operations.push(toggleFullDay(column.day.date, props.availabilitiesFor(column.day.date)[0]));
    }
    dispatch({ operations, splitRefused: false });
  },
});

function slotDragState(cell: SlotCell): 'add' | 'remove' | null {
  return slotDrag.selected.value.has(cell.key) ? slotDrag.mode.value : null;
}

function headerDragState(date: string): 'add' | 'remove' | null {
  return headerDrag.selected.value.has(date) ? headerDrag.mode.value : null;
}

function openDetails(date: string, event: Event) {
  const element = (event.currentTarget as HTMLElement | null)?.closest<HTMLElement>(
    '[data-day-index]'
  );
  if (element) emit('day-details', date, element.getBoundingClientRect());
}

/** Right-click on any slot opens the details for the day it belongs to. */
function openDetailsFromSlot(event: Event) {
  const node = (event.target as Element | null)?.closest<HTMLElement>('[data-slot-key]');
  const cell = node?.dataset.slotKey ? cellByKey.value.get(node.dataset.slotKey) : undefined;
  if (!cell) return;
  const column = headerRef.value?.querySelector<HTMLElement>(`[data-day-index="${cell.dayIndex}"]`);
  emit('day-details', cell.date, (column ?? node!).getBoundingClientRect());
}

/** Header cells are a drag surface, so the details control must opt out of it. */
function isDetailsControl(element: Element): boolean {
  return !!element.closest('[data-no-drag]');
}

function shiftWeek(days: number) {
  const date = new Date(
    Number(props.model.startISO.slice(0, 4)),
    Number(props.model.startISO.slice(5, 7)) - 1,
    Number(props.model.startISO.slice(8, 10)) + days
  );
  const iso = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(
    date.getDate()
  ).padStart(2, '0')}`;
  emit('week-change', iso);
}
</script>

<template>
  <section class="card p-3 md:p-4">
    <header v-if="showNavigation" class="mb-3 flex items-center justify-between gap-2">
      <button
        type="button"
        class="btn btn-ghost px-2 py-1"
        :aria-label="t('calendar.previousWeek')"
        @click="shiftWeek(-7)"
      >
        &lsaquo;
      </button>
      <h3 class="text-base font-semibold">
        {{ model.days[0].dateShort }} – {{ model.days[6].dateShort }}
      </h3>
      <button
        type="button"
        class="btn btn-ghost px-2 py-1"
        :aria-label="t('calendar.nextWeek')"
        @click="shiftWeek(7)"
      >
        &rsaquo;
      </button>
    </header>

    <div class="cal-week">
      <!-- Day headers double as a full-day toggle, dragged across days. -->
      <div ref="headerRef" class="cal-week-header" @pointerdown="headerDrag.onPointerDown">
        <span class="cal-week-gutter" aria-hidden="true" />
        <div
          v-for="(column, index) in model.columns"
          :key="column.day.date"
          class="cal-week-day-wrap"
          :data-day-index="index"
        >
          <button
            type="button"
            class="cal-week-day"
            :data-status="column.day.status"
            :data-enabled="column.enabled || undefined"
            :data-today="column.day.isToday || undefined"
            :data-holiday="column.day.isHoliday || undefined"
            :data-full-day="column.hasFullDayOwn || undefined"
            :data-drag="headerDragState(column.day.date) || undefined"
            :aria-label="column.day.ariaLabel"
            :disabled="!column.enabled"
            @contextmenu.prevent="openDetails(column.day.date, $event)"
          >
            <span class="cal-week-day-name">{{ column.day.weekdayShort }}</span>
            <span class="cal-week-day-num">{{ column.day.dayOfMonth }}</span>
          </button>
          <button
            type="button"
            class="cal-count"
            data-no-drag
            :title="t('calendar.viewParticipantsFor', { date: column.day.dateLong })"
            @click.stop="openDetails(column.day.date, $event)"
            @pointerdown.stop
          >
            {{ column.day.participantCount }}/{{ column.day.threshold }}
            <span v-if="column.day.meetsThreshold" aria-hidden="true"> &check;</span>
          </button>
        </div>
      </div>

      <div class="cal-week-body">
        <!-- Hour labels -->
        <div class="cal-week-times" aria-hidden="true">
          <span
            v-for="slot in model.slots"
            :key="slot.time"
            class="cal-week-time"
            :data-hour="slot.isHourStart || undefined"
          >
            {{ slot.isHourStart ? slot.time : '' }}
          </span>
          <!-- The grid has one label per row, so the closing edge of the last row has
               none; without this the end of the day has to be counted out. -->
          <span class="cal-week-time cal-week-time--last" data-hour>{{ endLabel }}</span>
        </div>

        <!-- One grid for every slot, rather than one grid container per row. -->
        <div
          ref="gridRef"
          class="cal-week-grid"
          role="grid"
          :style="{ '--cal-slot-count': slotCount, '--cal-hour-h': hourHeight }"
          @pointerdown="slotDrag.onPointerDown"
          @contextmenu.prevent="openDetailsFromSlot"
        >
          <div
            v-for="cell in model.cells"
            :key="cell.key"
            class="cal-slot"
            role="gridcell"
            :data-slot-key="cell.key"
            :data-enabled="cell.enabled || undefined"
            :data-own="cell.hasOwn || undefined"
            :aria-selected="cell.hasOwn || undefined"
            :data-hour="cell.isHourStart || undefined"
            :data-drag="slotDragState(cell) || undefined"
            :style="{ gridColumn: cell.dayIndex + 1, gridRow: cell.slotIndex + 1 }"
          />

          <!-- Coverage and threshold bands, one overlay per day column. Far fewer
               nodes than the per-cell fills they replace. -->
          <div
            v-for="(column, index) in model.columns"
            :key="`bands-${column.day.date}`"
            class="cal-week-bands"
            :style="{ gridColumn: index + 1, gridRow: `1 / span ${slotCount}` }"
            aria-hidden="true"
          >
            <div
              v-for="(band, bandIndex) in column.bands"
              :key="`${band.kind}-${band.startMin}-${bandIndex}`"
              class="cal-band"
              :data-kind="band.kind"
              :style="{ top: band.top, height: band.height }"
            >
              <template v-if="band.kind !== 'threshold' && band.count > 0">
                <span class="cal-band-count">{{ band.count }}/{{ threshold }}</span>
                <!-- Same progress reading as a month cell: how close this span is to
                     a workable date, without having to compare the two numbers. -->
                <span class="cal-band-gauge">
                  <span
                    class="cal-band-gauge-fill"
                    :style="{
                      width: `${Math.min(100, Math.round((band.count / threshold) * 100))}%`,
                    }"
                  />
                </span>
              </template>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
