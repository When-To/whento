/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AxiosAdapter, AxiosInstance, AxiosRequestConfig } from 'axios';

// The client imports the router at module scope, which would drag in every view and the
// auth store. It reads currentRoute.meta.public, .fullPath and .name, so a stub is
// enough — and it lets the tests choose the route the visitor is being thrown out of.
const routeMeta = { public: false as boolean };
const currentRoute = { fullPath: '/dashboard', name: 'dashboard' as string | undefined };
vi.mock('@/router', () => ({
  default: {
    currentRoute: {
      value: {
        get meta() {
          return routeMeta;
        },
        get fullPath() {
          return currentRoute.fullPath;
        },
        get name() {
          return currentRoute.name;
        },
      },
    },
  },
}));

const { apiClient } = await import('./client');

/** Reach the private axios instance so a fake adapter can stand in for the network. */
function instance(): AxiosInstance {
  return (apiClient as unknown as { client: AxiosInstance }).client;
}

interface Recorded {
  url: string;
  method: string;
  auth?: string;
}

interface Responder {
  (config: AxiosRequestConfig, callNumber: number): { status: number; data?: unknown };
}

/**
 * Install an adapter that records every request and answers from `responder`.
 *
 * This exercises the real interceptors — the retry, the refresh and the logout all run
 * as they do in the browser; only the socket is replaced.
 */
function withAdapter(responder: Responder): Recorded[] {
  const seen: Recorded[] = [];

  const adapter: AxiosAdapter = async config => {
    const url = config.url ?? '';
    const callNumber = seen.filter(r => r.url === url).length;

    seen.push({
      url,
      method: (config.method ?? 'get').toLowerCase(),
      auth: (config.headers as Record<string, string> | undefined)?.Authorization,
    });

    const { status, data } = responder(config, callNumber);
    const response = {
      data,
      status,
      statusText: String(status),
      headers: {},
      config: config as never,
    };

    if (status >= 200 && status < 300) return response as never;

    const error = new Error(`Request failed with status code ${status}`) as Error & {
      response?: unknown;
      config?: unknown;
      isAxiosError?: boolean;
    };
    error.response = response;
    error.config = config;
    error.isAxiosError = true;
    throw error;
  };

  instance().defaults.adapter = adapter;

  return seen;
}

const ok = (data: unknown) => ({ status: 200, data: { success: true, data } });
const unauthorized = () => ({
  status: 401,
  data: { success: false, error: { code: 'UNAUTHORIZED', message: 'nope' } },
});

