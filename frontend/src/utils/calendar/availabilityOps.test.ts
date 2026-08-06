/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import type { Availability } from '@/types';
import {
  addRange,
  createOrExtend,
  removeRange,
  toggleFullDay,
  toggleSlot,
} from './availabilityOps';

const DATE = '2026-04-08';

function av(start?: string, end?: string, date = DATE): Availability {
  return {
    id: `av-${date}-${start ?? 'all'}`,
    participant_id: 'p1',
    participant_name: 'Ada',
    participant_email_verified: false,
    date,
    start_time: start,
    end_time: end,
    created_at: '',
    updated_at: '',
  };
}

describe('createOrExtend', () => {
  it('creates when the day is empty', () => {
    expect(createOrExtend(DATE, undefined, '09:00', '10:00')).toEqual({
      type: 'create',
      date: DATE,
      startTime: '09:00',
      endTime: '10:00',
    });
  });

  it('merges an overlapping range', () => {
    expect(createOrExtend(DATE, av('09:00', '12:00'), '11:00', '14:00')).toEqual({
      type: 'update',
      date: DATE,
      oldStartTime: '09:00',
      oldEndTime: '12:00',
      startTime: '09:00',
      endTime: '14:00',
    });
  });

  it('merges a range that only touches, in either direction', () => {
    expect(createOrExtend(DATE, av('09:00', '12:00'), '12:00', '14:00')?.endTime).toBe('14:00');
    expect(createOrExtend(DATE, av('12:00', '14:00'), '09:00', '12:00')?.startTime).toBe('09:00');
  });

  it('absorbs a range already inside the existing one', () => {
    expect(createOrExtend(DATE, av('09:00', '17:00'), '11:00', '12:00')).toEqual({
      type: 'update',
      date: DATE,
      oldStartTime: '09:00',
      oldEndTime: '17:00',
      startTime: '09:00',
      endTime: '17:00',
    });
  });

  it('refuses a disjoint range, since a day holds only one', () => {
    expect(createOrExtend(DATE, av('09:00', '10:00'), '14:00', '15:00')).toBeNull();
  });

  it('caps a merged end at 23:59 rather than wrapping', () => {
    expect(createOrExtend(DATE, av('22:00', '23:59'), '23:45', '24:00')?.endTime).toBe('23:59');
  });

  it('treats absent times on the existing record as the whole day', () => {
    expect(createOrExtend(DATE, av(), '09:00', '10:00')).toEqual({
      type: 'update',
      date: DATE,
      oldStartTime: '00:00',
      oldEndTime: '23:59',
      startTime: '00:00',
      endTime: '23:59',
    });
  });
});

describe('toggleSlot', () => {
  const base = { date: DATE, slotDurationMin: 15 };

  it('adds one slot on an empty day', () => {
    expect(toggleSlot({ ...base, time: '09:00', dayAvailabilities: [] })).toEqual({
      operations: [{ type: 'create', date: DATE, startTime: '09:00', endTime: '09:15' }],
      splitRefused: false,
    });
  });

  it('extends an adjacent availability instead of creating a second', () => {
    const { operations } = toggleSlot({
      ...base,
      time: '12:00',
      dayAvailabilities: [av('09:00', '12:00')],
    });
    expect(operations).toEqual([
      {
        type: 'update',
        date: DATE,
        oldStartTime: '09:00',
        oldEndTime: '12:00',
        startTime: '09:00',
        endTime: '12:15',
      },
    ]);
  });

  it('emits nothing when the day already holds a disjoint availability', () => {
    expect(
      toggleSlot({ ...base, time: '18:00', dayAvailabilities: [av('09:00', '10:00')] }).operations
    ).toEqual([]);
  });

  it('deletes a single-slot availability outright', () => {
    expect(
      toggleSlot({ ...base, time: '09:00', dayAvailabilities: [av('09:00', '09:15')] }).operations
    ).toEqual([{ type: 'delete', date: DATE, startTime: '09:00', endTime: '09:15' }]);
  });

  it('shrinks from the start when the first slot is clicked', () => {
    expect(
      toggleSlot({ ...base, time: '09:00', dayAvailabilities: [av('09:00', '12:00')] }).operations
    ).toEqual([
      {
        type: 'update',
        date: DATE,
        oldStartTime: '09:00',
        oldEndTime: '12:00',
        startTime: '09:15',
        endTime: '12:00',
      },
    ]);
  });

  it('shrinks from the end when the last slot is clicked', () => {
    expect(
      toggleSlot({ ...base, time: '11:45', dayAvailabilities: [av('09:00', '12:00')] }).operations
    ).toEqual([
      {
        type: 'update',
        date: DATE,
        oldStartTime: '09:00',
        oldEndTime: '12:00',
        startTime: '09:00',
        endTime: '11:45',
      },
    ]);
  });

  it('deletes the whole range when a middle slot is clicked', () => {
    // Shrinking is impossible without leaving two ranges, which the day cannot hold.
    expect(
      toggleSlot({ ...base, time: '10:30', dayAvailabilities: [av('09:00', '12:00')] }).operations
    ).toEqual([{ type: 'delete', date: DATE, startTime: '09:00', endTime: '12:00' }]);
  });

  it('deletes an all-day availability when any slot is clicked', () => {
    expect(toggleSlot({ ...base, time: '10:30', dayAvailabilities: [av()] }).operations).toEqual([
      { type: 'delete', date: DATE, startTime: '00:00', endTime: '23:59' },
    ]);
  });
});

