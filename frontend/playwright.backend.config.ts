/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineConfig, devices } from '@playwright/test';

/**
 * Browser tests against a real backend.
 *
 * Kept separate from `playwright.config.ts` on purpose. That suite runs against
 * `dev/preview.html`, needs nothing but a checkout, and covers rendering and keyboard
 * behaviour. This one drives the actual `ParticipantView` against a running server and
 * a seeded calendar, so it covers the write paths — which are not merely untested in
 * the harness but unobservable, since no component there issues a request.
 *
 * The two cannot be merged into one config: the harness serves its own Vite on 8099
 * with no API behind it, while this needs the frontend proxying `/api` to a backend
 * with a database. They also run in different CI jobs for the same reason.
 *
 * Expects `make dev-fullstack` (frontend :8080 proxying /api to backend :5173), or the
 * equivalent in CI. Set WHENTO_BASE_URL / WHENTO_API to point elsewhere.
 *
 * **The server must run with `RATE_LIMIT_ENABLED=false`.** The limiter is per IP — 3
 * registrations a minute, 60 availability writes — and a suite driving a real calendar
 * from one address is precisely the traffic it exists to refuse. Leaving it on turns
 * every test into a 429. That is the limiter working, not a bug, which is why this is
 * a run-time flag rather than something the tests try to pace around.
 */
const baseURL = process.env.WHENTO_BASE_URL ?? 'http://127.0.0.1:8080';

export default defineConfig({
  testDir: './e2e-backend',
  // Registers the run's single owner account. It cannot be done per test: /auth/register
  // allows 3 requests per minute per IP and /auth/login 5, so a per-test registration
  // 429s on the fourth case.
  globalSetup: './e2e-backend/fixtures/global-setup.ts',
  // Each test seeds its own calendar under its own account, so they cannot collide.
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  // A real server and a real database are slower than a static harness.
  timeout: 60_000,
  expect: { timeout: 15_000 },
  reporter: process.env.CI ? 'line' : 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'desktop', use: { ...devices['Desktop Chrome'] } }],
});
