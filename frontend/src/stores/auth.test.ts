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
  bootstrap: vi.fn(),
  bootstrapStatus: vi.fn(async () => ({ needs_bootstrap: false, registration_enabled: true })),
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
  hasSession: vi.fn(() => false),
  signOut: vi.fn(),
  logoutThroughLock: vi.fn((run: () => unknown) => run()),
};

vi.mock('@/api/auth', () => ({ authApi }));
vi.mock('@/api/client', () => ({ apiClient }));

const { useAuthStore } = await import('./auth');
const { useCalendarStore } = await import('./calendar');

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
  // clearAllMocks clears calls, not implementations: without this a test that
  // opts into a session leaves every later test signed in.
  apiClient.hasSession.mockReturnValue(false);
  authApi.bootstrapStatus.mockResolvedValue({
    needs_bootstrap: false,
    registration_enabled: true,
  });
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
      expect(apiClient.setToken).toHaveBeenCalledWith('tok', undefined, undefined);
    });

    it('passes the token lifetime on, so the client can refresh ahead of it', async () => {
      authApi.login.mockResolvedValue(authResponse({ expires_in: 900 }));
      const store = freshStore();

      await store.login({ email: 'ada@example.com', password: 'pw' });

      expect(apiClient.setToken).toHaveBeenCalledWith('tok', 900, undefined);
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
      expect(apiClient.setToken).toHaveBeenCalledWith('tok', undefined, undefined);
    });

    it('clears bootstrapRequired after a successful first registration', async () => {
      authApi.bootstrapStatus.mockResolvedValue({
        needs_bootstrap: true,
        registration_enabled: true,
      });
      authApi.bootstrap.mockResolvedValue(authResponse());
      const store = freshStore();
      await store.initializeAuth();

      // A fresh instance: the guard is currently funnelling anonymous visitors
      // to /bootstrap.
      expect(store.bootstrapRequired).toBe(true);

      // The first account is then created through open registration instead. The
      // instance now has a user, so the client must stop treating bootstrap as
      // needed — otherwise the guard keeps bouncing the new visitor to a page
      // whose POST would 409. The server is the authority and never reads this
      // flag, so this can only correct the UI, never close a door.
      await store.register({ email: 'ada@example.com', password: 'pw', display_name: 'Ada' });

      expect(store.bootstrapRequired).toBe(false);
      expect(store.bootstrapStatusKnown).toBe(true);
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

  describe('bootstrap', () => {
    it('creates the first user and starts a session', async () => {
      authApi.bootstrap.mockResolvedValue(authResponse({ access_token: 'boot-token' }));
      authApi.bootstrapStatus.mockResolvedValue({
        needs_bootstrap: true,
        registration_enabled: false,
      });
      const store = freshStore();
      await store.initializeAuth();

      expect(store.bootstrapRequired).toBe(true);

      await store.bootstrap({
        boot_key: 'operator-key-123',
        email: 'ada@example.com',
        password: 'pw',
        display_name: 'Ada',
      });

      expect(store.bootstrapRequired).toBe(false);
    });

    it('reports failure through i18n', async () => {
      authApi.bootstrap.mockRejectedValue({
        code: 'UNAUTHORIZED',
        message: 'Invalid bootstrap key',
      });
      const store = freshStore();

      await expect(
        store.bootstrap({
          boot_key: 'wrong-key-1234567890',
          email: 'a@b.c',
          password: 'pw',
          display_name: 'A',
        })
      ).rejects.toBeDefined();

      expect(store.error).toBe(i18n.global.t('errors.unauthorized'));
    });
  });

  describe('registration capability flags', () => {
    it('defaults to registration open, no bootstrap needed', () => {
      const store = freshStore();
      // Not authoritative: the flags are a fallback until /auth/status answers.
      expect(store.registrationEnabled).toBe(true);
      expect(store.bootstrapRequired).toBe(false);
      expect(store.bootstrapStatusKnown).toBe(false);
    });

    it('loads both flags from the public status endpoint during init', async () => {
      authApi.bootstrapStatus.mockResolvedValue({
        needs_bootstrap: true,
        registration_enabled: false,
      });
      const store = freshStore();

      await store.initializeAuth();

      expect(store.bootstrapRequired).toBe(true);
      expect(store.registrationEnabled).toBe(false);
      // Now authoritative: the guard may redirect on these.
      expect(store.bootstrapStatusKnown).toBe(true);
    });

    it('marks the capability state unknown when the status call fails', async () => {
      authApi.bootstrapStatus.mockRejectedValue(new Error('offline'));
      const store = freshStore();

      await store.initializeAuth();

      // The defaults stay, but they are explicitly *not* authoritative: the
      // guard must not close /bootstrap on a stale "configured" guess when the
      // only authoritative answer was "unreachable".
      expect(store.registrationEnabled).toBe(true);
      expect(store.bootstrapRequired).toBe(false);
      expect(store.bootstrapStatusKnown).toBe(false);
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
      expect(apiClient.signOut).toHaveBeenCalled();
    });

    it('drops the session even when the call fails, and surfaces no error', async () => {
      authApi.getMe.mockResolvedValue(USER);
      authApi.logout.mockRejectedValue({ code: 'INTERNAL_ERROR' });
      const store = freshStore();
      await store.fetchUser();

      await expect(store.logout()).resolves.toBeUndefined();

      expect(store.user).toBeNull();
      expect(apiClient.signOut).toHaveBeenCalled();
      expect(store.error).toBeNull();
      expect(store.loading).toBe(false);
    });

    it('clears account-scoped calendar state so the next account cannot inherit it', async () => {
      authApi.logout.mockResolvedValue(undefined);
      const store = freshStore();
      store.user = USER;

      const calendarStore = useCalendarStore();
      // Account A's dashboard had populated its own calendars (with full participant
      // ids) on this browser; the store now tracks which account that data belongs to.
      calendarStore.calendars = [
        { id: 'c-1', name: 'A-owned', participants: [{ id: 'p-1', name: 'Ada' }] } as never,
      ];
      calendarStore.calendarsForUser = USER.id;

      await store.logout();

      // A public calendar link is a capability: the next account on this browser must
      // not be able to reuse A's participant ids as ownership evidence.
      expect(calendarStore.calendars).toEqual([]);
      expect(calendarStore.calendarsForUser).toBeNull();
    });
  });

  describe('fetchUser', () => {
    it('clears the session when the token is rejected', async () => {
      authApi.getMe.mockRejectedValue({ code: 'UNAUTHORIZED' });
      const store = freshStore();

      await expect(store.fetchUser()).rejects.toBeDefined();

      expect(store.user).toBeNull();
      expect(apiClient.clearToken).toHaveBeenCalled();
      // Local only. A refused /auth/me is not a decision to sign out, and must not
      // reach the other tabs on this browser.
      expect(apiClient.signOut).not.toHaveBeenCalled();
    });

    it('does not overwrite a replacement identity with an old /auth/me answer', async () => {
      // Audit 28 F2: an *unqualified* /auth/me (no epoch supplied) must still be
      // fenced against a replacement that lands while it is in flight. Hold the old
      // request, sign in as B through the real login action, then release A's stale
      // payload — it must not overwrite B.
      const old = deferred<User>();
      authApi.getMe.mockReturnValueOnce(old.promise);
      const store = freshStore();

      const pending = store.fetchUser();
      authApi.login.mockResolvedValue(
        authResponse({ user: { ...USER, id: 'u-b', email: 'b@example.test', display_name: 'B' } })
      );
      await store.login({ email: 'b@example.test', password: 'pw' });
      expect(store.user?.id).toBe('u-b');

      old.resolve(USER);
      await pending;

      expect(store.user?.id).toBe('u-b');
    });

    it('does not clear a replacement token when an old /auth/me fails', async () => {
      // Audit 28 F2 failure side: a stale error for a superseded restore must not
      // clear the replacement session's token. The login advanced the fence, so the
      // old failure is discarded.
      const old = deferred<User>();
      authApi.getMe.mockReturnValueOnce(old.promise);
      const store = freshStore();

      const pending = store.fetchUser().catch(() => undefined);
      authApi.login.mockResolvedValue(
        authResponse({ user: { ...USER, id: 'u-b', email: 'b@example.test', display_name: 'B' } })
      );
      await store.login({ email: 'b@example.test', password: 'pw' });
      apiClient.clearToken.mockClear();

      old.reject(new Error('old /auth/me failed'));
      await pending;

      expect(store.user?.id).toBe('u-b');
      expect(apiClient.clearToken).not.toHaveBeenCalled();
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

    it('does not write a late profile update into the account that superseded it', async () => {
      const store = freshStore();
      store.user = USER;

      const profileUpdate = deferred<User>();
      const updatedA = { ...USER, display_name: 'Ada L.' };
      authApi.updateProfile.mockReturnValue(profileUpdate.promise);
      const pending = store.updateProfile({ display_name: 'Ada L.' });

      // A signs out and B signs in while A's profile save is still in flight.
      store.user = null;
      const userB = { ...USER, id: 'u-9', email: 'b@example.test', display_name: 'Bee' };
      store.user = userB;

      profileUpdate.resolve(updatedA);
      await pending;

      // A's values never reached B's user, and the caller is told nothing was committed.
      await expect(pending).resolves.toBeNull();
      expect(store.user).toEqual(userB);
      expect(store.user?.display_name).toBe('Bee');
    });

    it('does not let a late save from an old session overwrite a replacement session for the same user', async () => {
      // The user-id fence alone cannot tell "same account, same session" from "same
      // account, replacement session": a logout followed by a fresh login to the same
      // account must still fence off the old session's in-flight continuations.
      const store = freshStore();
      authApi.login.mockResolvedValue(authResponse());
      await store.login({ email: 'ada@example.com', password: 'pw' });
      expect(store.user?.id).toBe(USER.id);

      const profileUpdate = deferred<User>();
      authApi.updateProfile.mockReturnValue(profileUpdate.promise);
      const pending = store.updateProfile({ display_name: 'Ada L.' });

      // A logs out, then signs straight back in as the *same* account.
      authApi.logout.mockResolvedValue(undefined);
      await store.logout();
      expect(store.user).toBeNull();

      authApi.login.mockResolvedValue(
        authResponse({ user: { ...USER, display_name: 'Ada (fresh)' } })
      );
      await store.login({ email: 'ada@example.com', password: 'pw' });
      expect(store.user?.display_name).toBe('Ada (fresh)');

      // The old session's save lands after the replacement session is already in use.
      profileUpdate.resolve({ ...USER, display_name: 'Ada L.' });
      const result = await pending;

      // Nothing from the dead session was committed; the replacement survives.
      expect(result).toBeNull();
      expect(store.user?.id).toBe(USER.id);
      expect(store.user?.display_name).toBe('Ada (fresh)');
    });

    it('does not transmit a queued A-save after B signed in (cross-account queue)', async () => {
      // The settings queue serialises the two forms behind a shared chain. A write
      // queued behind a still-in-flight one must recheck the account fence *before* it
      // is sent: if A's second save ran after B logged in, it would carry A's fields
      // with B's token on the wire — the post-response fence could only stop the Pinia
      // commit, never the server mutation.
      const store = freshStore();
      authApi.login.mockResolvedValue(authResponse());
      await store.login({ email: 'ada@example.com', password: 'pw' });
      expect(store.user?.id).toBe(USER.id);

      // Save 1 is in flight; save 2 queued behind it for the same account.
      const firstSave = deferred<User>();
      authApi.updateProfile.mockReturnValueOnce(firstSave.promise);
      const displaySave = store.updateProfile({ display_name: 'Ada L.' });
      const localeSave = store.updateProfile({ locale: 'en' });
      await Promise.resolve();
      expect(authApi.updateProfile).toHaveBeenCalledTimes(1);

      // A logs out, and B signs in while save 1 is still on the wire.
      authApi.logout.mockResolvedValue(undefined);
      await store.logout();
      authApi.login.mockResolvedValue(
        authResponse({
          user: { ...USER, id: 'u-9', email: 'b@example.test', display_name: 'Bee' },
        })
      );
      await store.login({ email: 'b@example.test', password: 'pw' });

      // Save 1 settles; save 2's queued turn comes up.
      firstSave.resolve({ ...USER, display_name: 'Ada L.', locale: 'en', id: USER.id });
      const localeResult = await localeSave;
      const displayResult = await displaySave;

      // Save 2 was never sent — the wire only ever saw save 1.
      expect(authApi.updateProfile).toHaveBeenCalledTimes(1);
      expect(localeResult).toBeNull();
      expect(displayResult).toBeNull();
      // B's account is untouched.
      expect(store.user?.id).toBe('u-9');
      expect(store.user?.display_name).toBe('Bee');
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
      expect(apiClient.setToken).toHaveBeenCalledWith('fresh', undefined, undefined);
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
      expect(apiClient.setToken).toHaveBeenCalledWith('mfa-issued', undefined, undefined);
    });
  });

  describe('the MFA temp token', () => {
    it('holds it in memory and never in storage', () => {
      const store = freshStore();

      store.setTempToken('half-signed-in');

      expect(store.tempToken).toBe('half-signed-in');
      // It authorises finishing a sign-in, so persisting it is the same defect as
      // persisting the access token.
      expect(Object.values(localStorage)).not.toContain('half-signed-in');
    });

    it('clears on demand, so a cancelled second factor leaves nothing behind', () => {
      const store = freshStore();
      store.setTempToken('half-signed-in');

      store.clearTempToken();

      expect(store.tempToken).toBeNull();
    });
  });

  describe('initializeAuth', () => {
    it('marks itself initialized without a call when there is no session', async () => {
      const store = freshStore();

      await store.initializeAuth();

      expect(authApi.getMe).not.toHaveBeenCalled();
      expect(store.initialized).toBe(true);
      expect(store.isAuthenticated).toBe(false);
    });

    it('restores the session from the refresh cookie', async () => {
      apiClient.hasSession.mockReturnValue(true);
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();

      await store.initializeAuth();

      // No token is loaded from anywhere: /auth/me goes out bare, 401s, and the
      // client's interceptor spends the httpOnly cookie to replay it.
      expect(apiClient.setToken).not.toHaveBeenCalled();
      expect(store.user).toEqual(USER);
      expect(store.initialized).toBe(true);
    });

    it('completes, and surfaces no error, when the stored token is stale', async () => {
      // An expired token is not a failure the user should read about: they are simply
      // signed out, and the guard sends them to /login if the route needs a session.
      apiClient.hasSession.mockReturnValue(true);
      authApi.getMe.mockRejectedValue({ code: 'UNAUTHORIZED' });
      const store = freshStore();

      await expect(store.initializeAuth()).resolves.toBeUndefined();

      expect(store.initialized).toBe(true);
      expect(store.isAuthenticated).toBe(false);
      expect(store.error).toBeNull();
      expect(apiClient.clearToken).toHaveBeenCalled();
    });

    it('does not clear a replacement token when a stale startup restore fails', async () => {
      // Audit 28 F2 outer path: initializeAuth's own cleanup must also be fenced. A
      // cold-start restore whose /auth/me fails after a replacement session was
      // established must not clear the replacement's token.
      apiClient.hasSession.mockReturnValue(true);
      const gate = deferred<User>();
      authApi.getMe.mockReturnValueOnce(gate.promise);
      const store = freshStore();

      const pending = store.initializeAuth();
      // Let the cold restore actually reach /auth/me (and capture its boundary)
      // before B signs in.
      await vi.waitFor(() => expect(authApi.getMe).toHaveBeenCalled());

      // B signs in while the cold restore is still in flight, advancing the fence.
      authApi.login.mockResolvedValue(
        authResponse({ user: { ...USER, id: 'u-b', email: 'b@example.test', display_name: 'B' } })
      );
      await store.login({ email: 'b@example.test', password: 'pw' });
      expect(store.user?.id).toBe('u-b');
      apiClient.clearToken.mockClear();

      gate.reject(new Error('cold restore failed'));
      await expect(pending).resolves.toBeUndefined();

      expect(store.user?.id).toBe('u-b');
      expect(apiClient.clearToken).not.toHaveBeenCalled();
    });

    it('runs once however many callers ask', async () => {
      apiClient.hasSession.mockReturnValue(true);
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();

      await Promise.all([store.initializeAuth(), store.whenReady(), store.whenReady()]);

      expect(authApi.getMe).toHaveBeenCalledTimes(1);
    });

    it('holds every concurrent caller until the one fetch completes', async () => {
      // Pinia wraps actions, so the promise objects handed back are not identical;
      // what has to hold is that they all wait on the same single `/auth/me`.
      apiClient.hasSession.mockReturnValue(true);
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
      apiClient.hasSession.mockReturnValue(true);
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();

      await store.initializeAuth();
      await store.whenReady();

      expect(authApi.getMe).toHaveBeenCalledTimes(1);
    });

    it('gives each Pinia instance its own initialisation', async () => {
      // Module-scoped state here would leak the first test's session into the next.
      apiClient.hasSession.mockReturnValue(true);
      authApi.getMe.mockResolvedValue(USER);

      await freshStore().initializeAuth();
      await freshStore().initializeAuth();

      expect(authApi.getMe).toHaveBeenCalledTimes(2);
    });
  });

  describe('whenReady', () => {
    it('starts the restore itself when nobody has', async () => {
      // The guard must not depend on main.ts having run first.
      apiClient.hasSession.mockReturnValue(true);
      authApi.getMe.mockResolvedValue(USER);
      const store = freshStore();

      await store.whenReady();

      expect(authApi.getMe).toHaveBeenCalledTimes(1);
      expect(store.initialized).toBe(true);
    });

    it('does not resolve before the session is known, however slow that is', async () => {
      apiClient.hasSession.mockReturnValue(true);
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
