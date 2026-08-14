/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { authApi } from '@/api/auth';
import { apiClient } from '@/api/client';
import { useAsyncActions } from '@/stores/asyncAction';
import type { User, LoginRequest, RegisterRequest } from '@/types';

export const useAuthStore = defineStore('auth', () => {
  // State
  const user = ref<User | null>(null);
  const initialized = ref(false);
  const { loading, error, run, clearError } = useAsyncActions();

  /**
   * The one in-flight (or settled) `initializeAuth` run.
   *
   * `main.ts` deliberately kicks initialisation off without awaiting it, so the app
   * shell paints immediately. Everything that needs a settled auth state — the router
   * guard above all — awaits this promise instead of polling `initialized`. The
   * previous guard slept in 100 ms slices up to fifty times and then gave up, which
   * both blocked navigation for up to five seconds and, on timeout, evaluated
   * `requiresAuth` against a store that had never been populated, bouncing a
   * perfectly authenticated user to /login.
   *
   * Held in the setup closure rather than at module scope so each Pinia instance —
   * and so each test — gets its own.
   */
  let initPromise: Promise<void> | null = null;

  // Getters
  const isAuthenticated = computed(() => user.value !== null);
  const isAdmin = computed(() => user.value?.role === 'admin');

  // Actions
  async function register(data: RegisterRequest) {
    return run('auth.registerError', async () => {
      const response = await authApi.register(data);
      user.value = response.user;
      if (response.access_token) {
        apiClient.setToken(response.access_token);
      }
      return response;
    });
  }

  async function login(data: LoginRequest) {
    return run(
      'auth.loginError',
      async () => {
        const response = await authApi.login(data);

        // With a second factor enabled the backend answers `require_mfa` and a
        // temp_token, and no access token at all. Storing the (absent) token wrote
        // the literal string "undefined" into localStorage, and setting the user
        // made the session look authenticated before the second factor was ever
        // verified. The session only starts in VerifyMFA, once the code checks out.
        if (response.require_mfa) {
          return response;
        }

        user.value = response.user;
        if (response.access_token) {
          apiClient.setToken(response.access_token);
        }
        return response;
      },
      // A 401 from /auth/login is not "you are signed out", it is "those credentials
      // are wrong" — the only phrasing that makes sense on a login form.
      { overrides: { UNAUTHORIZED: 'auth.invalidCredentials' } }
    );
  }

  async function logout() {
    try {
      await run('auth.logoutError', () => authApi.logout());
    } catch {
      // A failed logout still logs the user out locally: the access token is
      // dropped below either way, and the refresh cookie is short-lived.
      clearError();
    } finally {
      user.value = null;
      // signOut rather than clearToken: the other tabs on this browser share the
      // refresh cookie the backend just revoked, and have to be told.
      apiClient.signOut();
    }
  }

  async function fetchUser() {
    return run('auth.fetchUserError', async () => {
      try {
        user.value = await authApi.getMe();
      } catch (err) {
        user.value = null;
        apiClient.clearToken();
        throw err;
      }
    });
  }

  async function updateProfile(data: Partial<User>) {
    return run('auth.updateProfileError', async () => {
      user.value = await authApi.updateProfile(data);
    });
  }

  async function updatePassword(oldPassword: string, newPassword: string) {
    return run('settings.passwordChangeFailed', () =>
      authApi.updatePassword(oldPassword, newPassword)
    );
  }

  async function forgotPassword(email: string) {
    // Always returns success to prevent email enumeration
    return run('auth.forgotPassword.error', () => authApi.forgotPassword(email));
  }

  async function resetPassword(token: string, newPassword: string) {
    return run('auth.resetPassword.error', async () => {
      const response = await authApi.resetPassword(token, newPassword);

      // Auto-login after successful reset
      user.value = response.user;
      if (response.access_token) {
        apiClient.setToken(response.access_token);
      }

      return response;
    });
  }

  // Set tokens directly (for MFA verification and passkey login)
  function setTokens(accessToken: string) {
    apiClient.setToken(accessToken);
    // Note: refresh_token is httpOnly cookie, handled by backend
  }

  /**
   * The five-minute token that carries a half-finished login from the password (or
   * passkey) step to the second factor.
   *
   * In memory rather than in localStorage: it authorises completing a sign-in, so
   * persisting it is the same defect as persisting the access token, and it only has
   * to survive a `router.push` — an SPA navigation, which keeps module state. A hard
   * reload on /verify-mfa loses it, and that view sends the visitor back to /login.
   */
  const tempToken = ref<string | null>(null);

  function setTempToken(token: string) {
    tempToken.value = token;
  }

  function clearTempToken() {
    tempToken.value = null;
  }

  /**
   * Restore the session from the httpOnly refresh cookie.
   *
   * A cold load starts with no access token — it only ever lives in memory — so
   * `/auth/me` 401s and the client's interceptor spends the refresh cookie and
   * replays the call. That is the same path an expired token has always taken, so
   * there is no second refresh implementation to keep honest here.
   *
   * The session flag is what keeps an anonymous visitor from paying for that round
   * trip on every public page.
   *
   * Idempotent: concurrent and repeat callers all get the same promise, so the
   * router guard calling it never triggers a second `/auth/me`.
   */
  function initializeAuth(): Promise<void> {
    initPromise ??= (async () => {
      if (apiClient.hasSession()) {
        try {
          await fetchUser();
        } catch {
          // The refresh cookie is gone or expired. Not an error the user needs to
          // see: they are simply signed out, and the guard will send them to /login
          // if the route needs a session.
          apiClient.clearToken();
          clearError();
        }
      }
      initialized.value = true;
    })();
    return initPromise;
  }

  /**
   * Resolves once the session has been restored, starting the restore if nobody
   * has yet. This is the router guard's entry point — it must never depend on
   * `main.ts` having run first, or a directly-mounted router (tests, SSR probes)
   * would wait forever.
   */
  function whenReady(): Promise<void> {
    return initializeAuth();
  }

  return {
    // State
    user,
    loading,
    error,
    initialized,
    tempToken,

    // Getters
    isAuthenticated,
    isAdmin,

    // Actions
    register,
    login,
    logout,
    fetchUser,
    updateProfile,
    updatePassword,
    forgotPassword,
    resetPassword,
    setTokens,
    setTempToken,
    clearTempToken,
    initializeAuth,
    whenReady,
    clearError,
  };
});
