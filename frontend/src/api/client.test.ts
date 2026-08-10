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
// auth store. Only currentRoute.meta.public is read, so a stub is enough — and it lets
// the tests choose whether the current route is public.
const routeMeta = { public: false as boolean };
vi.mock('@/router', () => ({
  default: {
    currentRoute: {
      value: {
        get meta() {
          return routeMeta;
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
    // jsdom refuses real navigation; forceLogout assigns to it.
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { href: '' },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('tokens', () => {
    it('persists and restores the access token', () => {
      apiClient.setToken('a-token');
      expect(localStorage.getItem('access_token')).toBe('a-token');

      apiClient.clearToken();
      expect(localStorage.getItem('access_token')).toBeNull();

      localStorage.setItem('access_token', 'restored');
      apiClient.loadToken();

      const seen = withAdapter(() => ok({}));
      return apiClient.get('/anything').then(() => {
        expect(seen[0].auth).toBe('Bearer restored');
      });
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

      expect(localStorage.getItem('access_token')).toBeNull();
      expect(window.location.href).toBe('/login');
    });

    it('does not redirect away from a public route', async () => {
      // A participant following a calendar link is not signed in and must not be
      // bounced to a login page they have no account for.
      routeMeta.public = true;
      apiClient.setToken('expired');

      withAdapter(() => unauthorized());

      await expect(apiClient.get('/calendars')).rejects.toBeTruthy();

      expect(localStorage.getItem('access_token')).toBeNull();
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
