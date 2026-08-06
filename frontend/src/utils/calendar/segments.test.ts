/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { describe, expect, it } from 'vitest';
import type { ParticipantAvailabilitySummary } from '@/types';
import {
  buildCoverageMap,
  buildCoverageSegments,
  buildDayBands,
  buildThresholdIntervals,
  type GridGeometry,
} from './segments';

function p(name: string, start?: string, end?: string): ParticipantAvailabilitySummary {
  return { participant_name: name, start_time: start, end_time: end };
}

const ME = 'Ada';

describe('buildCoverageSegments', () => {
  it('returns nothing for nobody', () => {
    expect(buildCoverageSegments([], ME)).toEqual([]);
  });

  it('covers a single participant exactly', () => {
    expect(buildCoverageSegments([p(ME, '09:00', '12:00')], ME)).toEqual([
      { startMin: 540, endMin: 720, count: 1, includesCurrent: true },
    ]);
  });

  it('treats absent times as the whole day', () => {
    expect(buildCoverageSegments([p('Bob')], ME)).toEqual([
      { startMin: 0, endMin: 1440, count: 1, includesCurrent: false },
    ]);
  });

  it('keeps disjoint ranges separate', () => {
    const segments = buildCoverageSegments(
      [p('Bob', '09:00', '10:00'), p('Cleo', '14:00', '15:00')],
      ME
    );
    expect(segments).toEqual([
      { startMin: 540, endMin: 600, count: 1, includesCurrent: false },
      { startMin: 840, endMin: 900, count: 1, includesCurrent: false },
    ]);
  });

  it('splits an overlap into three spans with the right counts', () => {
    const segments = buildCoverageSegments(
      [p('Bob', '09:00', '12:00'), p('Cleo', '10:00', '14:00')],
      ME
    );
    expect(segments).toEqual([
      { startMin: 540, endMin: 600, count: 1, includesCurrent: false },
      { startMin: 600, endMin: 720, count: 2, includesCurrent: false },
      { startMin: 720, endMin: 840, count: 1, includesCurrent: false },
    ]);
  });

  it('handles a fully nested range', () => {
    const segments = buildCoverageSegments(
      [p('Bob', '09:00', '17:00'), p('Cleo', '12:00', '13:00')],
      ME
    );
    expect(segments.map(s => [s.startMin, s.endMin, s.count])).toEqual([
      [540, 720, 1],
      [720, 780, 2],
      [780, 1020, 1],
    ]);
  });

  it('does not break a band at a touching boundary', () => {
    // The start-before-end tie-break is what keeps this one continuous band rather
    // than two, with the count momentarily dropping to zero in between.
    const segments = buildCoverageSegments(
      [p('Bob', '09:00', '12:00'), p('Cleo', '12:00', '15:00')],
      ME
    );
    expect(segments.map(s => [s.startMin, s.endMin, s.count])).toEqual([
      [540, 720, 1],
      [720, 900, 1],
    ]);
  });

  it('collapses identical ranges into one span', () => {
    const segments = buildCoverageSegments(
      [p('Bob', '09:00', '12:00'), p('Cleo', '09:00', '12:00')],
      ME
    );
    expect(segments).toEqual([{ startMin: 540, endMin: 720, count: 2, includesCurrent: false }]);
  });

  it('flags only the spans the current participant is inside', () => {
    const segments = buildCoverageSegments(
      [p(ME, '10:00', '12:00'), p('Bob', '09:00', '14:00')],
      ME
    );
    expect(segments.map(s => [s.startMin, s.endMin, s.includesCurrent])).toEqual([
      [540, 600, false],
      [600, 720, true],
      [720, 840, false],
    ]);
  });

  it('mixes all-day and timed availabilities', () => {
    const segments = buildCoverageSegments([p('Bob'), p(ME, '09:00', '12:00')], ME);
    expect(segments.map(s => [s.startMin, s.endMin, s.count, s.includesCurrent])).toEqual([
      [0, 540, 1, false],
      [540, 720, 2, true],
      [720, 1440, 1, false],
    ]);
  });

  it('produces sorted, non-overlapping segments that conserve total coverage', () => {
    const participants = [
      p('Bob', '08:00', '12:00'),
      p('Cleo', '09:30', '18:00'),
      p(ME, '11:00', '11:30'),
      p('Dee'),
      p('Eve', '17:00', '19:00'),
    ];
    const segments = buildCoverageSegments(participants, ME);

    for (let i = 1; i < segments.length; i++) {
      expect(segments[i].startMin).toBeGreaterThanOrEqual(segments[i - 1].endMin);
    }

    const covered = segments.reduce((sum, s) => sum + (s.endMin - s.startMin) * s.count, 0);
    const expected = participants.reduce((sum, participant) => {
      const start = participant.start_time
        ? Number(participant.start_time.slice(0, 2)) * 60 +
          Number(participant.start_time.slice(3, 5))
        : 0;
      const end = participant.end_time
        ? Number(participant.end_time.slice(0, 2)) * 60 + Number(participant.end_time.slice(3, 5))
        : 1440;
      return sum + (end - start);
    }, 0);
    expect(covered).toBe(expected);
  });
});

