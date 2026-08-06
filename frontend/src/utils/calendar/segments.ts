/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Participant coverage over a day, as non-overlapping segments.
 *
 * This is the one genuinely valuable algorithm in the old week grid: two sweep lines
 * that turn a list of per-participant time ranges into a partition of the day by how
 * many people are available, and into the ranges where the threshold is met. It is
 * ported here verbatim in behaviour, and made pure so it can be tested at all — it
 * previously lived in two computed properties inside a 2800-line component.
 */

import { mergeIntervals, timeToMinutes, type Interval } from '@/utils/date/timeRange';
import type { DateAvailabilitySummary, ParticipantAvailabilitySummary } from '@/types';
import type { ISODate } from '@/utils/date/isoDate';

/** A span of the day covered by a fixed set of participants. */
export interface CoverageSegment extends Interval {
  readonly count: number;
  /** Whether the current participant is one of them. */
  readonly includesCurrent: boolean;
}

/** Start and end of a participant's availability, in minutes since local midnight. */
function boundsOf(participant: ParticipantAvailabilitySummary): Interval {
  return {
    startMin: participant.start_time ? timeToMinutes(participant.start_time) : 0,
    endMin: participant.end_time ? timeToMinutes(participant.end_time) : 24 * 60,
  };
}

/**
 * Partition the day into spans of constant participant count.
 *
 * Sweep line over start/end events. The tie-break — a start sorts before an end at the
 * same instant — is load-bearing: it keeps two back-to-back availabilities from
 * momentarily dropping the count to zero and splitting a continuous band in two.
 */
export function buildCoverageSegments(
  participants: readonly ParticipantAvailabilitySummary[],
  currentParticipantName: string
): CoverageSegment[] {
  const events: { time: number; type: 'start' | 'end'; name: string }[] = [];
  for (const participant of participants) {
    const { startMin, endMin } = boundsOf(participant);
    events.push({ time: startMin, type: 'start', name: participant.participant_name });
    events.push({ time: endMin, type: 'end', name: participant.participant_name });
  }

  events.sort((a, b) => {
    if (a.time !== b.time) return a.time - b.time;
    return a.type === 'start' ? -1 : 1;
  });

  const segments: CoverageSegment[] = [];
  const active = new Set<string>();
  let segmentStart: number | null = null;

  for (const event of events) {
    if (segmentStart !== null && active.size > 0 && event.time > segmentStart) {
      segments.push({
        startMin: segmentStart,
        endMin: event.time,
        count: active.size,
        includesCurrent: active.has(currentParticipantName),
      });
    }

    if (event.type === 'start') active.add(event.name);
    else active.delete(event.name);

    segmentStart = active.size > 0 ? event.time : null;
  }

  return segments;
}

/**
 * The ranges where at least `threshold` participants overlap.
 *
 * A second sweep line, kept separate from {@link buildCoverageSegments} because it
 * counts *availabilities* rather than distinct participants: a participant with two
 * ranges on the same day contributes twice here. That is the previous behaviour and is
 * preserved.
 */
export function buildThresholdIntervals(
  participants: readonly ParticipantAvailabilitySummary[],
  threshold: number
): Interval[] {
  if (participants.length < threshold) return [];

  const events: { time: number; type: 'start' | 'end' }[] = [];
  for (const participant of participants) {
    const { startMin, endMin } = boundsOf(participant);
    events.push({ time: startMin, type: 'start' });
    events.push({ time: endMin, type: 'end' });
  }

  events.sort((a, b) => {
    if (a.time !== b.time) return a.time - b.time;
    return a.type === 'start' ? -1 : 1;
  });

  let count = 0;
  let thresholdStart: number | null = null;
  const ranges: Interval[] = [];

  for (const event of events) {
    if (event.type === 'start') {
      count++;
      if (count >= threshold && thresholdStart === null) thresholdStart = event.time;
    } else {
      if (thresholdStart !== null && count === threshold) {
        ranges.push({ startMin: thresholdStart, endMin: event.time });
        thresholdStart = null;
      }
      count--;
    }
  }

  // Unreachable: every start has a matching end, so the count always returns to zero
  // and `thresholdStart` is always cleared. Kept, and asserted unreachable by a test,
  // rather than dropped silently — if the event stream ever became unbalanced this is
  // what stopped a band from vanishing.
  if (thresholdStart !== null) {
    ranges.push({ startMin: thresholdStart, endMin: 24 * 60 });
  }

  return mergeIntervals(ranges);
}

/** Coverage and threshold bands for a whole range of dates, keyed by date. */
export interface CoverageMap {
  readonly coverage: ReadonlyMap<ISODate, CoverageSegment[]>;
  readonly thresholds: ReadonlyMap<ISODate, Interval[]>;
}

export function buildCoverageMap(
  summaries: readonly DateAvailabilitySummary[] | null | undefined,
  currentParticipantName: string,
  threshold: number
): CoverageMap {
  const coverage = new Map<ISODate, CoverageSegment[]>();
  const thresholds = new Map<ISODate, Interval[]>();

  for (const summary of summaries ?? []) {
    coverage.set(summary.date, buildCoverageSegments(summary.participants, currentParticipantName));
    const met = buildThresholdIntervals(summary.participants, threshold);
    if (met.length > 0) thresholds.set(summary.date, met);
  }

  return { coverage, thresholds };
}

/** The visible slice of the day, in minutes, and how it is divided into slots. */
export interface GridGeometry {
  readonly firstSlotMin: number;
  readonly lastSlotEndMin: number;
  readonly slotDurationMin: number;
  readonly slotCount: number;
}

/** One painted band in a day column, positioned as percentages of the column. */
export interface Band {
  readonly kind: 'own' | 'others' | 'threshold';
  readonly count: number;
  /** Pre-stringified so the template does no arithmetic. */
  readonly top: string;
  readonly height: string;
  readonly startMin: number;
  readonly endMin: number;
}

function pct(value: number): string {
  return `${(Math.round(value * 10000) / 100).toFixed(2)}%`;
}

/**
 * Project a day's segments onto the visible grid as absolutely-positioned bands.
 *
 * Replaces a per-cell projection that walked *every* slot for *every* segment, calling
 * `timeToMinutes` inside the inner loop, and was then read back from the template
 * through eight helper functions several times per cell — some of which allocated a
 * fresh array or object on each call. This is O(segments), runs once per day, and the
 * result is a plain array the template iterates.
 */
export function buildDayBands(
  coverage: readonly CoverageSegment[],
  thresholds: readonly Interval[],
  geometry: GridGeometry
): Band[] {
  const { firstSlotMin, lastSlotEndMin } = geometry;
  const span = lastSlotEndMin - firstSlotMin;
  if (span <= 0) return [];

  const bands: Band[] = [];

  const push = (kind: Band['kind'], startMin: number, endMin: number, count: number) => {
    const clippedStart = Math.max(startMin, firstSlotMin);
    const clippedEnd = Math.min(endMin, lastSlotEndMin);
    if (clippedEnd <= clippedStart) return;
    bands.push({
      kind,
      count,
      top: pct((clippedStart - firstSlotMin) / span),
      height: pct((clippedEnd - clippedStart) / span),
      startMin: clippedStart,
      endMin: clippedEnd,
    });
  };

  for (const segment of coverage) {
    push(
      segment.includesCurrent ? 'own' : 'others',
      segment.startMin,
      segment.endMin,
      segment.count
    );
  }

  // Threshold bands are outlines drawn over the fills, so they come last.
  for (const interval of thresholds) {
    push('threshold', interval.startMin, interval.endMin, 0);
  }

  return bands;
}
