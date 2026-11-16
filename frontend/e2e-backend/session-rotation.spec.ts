/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * PR-7 real-browser acceptance, rotation flavour.
 *
 * Acceptance 2 ("force one refresh due in both tabs: bounded coordinated rotation,
 * stable family, no account-store reset, no reload") and acceptance 7 (family stable
 * across refresh, distinct across new logins) need a refresh to be *due* within a
 * test lifetime. That is impossible against a server with the normal 15-minute access
 * TTL (and the acceptance-1 loop test must keep that TTL), so this spec runs against
 * a dedicated backend started with a short `JWT_ACCESS_EXPIRY` (2 minutes), which
 * serves the same built frontend on its own origin (default http://127.0.0.1:5174).
 *
 * REQUIRES that backend; point WHENTO_ROTATION_BASE_URL / WHENTO_ROTATION_API at it.
 */

import { test } from '@playwright/test';
import {
  countRefreshTokenRows,
  expect,
  installTrafficCounter,
  loginViaUI,
  openDashboard,
  openTwoPages,
  wait,
} from './session-helpers';

const BASE = process.env.WHENTO_ROTATION_BASE_URL ?? 'http://127.0.0.1:5174';
const API = process.env.WHENTO_ROTATION_API ?? 'http://127.0.0.1:5174/api/v1';
const DATABASE_URL =
  process.env.WHENTO_ROTATION_DATABASE_URL ??
  process.env.WHENTO_DATABASE_URL ??
  process.env.DATABASE_URL;

async function registerAccount(tag: string): Promise<{ email: string; password: string }> {
  const unique = `${tag}-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
  const account = {
    email: `e2e-${unique}@example.test`,
    password: 'Str0ng!Passw0rd#2026',
  };
  const response = await fetch(`${API}/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      email: account.email,
      password: account.password,
      display_name: `E2E ${tag}`,
    }),
  });
  if (!response.ok) {
    throw new Error(`${tag} registration failed: ${response.status} ${await response.text()}`);
  }
  return account;
}

test.describe('two-page forced refresh (short-TTL backend)', () => {
  test('acceptance 2 + 7: both tabs see one coordinated same-family rotation, no reload, no store reset; a new login is a new family', async ({
    browser,
  }) => {
    test.setTimeout(180_000);
    const account = await registerAccount('rot');
    const { context, pageA, pageB } = await openTwoPages(browser);
    try {
      const countA = await loginViaUI(pageA, BASE, account.email, account.password);
      const countB = installTrafficCounter(pageB);

      await openDashboard(pageA, BASE);
      await openDashboard(pageB, BASE);

      // Capture the family minted by this login.
      await expect.poll(() => countA.capturedFamilies[0] ?? null, { timeout: 15_000 }).toBeTruthy();
      const loginFamily = countA.capturedFamilies[0];
      expect(loginFamily).toBeTruthy();

      // Cold page loads may restore the cookie before the proactive timer is due.
      // Exclude those responses so they cannot satisfy the rotation assertion.
      await wait(2_000);
      countA.capturedRefreshFamilies.length = 0;
      countB.capturedRefreshFamilies.length = 0;
      const rowsBefore = await countRefreshTokenRows(DATABASE_URL, loginFamily);
      const refreshesBefore = countA.authRefresh + countB.authRefresh;
      const identityReadsBefore = countA.authMe + countB.authMe;

      // Wait for the proactive refresh: with the 2-minute TTL the client schedules it
      // at TTL - 60s = 60s. Bounded: the cookie lock lets one tab rotate, the other
      // follows over the channel without spending the cookie itself.
      const navA = countA.documentResponses;
      const navB = countB.documentResponses;
      await expect
        .poll(
          () => countA.capturedRefreshFamilies[0] ?? countB.capturedRefreshFamilies[0] ?? null,
          { timeout: 110_000, intervals: [1_000] }
        )
        .toBeTruthy();
      await wait(2_000);

      // The refreshed family is the SAME server session family — rotation, not a new
      // login, and (acceptance 7) distinct from any new login.
      const refreshFamily = countA.capturedRefreshFamilies[0] ?? countB.capturedRefreshFamilies[0];
      expect(refreshFamily).toBe(loginFamily);

      // Bounded rotation: at most one cookie-spending refresh (or two if the second
      // tab's timer happened to fire first in the same window).
      const rotations = countA.authRefresh + countB.authRefresh - refreshesBefore;
      expect(rotations).toBeGreaterThan(0);
      expect(rotations).toBeLessThanOrEqual(2);

      // No document reloads and no account-store reset: if either tab had treated the
      // rotated token as a new session it would have re-run /auth/me and reloaded.
      expect(countA.documentResponses).toBe(navA);
      expect(countB.documentResponses).toBe(navB);
      expect(countA.authMe + countB.authMe).toBe(identityReadsBefore);

      // Each observed rotation retains its consumed ancestor and inserts a
      // successor. Only unexplained growth is a session-minting regression.
      const rowsAfter = await countRefreshTokenRows(DATABASE_URL, loginFamily);
      expect(rowsAfter - rowsBefore).toBe(rotations);

      // Acceptance 7: a brand-new login is a brand-new family.
      const other = await registerAccount('rot-b');
      await pageA.getByRole('button', { name: /logout|d[eé]connexion/i }).click();
      await pageA.waitForURL(url => new URL(url).pathname === '/login', { timeout: 20_000 });
      await loginViaUI(pageA, BASE, other.email, other.password);
      await expect
        .poll(() => countA.capturedFamilies[countA.capturedFamilies.length - 1] ?? null, {
          timeout: 15_000,
        })
        .toBeTruthy();
      const newFamily = countA.capturedFamilies[countA.capturedFamilies.length - 1];
      expect(newFamily).toBeTruthy();
      expect(newFamily).not.toBe(loginFamily);
    } finally {
      await context.close();
    }
  });
});
