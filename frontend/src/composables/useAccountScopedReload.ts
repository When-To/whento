/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { watch } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '@/stores/auth';

/**
 * Re-run an account-scoped loader when the local account generation advances.
 *
 * The generation only moves on a real account boundary — login, logout, a remote
 * account replacement, a refresh that discovered a different server family — never on
 * a same-family token rotation. Route loaders that hold account-scoped data
 * (dashboard calendars and feed, calendar settings, admin lists) use this to clear and
 * reload only their own data when the identity changes, which is what replaces the
 * old whole-document reload on a session-restored broadcast. Public/participant views
 * keep their public payload, protected-route auth checks are untouched, and the
 * reload only fires once the replacement identity has been confirmed (/auth/me has
 * re-populated the signed-in user), so the loader never runs against a cleared store.
 */
export function useAccountScopedReload(loader: () => void | Promise<void>): void {
  const authStore = useAuthStore();
  const router = useRouter();
  watch(
    () => authStore.accountGeneration,
    () => {
      if (!authStore.isAuthenticated) return;
      // Route guards only run on navigation. A non-admin replacing an admin on an
      // admin page leaves it rather than keeping the previous account's list on screen.
      if (router.currentRoute.value.meta.requiresAdmin && !authStore.isAdmin) {
        void router.replace({ name: 'dashboard' });
        return;
      }
      void loader();
    }
  );
}