describe('addRange', () => {
  const dates = ['2026-04-06', '2026-04-07', '2026-04-08'];

  it('creates on every empty day', () => {
    const { operations } = addRange({ dates, startTime: '09:00', endTime: '12:00' }, () => []);
    expect(operations).toHaveLength(3);
    expect(operations.every(op => op.type === 'create')).toBe(true);
  });

  it('skips days whose existing availability is disjoint, without failing the batch', () => {
    const existing: Record<string, Availability[]> = {
      '2026-04-07': [av('18:00', '19:00', '2026-04-07')],
    };
    const { operations } = addRange(
      { dates, startTime: '09:00', endTime: '12:00' },
      date => existing[date] ?? []
    );
    expect(operations.map(op => op.date)).toEqual(['2026-04-06', '2026-04-08']);
  });

  it('merges rather than duplicating on days that can take it', () => {
    const existing: Record<string, Availability[]> = {
      '2026-04-07': [av('12:00', '14:00', '2026-04-07')],
    };
    const { operations } = addRange(
      { dates, startTime: '09:00', endTime: '12:00' },
      date => existing[date] ?? []
    );
    const merged = operations.find(op => op.date === '2026-04-07');
    expect(merged).toMatchObject({ type: 'update', startTime: '09:00', endTime: '14:00' });
  });
});

describe('removeRange', () => {
  it('deletes an availability the selection fully covers', () => {
    const { operations, splitRefused } = removeRange(
      { dates: [DATE], startTime: '08:00', endTime: '13:00' },
      () => [av('09:00', '12:00')]
    );
    expect(splitRefused).toBe(false);
    expect(operations).toEqual([
      { type: 'delete', date: DATE, startTime: '09:00', endTime: '12:00' },
    ]);
  });

  it('keeps the tail when the selection cuts the beginning', () => {
    const { operations } = removeRange(
      { dates: [DATE], startTime: '08:00', endTime: '10:00' },
      () => [av('09:00', '12:00')]
    );
    expect(operations).toEqual([
      {
        type: 'update',
        date: DATE,
        oldStartTime: '09:00',
        oldEndTime: '12:00',
        startTime: '10:00',
        endTime: '12:00',
      },
    ]);
  });

  it('keeps the head when the selection cuts the end', () => {
    const { operations } = removeRange(
      { dates: [DATE], startTime: '11:00', endTime: '13:00' },
      () => [av('09:00', '12:00')]
    );
    expect(operations).toEqual([
      {
        type: 'update',
        date: DATE,
        oldStartTime: '09:00',
        oldEndTime: '12:00',
        startTime: '09:00',
        endTime: '11:00',
      },
    ]);
  });

  it('refuses the whole batch rather than splitting an availability in two', () => {
    const result = removeRange({ dates: [DATE], startTime: '10:00', endTime: '11:00' }, () => [
      av('09:00', '12:00'),
    ]);
    expect(result.splitRefused).toBe(true);
    expect(result.operations).toEqual([]);
  });

  it('refuses the batch even when only one of several days would split', () => {
    // A half-applied removal is worse than none, so a single bad day aborts the rest.
    const dates = ['2026-04-06', '2026-04-07'];
    const existing: Record<string, Availability[]> = {
      '2026-04-06': [av('09:00', '10:00', '2026-04-06')],
      '2026-04-07': [av('08:00', '18:00', '2026-04-07')],
    };
    const result = removeRange(
      { dates, startTime: '09:00', endTime: '10:00' },
      date => existing[date] ?? []
    );
    expect(result.splitRefused).toBe(true);
    expect(result.operations).toEqual([]);
  });

  it('ignores availabilities that do not overlap the selection', () => {
    const { operations } = removeRange(
      { dates: [DATE], startTime: '14:00', endTime: '15:00' },
      () => [av('09:00', '12:00')]
    );
    expect(operations).toEqual([]);
  });

  it('treats a touching boundary as no overlap', () => {
    const { operations } = removeRange(
      { dates: [DATE], startTime: '12:00', endTime: '13:00' },
      () => [av('09:00', '12:00')]
    );
    expect(operations).toEqual([]);
  });
});

describe('toggleFullDay', () => {
  it('creates an all-day availability with empty times', () => {
    // The empty strings are the contract with ParticipantView's batch handler.
    expect(toggleFullDay(DATE, undefined)).toEqual({
      type: 'create',
      date: DATE,
      startTime: '',
      endTime: '',
    });
  });

  it('deletes whatever the day holds', () => {
    expect(toggleFullDay(DATE, av('09:00', '12:00'))).toEqual({
      type: 'delete',
      date: DATE,
      startTime: '09:00',
      endTime: '12:00',
    });
  });
});
