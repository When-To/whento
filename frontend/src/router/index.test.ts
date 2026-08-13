/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import type { RouteLocationNormalized } from 'vue-router';
import type { User } from '@/types';

const authApi = {
  register: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  getMe: vi.fn(),
  updateProfile: vi.fn(),
  updatePassword: vi.fn(),
  forgotPassword: vi.fn(),
  resetPassword: vi.fn(),
};

const apiClient = {
  setToken: vi.fn(),
  clearToken: vi.fn(),
  loadToken: vi.fn(),
};

vi.mock('@/api/auth', () => ({ authApi }));
vi.mock('@/api/client', () => ({ apiClient }));

const { authGuard, routes } = await import('./index');
const { useAuthStore } = await import('@/stores/auth');

const USER = {
  id: 'u-1',
  email: 'ada@example.com',
  display_name: 'Ada',
  role: 'user',
} as User;

const ADMIN = { ...USER, role: 'admin' } as User;

/** A promise with its resolvers exposed, so a test can control when it settles. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(res => {
    resolve = res;
  });
  return { promise, resolve };
}

/**
 * The guard only ever reads `meta` and `fullPath`, so a route stub is enough and
 * keeps the test about the guard rather than about vue-router's matcher.
 */
function target(meta: RouteLocationNormalized['meta'], fullPath = '/somewhere') {
  return { meta, fullPath } as RouteLocationNormalized;
}

const from = target({}, '/');

/** Run the guard and report what it passed to `next`. */
async function navigate(to: RouteLocationNormalized) {
  const next = vi.fn();
  await authGuard(to, from, next);
  return next;
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setActivePinia(createPinia());
});

describe('routes', () => {
  it('has a catch-all last, so no path falls through', () => {
    expect(routes[routes.length - 1].name).toBe('not-found');
  });

  it('marks every unauthenticated entry point public', () => {
    const publicNames = routes.filter(r => r.meta?.public).map(r => r.name);
    for (const name of ['home', 'login', 'register', 'calendar-public', 'participant-view']) {
      expect(publicNames).toContain(name);
    }
  });

  it('guards the admin area on both flags', () => {
    const admin = routes.find(r => r.name === 'admin');
    expect(admin?.meta).toMatchObject({ requiresAuth: true, requiresAdmin: true });
  });

  describe('lazy component loaders', () => {
    // Every route is `() => import('@/views/X.vue')`. A typo in one of those paths is
    // invisible to the type checker and to every other test: it only surfaces as a
    // blank page when a user reaches that route. Resolving them all here turns that
    // into a build-time failure.
    const lazy = routes
      .filter(route => typeof route.component === 'function')
      .map(route => [String(route.name), route.component as () => Promise<unknown>] as const);

    it('covers every route', () => {
      expect(lazy).toHaveLength(routes.length);
    });

    // Generous: the first case pulls the whole view layer through the transform
    // pipeline, which is far more than the default 5 s budget on a loaded machine.
    it.each(lazy)(
      '%s resolves to a component',
      async (_name, load) => {
        const module = (await load()) as { default?: unknown };
        expect(module.default).toBeDefined();
      },
      60_000
    );
  });
});

