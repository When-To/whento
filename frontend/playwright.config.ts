/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineConfig, devices } from '@playwright/test';

/**
 * Browser tests for the calendar views.
 *
 * They run against `dev/preview.html`, which renders the components with fabricated
 * data, so they need no database, no backend and no seeded calendar. That keeps them
 * runnable in CI and by anyone who has just cloned the repo.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'line' : 'list',
  use: {
    baseURL: 'http://127.0.0.1:8099',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['Pixel 5'] } },
  ],
  webServer: {
    command: 'npx vite --port 8099 --host 127.0.0.1',
    url: 'http://127.0.0.1:8099/dev/preview.html',
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
});
