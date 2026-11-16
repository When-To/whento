/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { DOMWrapper, flushPromises } from '@vue/test-utils';

import { mountWithI18n } from '@/test/harness';

const getAll = vi.fn();
const deleteAccount = vi.fn();
const routerPush = vi.fn();

vi.mock('@/api/calendars', () => ({ calendarsApi: { getAll } }));
vi.mock('@/api/auth', () => ({ authApi: { deleteAccount } }));
vi.mock('@/api/client', () => ({ apiClient: { signOut: vi.fn() } }));
vi.mock('vue-router', () => ({ useRouter: () => ({ push: routerPush }) }));

const { default: DeleteAccountModal } = await import('./DeleteAccountModal.vue');

function calendar(name: string, participants: number) {
  return {
    id: name,
    name,
    participants: Array.from({ length: participants }, (_, i) => ({ id: `${name}-${i}` })),
  };
}

/**
 * The dialog is teleported to <body>, so it is queried there. The real Teleport, not
 * VTU's stub: the stub re-creates its children on every render, which drops focus and
 * would make the initial-focus check meaningless.
 */
const page = () => new DOMWrapper(document.body);

async function mountModal() {
  const wrapper = mountWithI18n(DeleteAccountModal, {
    props: { open: true },
    attachTo: document.body,
    global: { plugins: [createPinia()] },
  });
  await flushPromises();
  return wrapper;
}

function confirmButton() {
  return page().find('[data-testid="confirm-delete"]').element as HTMLButtonElement;
}

describe('DeleteAccountModal.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setActivePinia(createPinia());
    getAll.mockResolvedValue([calendar('Team', 2), calendar('Family', 1)]);
  });

  it('keeps the delete button disabled until the box is ticked and a password typed', async () => {
    const wrapper = await mountModal();
    expect(confirmButton().disabled).toBe(true);

    await page().find('#delete-account-password').setValue('secret');
    expect(confirmButton().disabled).toBe(true);

    await page().find('#delete-account-password').setValue('');
    await page().find('#delete-account-confirm').setValue(true);
    expect(confirmButton().disabled).toBe(true);

    await page().find('#delete-account-password').setValue('secret');
    expect(confirmButton().disabled).toBe(false);
    wrapper.unmount();
  });

  it('opens with focus on Cancel', async () => {
    const wrapper = await mountModal();
    expect(document.activeElement?.textContent?.trim()).toBe('Cancel');
    wrapper.unmount();
  });

  it('lists the calendars and counts the participants', async () => {
    const wrapper = await mountModal();
    const text = page().text();
    expect(text).toContain('Your 2 calendars will be deleted:');
    expect(page().find('[data-testid="calendar-names"]').text()).toContain('Team');
    expect(page().find('[data-testid="calendar-names"]').text()).toContain('Family');
    expect(text).toContain('The responses of 3 participants will be erased');
    expect(text).toContain('Participants will not be notified by WhenTo');
    wrapper.unmount();
  });

  it('caps the list at five names', async () => {
    getAll.mockResolvedValue(Array.from({ length: 7 }, (_, i) => calendar(`Cal ${i}`, 0)));
    const wrapper = await mountModal();
    const items = page().findAll('[data-testid="calendar-names"] li');
    expect(items).toHaveLength(6);
    expect(items[5].text()).toBe('and 2 others');
    // No participants: no reminder to warn them.
    expect(page().text()).not.toContain('Participants will not be notified');
    wrapper.unmount();
  });

  it('falls back to the generic summary when the calendars cannot be loaded', async () => {
    getAll.mockRejectedValue(new Error('offline'));
    const wrapper = await mountModal();
    expect(page().text()).toContain('All your calendars and their participants');
    expect(page().text()).not.toContain('will be deleted:');
    wrapper.unmount();
  });

  it('shows a wrong password under the field and clears it', async () => {
    deleteAccount.mockRejectedValue({ code: 'BAD_REQUEST', message: 'Password is incorrect' });
    const wrapper = await mountModal();
    await page().find('#delete-account-password').setValue('wrong');
    await page().find('#delete-account-confirm').setValue(true);
    await page().find('form').trigger('submit');
    await flushPromises();

    expect(deleteAccount).toHaveBeenCalledWith('wrong');
    expect(page().text()).toContain('Incorrect password.');
    expect((page().find('#delete-account-password').element as HTMLInputElement).value).toBe('');
    expect(wrapper.emitted('close')).toBeUndefined();
    wrapper.unmount();
  });

  it('explains the last-admin refusal and disables the button', async () => {
    deleteAccount.mockRejectedValue({ code: 'CONFLICT', message: 'last admin' });
    const wrapper = await mountModal();
    await page().find('#delete-account-password').setValue('secret');
    await page().find('#delete-account-confirm').setValue(true);
    await page().find('form').trigger('submit');
    await flushPromises();

    expect(page().text()).toContain('You are the only administrator.');
    expect(confirmButton().disabled).toBe(true);
    wrapper.unmount();
  });

  it('closes and goes home once the account is deleted', async () => {
    deleteAccount.mockResolvedValue(undefined);
    const wrapper = await mountModal();
    await page().find('#delete-account-password').setValue('secret');
    await page().find('#delete-account-confirm').setValue(true);
    await page().find('form').trigger('submit');
    await flushPromises();

    expect(wrapper.emitted('close')).toHaveLength(1);
    expect(routerPush).toHaveBeenCalledWith('/');
    wrapper.unmount();
  });
});
