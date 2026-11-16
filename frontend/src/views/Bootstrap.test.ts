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
import en from '@/locales/en.json';
import fr from '@/locales/fr.json';

/**
 * The regression this file guards: on a *failed* `/auth/status` read the store
 * deliberately leaves `bootstrapRequired=false` and `bootstrapStatusKnown=false`
 * (unknown, not configured). The router guard already refuses to redirect on an
 * unknown capability state — but Bootstrap.vue used to compute
 * `alreadyConfigured = !bootstrapRequired`, which turned the unknown default into
 * "configured" and sent the operator back to /login, hiding the only setup page.
 *
 * The router tests cover the guard. This component test pins the component to the
 * same rule, so the two cannot contradict each other again.
 */

const routerReplace = vi.fn();
const routerPush = vi.fn();

vi.mock('vue-router', () => ({
  useRouter: () => ({ replace: routerReplace, push: routerPush }),
}));

const authApi = {
  register: vi.fn(),
  bootstrap: vi.fn(),
  getAuthStatus: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  getMe: vi.fn(),
  updateProfile: vi.fn(),
  forgotPassword: vi.fn(),
  completeAccountRecovery: vi.fn(),
};

const apiClient = {
  setToken: vi.fn(),
  clearToken: vi.fn(),
  hasSession: vi.fn(() => false),
  signOut: vi.fn(),
};

vi.mock('@/api/auth', () => ({ authApi }));
vi.mock('@/api/client', () => ({ apiClient }));

// Dynamic imports only once the mocks above are registered: a static import is
// hoisted past the vi.mock calls, so the component would load the real (unmocked)
// API modules.
const { useAuthStore } = await import('@/stores/auth');
const { default: Bootstrap } = await import('./Bootstrap.vue');
const { default: Register } = await import('./Register.vue');

