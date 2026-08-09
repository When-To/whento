/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Turning a week-grid gesture into a batch of availability changes.
 *
 * This is the most intricate domain logic in the calendar, and it lived inline in a
 * pointer-up handler where it could not be tested: four cases for a single-slot click,
 * three for a range removal, an adjacency merge for a range addition, and a pre-check
 * that aborts the *whole* batch rather than splitting an availability in two.
 *
 * The rules it encodes come from the backend's one-availability-per-participant-per-day
 * constraint: a day holds at most one range, so anything that would leave two is
 * refused rather than silently applied.
 *
 * Behaviour is ported verbatim, including the string-comparison of `HH:MM` values and
 * the treatment of absent times as `00:00`/`23:59`.
 */

import type { Availability } from '@/types';
import type { AvailabilityOperation } from '@/types/calendar';
import type { ISODate } from '@/utils/date/isoDate';
import { addMinutesCapped, timeToMinutes, type HHMM } from '@/utils/date/timeRange';

/** What a gesture asked for, once the drag rectangle has been normalized. */
export interface RangeSelection {
  readonly dates: readonly ISODate[];
  /** Inclusive start of the first selected slot. */
  readonly startTime: HHMM;
  /** Exclusive end of the last selected slot. */
  readonly endTime: HHMM;
}

export interface OperationResult {
  readonly operations: AvailabilityOperation[];
  /**
   * Set when the selection would cut an availability in two. The caller must discard
   * the whole batch and tell the user — a partial apply would corrupt the day.
   */
  readonly splitRefused: boolean;
}

const EMPTY: readonly Availability[] = [];

function boundsOf(availability: Availability): { start: HHMM; end: HHMM } {
  return {
    start: availability.start_time || '00:00',
    end: availability.end_time || '23:59',
  };
}

/**
 * Add a range to a day, merging with whatever is already there.
 *
 * Returns null when the day already holds an availability that the new range neither
 * touches nor overlaps: the day cannot hold two, and the caller decides whether that is
 * an error or just a skipped day in a multi-day selection.
 */
export function createOrExtend(
  date: ISODate,
  existing: Availability | undefined,
  startTime: HHMM,
  endTime: HHMM
): AvailabilityOperation | null {
  if (!existing) {
    return { type: 'create', date, startTime, endTime };
  }

  const { start, end } = boundsOf(existing);
  const newStart = timeToMinutes(startTime);
  const newEnd = timeToMinutes(endTime);
  const oldStart = timeToMinutes(start);
  const oldEnd = timeToMinutes(end);

  // Touching counts as mergeable, not just overlapping: dragging the slot right after
  // an existing range should extend it rather than be refused.
  const mergeable = newStart <= oldEnd && newEnd >= oldStart;
  if (!mergeable) return null;

  const mergedStart = Math.min(newStart, oldStart);
  const mergedEnd = Math.max(newEnd, oldEnd);

  return {
    type: 'update',
    date,
    oldStartTime: start,
    oldEndTime: end,
    // Capped at 23:59 so a range reaching midnight does not wrap to 00:00.
    startTime: addMinutesCapped('00:00', mergedStart),
    endTime: addMinutesCapped('00:00', mergedEnd),
  };
}

/**
 * Toggle a single slot.
 *
 * On an empty slot this adds one slot's worth of availability. On a covered slot it
 * removes it, in one of three ways: shrink from the start, shrink from the end, or —
 * for a single-slot range, or a click in the middle where shrinking is impossible
 * without splitting — delete the whole thing.
 */
