/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Timezone control for unit tests.
 *
 * Calendar date handling is only correct if it stays in local time, and the failure
 * mode is invisible in the CI timezone. These helpers let a test pin the process
 * timezone so the UTC/local and DST bugs are actually reachable.
 *
 * `process` is declared locally rather than pulled from `@types/node`: the app
 * tsconfig is deliberately browser-only, and test files are the sole exception.
 */

declare const process: { env: Record<string, string | undefined> };

/** Run `body` with the process timezone temporarily overridden, then restore it. */
export function inTimezone(timeZone: string, body: () => void): void {
  const previous = process.env.TZ;
  process.env.TZ = timeZone;
  try {
    body();
  } finally {
    process.env.TZ = previous;
  }
}

/** IANA zones that exercise the interesting edges: UTC, negative, positive, extreme. */
export const SAMPLE_TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Los_Angeles',
  'Europe/Paris',
  'Australia/Sydney',
  'Pacific/Kiritimati',
] as const;