describe('buildThresholdIntervals', () => {
  it('returns nothing below the threshold', () => {
    expect(buildThresholdIntervals([p('Bob', '09:00', '12:00')], 2)).toEqual([]);
  });

  it('finds the overlap of exactly the threshold count', () => {
    expect(
      buildThresholdIntervals([p('Bob', '09:00', '12:00'), p('Cleo', '10:00', '14:00')], 2)
    ).toEqual([{ startMin: 600, endMin: 720 }]);
  });

  it('accepts a threshold of one, covering everything', () => {
    expect(buildThresholdIntervals([p('Bob', '09:00', '12:00')], 1)).toEqual([
      { startMin: 540, endMin: 720 },
    ]);
  });

  it('needs all three to overlap at a threshold of three', () => {
    const participants = [
      p('Bob', '09:00', '13:00'),
      p('Cleo', '10:00', '14:00'),
      p('Dee', '11:00', '12:00'),
    ];
    expect(buildThresholdIntervals(participants, 3)).toEqual([{ startMin: 660, endMin: 720 }]);
    expect(buildThresholdIntervals(participants, 4)).toEqual([]);
  });

  it('merges adjacent qualifying ranges', () => {
    const participants = [
      p('Bob', '09:00', '12:00'),
      p('Cleo', '09:00', '12:00'),
      p('Dee', '12:00', '15:00'),
      p('Eve', '12:00', '15:00'),
    ];
    expect(buildThresholdIntervals(participants, 2)).toEqual([{ startMin: 540, endMin: 900 }]);
  });

  it('never leaves a band running to midnight', () => {
    // The trailing fallback in the sweep is unreachable while every start has a
    // matching end. If this ever fails, the event stream became unbalanced.
    const participants = [
      p('Bob', '09:00', '12:00'),
      p('Cleo', '10:00', '14:00'),
      p('Dee', '11:00', '23:00'),
    ];
    for (const threshold of [1, 2, 3]) {
      const ranges = buildThresholdIntervals(participants, threshold);
      expect(ranges.every(r => r.endMin < 1440 || r.startMin === 0)).toBe(true);
    }
  });
});

describe('buildCoverageMap', () => {
  it('indexes by date and omits days that never meet the threshold', () => {
    const map = buildCoverageMap(
      [
        {
          date: '2026-04-08',
          total_count: 2,
          participants: [p('Bob', '09:00', '12:00'), p('Cleo', '10:00', '14:00')],
        },
        { date: '2026-04-09', total_count: 1, participants: [p('Bob', '09:00', '12:00')] },
      ],
      ME,
      2
    );

    expect(map.coverage.get('2026-04-08')).toHaveLength(3);
    expect(map.thresholds.get('2026-04-08')).toEqual([{ startMin: 600, endMin: 720 }]);
    expect(map.coverage.get('2026-04-09')).toHaveLength(1);
    expect(map.thresholds.has('2026-04-09')).toBe(false);
  });

  it('tolerates absent summaries', () => {
    expect(buildCoverageMap(null, ME, 1).coverage.size).toBe(0);
    expect(buildCoverageMap(undefined, ME, 1).thresholds.size).toBe(0);
  });
});

describe('buildDayBands', () => {
  // 08:00 to 20:00 in 15-minute slots.
  const geometry: GridGeometry = {
    firstSlotMin: 480,
    lastSlotEndMin: 1200,
    slotDurationMin: 15,
    slotCount: 48,
  };

  it('positions a band as a percentage of the visible column', () => {
    const bands = buildDayBands(
      [{ startMin: 540, endMin: 720, count: 1, includesCurrent: true }],
      [],
      geometry
    );
    // 09:00 is 60 minutes into a 720-minute column; the band runs 180 minutes.
    expect(bands).toEqual([
      {
        kind: 'own',
        count: 1,
        top: '8.33%',
        height: '25.00%',
        startMin: 540,
        endMin: 720,
      },
    ]);
  });

  it('fills the column for a segment covering the whole visible range', () => {
    const [band] = buildDayBands(
      [{ startMin: 0, endMin: 1440, count: 3, includesCurrent: false }],
      [],
      geometry
    );
    expect(band.top).toBe('0.00%');
    expect(band.height).toBe('100.00%');
    expect(band.kind).toBe('others');
  });

  it('clips at both ends of the visible range', () => {
    const [band] = buildDayBands(
      [{ startMin: 300, endMin: 1380, count: 1, includesCurrent: false }],
      [],
      geometry
    );
    expect(band.startMin).toBe(480);
    expect(band.endMin).toBe(1200);
  });

  it('drops segments entirely outside the visible range', () => {
    expect(
      buildDayBands([{ startMin: 0, endMin: 300, count: 1, includesCurrent: false }], [], geometry)
    ).toEqual([]);
    expect(
      buildDayBands(
        [{ startMin: 1300, endMin: 1440, count: 1, includesCurrent: false }],
        [],
        geometry
      )
    ).toEqual([]);
  });

  it('draws threshold outlines after the fills so they sit on top', () => {
    const bands = buildDayBands(
      [{ startMin: 540, endMin: 720, count: 2, includesCurrent: false }],
      [{ startMin: 600, endMin: 660 }],
      geometry
    );
    expect(bands.map(b => b.kind)).toEqual(['others', 'threshold']);
  });

  it('returns nothing for a degenerate geometry', () => {
    expect(
      buildDayBands([{ startMin: 540, endMin: 720, count: 1, includesCurrent: true }], [], {
        ...geometry,
        lastSlotEndMin: 480,
      })
    ).toEqual([]);
  });
});