export function toggleSlot(input: {
  readonly date: ISODate;
  readonly time: HHMM;
  readonly slotDurationMin: number;
  readonly dayAvailabilities: readonly Availability[];
}): OperationResult {
  const { date, time, slotDurationMin } = input;
  const availabilities = input.dayAvailabilities ?? EMPTY;

  const covering = availabilities.find(availability => {
    const { start, end } = boundsOf(availability);
    return time >= start && time < end;
  });

  if (!covering) {
    const operation = createOrExtend(
      date,
      availabilities[0],
      time,
      addMinutesCapped(time, slotDurationMin)
    );
    return { operations: operation ? [operation] : [], splitRefused: false };
  }

  const { start, end } = boundsOf(covering);
  const slotEnd = addMinutesCapped(time, slotDurationMin);
  const isSingleSlot = addMinutesCapped(start, slotDurationMin) >= end;
  const isFirstSlot = time === start;
  const isLastSlot = slotEnd >= end;

  if (isSingleSlot || (!isFirstSlot && !isLastSlot)) {
    return {
      operations: [{ type: 'delete', date: covering.date, startTime: start, endTime: end }],
      splitRefused: false,
    };
  }

  if (isFirstSlot) {
    return {
      operations: [
        {
          type: 'update',
          date: covering.date,
          oldStartTime: start,
          oldEndTime: end,
          startTime: addMinutesCapped(start, slotDurationMin),
          endTime: end,
        },
      ],
      splitRefused: false,
    };
  }

  return {
    operations: [
      {
        type: 'update',
        date: covering.date,
        oldStartTime: start,
        oldEndTime: end,
        startTime: start,
        endTime: time,
      },
    ],
    splitRefused: false,
  };
}

/** Add the selected range to every selected day, skipping days that cannot take it. */
export function addRange(
  selection: RangeSelection,
  availabilitiesFor: (date: ISODate) => readonly Availability[]
): OperationResult {
  const operations: AvailabilityOperation[] = [];

  for (const date of selection.dates) {
    const operation = createOrExtend(
      date,
      availabilitiesFor(date)[0],
      selection.startTime,
      selection.endTime
    );
    // A day whose existing availability is disjoint from the selection is skipped
    // silently; the caller reports only a batch that ends up entirely empty.
    if (operation) operations.push(operation);
  }

  return { operations, splitRefused: false };
}

/**
 * Remove the selected range from every selected day.
 *
 * Refuses the entire batch if any day would be split in two, rather than applying the
 * days that happen to work — a half-applied removal is worse than none.
 */
export function removeRange(
  selection: RangeSelection,
  availabilitiesFor: (date: ISODate) => readonly Availability[]
): OperationResult {
  const { startTime, endTime } = selection;

  const overlappingFor = (date: ISODate) =>
    availabilitiesFor(date).filter(availability => {
      const { start, end } = boundsOf(availability);
      return start < endTime && end > startTime;
    });

  // Pre-check every day before emitting anything.
  for (const date of selection.dates) {
    for (const availability of overlappingFor(date)) {
      const { start, end } = boundsOf(availability);
      if (startTime > start && endTime < end) {
        return { operations: [], splitRefused: true };
      }
    }
  }

  const operations: AvailabilityOperation[] = [];

  for (const date of selection.dates) {
    for (const availability of overlappingFor(date)) {
      const { start, end } = boundsOf(availability);

      if (startTime <= start && endTime >= end) {
        // Fully covered: drop it.
        operations.push({
          type: 'delete',
          date: availability.date,
          startTime: start,
          endTime: end,
        });
      } else if (startTime <= start) {
        // Cuts the beginning: keep the tail.
        operations.push({
          type: 'update',
          date: availability.date,
          oldStartTime: start,
          oldEndTime: end,
          startTime: endTime,
          endTime: end,
        });
      } else {
        // Cuts the end: keep the head. The middle case was refused above.
        operations.push({
          type: 'update',
          date: availability.date,
          oldStartTime: start,
          oldEndTime: end,
          startTime: start,
          endTime: startTime,
        });
      }
    }
  }

  return { operations, splitRefused: false };
}

/**
 * Mark a whole day as available, or clear it.
 *
 * Used by the day-header gesture. Empty start and end times mean "all day"; that
 * convention is part of the contract with `ParticipantView`.
 */
export function toggleFullDay(
  date: ISODate,
  existing: Availability | undefined
): AvailabilityOperation {
  if (!existing) {
    return { type: 'create', date, startTime: '', endTime: '' };
  }
  const { start, end } = boundsOf(existing);
  return { type: 'delete', date, startTime: start, endTime: end };
}
