/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * The week grid, resolved once.
 *
 * Every per-cell question the old grid asked from its template — is this day enabled,
 * is this slot inside the opening hours, does the participant already cover it — is
 * answered here, once, in a single pass. `getCellClasses` used to re-derive all of it
 * two to four times per cell, and the opening-hours resolution alone walked the holiday
 * rules on every call.
 */

import type { DayModel } from '@/types/calendar';
import { addDaysISO, startOfWeekISO, type ISODate } from '@/utils/date/isoDate';
import {
  coversTime,
  isFullDay,
  minutesToTime,
  type HHMM,
  type Interval,
} from '@/utils/date/timeRange';
import { isSlotAllowed, resolveTimeWindow } from './dateRules';
import { buildDayModel, type ModelDeps } from './dayModel';
import { buildDayBands, type Band, type CoverageSegment, type GridGeometry } from './segments';

/** One row of the grid. */
export interface SlotSpec {
  readonly time: HHMM;
  readonly startMin: number;
  /** Whether this slot begins a new hour, for the heavier separator. */
  readonly isHourStart: boolean;
}

/** One interactive cell. Everything the template needs is a property read. */
export interface SlotCell {
  /** `${date}:${time}` — the format the drag selection keys on. */
  readonly key: string;
  readonly date: ISODate;
  readonly time: HHMM;
  readonly dayIndex: number;
  readonly slotIndex: number;
  /** Day is open *and* the slot falls inside its opening hours. */
  readonly enabled: boolean;
  /** The current participant already covers this slot. */
  readonly hasOwn: boolean;
  readonly isHourStart: boolean;
}

/** One day column: its model, its gate, and the bands painted over it. */
export interface WeekColumn {
  readonly day: DayModel;
  readonly enabled: boolean;
  readonly hasFullDayOwn: boolean;
  readonly bands: readonly Band[];
}

export interface WeekModel {
  readonly key: string;
  readonly startISO: ISODate;
  readonly endISO: ISODate;
  readonly days: readonly DayModel[];
  readonly slots: readonly SlotSpec[];
  /** Row-major: day 0 slots 0..n, then day 1, and so on. */
  readonly cells: readonly SlotCell[];
  readonly columns: readonly WeekColumn[];
  readonly geometry: GridGeometry;
}

export interface WeekOptions {
  readonly startHour: number;
  readonly endHour: number;
  readonly slotDurationMin: number;
  readonly coverage?: ReadonlyMap<ISODate, CoverageSegment[]>;
  readonly thresholds?: ReadonlyMap<ISODate, Interval[]>;
}

const NO_SEGMENTS: CoverageSegment[] = [];
const NO_INTERVALS: Interval[] = [];

/** The rows of the grid, from `startHour` to `endHour` in `slotDurationMin` steps. */
export function buildSlots(options: WeekOptions): SlotSpec[] {
  const firstMin = options.startHour * 60;
  const lastMin = options.endHour * 60;
  const step = Math.max(1, options.slotDurationMin);

  const slots: SlotSpec[] = [];
  for (let minute = firstMin; minute < lastMin; minute += step) {
    slots.push({
      time: minutesToTime(minute),
      startMin: minute,
      isHourStart: minute % 60 === 0,
    });
  }
  return slots;
}

export function buildWeekModel(
  anchorISO: ISODate,
  deps: ModelDeps,
  options: WeekOptions
): WeekModel {
  const startISO = startOfWeekISO(anchorISO, deps.weekStartDay);
  const slots = buildSlots(options);

  const geometry: GridGeometry = {
    firstSlotMin: options.startHour * 60,
    lastSlotEndMin: options.endHour * 60,
    slotDurationMin: options.slotDurationMin,
    slotCount: slots.length,
  };

  const days: DayModel[] = [];
  const columns: WeekColumn[] = [];
  const cells: SlotCell[] = [];

  let cursor = startISO;
  for (let dayIndex = 0; dayIndex < 7; dayIndex++) {
    const day = buildDayModel(cursor, deps, true);
    days.push(day);

    // Opening hours are resolved once per day, not once per cell.
    const window = resolveTimeWindow(
      {
        date: day.date,
        dayOfWeek: day.dayOfWeek,
        isHoliday: day.isHoliday,
        isHolidayEve: day.isHolidayEve,
      },
      deps.rules
    );
    const dayEnabled = window.allowed;
    const own = deps.index.ownFor(day.date);

    for (let slotIndex = 0; slotIndex < slots.length; slotIndex++) {
      const slot = slots[slotIndex];
      cells.push({
        key: `${day.date}:${slot.time}`,
        date: day.date,
        time: slot.time,
        dayIndex,
        slotIndex,
        enabled:
          dayEnabled &&
          isSlotAllowed(window, slot.startMin, slot.startMin + options.slotDurationMin),
        hasOwn: own.some(availability => coversTime(availability, slot.time)),
        isHourStart: slot.isHourStart,
      });
    }

    columns.push({
      day,
      enabled: dayEnabled,
      hasFullDayOwn: own.some(a => isFullDay(a.start_time, a.end_time)),
      bands: buildDayBands(
        options.coverage?.get(day.date) ?? NO_SEGMENTS,
        options.thresholds?.get(day.date) ?? NO_INTERVALS,
        geometry
      ),
    });

    cursor = addDaysISO(cursor, 1);
  }

  return {
    key: startISO,
    startISO,
    endISO: days[6].date,
    days,
    slots,
    cells,
    columns,
    geometry,
  };
}
