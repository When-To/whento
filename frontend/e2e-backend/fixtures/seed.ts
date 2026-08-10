/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Seed a calendar through the public API, for browser tests that need a real backend.
 *
 * The repository had no seeding of any kind. The existing browser suite runs against
 * `dev/preview.html`, a harness where the components emit into the void — no Pinia, no
 * router, no HTTP — so every write path was not merely untested but *unobservable*:
 * nothing issues a request, and `page.route` has nothing to intercept. These fixtures
 * exist so the write paths can be driven against the real thing.
 *
 * Everything here goes through the same HTTP API the frontend uses. Nothing touches the
 * database directly, so a schema change that breaks the API breaks these too, which is
 * the point.
 */

import { apiFetch as api, ownerToken } from './api';

export interface SeededParticipant {
  readonly id: string;
  readonly name: string;
}

export interface SeededCalendar {
  readonly publicToken: string;
  readonly participants: Record<string, SeededParticipant>;
  /** ISO date of the Monday the fixture is anchored on. */
  readonly monday: string;
  /** `day(0)` is that Monday, `day(6)` the Sunday after it. */
  day(offset: number): string;
  readonly recurrenceId: string;
  /** URL of the participant view for one of the seeded participants. */
  participantUrl(baseURL: string, participantName: string): string;
}

function iso(date: Date): string {
  return [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, '0'),
    String(date.getDate()).padStart(2, '0'),
  ].join('-');
}

/**
 * The Monday of the week after next.
 *
 * Far enough ahead that nothing in the fixture is in the past — the calendar refuses
 * edits to past dates — and never so far that it leaves the range the view loads.
 */
function upcomingMonday(): Date {
  const date = new Date();
  date.setHours(0, 0, 0, 0);
  date.setDate(date.getDate() + ((8 - date.getDay()) % 7) + 7);
  return date;
}

export interface SeedOptions {
  /** Minimum participants for a date to count as workable. */
  readonly threshold?: number;
  /** Weekdays the calendar accepts, Sunday = 0. Defaults to every day but Wednesday. */
  readonly allowedWeekdays?: number[];
  readonly lockParticipants?: boolean;
  /**
   * Set false to configure no per-weekday hours at all. Without a window the server has
   * nothing to clamp an untimed availability to, so it stores null times.
   */
  readonly weekdayTimes?: boolean;
}

/**
 * Create a calendar with three participants, overlapping availabilities and a
 * recurrence carrying one exception.
 *
 * The owner account is shared across the run — globalSetup registers it once, because
 * the auth endpoints are rate limited per IP — but every call here makes a *fresh
 * calendar*. Tests therefore never share state and none has to clean up after itself.
 */
export async function seedCalendar(options: SeedOptions = {}): Promise<SeededCalendar> {
  const unique = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;

  const monday = upcomingMonday();
  const day = (offset: number): string => {
    const date = new Date(monday);
    date.setDate(date.getDate() + offset);
    return iso(date);
  };

  // Wednesday is closed by default, so tests have a disabled day to drag across and to
  // check the grid refuses. Hours are wide enough that the week view shows a full grid.
  const allowedWeekdays = options.allowedWeekdays ?? [0, 1, 2, 4, 5, 6];
  const weekdayTimes =
    options.weekdayTimes === false
      ? undefined
      : Object.fromEntries(
          allowedWeekdays.map(weekday => [
            String(weekday),
            { min_time: '08:00', max_time: '20:00' },
          ])
        );

  const calendar = await api<{
    public_token: string;
    participants: { id: string; name: string }[];
  }>('/calendars', {
    method: 'POST',
    token: ownerToken(),
    body: {
      name: `E2E ${unique}`,
      threshold: options.threshold ?? 2,
      timezone: 'Europe/Paris',
      allowed_weekdays: allowedWeekdays,
      lock_participants: options.lockParticipants ?? false,
      participants: ['Ada', 'Grace', 'Linus'],
      weekday_times: weekdayTimes,
      start_date: day(-14),
      end_date: day(60),
    },
  });

  const participants: Record<string, SeededParticipant> = {};
  for (const participant of calendar.participants) {
    participants[participant.name] = { id: participant.id, name: participant.name };
  }

  const publicToken = calendar.public_token;
  const base = `/availabilities/calendar/${publicToken}/participant`;

  const addAvailability = (who: string, date: string, start?: string, end?: string) =>
    api(`${base}/${participants[who].id}`, {
      method: 'POST',
      body: { date, start_time: start, end_time: end },
    });

  // Monday: two people overlap for two hours, so the threshold of two is met.
  await addAvailability('Ada', day(0), '09:00', '17:00');
  await addAvailability('Grace', day(0), '10:00', '12:00');
  // Tuesday: one person only, so the threshold is not met.
  await addAvailability('Linus', day(1), '14:00', '18:00');
  // Thursday: an untimed answer. The server clamps it to the configured weekday
  // window, so it lands as 08:00-20:00 unless the calendar sets no hours.
  await addAvailability('Grace', day(3));

  // A recurrence for Ada every Friday. The Friday of the seeded week — day(4) — is
  // left live so tests have an occurrence to click; the one a week later is excepted,
  // so they also have a date where the rule exists but deliberately does not apply.
  const recurrence = await api<{ id: string }>(`${base}/${participants.Ada.id}/recurrence`, {
    method: 'POST',
    body: {
      day_of_week: 5,
      start_time: '15:00',
      end_time: '18:00',
      start_date: day(-14),
    },
  });

  await api(`${base}/${participants.Ada.id}/recurrence/${recurrence.id}/exception`, {
    method: 'POST',
    body: { excluded_date: day(11) },
  });

  return {
    publicToken,
    participants,
    monday: day(0),
    day,
    recurrenceId: recurrence.id,
    participantUrl: (baseURL, participantName) =>
      `${baseURL}/c/${publicToken}/p/${participants[participantName].id}`,
  };
}

/** Read a participant's stored availabilities back, to assert what actually persisted. */
export async function fetchAvailabilities(
  publicToken: string,
  participantId: string,
  start: string,
  end: string
): Promise<{ date: string; start_time?: string | null; end_time?: string | null }[]> {
  const result = await api<{
    availabilities: { date: string; start_time?: string | null; end_time?: string | null }[];
  }>(
    `/availabilities/calendar/${publicToken}/participant/${participantId}` +
      `?start_date=${start}&end_date=${end}`
  );

  return result.availabilities ?? [];
}

/**
 * Read the range summary, which is what the calendar views render from.
 *
 * The array coercion is now belt and braces: the server returns [] for an empty range.
 * It used to serialise as `"data": null`, which is why this and ParticipantView both
 * carry a guard.
 */
export async function fetchRangeSummary(
  publicToken: string,
  start: string,
  end: string
): Promise<{ date: string; total_count: number }[]> {
  const summary = await api<{ date: string; total_count: number }[] | null>(
    `/availabilities/calendar/${publicToken}/range?start=${start}&end=${end}`
  );

  return Array.isArray(summary) ? summary : [];
}