describe('apiClient', () => {
  beforeEach(() => {
    localStorage.clear();
    apiClient.clearToken();
    routeMeta.public = false;
    currentRoute.fullPath = '/dashboard';
    currentRoute.name = 'dashboard';
    // jsdom refuses real navigation; forceLogout assigns to it.
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { href: '' },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    // restoreAllMocks does not undo stubGlobal, and a stubbed navigator carrying a
    // fake Web Lock would follow us into the next test.
    vi.unstubAllGlobals();
  });

  describe('tokens', () => {
    it('keeps the access token out of storage entirely', () => {
      apiClient.setToken('a-token');

      // The whole point of the change: a script that can read stored data must not
      // find the token there. Only the flag survives, and it is not a credential.
      expect(Object.values(localStorage)).not.toContain('a-token');
      expect(localStorage.getItem('whento.session')).toBe('1');
    });

    it('uses the in-memory token for requests', async () => {
      apiClient.setToken('a-token');

      const seen = withAdapter(() => ok({}));
      await apiClient.get('/anything');

      expect(seen[0].auth).toBe('Bearer a-token');
    });

    it('drops the token and the flag on clear', async () => {
      apiClient.setToken('a-token');
      apiClient.clearToken();

      expect(apiClient.hasSession()).toBe(false);
      expect(localStorage.getItem('whento.session')).toBeNull();

      const seen = withAdapter(() => ok({}));
      await apiClient.get('/anything');

      expect(seen[0].auth).toBeUndefined();
    });

    it('reports a session so a cold load knows to refresh', () => {
      expect(apiClient.hasSession()).toBe(false);

      apiClient.setToken('a-token');

      expect(apiClient.hasSession()).toBe(true);
    });

    it('sends no Authorization header when there is no token', async () => {
      const seen = withAdapter(() => ok({}));

      await apiClient.get('/anything');

      expect(seen[0].auth).toBeUndefined();
    });
  });

  describe('unwrapping', () => {
    it('returns response.data.data rather than the envelope', async () => {
      withAdapter(() => ok({ id: 7, name: 'Calendar' }));

      await expect(apiClient.get('/calendars/7')).resolves.toEqual({ id: 7, name: 'Calendar' });
    });

    it('unwraps for every verb', async () => {
      withAdapter(() => ok('payload'));

      await expect(apiClient.post('/x', {})).resolves.toBe('payload');
      await expect(apiClient.patch('/x', {})).resolves.toBe('payload');
      await expect(apiClient.delete('/x')).resolves.toBe('payload');
    });
  });

  describe('signing out across tabs', () => {
    it('tells the other tabs when the user signs out', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('a-token');

      apiClient.signOut();

      expect(apiClient.hasSession()).toBe(false);
      expect(posted).toContainEqual({ type: 'logout' });
    });

    it('stays quiet when a request merely fails', () => {
      const posted: unknown[] = [];
      vi.spyOn(BroadcastChannel.prototype, 'postMessage').mockImplementation(m => posted.push(m));
      apiClient.setToken('a-token');
      posted.length = 0;

      // The error paths — a refused /auth/me, a failed restore — go through
      // clearToken. A transient failure in one tab must not sign the others out.
      apiClient.clearToken();

      expect(posted).toEqual([]);
    });
  });

  describe('refreshing before the token dies', () => {
    it('schedules a refresh a minute short of expiry', async () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));

        // A fifteen-minute token: the refresh is due at fourteen.
        apiClient.setToken('current', 900);

        vi.advanceTimersByTime(13 * 60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);

        await vi.advanceTimersByTimeAsync(2 * 60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
      } finally {
        vi.useRealTimers();
      }
    });

    it('does nothing when the token carries no expiry', () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh' }));

        // The MFA and passkey paths used to hand over a token without one. The 401
        // path still covers those; this is an optimisation, not the mechanism.
        apiClient.setToken('current');

        vi.advanceTimersByTime(60 * 60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
      } finally {
        vi.useRealTimers();
      }
    });

    it('drops the pending refresh when the session ends', () => {
      vi.useFakeTimers();
      try {
        const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));
        apiClient.setToken('current', 900);

        apiClient.clearToken();

        vi.advanceTimersByTime(60 * 60_000);
        expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
      } finally {
        vi.useRealTimers();
      }
    });

    // A backgrounded tab does not get its timers on time — browsers throttle them and
    // suspend them outright on a discarded tab — so the wake-up is what covers a tab
    // left alone for an hour. Without it the user's first click is a 401 and a replay.
    it('refreshes on the way back from a sleeping tab', async () => {
      const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));

      // A token already inside the lead window, as one restored from sleep would be.
      apiClient.setToken('nearly-dead', 30);

      Object.defineProperty(document, 'hidden', { configurable: true, value: false });
      document.dispatchEvent(new Event('visibilitychange'));

      await vi.waitFor(() => expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1));
    });

    it('leaves a healthy token alone when the tab comes back', async () => {
      const seen = withAdapter(() => ok({ access_token: 'fresh', expires_in: 900 }));
      apiClient.setToken('plenty-of-life', 900);

      Object.defineProperty(document, 'hidden', { configurable: true, value: false });
      document.dispatchEvent(new Event('visibilitychange'));
      window.dispatchEvent(new Event('online'));
      await new Promise(resolve => setTimeout(resolve, 10));

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
    });
  });

  describe('the 401 refresh', () => {
    it('refreshes once and replays the original request', async () => {
      apiClient.setToken('expired');

      const seen = withAdapter((config, callNumber) => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'fresh' });
        // The protected call fails the first time and succeeds after the refresh.
        return callNumber === 0 ? unauthorized() : ok({ ok: true });
      });

      await expect(apiClient.get('/calendars')).resolves.toEqual({ ok: true });

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
      // The replay carries the new token, not the expired one.
      const replay = seen.filter(r => r.url === '/calendars');
      expect(replay).toHaveLength(2);
      expect(replay[1].auth).toBe('Bearer fresh');
      expect(apiClient).toBeTruthy();
    });

    /**
     * The reason refreshToken shares one in-flight promise.
     *
     * A page load fires several requests at once — the calendar issues three — and each
     * one that 401s used to start its own refresh. That is a burst against an endpoint
     * rate limited to 5 per minute per IP, and worse, refresh *rotates* the token: the
     * last response could invalidate the token an earlier one had just stored, logging
     * the user out mid-session.
     */
    it('issues one refresh for several concurrent 401s', async () => {
      apiClient.setToken('expired');

      const seen = withAdapter((config, callNumber) => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'fresh' });
        return callNumber === 0 ? unauthorized() : ok({ url: config.url });
      });

      const results = await Promise.all([
        apiClient.get('/calendars'),
        apiClient.get('/availabilities'),
        apiClient.get('/quota/limits'),
      ]);

      expect(results).toHaveLength(3);
      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
    });

    it('starts a fresh refresh after the previous one has settled', async () => {
      // The in-flight promise must be cleared, or the second expiry would reuse a
      // resolved one and never refresh again.
      apiClient.setToken('expired');

      const seen = withAdapter((config, callNumber) => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'fresh' });
        return callNumber % 2 === 0 ? unauthorized() : ok({});
      });

      await apiClient.get('/first');
      await apiClient.get('/second');

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(2);
    });

    it('does not retry a request that already retried once', async () => {
      apiClient.setToken('expired');

      const seen = withAdapter(config => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'fresh' });
        return unauthorized(); // still 401 after the refresh
      });

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      // Two attempts at the original, one refresh — not an endless loop.
      expect(seen.filter(r => r.url === '/calendars')).toHaveLength(2);
      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(1);
    });

    it('never tries to refresh for the auth endpoints themselves', async () => {
      const seen = withAdapter(() => unauthorized());

      await expect(apiClient.post('/auth/login', {})).rejects.toBeTruthy();

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
    });

    it('logs out when the refresh itself fails', async () => {
      apiClient.setToken('expired');

      withAdapter(config => (config.url === '/auth/refresh' ? unauthorized() : unauthorized()));

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(apiClient.hasSession()).toBe(false);
      expect(window.location.href).toBe('/login?redirect=%2Fdashboard');
    });

    it('carries the page they were thrown out of into the login URL', async () => {
      // Without the query, signing back in always landed on the dashboard however deep
      // the page the session expired on. The guard already produces this shape for an
      // anonymous visitor; a mid-session expiry now matches it.
      currentRoute.fullPath = '/calendars/abc-123/settings';
      currentRoute.name = 'calendar-settings';
      apiClient.setToken('expired');

      withAdapter(() => unauthorized());

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(window.location.href).toBe('/login?redirect=%2Fcalendars%2Fabc-123%2Fsettings');
    });

    it('does not ask to be sent back to the login page', async () => {
      currentRoute.fullPath = '/login';
      currentRoute.name = 'login';
      apiClient.setToken('expired');

      withAdapter(() => unauthorized());

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(window.location.href).toBe('/login');
    });

    it('skips the call when another tab refreshed while we queued', async () => {
      apiClient.setToken('expired');

      // Stand in for the Web Lock: while the second tab waits its turn, the winning
      // tab's token arrives over the channel. Spending the cookie again here would
      // rotate away the token we were just handed and sign both tabs out.
      const locks = {
        request: async (_name: string, fn: () => Promise<void>) => {
          apiClient.setToken('from-another-tab');
          return fn();
        },
      };
      vi.stubGlobal('navigator', { ...navigator, locks });

      const seen = withAdapter((config, callNumber) => {
        if (config.url === '/auth/refresh') return ok({ access_token: 'rotated-away' });
        return callNumber === 0 ? unauthorized() : ok({ ok: true });
      });

      await expect(apiClient.get('/calendars')).resolves.toEqual({ ok: true });

      expect(seen.filter(r => r.url === '/auth/refresh')).toHaveLength(0);
      const replay = seen.filter(r => r.url === '/calendars');
      expect(replay[replay.length - 1].auth).toBe('Bearer from-another-tab');
    });

    it('does not redirect away from a public route', async () => {
      // A participant following a calendar link is not signed in and must not be
      // bounced to a login page they have no account for.
      routeMeta.public = true;
      apiClient.setToken('expired');

      withAdapter(() => unauthorized());

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(apiClient.hasSession()).toBe(false);
      expect(window.location.href).toBe('');
    });
  });

  describe('error normalisation', () => {
    it('surfaces the server error envelope', async () => {
      withAdapter(() => ({
        status: 409,
        data: { success: false, error: { code: 'CONFLICT', message: 'Already exists' } },
      }));

      await expect(apiClient.post('/calendars', {})).rejects.toMatchObject({
        code: 'CONFLICT',
        message: 'Already exists',
      });
    });

    it('falls back when the response carries no envelope', async () => {
      withAdapter(() => ({ status: 500, data: 'plain text' }));

      await expect(apiClient.get('/calendars')).rejects.toMatchObject({
        message: expect.stringContaining('500'),
      });
    });
  });
});
