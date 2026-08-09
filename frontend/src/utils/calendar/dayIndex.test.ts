/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import type { Availability, RecurrenceException, RecurrenceWithExceptions } from '@/types';
import { buildDayIndex } from './dayIndex';

function availability(date: string, overrides: Partial<Availability> = {}): Availability {
  return {
    id: `av-${date}-${overrides.start_time ?? 'all'}`,
    participant_id: 'p1',
    participant_name: 'Ada',
    participant_email_verified: false,
    date,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function exception(date: string): RecurrenceException {
  return {
    id: `ex-${date}`,
    recurrence_id: 'r1',
    excluded_date: date,
    created_at: '2026-01-01T00:00:00Z',
  };
}

function recurrence(overrides: Partial<RecurrenceWithExceptions> = {}): RecurrenceWithExceptions {
  return {
    id: 'r1',
    participant_id: 'p1',
    day_of_week: 3, // Wednesday
    start_date: '2026-04-01',
    created_at: '2026-01-01T00:00:00Z',
    exceptions: [],
    ...overrides,
  };
}

describe('ownFor', () => {
  it('groups availabilities by date', () => {
    const index = buildDayIndex({
      availabilities: [
        availability('2026-04-08', { start_time: '09:00', end_time: '12:00' }),
        availability('2026-04-08', { start_time: '14:00', end_time: '18:00' }),
        availability('2026-04-09'),
      ],
    });

    expect(index.ownFor('2026-04-08')).toHaveLength(2);
    expect(index.ownFor('2026-04-09')).toHaveLength(1);
    expect(index.ownFor('2026-04-10')).toEqual([]);
  });

  it('preserves input order within a date', () => {
    const index = buildDayIndex({
      availabilities: [
        availability('2026-04-08', { start_time: '14:00' }),
        availability('2026-04-08', { start_time: '09:00' }),
      ],
    });
    expect(index.ownFor('2026-04-08').map(a => a.start_time)).toEqual(['14:00', '09:00']);
  });

  it('returns the same empty array instance for unknown dates', () => {
    // Allocating a fresh [] per miss would defeat the point in a v-for.
    const index = buildDayIndex({});
    expect(index.ownFor('2026-04-08')).toBe(index.ownFor('2026-04-09'));
  });

  it('tolerates missing input', () => {
    const index = buildDayIndex({ availabilities: null, recurrences: undefined });
    expect(index.ownFor('2026-04-08')).toEqual([]);
  });
});

describe('recurrenceFor', () => {
  const base = recurrence({ start_time: '10:00', end_time: '12:00' });

  it('matches only the recurrence weekday', () => {
    const index = buildDayIndex({ recurrences: [base] });
    // 2026-04-08 is a Wednesday, 2026-04-09 a Thursday.
    expect(index.recurrenceFor('2026-04-08', 3)?.id).toBe('r1');
    expect(index.recurrenceFor('2026-04-09', 4)).toBeNull();
  });

  it('respects the recurrence start date', () => {
    const index = buildDayIndex({ recurrences: [recurrence({ start_date: '2026-04-08' })] });
    expect(index.recurrenceFor('2026-04-01', 3)).toBeNull();
    expect(index.recurrenceFor('2026-04-08', 3)).not.toBeNull();
  });

  it('respects the recurrence end date', () => {
    const index = buildDayIndex({ recurrences: [recurrence({ end_date: '2026-04-15' })] });
    expect(index.recurrenceFor('2026-04-15', 3)).not.toBeNull();
    expect(index.recurrenceFor('2026-04-22', 3)).toBeNull();
  });

  it('is open-ended without an end date', () => {
    const index = buildDayIndex({ recurrences: [base] });
    expect(index.recurrenceFor('2099-04-08', 3)).not.toBeNull();
  });

  it('skips excluded dates', () => {
    const index = buildDayIndex({
      recurrences: [recurrence({ exceptions: [exception('2026-04-08')] })],
    });
    expect(index.recurrenceFor('2026-04-08', 3)).toBeNull();
    expect(index.recurrenceFor('2026-04-15', 3)).not.toBeNull();
  });

  it('carries the recurrence times and note', () => {
    const index = buildDayIndex({
      recurrences: [recurrence({ start_time: '10:00', end_time: '12:00', note: 'standup' })],
    });
    expect(index.recurrenceFor('2026-04-08', 3)).toEqual({
      id: 'r1',
      startTime: '10:00',
      endTime: '12:00',
      note: 'standup',
    });
  });

  it('falls through to the next recurrence on the same weekday', () => {
    const index = buildDayIndex({
      recurrences: [
        recurrence({ id: 'r1', exceptions: [exception('2026-04-08')] }),
        recurrence({ id: 'r2' }),
      ],
    });
    expect(index.recurrenceFor('2026-04-08', 3)?.id).toBe('r2');
  });

  it('ignores an out-of-range weekday', () => {
    const index = buildDayIndex({ recurrences: [recurrence({ day_of_week: 9 })] });
    expect(index.recurrenceFor('2026-04-08', 3)).toBeNull();
  });
});

describe('countFor and maxCount', () => {
  it('reads participant counts and defaults to zero', () => {
    const index = buildDayIndex({
      participantCounts: { '2026-04-08': 5, '2026-04-09': 2 },
    });
    expect(index.countFor('2026-04-08')).toBe(5);
    expect(index.countFor('2026-04-10')).toBe(0);
  });

  it('reports the highest count in the range', () => {
    expect(buildDayIndex({ participantCounts: { a: 1, b: 7, c: 3 } }).maxCount).toBe(7);
    expect(buildDayIndex({}).maxCount).toBe(0);
  });
});

describe('summaryFor', () => {
  it('indexes date summaries', () => {
    const summary = { date: '2026-04-08', total_count: 3, participants: [] };
    const index = buildDayIndex({ dateSummaries: [summary] });
    expect(index.summaryFor('2026-04-08')).toBe(summary);
    expect(index.summaryFor('2026-04-09')).toBeUndefined();
  });
});
