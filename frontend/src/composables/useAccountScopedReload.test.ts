/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { withComposable } from '@/test/harness';
import { useAccountScopedReload } from './useAccountScopedReload';

const mocks = vi.hoisted(() => ({
  auth: null as unknown as {
    accountGeneration: number;
    isAuthenticated: boolean;
    isAdmin: boolean;
  },
  router: {
    replace: vi.fn(),
    currentRoute: { value: { meta: { requiresAdmin: true } } },
  },
}));
vi.mock('vue-router', () => ({ useRouter: () => mocks.router }));
vi.mock('@/stores/auth', async () => {
  const { reactive } = await import('vue');
  mocks.auth = reactive({ accountGeneration: 0, isAuthenticated: true, isAdmin: true });
  return { useAuthStore: () => mocks.auth };
});

describe('useAccountScopedReload', () => {
  it('leaves an admin page instead of reloading it for a non-admin account', async () => {
    const loader = vi.fn();
    withComposable(() => useAccountScopedReload(loader));

    mocks.auth.isAdmin = false;
    mocks.auth.accountGeneration += 1;
    await nextTick();

    expect(loader).not.toHaveBeenCalled();
    expect(mocks.router.replace).toHaveBeenCalledWith({ name: 'dashboard' });

    mocks.auth.isAdmin = true;
    mocks.auth.accountGeneration += 1;
    await nextTick();

    expect(loader).toHaveBeenCalledTimes(1);
  });
});