describe('authGuard', () => {
  describe('waiting for the session', () => {
    it('does not decide anything before the session is known', async () => {
      // The regression this replaces: the guard polled `initialized` every 100 ms and
      // gave up after fifty attempts. A slow `/auth/me` therefore either blocked
      // navigation for five seconds or, worse, timed out and evaluated `requiresAuth`
      // against an empty store — bouncing a signed-in user to /login.
      localStorage.setItem('access_token', 'stored');
      const gate = deferred<User>();
      authApi.getMe.mockReturnValue(gate.promise);

      const next = vi.fn();
      const navigation = authGuard(target({ requiresAuth: true }, '/dashboard'), from, next);

      await new Promise(resolve => setTimeout(resolve, 0));
      expect(next).not.toHaveBeenCalled();

      gate.resolve(USER);
      await navigation;

      // Allowed through, not redirected — the whole point.
      expect(next).toHaveBeenCalledTimes(1);
      expect(next).toHaveBeenCalledWith();
    });

    it('lets a slow restore through however long it takes', async () => {
      localStorage.setItem('access_token', 'stored');
      const gate = deferred<User>();
      authApi.getMe.mockReturnValue(gate.promise);

      const next = vi.fn();
      const navigation = authGuard(target({ requiresAuth: true }, '/dashboard'), from, next);

      // Longer than the old 50 x 100 ms budget.
      vi.useFakeTimers();
      await vi.advanceTimersByTimeAsync(30_000);
      vi.useRealTimers();
      expect(next).not.toHaveBeenCalled();

      gate.resolve(USER);
      await navigation;

      expect(next).toHaveBeenCalledWith();
    });

    it('starts the restore itself when main.ts has not', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);

      await navigate(target({ requiresAuth: true }, '/dashboard'));

      expect(authApi.getMe).toHaveBeenCalledTimes(1);
      expect(useAuthStore().initialized).toBe(true);
    });

    it('does not re-restore on a second navigation', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);

      await navigate(target({ requiresAuth: true }, '/dashboard'));
      await navigate(target({ requiresAuth: true }, '/settings'));

      expect(authApi.getMe).toHaveBeenCalledTimes(1);
    });
  });

  describe('requiresAuth', () => {
    it('redirects an anonymous visitor to login, keeping where they were going', async () => {
      const next = await navigate(target({ requiresAuth: true }, '/calendars/abc/settings'));

      expect(next).toHaveBeenCalledWith({
        name: 'login',
        query: { redirect: '/calendars/abc/settings' },
      });
    });

    it('lets a signed-in user through', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);

      const next = await navigate(target({ requiresAuth: true }, '/dashboard'));

      expect(next).toHaveBeenCalledWith();
    });

    it('redirects when the stored token turns out to be stale', async () => {
      localStorage.setItem('access_token', 'stale');
      authApi.getMe.mockRejectedValue({ code: 'UNAUTHORIZED' });

      const next = await navigate(target({ requiresAuth: true }, '/dashboard'));

      expect(next).toHaveBeenCalledWith({
        name: 'login',
        query: { redirect: '/dashboard' },
      });
    });
  });

  describe('requiresAdmin', () => {
    it('sends a non-admin back to the dashboard', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);

      const next = await navigate(target({ requiresAuth: true, requiresAdmin: true }, '/admin'));

      expect(next).toHaveBeenCalledWith({ name: 'dashboard' });
    });

    it('lets an admin through', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(ADMIN);

      const next = await navigate(target({ requiresAuth: true, requiresAdmin: true }, '/admin'));

      expect(next).toHaveBeenCalledWith();
    });

    it('checks authentication before admin, so an anonymous visitor lands on login', async () => {
      const next = await navigate(target({ requiresAuth: true, requiresAdmin: true }, '/admin'));

      expect(next).toHaveBeenCalledWith({ name: 'login', query: { redirect: '/admin' } });
    });
  });

  describe('hideForAuth', () => {
    it('keeps a signed-in user off the login page', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);

      const next = await navigate(target({ public: true, hideForAuth: true }, '/login'));

      expect(next).toHaveBeenCalledWith({ name: 'dashboard' });
    });

    it('leaves an anonymous visitor on it', async () => {
      const next = await navigate(target({ public: true, hideForAuth: true }, '/login'));

      expect(next).toHaveBeenCalledWith();
    });
  });

  describe('public routes', () => {
    it.each([
      ['/', {}],
      ['/c/token', { public: true }],
      ['/c/token/p/participant', { public: true }],
    ])('lets an anonymous visitor reach %s', async (fullPath, meta) => {
      const next = await navigate(target(meta, fullPath));

      expect(next).toHaveBeenCalledWith();
    });

    it('does not call /auth/me when there is no stored token', async () => {
      await navigate(target({ public: true }, '/c/token'));

      expect(authApi.getMe).not.toHaveBeenCalled();
    });
  });

  it('calls next exactly once on every path', async () => {
    localStorage.setItem('access_token', 'stored');
    authApi.getMe.mockResolvedValue(USER);

    for (const meta of [
      {},
      { public: true },
      { requiresAuth: true },
      { requiresAuth: true, requiresAdmin: true },
      { public: true, hideForAuth: true },
    ]) {
      const next = await navigate(target(meta));
      expect(next).toHaveBeenCalledTimes(1);
    }
  });
});
