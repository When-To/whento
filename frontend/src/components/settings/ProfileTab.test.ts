/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 *
 * The regression this file guards: a profile/preferences save that settles after the
 * account changed must not write the old account's values — including the global
 * locale — into the account that is on screen by then. Vue does not cancel the async
 * continuation when the submitting view is left, so the store action has to be the one
 * deciding whether the response still belongs to the current session.
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { mount, flushPromises } from '@vue/test-utils';
import type { User } from '@/types';
import { createTestI18n } from '@/test/harness';

const authApi = {
  updateProfile: vi.fn(),
};

vi.mock('@/api/auth', () => ({ authApi }));

const apiClient = {
  setToken: vi.fn(),
  clearToken: vi.fn(),
  hasSession: vi.fn(() => false),
  signOut: vi.fn(),
};

vi.mock('@/api/client', () => ({ apiClient }));

// Dynamic import once the mocks above are registered: a static import is hoisted past
// the vi.mock calls and would pull in the real API modules before `authApi` exists.
const { default: ProfileTab } = await import('@/components/settings/ProfileTab.vue');

// The saved user carries a locale/timezone that the preferences form binds to.
const USER_A: User = {
  id: 'u-a',
  email: 'a@example.test',
  display_name: 'A',
  role: 'user',
  locale: 'fr',
  timezone: 'Europe/Paris',
  created_at: '2026-01-01T00:00:00Z',
} as User;

const USER_B: User = {
  id: 'u-b',
  email: 'b@example.test',
  display_name: 'B',
  role: 'user',
  locale: 'fr',
  timezone: 'Europe/Paris',
  created_at: '2026-01-01T00:00:00Z',
} as User;

const TimezoneSelectorStub = {
  name: 'TimezoneSelector',
  props: ['modelValue', 'label'],
  emits: ['update:modelValue'],
  template:
    '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(res => {
    resolve = res;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setActivePinia(createPinia());
});

describe('ProfileTab.vue — delayer profile continuations against account switches', () => {
  it("does not switch the global locale to an old account's late preferences save", async () => {
    const { useAuthStore } = await import('@/stores/auth');
    const authStore = useAuthStore();
    authStore.user = USER_A;

    const i18n = createTestI18n('fr');
    const wrapper = mount(ProfileTab, {
      global: { plugins: [i18n], stubs: { TimezoneSelector: TimezoneSelectorStub } },
    });
    await flushPromises();

    // A selects English and submits the preferences form; the save stays on the wire.
    const update = deferred<User>();
    authApi.updateProfile.mockReturnValue(update.promise);
    await wrapper.find('#profile-language').setValue('en');
    await wrapper.findAll('form')[1].trigger('submit');
    await flushPromises();

    // A signs out and B signs in while the save is still in flight.
    authStore.user = null;
    authStore.user = USER_B;

    update.resolve({ ...USER_A, locale: 'en' });
    await flushPromises();

    // B's account is untouched, the global locale did not change to A's selection, and
    // no persisted locale was written for the dead session.
    expect(authStore.user).toEqual(USER_B);
    expect(i18n.global.locale.value).toBe('fr');
    expect(localStorage.getItem('locale')).toBeNull();
    wrapper.unmount();
  });

  it('does not overwrite the account that superseded a pending profile save', async () => {
    const { useAuthStore } = await import('@/stores/auth');
    const authStore = useAuthStore();
    authStore.user = USER_A;

    const i18n = createTestI18n('fr');
    const wrapper = mount(ProfileTab, {
      global: { plugins: [i18n], stubs: { TimezoneSelector: TimezoneSelectorStub } },
    });
    await flushPromises();

    const update = deferred<User>();
    authApi.updateProfile.mockReturnValue(update.promise);
    await wrapper.find('#profile-display-name').setValue('A 2.0');
    await wrapper.findAll('form')[0].trigger('submit');
    await flushPromises();

    authStore.user = null;
    authStore.user = USER_B;

    update.resolve({ ...USER_A, display_name: 'A 2.0' });
    await flushPromises();

    expect(authStore.user).toEqual(USER_B);
    expect(authStore.user?.display_name).toBe('B');
    wrapper.unmount();
  });

  it('does not restore an old session\u2019s locale into a replacement session for the same user', async () => {
    const { useAuthStore } = await import('@/stores/auth');
    const authStore = useAuthStore();
    authStore.user = USER_A;

    const i18n = createTestI18n('fr');
    const wrapper = mount(ProfileTab, {
      global: { plugins: [i18n], stubs: { TimezoneSelector: TimezoneSelectorStub } },
    });
    await flushPromises();

    const update = deferred<User>();
    authApi.updateProfile.mockReturnValue(update.promise);
    await wrapper.find('#profile-language').setValue('en');
    await wrapper.findAll('form')[1].trigger('submit');
    await flushPromises();

    // The same user (same id) logs out and back in while the save is in flight — a
    // replacement session the user-id fence cannot tell apart from the original.
    authStore.resetSession();
    authStore.user = USER_A;

    update.resolve({ ...USER_A, locale: 'en' });
    await flushPromises();

    // The replacement session must keep its own in-memory user and locale, and the old
    // continuation's global/persisted locale must not be applied.
    expect(authStore.user).toEqual(USER_A);
    expect(authStore.user?.locale).toBe('fr');
    expect(i18n.global.locale.value).toBe('fr');
    expect(localStorage.getItem('locale')).toBeNull();
    wrapper.unmount();
  });
});
