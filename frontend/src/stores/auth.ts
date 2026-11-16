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
import type {
  User,
  LoginRequest,
  RegisterRequest,
  BootstrapRequest,
  BootstrapStatus,
} from '@/types';

export const useAuthStore = defineStore('auth', () => {
  // State
  const user = ref<User | null>(null);
  const initialized = ref(false);
  const { loading, error, run, clearError } = useAsyncActions();

  /**
   * Whether the instance will accept new registrations. Comes from the server's
   * ALLOWED_REGISTER setting; when false the UI must offer no register button,
   * and /register may as well not exist.
   *
   * Defaults to `true` (registration shown): a wrong guess here cannot admit
   * anyone, because the server still rejects the attempt with a 403. The
   * dangerous default is the reverse — hiding a working door the server would
   * have opened.
   */
  const registrationEnabled = ref(true);
  /**
   * Whether the instance has no users yet and so still needs its first account
   * created (through /bootstrap).
   */
  const bootstrapRequired = ref(false);
  /**
   * Whether the capability flags above reflect a real /auth/status answer rather
   * than the startup defaults. A failed (or not-yet-completed) status read must
   * not be acted on as if it were "registration open, nothing to bootstrap":
   * that exact combination is backwards on a fresh closed instance, and would
   * hide the only setup route. The guard therefore only closes /bootstrap and
   * /register when this is true.
   */
  const bootstrapStatusKnown = ref(false);

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
        apiClient.setToken(response.access_token, response.expires_in);
      }
      // A successful registration means the instance now has a user, so it can
      // no longer need bootstrapping. Clearing the flag here keeps the guard
      // from funnelling the just-registered visitor to /bootstrap — the server
      // never reads this flag, so this is purely a local-UI correction and can
      // never close a door server-side.
      bootstrapRequired.value = false;
      bootstrapStatusKnown.value = true;
      return response;
    });
  }

  /**
   * Create the first (administrator) account of an unconfigured instance.
   *
   * `bootstrapRequired` is cleared on success because the server has a user now;
   * the router guard then stops funnelling anonymous visitors to /bootstrap.
   */
  async function bootstrap(data: BootstrapRequest) {
    return run('auth.registerError', async () => {
      const response = await authApi.bootstrap(data);
      bootstrapRequired.value = false;
      user.value = response.user;
      if (response.access_token) {
        apiClient.setToken(response.access_token, response.expires_in);
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
          apiClient.setToken(response.access_token, response.expires_in);
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

  /**
   * Delete the account, then end the session locally the way logout does. There is
   * no /auth/logout call: the server already revoked every session with the account,
   * so it would only fail.
   */
  async function deleteAccount(password: string) {
    await run('settings.deleteAccount.error', () => authApi.deleteAccount(password));
    user.value = null;
    apiClient.signOut();
  }

  async function forgotPassword(email: string) {
    // Always returns success to prevent email enumeration
    return run('auth.forgotPassword.error', () => authApi.forgotPassword(email));
  }

  async function resetPassword(token: string, newPassword: string) {
    return run('auth.resetPassword.error', async () => {
      const response = await authApi.resetPassword(token, newPassword);

      // An MFA-protected account gets a pending challenge from a reset, not a
      // session: the password has been changed, but the login must not complete
      // until the second factor is verified. Mirroring login, nothing is stored
      // and no token set here — the view routes to the MFA page instead.
      if (response.require_mfa) {
        return response;
      }

      // Auto-login after successful reset
      user.value = response.user;
      if (response.access_token) {
        apiClient.setToken(response.access_token, response.expires_in);
      }

      return response;
    });
  }

  // Set tokens directly (for MFA verification and passkey login)
  function setTokens(accessToken: string, expiresIn?: number) {
    apiClient.setToken(accessToken, expiresIn);
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
   * Refresh the public auth capability state (bootstrap/mount needs, registration
   * open or not). Idempotent enough to call on every initializeAuth: it is one
   * cheap public read, and the router guard makes its redirect decisions on the
   * result.
   *
   * On failure the capability flags are *not* overwritten with the startup
   * defaults-as-if-authoritative: `bootstrapStatusKnown` goes false instead, so
   * the guard knows it has no answer and must not close the only setup route on
   * a guess. The server remains the authority for either direction.
   */
  async function loadAuthStatus() {
    try {
      const status: BootstrapStatus = await authApi.getAuthStatus();
      registrationEnabled.value = status.registration_enabled;
      bootstrapRequired.value = status.needs_bootstrap;
      bootstrapStatusKnown.value = true;
    } catch {
      bootstrapStatusKnown.value = false;
    }
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
      // Capability state first: the guard answers "may this visitor register / must
      // they bootstrap?" from it, before it worries about a session at all.
      await loadAuthStatus();

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
    registrationEnabled,
    bootstrapRequired,
    bootstrapStatusKnown,

    // Getters
    isAuthenticated,
    isAdmin,

    // Actions
    register,
    bootstrap,
    login,
    logout,
    fetchUser,
    updateProfile,
    updatePassword,
    deleteAccount,
    forgotPassword,
    resetPassword,
    setTokens,
    setTempToken,
    clearTempToken,
    loadAuthStatus,
    initializeAuth,
    whenReady,
    clearError,
  };
});
