/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { i18n } from '@/i18n';
import type { AuthResponse, User } from '@/types';

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

const { useAuthStore } = await import('./auth');

const USER: User = {
  id: 'u-1',
  email: 'ada@example.com',
  display_name: 'Ada',
  role: 'user',
  locale: 'en',
  timezone: 'Europe/Paris',
  created_at: '2026-01-01T00:00:00Z',
} as User;

const ADMIN: User = { ...USER, id: 'u-2', role: 'admin' };

function authResponse(overrides: Partial<AuthResponse> = {}): AuthResponse {
  return { user: USER, access_token: 'tok', ...overrides } as AuthResponse;
}

/** A promise with its resolvers exposed, so a test can control when it settles. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function freshStore() {
  setActivePinia(createPinia());
  return useAuthStore();
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe('auth store', () => {
  describe('getters', () => {
    it('is unauthenticated with no user', () => {
      const store = freshStore();
      expect(store.isAuthenticated).toBe(false);
      expect(store.isAdmin).toBe(false);
    });

    it('reflects the signed-in user', async () => {
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();
      await store.fetchUser();

      expect(store.isAuthenticated).toBe(true);
      expect(store.isAdmin).toBe(false);
    });

    it('recognises an admin', async () => {
      authApi.getMe.mockResolvedValue(ADMIN);
      const store = freshStore();
      await store.fetchUser();

      expect(store.isAdmin).toBe(true);
    });
  });

  describe('login', () => {
    it('stores the user and the access token', async () => {
      authApi.login.mockResolvedValue(authResponse());
      const store = freshStore();

      await store.login({ email: 'ada@example.com', password: 'pw' });

      expect(store.user).toEqual(USER);
      expect(apiClient.setToken).toHaveBeenCalledWith('tok');
    });

    it('does not start a session when a second factor is required', async () => {
      // The session must only start once VerifyMFA checks the code out: setting the
      // user here made the app look authenticated before the second factor.
      authApi.login.mockResolvedValue({ require_mfa: true, temp_token: 'temp' });
      const store = freshStore();

      const response = await store.login({ email: 'ada@example.com', password: 'pw' });

      expect(response.require_mfa).toBe(true);
      expect(store.user).toBeNull();
      expect(store.isAuthenticated).toBe(false);
      expect(apiClient.setToken).not.toHaveBeenCalled();
    });

    it('reports a 401 as bad credentials, in the user language', async () => {
      authApi.login.mockRejectedValue({
        code: 'UNAUTHORIZED',
        message: 'Invalid email or password',
      });
      const store = freshStore();

      await expect(
        store.login({ email: 'ada@example.com', password: 'nope' })
      ).rejects.toBeDefined();

      expect(store.error).toBe(i18n.global.t('auth.invalidCredentials'));
    });

    it('does not leak the backend message', async () => {
      authApi.login.mockRejectedValue({ code: 'INTERNAL_ERROR', message: 'pq: deadlock detected' });
      const store = freshStore();

      await expect(store.login({ email: 'a@b.c', password: 'pw' })).rejects.toBeDefined();

      expect(store.error).toBe(i18n.global.t('errors.serverError'));
      expect(store.error).not.toContain('pq:');
    });

    it('clears loading on both paths', async () => {
      authApi.login.mockRejectedValue(new Error('offline'));
      const store = freshStore();

      await expect(store.login({ email: 'a@b.c', password: 'pw' })).rejects.toThrow();
      expect(store.loading).toBe(false);
    });
  });

  describe('register', () => {
    it('stores the user and the token', async () => {
      authApi.register.mockResolvedValue(authResponse());
      const store = freshStore();

      await store.register({ email: 'ada@example.com', password: 'pw', display_name: 'Ada' });

      expect(store.user).toEqual(USER);
      expect(apiClient.setToken).toHaveBeenCalledWith('tok');
    });

    it('reports failure through i18n', async () => {
      authApi.register.mockRejectedValue({ code: 'BAD_REQUEST', message: 'Registration failed' });
      const store = freshStore();

      await expect(
        store.register({ email: 'a@b.c', password: 'pw', display_name: 'A' })
      ).rejects.toBeDefined();

      expect(store.error).toBe(i18n.global.t('errors.badRequest'));
    });
  });

  describe('logout', () => {
    it('drops the session', async () => {
      authApi.getMe.mockResolvedValue(USER);
      authApi.logout.mockResolvedValue(undefined);
      const store = freshStore();
      await store.fetchUser();

      await store.logout();

      expect(store.user).toBeNull();
      expect(apiClient.clearToken).toHaveBeenCalled();
    });

    it('drops the session even when the call fails, and surfaces no error', async () => {
      authApi.getMe.mockResolvedValue(USER);
      authApi.logout.mockRejectedValue({ code: 'INTERNAL_ERROR' });
      const store = freshStore();
      await store.fetchUser();

      await expect(store.logout()).resolves.toBeUndefined();

      expect(store.user).toBeNull();
      expect(apiClient.clearToken).toHaveBeenCalled();
      expect(store.error).toBeNull();
      expect(store.loading).toBe(false);
    });
  });

  describe('fetchUser', () => {
    it('clears the session when the token is rejected', async () => {
      authApi.getMe.mockRejectedValue({ code: 'UNAUTHORIZED' });
      const store = freshStore();

      await expect(store.fetchUser()).rejects.toBeDefined();

      expect(store.user).toBeNull();
      expect(apiClient.clearToken).toHaveBeenCalled();
    });
  });

  describe('profile and password', () => {
    it('replaces the user on a successful update', async () => {
      const updated = { ...USER, display_name: 'Ada L.' };
      authApi.updateProfile.mockResolvedValue(updated);
      const store = freshStore();

      await store.updateProfile({ display_name: 'Ada L.' });

      expect(store.user).toEqual(updated);
    });

    it('translates a profile update failure', async () => {
      authApi.updateProfile.mockRejectedValue(new Error('offline'));
      const store = freshStore();

      await expect(store.updateProfile({ display_name: 'x' })).rejects.toThrow();
      expect(store.error).toBe(i18n.global.t('auth.updateProfileError'));
    });

    it('translates a password change failure', async () => {
      authApi.updatePassword.mockRejectedValue({ code: 'UNAUTHORIZED' });
      const store = freshStore();

      await expect(store.updatePassword('old', 'new')).rejects.toBeDefined();
      expect(store.error).toBe(i18n.global.t('errors.unauthorized'));
    });

    it('signs the user in after a password reset', async () => {
      authApi.resetPassword.mockResolvedValue(authResponse({ access_token: 'fresh' }));
      const store = freshStore();

      await store.resetPassword('reset-token', 'new-password');

      expect(store.user).toEqual(USER);
      expect(apiClient.setToken).toHaveBeenCalledWith('fresh');
    });

    it('translates a forgotten-password failure', async () => {
      authApi.forgotPassword.mockRejectedValue({ code: 'RATE_LIMITED' });
      const store = freshStore();

      await expect(store.forgotPassword('a@b.c')).rejects.toBeDefined();
      expect(store.error).toBe(i18n.global.t('errors.rateLimited'));
    });
  });

  describe('setTokens', () => {
    it('hands the token straight to the client', () => {
      freshStore().setTokens('mfa-issued');
      expect(apiClient.setToken).toHaveBeenCalledWith('mfa-issued');
    });
  });

  describe('initializeAuth', () => {
    it('marks itself initialized without a call when there is no token', async () => {
      const store = freshStore();

      await store.initializeAuth();

      expect(authApi.getMe).not.toHaveBeenCalled();
      expect(store.initialized).toBe(true);
      expect(store.isAuthenticated).toBe(false);
    });

    it('restores the session from a stored token', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();

      await store.initializeAuth();

      expect(apiClient.loadToken).toHaveBeenCalled();
      expect(store.user).toEqual(USER);
      expect(store.initialized).toBe(true);
    });

    it('completes, and surfaces no error, when the stored token is stale', async () => {
      // An expired token is not a failure the user should read about: they are simply
      // signed out, and the guard sends them to /login if the route needs a session.
      localStorage.setItem('access_token', 'stale');
      authApi.getMe.mockRejectedValue({ code: 'UNAUTHORIZED' });
      const store = freshStore();

      await expect(store.initializeAuth()).resolves.toBeUndefined();

      expect(store.initialized).toBe(true);
      expect(store.isAuthenticated).toBe(false);
      expect(store.error).toBeNull();
      expect(apiClient.clearToken).toHaveBeenCalled();
    });

    it('runs once however many callers ask', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();

      await Promise.all([store.initializeAuth(), store.whenReady(), store.whenReady()]);

      expect(authApi.getMe).toHaveBeenCalledTimes(1);
    });

    it('holds every concurrent caller until the one fetch completes', async () => {
      // Pinia wraps actions, so the promise objects handed back are not identical;
      // what has to hold is that they all wait on the same single `/auth/me`.
      localStorage.setItem('access_token', 'stored');
      const gate = deferred<User>();
      authApi.getMe.mockReturnValue(gate.promise);
      const store = freshStore();

      const settled: string[] = [];
      const waits = [
        store.initializeAuth().then(() => settled.push('a')),
        store.whenReady().then(() => settled.push('b')),
        store.whenReady().then(() => settled.push('c')),
      ];

      await new Promise(resolve => setTimeout(resolve, 0));
      expect(settled).toEqual([]);
      expect(authApi.getMe).toHaveBeenCalledTimes(1);

      gate.resolve(USER);
      await Promise.all(waits);

      expect(settled).toHaveLength(3);
      expect(authApi.getMe).toHaveBeenCalledTimes(1);
    });

    it('does not re-run after it has settled', async () => {
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();

      await store.initializeAuth();
      await store.whenReady();

      expect(authApi.getMe).toHaveBeenCalledTimes(1);
    });

    it('gives each Pinia instance its own initialisation', async () => {
      // Module-scoped state here would leak the first test's session into the next.
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);

      await freshStore().initializeAuth();
      await freshStore().initializeAuth();

      expect(authApi.getMe).toHaveBeenCalledTimes(2);
    });
  });

  describe('whenReady', () => {
    it('starts the restore itself when nobody has', async () => {
      // The guard must not depend on main.ts having run first.
      localStorage.setItem('access_token', 'stored');
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();

      await store.whenReady();

      expect(authApi.getMe).toHaveBeenCalledTimes(1);
      expect(store.initialized).toBe(true);
    });

    it('does not resolve before the session is known, however slow that is', async () => {
      localStorage.setItem('access_token', 'stored');
      const gate = deferred<User>();
      authApi.getMe.mockReturnValue(gate.promise);
      const store = freshStore();

      let settled = false;
      const ready = store.whenReady().then(() => {
        settled = true;
      });

      // Well past the five seconds the old polling guard gave up after.
      await new Promise(resolve => setTimeout(resolve, 0));
      expect(settled).toBe(false);
      expect(store.initialized).toBe(false);

      gate.resolve(USER);
      await ready;

      expect(settled).toBe(true);
      expect(store.isAuthenticated).toBe(true);
    });
  });
});
