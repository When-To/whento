/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { authApi } from '@/api/auth';
import { apiClient } from '@/api/client';
import { isDefinitiveRejection } from '@/api/failureClassification';
import { useAsyncActions } from '@/stores/asyncAction';
import { resetAccountScopedState } from '@/stores/remoteSignout';
import { currentAccountGeneration, bumpAccountGeneration } from '@/accountFence';
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
   * created (through /bootstrap or through the first registration).
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
      bumpAccountGeneration();
      if (response.access_token) {
        apiClient.setToken(response.access_token, response.expires_in, response.session_id);
      }
      // A successful (first) registration means the instance now has a user, so
      // it can no longer need bootstrapping. Clearing the flag here keeps the
      // guard from funnelling the just-registered visitor to /bootstrap — the
      // server never reads this flag, so this is purely a local-UI correction
      // and can never close a door server-side.
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
      bumpAccountGeneration();
      if (response.access_token) {
        apiClient.setToken(response.access_token, response.expires_in, response.session_id);
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
        bumpAccountGeneration();
        if (response.access_token) {
          apiClient.setToken(response.access_token, response.expires_in, response.session_id);
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
      await run('auth.logoutError', () => apiClient.logoutThroughLock(() => authApi.logout()));
    } catch {
      // A failed logout still logs the user out locally: the access token is
      // dropped below either way, and the refresh cookie is short-lived.
      clearError();
    } finally {
      // Drop everything that belonged to the previous account — the session user, the
      // temp token, the dashboard list (full participant ids) and the unified-feed
      // capability — through the one account-scoped reset. Deliberate logout, forced
      // expiry and remote cross-tab logout all funnel through it, so none of them can
      // leave this account's state behind for the next one on this browser.
      resetAccountScopedState();
      // signOut rather than clearToken: the other tabs on this browser share the
      // refresh cookie the backend just revoked, and have to be told. It also bumps
      // the session generation, so a refresh that was in flight when we logged out
      // cannot store its response afterwards.
      apiClient.signOut();
    }
  }

  async function fetchUser(expectedEpoch?: number | null): Promise<User | null> {
    return run('auth.fetchUserError', async () => {
      // The account boundary this restore started under. Even an *unqualified*
      // /auth/me (cold hydrate, or an ordinary re-fetch) is fenced here: a reset or
      // replacement that lands while the request is on the wire must not let the
      // stale answer overwrite the replacement session's user, or let a stale failure
      // clear the replacement session's token.
      //
      // The account generation is advanced by explicit account-boundary events
      // (login, register, logout, a remote restore), *not* by the client's own
      // refresh that first obtains a token during a cold restore — so an ordinary
      // null-to-new-session startup is not mistaken for a replacement and rejected.
      const boundary = currentAccountGeneration();
      try {
        const me = await authApi.getMe();
        // A remote-session restore can be overtaken by a still newer session while
        // /auth/me is on the wire: a late answer for an older epoch must not overwrite
        // the account the receiver accepted in the meantime.
        if (expectedEpoch !== undefined && apiClient.getSessionEpoch() !== expectedEpoch) {
          return null;
        }
        // A reset or replacement landed while /auth/me was in flight: the answer
        // belongs to a session that is no longer the current one. Discard it without
        // touching user.value — the replacement session's login already wrote it.
        if (boundary !== currentAccountGeneration()) {
          return null;
        }
        user.value = me;
        // A /auth/me answer installs a session (cold hydrate or another tab's fresh
        // login): advance the account fence so nothing started under the previous
        // account can still commit.
        bumpAccountGeneration();
        return me;
      } catch (err) {
        // Only a *definitive* rejection of the current session may clear it: the
        // backend answered UNAUTHORIZED, so the credential is genuinely dead. A
        // transient failure (HTTP 5xx, network error, timeout) says nothing about
        // the session — the refresh cookie may still be valid, and clearing here
        // would destroy the shared restore marker and evict a recoverable account.
        // Preserve the user and the token, and rethrow so callers can retry.
        const fenceHolds =
          boundary === currentAccountGeneration() &&
          (expectedEpoch === undefined || apiClient.getSessionEpoch() === expectedEpoch);
        if (fenceHolds && isDefinitiveRejection(err)) {
          user.value = null;
          apiClient.clearToken();
        }
        throw err;
      }
    });
  }

  /**
   * Serialises profile/preferences writes, one at a time.
   *
   * The backend rewrites the whole profile row from each request's submitted
   * snapshot (`internal/auth/repository/user_repository.go`), so two overlapping
   * partial saves — the display-name form and the preferences form can both be
   * in flight with independent busy flags — would silently lose each other's field
   * in the database. Chaining the PATCHes closes that window client-side; each
   * response is then merged field-wise so a slightly older snapshot cannot roll
   * back a newer save's values in memory either.
   */
  let profileWriteChain: Promise<unknown> = Promise.resolve();

  /**
   * Update the signed-in user's profile/preferences.
   *
   * The response can land after a logout or an account switch, and Vue does not cancel
   * an async continuation when the submitting view unmounts. Both the initiating user's
   * id and the account-fence generation are captured up front; the commit happens only
   * if neither moved while the request was on the wire — and the queued write itself
   * rechecks the same fence *before* the HTTP request is issued, so a save queued
   * behind an earlier one on account A can never ride account B's token on the wire.
   * The id alone cannot tell "same account, same session" from "same account,
   * replacement session" (a logout followed by a fresh login to the same account), so
   * the generation — advanced by every reset/replacement — is the primary fence and
   * the id a secondary guard. Returns the fresh user, or `null` when the session
   * changed and nothing was committed.
   */
  async function updateProfile(data: Partial<User>): Promise<User | null> {
    return run('auth.updateProfileError', async () => {
      const requesterId = user.value?.id ?? null;
      const generation = currentAccountGeneration();
      // Queue behind any profile write still on the wire so the two settings forms
      // cannot overlap on the backend's whole-row update. The callback rechecks the
      // account fence *before* sending: by the time an earlier write has settled, the
      // session may have been replaced, and a stale account's fields must not be
      // transmitted at all — not merely fail to commit.
      const write = profileWriteChain.then(() => {
        if (generation !== currentAccountGeneration()) {
          return null;
        }
        if ((user.value?.id ?? null) !== requesterId) {
          return null;
        }
        return authApi.updateProfile(data);
      });
      profileWriteChain = write.catch(() => {});
      const updated = (await write) as User | null;
      if (updated === null) {
        return null;
      }
      if (generation !== currentAccountGeneration()) {
        return null;
      }
      if ((user.value?.id ?? null) !== requesterId) {
        return null;
      }
      // Field-wise merge: commit only the columns this request submitted. `updated`
      // is a full snapshot of the server row and may be older than a save for the
      // other form that has since landed — overwriting wholesale would roll it back.
      if (user.value) {
        const patch = Object.fromEntries(
          Object.keys(data).map(key => [key, updated[key as keyof User]])
        ) as Partial<User>;
        user.value = { ...user.value, ...patch };
        return user.value;
      }
      user.value = updated;
      return updated;
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
      bumpAccountGeneration();
      if (response.access_token) {
        apiClient.setToken(response.access_token, response.expires_in, response.session_id);
      }

      return response;
    });
  }

  // Set tokens directly (for MFA verification and passkey login)
  function setTokens(accessToken: string, expiresIn?: number, sessionId?: string) {
    apiClient.setToken(accessToken, expiresIn, sessionId);
    // A session is being established (MFA complete, passkey): advance the account
    // fence so older in-flight continuations cannot overwrite this new session.
    bumpAccountGeneration();
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
   * Drop the signed-in user locally, without calling the server or broadcasting.
   *
   * The shared account-scoped reset (stores/remoteSignout.ts) calls this for every
   * sign-out path: a deliberate logout in this tab, one reported by another tab, and
   * a locally detected expiry. None of those should call `/auth/logout` again (the
   * deliberate logout already did, and re-broadcasting would loop), and all of them
   * must end up with no user left in memory — that is what keeps a public calendar
   * link from rendering the previous account's owner state after a sign-out.
   */
  function resetSession() {
    user.value = null;
    tempToken.value = null;
    // A sign-out is an account boundary: fence off any account-scoped continuation
    // that was still in flight (a profile save whose response lands after a logout
    // must not resurrect the old account or overwrite a replacement session's state),
    // and drop queued profile writes so none can ride a replacement account's token.
    profileWriteChain = Promise.resolve();
    bumpAccountGeneration();
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
      const status: BootstrapStatus = await authApi.bootstrapStatus();
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
        // The boundary this cold start began its restore under. The account-fence
        // generation only advances on an explicit account-boundary event (login,
        // logout, a remote restore, a first /auth/me that found a session), so a
        // replacement that lands while the restore is on the wire is visible here —
        // and the ordinary client refresh that first obtains the token is not (it
        // does not advance the fence), so a successful cold hydrate is not mistaken
        // for a replacement and rejected.
        const restoreBoundary = currentAccountGeneration();
        try {
          await fetchUser();
        } catch (err) {
          // Only a *definitive* rejection of the refresh cookie (UNAUTHORIZED) proves
          // the session is over: drop the restore marker. A transient fault (HTTP 5xx,
          // network error, timeout) says nothing about the cookie — it may still be
          // valid, and clearing it here would remove the shared restore flag that a
          // later load uses to find the session. Preserve it and allow initialization
          // to be retried rather than settling a permanently anonymous state. In both
          // cases a failure for a restore that a replacement overtook must not clear
          // the replacement session's token, so the cleanup only runs when the
          // boundary has not moved.
          if (restoreBoundary === currentAccountGeneration()) {
            if (isDefinitiveRejection(err)) {
              apiClient.clearToken();
              clearError();
            } else {
              // Transient: keep the recoverable session and let the next
              // initializeAuth()/whenReady() (e.g. the router guard on the next
              // navigation, or a reload) retry the restore.
              initPromise = null;
            }
          } else {
            clearError();
          }
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
    forgotPassword,
    resetPassword,
    setTokens,
    setTempToken,
    clearTempToken,
    resetSession,
    initializeAuth,
    whenReady,
    clearError,
  };
});
