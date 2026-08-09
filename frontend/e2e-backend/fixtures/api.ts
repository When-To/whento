/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

export const API = process.env.WHENTO_API ?? 'http://127.0.0.1:5173/api/v1';

/** Where globalSetup leaves the run's credentials for the workers to read. */
export const ACCOUNT_FILE = resolve(process.cwd(), 'test-results/.e2e-account.json');

export interface RequestOptions {
  readonly method?: string;
  readonly body?: unknown;
  readonly token?: string;
}

/**
 * A fetch wrapper that fails loudly and specifically.
 *
 * Two failure modes are worth distinguishing by hand, because both arrive as "not
 * JSON" and each sends you looking in a different place:
 *
 * - **429 as text/plain.** The auth endpoints are rate limited per IP — register at 3
 *   per minute, login at 5 — and the limiter answers in plain text.
 * - **200 with HTML.** An unknown `/api` path falls through to the SPA rather than
 *   404ing, so a typo in a route reads as success and writes nothing.
 */
export async function apiFetch<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET';

  const response = await fetch(`${API}${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(options.token ? { Authorization: `Bearer ${options.token}` } : {}),
    },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });

  const contentType = response.headers.get('content-type') ?? '';
  const raw = await response.text();

  if (!contentType.includes('application/json')) {
    if (response.status === 429) {
      throw new Error(
        `${method} ${path} was rate limited (429). The auth endpoints allow only a few ` +
          `requests per minute per IP; seeding must not register or log in per test.`
      );
    }
    throw new Error(
      `${method} ${path} returned ${response.status} as ${contentType || 'no content-type'}. ` +
        `An unknown /api path serves the SPA instead of 404ing, so this usually means the ` +
        `route is wrong: ${raw.slice(0, 120)}`
    );
  }

  const payload = JSON.parse(raw) as { data?: T; error?: { message?: string } };

  if (!response.ok) {
    throw new Error(
      `${method} ${path} -> ${response.status}: ${payload.error?.message ?? raw.slice(0, 200)}`
    );
  }

  // `data` is unwrapped when the key exists *even if it is null* — an empty range
  // summary serialises as `"data": null` rather than `[]`, and `??` would fall through
  // to the envelope object and hand back something that is not an array.
  return ('data' in payload ? payload.data : (payload as T)) as T;
}

let cachedToken: string | null = null;

/** The access token for the run's shared owner account, created by globalSetup. */
export function ownerToken(): string {
  if (cachedToken) return cachedToken;

  try {
    const account = JSON.parse(readFileSync(ACCOUNT_FILE, 'utf8')) as { accessToken: string };
    cachedToken = account.accessToken;
    return cachedToken;
  } catch (cause) {
    throw new Error(
      `Could not read the seeded account at ${ACCOUNT_FILE}. globalSetup registers it, ` +
        `so this suite has to run through playwright.backend.config.ts.`,
      { cause }
    );
  }
}
