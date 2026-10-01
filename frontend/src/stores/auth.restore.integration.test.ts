/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

/**
 * Integration regressions for audit 31 F1: the auth-store callers must carry the
 * client's definitive-versus-transient failure distinction all the way through.
 *
 * The real singleton client, the real authApi and the real auth store run here —
 * only the network adapter and the router shape are replaced. A temporary
 * infrastructure failure (HTTP 500) during a cold restore or a re-fetch must not
 * evict the recoverable session: the `whento.session` restore marker and the
 * in-memory token have to survive, so a later load (or a fresh store) can retry
 * `/auth/me` once the outage is over. `auth.test.ts` cannot cover these caller
 * paths because it mocks both `@/api/client` and `@/api/auth` outright.
 */

import { afterEach, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import type { AxiosInstance } from 'axios';

// The client imports the router at module scope and reads currentRoute.meta.public
// to decide where forceLogout lands; a public route keeps the redirect a no-op.
vi.mock('@/router', () => ({
  default: { currentRoute: { value: { meta: { public: true }, fullPath: '/', name: 'home' } } },
}));

const { apiClient } = await import('@/api/client');
const { useAuthStore } = await import('./auth');
const USER = {
  id: 'audit-user',
  email: 'audit@example.test',
  display_name: 'Audit',
  role: 'user',
  locale: 'en',
  timezone: 'Europe/Berlin',
  email_verified: true,
  created_at: '2026-01-01T00:00:00Z',
} as const;

afterEach(() => {
  apiClient.clearToken();
  localStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

/** Reach the singleton's private axios instance so a fake adapter can stand in for the network. */
function instance(): AxiosInstance {
  return (apiClient as unknown as { client: AxiosInstance }).client;
}

/** Reject like axios: an AxiosError-shaped object carrying the response. */
function fail(config: unknown, status: number, code: string): never {
  throw Object.assign(new Error(`Request failed with status code ${status}`), {
    config,
    isAxiosError: true,
    response: { status, config, data: { success: false, error: { code, message: 'boom' } } },
  });
}

it('preserves the recoverable session through a cold-start refresh 500 and recovers on a later load', async () => {
  localStorage.clear();
  // The client uses Web Locks when available; stub them so the refresh path stays
  // simple and deterministic in jsdom.
  vi.stubGlobal('navigator', { locks: { request: (_name: string, fn: () => unknown) => fn() } });
  localStorage.setItem('whento.session', '1');
  setActivePinia(createPinia());
  const auth = useAuthStore();

  const seen: string[] = [];
  let outage = true;
  instance().defaults.adapter = async config => {
    const url = config.url ?? '';
    seen.push(url);
    const ok = (data: unknown) => ({
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
      data: { success: true, data },
    });
    if (url === '/auth/status') return ok({ needs_bootstrap: false, registration_enabled: true });
    if (url === '/auth/refresh') {
      if (outage) fail(config, 500, 'INTERNAL');
      return ok({ access_token: 'recovered', session_id: 'recoverable-family', expires_in: 900 });
    }
    if (url === '/auth/me') {
      if (!config.headers.Authorization) fail(config, 401, 'UNAUTHORIZED');
      return ok(USER);
    }
    throw new Error(`unexpected request ${url}`);
  };

  await auth.initializeAuth();
  const preserved = apiClient.hasSession();

  // Backend recovers; the persisted restore marker must let a fresh store (a new
  // load) retry /auth/me instead of settling for anonymous.
  outage = false;
  setActivePinia(createPinia());
  const reloaded = useAuthStore();
  await reloaded.initializeAuth();

  expect({ preserved, recovered: reloaded.isAuthenticated, seen }).toEqual({
    preserved: true,
    recovered: true,
    seen: [
      '/auth/status',
      '/auth/me',
      '/auth/refresh',
      '/auth/status',
      '/auth/me',
      '/auth/refresh',
      '/auth/me',
    ],
  });
});

it('does not evict an already known user when a re-fetch /auth/me fails with 500', async () => {
  localStorage.clear();
  setActivePinia(createPinia());
  const auth = useAuthStore();
  auth.user = USER;
  apiClient.setToken('healthy-token', undefined, 'healthy-family');

  instance().defaults.adapter = async config => fail(config, 500, 'INTERNAL');

  await auth.fetchUser().catch(() => undefined);

  expect({ session: apiClient.hasSession(), user: auth.user?.id }).toEqual({
    session: true,
    user: 'audit-user',
  });
});

it('keeps the accepted remote session on a transient /auth/me hydration failure', async () => {
  const { handleRemoteSessionRestored } = await import('./remoteSignout');
  localStorage.clear();
  setActivePinia(createPinia());
  // The client has accepted another tab's fresh login: an access token and the
  // shared restore marker are in place, with the epoch presented by that session.
  apiClient.setToken('healthy-token', undefined, 'healthy-family');
  const epoch = apiClient.getSessionEpoch();

  // The hydration /auth/me hits a transient server fault.
  instance().defaults.adapter = async config => fail(config, 500, 'INTERNAL');

  const reload = () => {};
  handleRemoteSessionRestored(epoch ?? 0, { reload });

  await new Promise(resolve => setTimeout(resolve, 0));
  await new Promise(resolve => setTimeout(resolve, 0));

  // A transient failure proves nothing about the accepted session: the token and
  // the restore marker survive so the next load can hydrate the account.
  expect(apiClient.hasSession()).toBe(true);
});