describe('Bootstrap.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setActivePinia(createPinia());
  });

  it('stays mounted when the status read failed (state unknown, not configured)', async () => {
    // The store's post-failure state: startup defaults plus "no real answer yet".
    const store = useAuthStore();
    store.bootstrapRequired = false;
    store.bootstrapStatusKnown = false;

    await mountWithI18n(Bootstrap);

    expect(routerReplace).not.toHaveBeenCalled();
  });

  it('redirects to /login when a real status answer says the instance is configured', async () => {
    const store = useAuthStore();
    store.bootstrapRequired = false;
    store.bootstrapStatusKnown = true;

    await mountWithI18n(Bootstrap);

    // The redirect is what the router guard would already do; the component is
    // only allowed to mirror it once the status is authoritative.
    expect(routerReplace).toHaveBeenCalledWith('/login');
  });

  it('stays mounted on an empty instance that genuinely needs bootstrap', async () => {
    const store = useAuthStore();
    store.bootstrapRequired = true;
    store.bootstrapStatusKnown = true;

    await mountWithI18n(Bootstrap);

    expect(routerReplace).not.toHaveBeenCalled();
  });

  describe('submit', () => {
    async function submitKey(key: string, locale: 'en' | 'fr' = 'en') {
      const wrapper = mountWithI18n(Bootstrap, {}, locale);
      await wrapper.find('#boot_key').setValue(key);
      await wrapper.find('#display_name').setValue('Owner');
      await wrapper.find('#email').setValue('owner@example.test');
      await wrapper.find('#password').setValue('Str0ng!Passw0rd');
      await wrapper.find('form').trigger('submit');
      await flushPromises();
      return wrapper;
    }

    it.each(['en', 'fr'] as const)('validates boot-key code-point bounds in %s', async locale => {
      for (const key of ['', 'a'.repeat(15), '🔑'.repeat(15), 'a'.repeat(257), '🔑'.repeat(257)]) {
        const wrapper = await submitKey(key, locale);
        expect(authApi.bootstrap).not.toHaveBeenCalled();
        expect(wrapper.find('.input-error').exists()).toBe(true);
        expect(wrapper.text()).not.toContain('validation.fields.');
        wrapper.unmount();
      }
    });

    it.each(['a'.repeat(16), '🔑'.repeat(16), 'a'.repeat(256), '🔑'.repeat(256)])(
      'accepts valid Unicode boot-key boundaries',
      async key => {
        authApi.bootstrap.mockRejectedValue({ code: 'UNAUTHORIZED' });
        await submitKey(key);
        expect(authApi.bootstrap).toHaveBeenCalledTimes(1);
      }
    );

    it.each(['en', 'fr'] as const)('translates server boot-key validation in %s', async locale => {
      authApi.bootstrap.mockRejectedValue({
        code: 'VALIDATION_ERROR',
        details: [{ field: 'boot_key', message: 'must be at least 16 characters' }],
      });
      const wrapper = await submitKey('operator-key-1234567890', locale);
      const messages = locale === 'en' ? en : fr;
      expect(wrapper.text()).toContain(
        messages.validation.fields.boot_key.min.replace('{count}', '16')
      );
    });

    it.each(['en', 'fr'] as const)(
      'keeps the configured message and login action visible in %s',
      async locale => {
        authApi.bootstrap.mockRejectedValue({ code: 'CONFLICT' });
        const wrapper = await submitKey('operator-key-1234567890', locale);
        const messages = locale === 'en' ? en : fr;
        expect(wrapper.find('[role="alert"]').text()).toContain(
          messages.auth.bootstrap.alreadyConfigured
        );
        expect(wrapper.find('[to="/login"]').exists()).toBe(true);
        expect(wrapper.find('form').exists()).toBe(false);
        expect(routerReplace).not.toHaveBeenCalled();
        expect(useAuthStore().bootstrapStatusKnown).toBe(true);
        expect(useAuthStore().bootstrapRequired).toBe(false);
      }
    );

    it.each([
      ['UNAUTHORIZED', en.auth.bootstrap.invalidKey],
      ['INTERNAL_ERROR', en.errors.serverError],
      ['TOO_MANY_REQUESTS', en.errors.rateLimited],
      ['SERVICE_UNAVAILABLE', en.errors.serviceUnavailable],
      ['ERR_NETWORK', en.errors.network],
      ['UNMAPPED_ERROR', en.errors.generic],
    ])('shows an accurate message for %s', async (code, message) => {
      authApi.bootstrap.mockRejectedValue({ code });
      const wrapper = await submitKey('operator-key-1234567890');
      expect(wrapper.find('[role="alert"]').text()).toBe(message);
      expect(routerReplace).not.toHaveBeenCalled();
    });

    it('creates the first user and navigates to the dashboard', async () => {
      authApi.bootstrap.mockResolvedValue({
        user: { id: 'u-1', email: 'owner@example.test', display_name: 'Owner' },
        access_token: 'tok',
      });
      const store = useAuthStore();
      store.bootstrapRequired = true;
      store.bootstrapStatusKnown = true;

      const wrapper = await mountWithI18n(Bootstrap);

      await wrapper.find('#boot_key').setValue('operator-key-1234567890');
      await wrapper.find('#display_name').setValue('Owner');
      await wrapper.find('#email').setValue('owner@example.test');
      await wrapper.find('#password').setValue('Str0ng!Passw0rd');
      await wrapper.find('form').trigger('submit');
      await flushPromises();

      expect(routerPush).toHaveBeenCalledWith('/dashboard');
    });

    it('keeps the form on the page when the server rejects the attempt', async () => {
      authApi.bootstrap.mockRejectedValue({ code: 'VALIDATION_ERROR', message: 'Invalid input' });
      const store = useAuthStore();
      store.bootstrapRequired = true;
      store.bootstrapStatusKnown = true;

      const wrapper = await mountWithI18n(Bootstrap);

      await wrapper.find('#boot_key').setValue('wrong-long-enough-key');
      await wrapper.find('#display_name').setValue('Owner');
      await wrapper.find('#email').setValue('owner@example.test');
      await wrapper.find('#password').setValue('Str0ng!Passw0rd');
      await wrapper.find('form').trigger('submit');
      await flushPromises();

      expect(routerPush).not.toHaveBeenCalled();
      expect(wrapper.find('.card').exists()).toBe(true);
    });

    describe('client-side password rule', () => {
      const bootKey = 'operator-key-1234567890';

      async function submitPassword(password: string) {
        const wrapper = await mountWithI18n(Bootstrap);

        await wrapper.find('#boot_key').setValue(bootKey);
        await wrapper.find('#display_name').setValue('Owner');
        await wrapper.find('#email').setValue('owner@example.test');
        await wrapper.find('#password').setValue(password);
        await wrapper.find('form').trigger('submit');
        await flushPromises();
        return wrapper;
      }

      it('refuses a password shorter than 12 characters without contacting the server', async () => {
        // If the validator were broken this would reach authApi and push.
        await submitPassword('Str0ng!Pa');
        expect(authApi.bootstrap).not.toHaveBeenCalled();
        expect(routerPush).not.toHaveBeenCalled();
      });

      it('refuses a password without an uppercase letter', async () => {
        await submitPassword('str0ng!passw0rd');
        expect(authApi.bootstrap).not.toHaveBeenCalled();
      });

      it('refuses a password without a lowercase letter', async () => {
        await submitPassword('STR0NG!PASSW0RD');
        expect(authApi.bootstrap).not.toHaveBeenCalled();
      });

      it('refuses a password without a digit', async () => {
        await submitPassword('Strpng!Password');
        expect(authApi.bootstrap).not.toHaveBeenCalled();
      });

      it('refuses a password without a special character', async () => {
        await submitPassword('Str0ngPassword1');
        expect(authApi.bootstrap).not.toHaveBeenCalled();
      });

      it('submits a password that satisfies every requirement', async () => {
        authApi.bootstrap.mockResolvedValue({
          user: { id: 'u-1', email: 'owner@example.test', display_name: 'Owner' },
          access_token: 'tok',
        });
        await submitPassword('Str0ng!Passw0rd');
        expect(authApi.bootstrap).toHaveBeenCalledTimes(1);
        expect(routerPush).toHaveBeenCalledWith('/dashboard');
      });
    });
  });
});

describe('Register.vue bootstrap recovery', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setActivePinia(createPinia());
  });

  it.each(['en', 'fr'] as const)('offers setup after a failed status probe in %s', async locale => {
    const store = useAuthStore();
    store.bootstrapStatusKnown = false;
    authApi.register.mockRejectedValue({ code: 'BOOTSTRAP_REQUIRED' });
    const wrapper = mountWithI18n(Register, {}, locale);
    await wrapper.find('#display_name').setValue('Owner');
    await wrapper.find('#email').setValue('owner@example.test');
    await wrapper.find('#password').setValue('Str0ng!Passw0rd');
    await wrapper.find('form').trigger('submit');
    await flushPromises();
    expect(wrapper.text()).toContain(
      (locale === 'en' ? en : fr).auth.bootstrap.registrationRequired
    );
    expect(wrapper.find('[to="/bootstrap"]').exists()).toBe(true);
    expect(store.bootstrapStatusKnown).toBe(true);
    expect(store.bootstrapRequired).toBe(true);
    expect(routerPush).not.toHaveBeenCalled();
  });
});
