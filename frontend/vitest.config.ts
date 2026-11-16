/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineConfig } from 'vitest/config';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig({
  // Single-file components are compiled here too, so component tests can mount them.
  // Without this, importing a `.vue` file from a test fails in the import analysis
  // pass, long before any assertion runs.
  // Root-relative asset URLs (`/logo.png`, served from public/) stay plain strings:
  // imported, they resolve to `file:///logo.png`, which Node rejects on Windows.
  plugins: [vue({ template: { transformAssetUrls: { includeAbsolute: false } } })],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    // Two environments on purpose.
    //
    // src/utils/** is framework-free and its date tests drive process.env.TZ, which
    // jsdom does not carry; those stay in plain Node, and it keeps the pure layer fast.
    // Files that need a DOM — stores, composables, the API client — opt in with a
    // `@vitest-environment jsdom` docblock at the top. That is per-file rather than
    // path-matched on purpose: environmentMatchGlobs was removed in Vitest 4, and the
    // docblock makes each file's requirement visible where it is read.
    environment: 'node',
    include: ['src/**/*.test.ts', 'e2e-backend/fixtures/**/*.test.ts'],
    reporters: ['dot'],
    coverage: {
      provider: 'v8',
      reporter: ['text-summary', 'html', 'lcov'],
      include: ['src/**/*.ts'],
      exclude: ['src/**/*.test.ts', 'src/test/**', 'src/types/**', 'src/locales/**', 'src/main.ts'],
      // A ratchet, not a target. Set just under what the suite currently reaches so
      // that `npm run test:coverage` fails if coverage falls, without failing on the
      // noise of a line or two. Raise it when the next batch of tests lands.
      thresholds: {
        statements: 54,
        branches: 64,
        functions: 43,
        lines: 52,
      },
    },
  },
});
