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
 * The regression this file guards: the password-change form used to accept any
 * password of 12+ characters (no strength check, no byte ceiling), while the
 * backend enforces strongpassword + maxbytes — so a user could be told the form
 * was valid and then get a rejection from the server. The form now runs
 * validatePassword (the same rule as registration, bootstrap, and reset) and
 * shows the exact failure key under the field. PasskeyManager and MFASetup are
 * stubbed out: they are independent sections with their own API traffic and are
 * not what this file is about.
 */

const apiClient = {
  patch: vi.fn(),
};

vi.mock('@/api/client', () => ({ apiClient }));

const { useToastStore } = await import('@/stores/toast');
const { default: SecurityTab } = await import('./SecurityTab.vue');

async function mountTab() {
  const wrapper = mountWithI18n(SecurityTab, {
    global: {
      plugins: [createPinia()],
      stubs: {
        PasskeyManager: { template: '<div />' },
        MFASetup: { template: '<div />' },
      },
    },
  });
  return wrapper;
}

describe('SecurityTab.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setActivePinia(createPinia());
    apiClient.patch.mockResolvedValue({});
  });

  it('keeps the submit button disabled while the new password is weak', async () => {
    const wrapper = await mountTab();
    await wrapper.find('#security-current-password').setValue('OldP@ssw0rd123!');
    // 12 characters but no uppercase/special: old code accepted this.
    await wrapper.find('#security-new-password').setValue('password12345');
    await wrapper.find('#security-confirm-password').setValue('password12345');
    await wrapper.find('#security-new-password').trigger('blur');

    expect(wrapper.text()).toContain('Password must be at least 12 characters');
    const submit = wrapper.find('form').find('button');
    expect((submit.element as HTMLButtonElement).disabled).toBe(true);
    expect(apiClient.patch).not.toHaveBeenCalled();
  });

  it('shows no validation error before the field has been touched', async () => {
    const wrapper = await mountTab();
    await wrapper.find('#security-current-password').setValue('OldP@ssw0rd123!');

    expect(wrapper.text()).not.toContain('Password must be at least 12 characters');
    expect(wrapper.text()).not.toContain('must not exceed 72');
  });

  it('shows the byte-limit error for a password whose UTF-8 length exceeds 72 bytes', async () => {
    const wrapper = await mountTab();
    await wrapper.find('#security-current-password').setValue('OldP@ssw0rd123!');
    // 30 × 3-byte characters = 90 bytes, over bcrypt's ceiling of 72.
    await wrapper.find('#security-new-password').setValue('€'.repeat(30) + 'Ab1!');
    await wrapper.find('#security-confirm-password').setValue('€'.repeat(30) + 'Ab1!');
    await wrapper.find('#security-new-password').trigger('blur');

    expect(wrapper.text()).toContain('must not exceed 72');
    const submit = wrapper.find('form').find('button');
    expect((submit.element as HTMLButtonElement).disabled).toBe(true);
  });

  it('submits a matching strong password and confirms via toast', async () => {
    const wrapper = await mountTab();
    await wrapper.find('#security-current-password').setValue('OldP@ssw0rd123!');
    await wrapper.find('#security-new-password').setValue('NewP@ssw0rd123!');
    await wrapper.find('#security-confirm-password').setValue('NewP@ssw0rd123!');

    await wrapper.find('form').trigger('submit');
    await flushPromises();

    expect(apiClient.patch).toHaveBeenCalledWith('/auth/me/password', {
      current_password: 'OldP@ssw0rd123!',
      new_password: 'NewP@ssw0rd123!',
    });
    expect(useToastStore().toasts.map(t => t.message)).toContain('Password changed successfully');
    // The form resets after a successful change.
    expect(wrapper.find('#security-new-password').element).toHaveProperty('value', '');
  });

  it('does not re-show a validation error on the form reset after success', async () => {
    const wrapper = await mountTab();
    const newPassword = wrapper.find('#security-new-password');
    await newPassword.setValue('Weak1!');
    await newPassword.trigger('blur');
    expect(wrapper.text()).toContain('Password must be at least 12 characters');

    await wrapper.find('#security-new-password').setValue('NewP@ssw0rd123!');
    await wrapper.find('#security-current-password').setValue('OldP@ssw0rd123!');
    await wrapper.find('#security-confirm-password').setValue('NewP@ssw0rd123!');
    await wrapper.find('form').trigger('submit');
    await flushPromises();

    // The form has been cleared, so the previously-seen error must not reappear
    // on the freshly-reset (pristine) fields.
    expect(wrapper.text()).not.toContain('Password must be at least 12 characters');
    expect(wrapper.find('#security-new-password').element).toHaveProperty('value', '');
  });
});
