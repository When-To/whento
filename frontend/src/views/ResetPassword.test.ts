/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { flushPromises } from '@vue/test-utils';

import { mountWithI18n } from '@/test/harness';

/**
 * The regression this file guards: the reset form used to return a password
 * "valid" at 8 characters, while the server's strongpassword rule (mirrored by
 * validatePassword) demands 12 with the usual classes — so a user could fill the
 * form, submit, and get a round-trip rejection the client could have caught.
 * The component now runs the same validator as registration and bootstrap, and
 * surfaces the failure key under the field before the submit button enables.
 */

const routerReplace = vi.fn();
const routerPush = vi.fn();
let routeParams: Record<string, string> = { token: 'a'.repeat(64) };

vi.mock('vue-router', () => ({
  useRouter: () => ({ replace: routerReplace, push: routerPush }),
  useRoute: () => ({ params: routeParams }),
}));

const authApi = {
  register: vi.fn(),
  bootstrap: vi.fn(),
  bootstrapStatus: vi.fn(),
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
};

vi.mock('@/api/auth', () => ({ authApi }));
vi.mock('@/api/client', () => ({ apiClient }));

const { useToastStore } = await import('@/stores/toast');
const { default: ResetPassword } = await import('./ResetPassword.vue');

async function mountPage() {
  return mountWithI18n(ResetPassword, {
    global: {
      plugins: [createPinia()],
      stubs: { RouterLink: { template: '<a><slot /></a>', props: ['to'] } },
    },
  });
}

describe('ResetPassword.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setActivePinia(createPinia());
    routeParams = { token: 'a'.repeat(64) };
    authApi.resetPassword.mockResolvedValue({
      user: { id: 'u1', email: 'user@example.test', display_name: 'User', role: 'admin' },
      access_token: 'access-1',
      expires_in: 900,
    });
  });

  it('rejects an invalid link token on mount', async () => {
    routeParams = { token: 'short' };
    const wrapper = await mountPage();

    expect(wrapper.text()).toContain('Invalid or expired reset link');
  });

  it('does not enable submit while the new password is weak', async () => {
    const wrapper = await mountPage();
    const newPassword = wrapper.find('#new-password');
    const confirmPassword = wrapper.find('#confirm-password');

    // Exactly 8 characters satisfies the old buggy rule and nothing else.
    await newPassword.setValue('Abcd1234');
    await confirmPassword.setValue('Abcd1234');
    await newPassword.trigger('blur');

    expect(wrapper.find('form').exists()).toBe(true);
    expect(wrapper.text()).toContain('Password must be at least 12 characters');
    const submit = wrapper.find('#new-password').element.closest('form')?.querySelector('button');
    expect((submit as HTMLButtonElement | null)?.disabled).toBe(true);
  });

  it('shows no validation error before the field has been touched', async () => {
    const wrapper = await mountPage();

    expect(wrapper.text()).not.toContain('Password must be at least 12 characters');
    expect(wrapper.text()).not.toContain('Password must not exceed 72');
  });

  it('shows a translated byte-limit error for a too-long password', async () => {
    const wrapper = await mountPage();
    const newPassword = wrapper.find('#new-password');

    // 24 multibyte characters encoded as UTF-8 exceed bcrypt's 72 bytes while
    // still being far below any character-count ceiling: byte ceiling caught.
    await newPassword.setValue('€'.repeat(24) + 'Ab1!');
    await newPassword.trigger('blur');

    expect(wrapper.text()).toContain('must not exceed 72');
  });

  it('submits a matching strong password and shows the success state', async () => {
    const wrapper = await mountPage();
    await wrapper.find('#new-password').setValue('StrongP@ssw0rd123!');
    await wrapper.find('#confirm-password').setValue('StrongP@ssw0rd123!');

    await wrapper.find('form').trigger('submit');
    await flushPromises();

    expect(authApi.resetPassword).toHaveBeenCalledWith('a'.repeat(64), 'StrongP@ssw0rd123!');
    // The toast is pushed onto the store (rendered by a global container, not
    // this component's markup) and the page swaps to its success state.
    expect(useToastStore().toasts.map(t => t.message)).toContain(
      'Password reset successful. Welcome back!'
    );
    expect(wrapper.text()).toContain('Password Reset Successful!');
  });

  it('refuses to submit when the confirm field does not match', async () => {
    const wrapper = await mountPage();
    await wrapper.find('#new-password').setValue('StrongP@ssw0rd123!');
    await wrapper.find('#confirm-password').setValue('StrongP@ssw0rd456!');

    await wrapper.find('form').trigger('submit');
    await flushPromises();

    expect(authApi.resetPassword).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain('Passwords do not match');
  });
});
