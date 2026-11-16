/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { Client } from 'pg';

/**
 * Two-page session-coordination helpers.
 *
 * The acceptance suite for PR-7 has to see two tabs of one browser (shared refresh
 * cookie, shared localStorage, shared BroadcastChannel) driving the *real* server.
 * These helpers stand between the spec and Playwright: login through the UI, count
 * auth traffic, and count refresh-token rows directly in
 * the disposable Postgres so "no growing token rows" is measured, not assumed.
 */

export interface TrafficCounter {
  authRefresh: number;
  authMe: number;
  authLogin: number;
  authLogout: number;
  authRegister: number;
  documentRequests: string[];
  /** Successful main-frame navigations (document responses in the top frame). */
  documentResponses: number;
  capturedTokens: string[];
  capturedFamilies: string[];
  capturedRefreshFamilies: string[];
}

export function newCounter(): TrafficCounter {
  return {
    authRefresh: 0,
    authMe: 0,
    authLogin: 0,
    authLogout: 0,
    authRegister: 0,
    documentRequests: [],
    documentResponses: 0,
    capturedTokens: [],
    capturedFamilies: [],
    capturedRefreshFamilies: [],
  };
}

/**
 * Watch every request and every top-frame document load a tab makes, plus the auth
 * session family handed out by the login endpoint (for stale-message tests).
 */
export function installTrafficCounter(page: Page): TrafficCounter {
  const counter = newCounter();

  page.on('request', request => {
    const url = new URL(request.url());
    const path = url.pathname;
    if (path.endsWith('/auth/refresh')) counter.authRefresh += 1;
    else if (path.endsWith('/auth/me')) counter.authMe += 1;
    else if (path.endsWith('/auth/login')) counter.authLogin += 1;
    else if (path.endsWith('/auth/logout')) counter.authLogout += 1;
    else if (path.endsWith('/auth/register')) counter.authRegister += 1;

    if (request.resourceType() === 'document') {
      counter.documentRequests.push(url.pathname);
    }

    const auth = request.headers()['authorization'];
    if (auth?.startsWith('Bearer ') && url.pathname.startsWith('/api/v1')) {
      counter.capturedTokens.push(auth.slice('Bearer '.length));
    }
  });

  page.on('requestfinished', async request => {
    if (request.resourceType() !== 'document') return;
    counter.documentResponses += 1;
  });

  page.on('response', response => {
    const url = new URL(response.url());
    if (url.pathname.endsWith('/auth/login') || url.pathname.endsWith('/auth/refresh')) {
      void response
        .json()
        .then(body => {
          const family = (body?.data as { session_id?: string } | undefined)?.session_id;
          if (typeof family !== 'string') return;
          if (url.pathname.endsWith('/auth/refresh')) {
            counter.capturedRefreshFamilies.push(family);
          } else {
            counter.capturedFamilies.push(family);
          }
        })
        .catch(() => {});
    }
  });

  return counter;
}

export interface TwoPages {
  context: BrowserContext;
  pageA: Page;
  pageB: Page;
}

/** One browser context, two tabs: shared cookies, localStorage and BroadcastChannel. */
export async function openTwoPages(browser: Browser): Promise<TwoPages> {
  const context = await browser.newContext();
  return { context, pageA: await context.newPage(), pageB: await context.newPage() };
}

/** Log in through the real login form. Returns the traffic counter for the tab. */
export async function loginViaUI(page: Page, baseURL: string, email: string, password: string) {
  const counter = installTrafficCounter(page);
  await page.goto(`${baseURL}/login`, { waitUntil: 'domcontentloaded' });
  await page.fill('#email', email);
  await page.fill('#password', password);
  await Promise.all([
    page.waitForURL('**/dashboard', { timeout: 20_000 }),
    // The form's submit button, not the "sign in with passkey" secondary button.
    page.locator('button[type="submit"]').click(),
  ]);
  // The dashboard is live: the account list promise has settled (calendars render,
  // or the empty state does).
  await page.waitForSelector('body');
  await page.waitForTimeout(300);
  return counter;
}

/** An explicit, deliberate navigation to the dashboard in `page`. */
export async function openDashboard(page: Page, baseURL: string) {
  await page.goto(`${baseURL}/dashboard`, { waitUntil: 'domcontentloaded' });
  await page.waitForURL('**/dashboard');
  await page.waitForSelector('body');
}

export function wait(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Rows for this scenario's family only. This assertion is mandatory: missing
 * configuration or a database client failure must fail the acceptance suite.
 */
export async function countRefreshTokenRows(
  databaseUrl: string | undefined,
  family: string | undefined
): Promise<number> {
  if (!databaseUrl || !family) {
    throw new Error('Session acceptance requires DATABASE_URL and a captured server family.');
  }
  const client = new Client({ connectionString: databaseUrl });
  try {
    await client.connect();
    const result = await client.query<{ n: number }>(
      'SELECT COUNT(*)::int AS n FROM refresh_tokens WHERE family_id = $1',
      [family]
    );
    return result.rows[0].n;
  } finally {
    await client.end().catch(() => {});
  }
}

/**
 * Post a BroadcastChannel message, exactly as one *other tab* would, from inside the
 * page. The channel names are the same origin strings the app uses.
 */
export async function postChannel(
  page: Page,
  message: unknown,
  channel = 'whento.auth'
): Promise<void> {
  await page.evaluate(
    ([ch, msg]) => {
      const channelInstance = new BroadcastChannel(ch);
      channelInstance.postMessage(msg);
      channelInstance.close();
    },
    [channel, message] as const
  );
}

export { expect };
