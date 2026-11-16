/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 *
 * The regression this file guards: when *another* tab reports a sign-out through the
 * BroadcastChannel, the API client only clears the token. On a public route (a
 * participant link) it deliberately does not navigate, so this tab would keep the
 * previous account's user, owner calendar list and pre-rendered participant ids in
 * memory. The client raises `whento:remote-signout` instead; this module is what the
 * app wires up to answer it by resetting every account-scoped store in place.
 *
 * The remote *session* restore is family-based (the server `session_id`), not
 * epoch-based, and — unlike the fork it ports — never reloads the document.
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { REMOTE_SIGNOUT_EVENT, REMOTE_SESSION_EVENT } from '@/sessionEvents';

const authApi = {
  login: vi.fn(),
  register: vi.fn(),
  bootstrap: vi.fn(),
  getAuthStatus: vi.fn(),
  getMe: vi.fn(),
  updateProfile: vi.fn(),
  logout: vi.fn(),
  forgotPassword: vi.fn(),
  completeAccountRecovery: vi.fn(),
  checkMagicLinkAvailable: vi.fn(),
};

const apiClient = {
  setToken: vi.fn(),
  clearToken: vi.fn(),
  hasSession: vi.fn(() => false),
  signOut: vi.fn(),
  getFamilyId: vi.fn(() => null),
};

const calendarsApi = {
  getAll: vi.fn(),
};

const unifiedFeedApi = {
  getConfig: vi.fn(),
};

vi.mock('@/api/auth', () => ({ authApi }));
vi.mock('@/api/client', () => ({ apiClient }));
vi.mock('@/api/calendars', () => ({ calendarsApi }));
vi.mock('@/api/unifiedFeed', () => ({ unifiedFeedApi }));

const { useAuthStore } = await import('./auth');
const { useCalendarStore } = await import('./calendar');
const { useUnifiedFeedStore } = await import('./unifiedFeed');
const {
  registerRemoteSignoutListener,
  registerRemoteSessionListener,
  handleRemoteSessionRestored,
} = await import('./remoteSignout');

const USER_B = {
  id: 'u-b',
  email: 'b@example.test',
  display_name: 'B',
  role: 'user',
  locale: 'fr',
  timezone: 'Europe/Paris',
  email_verified: true,
  created_at: '2026-02-01T00:00:00Z',
};

function populateStores() {
  const authStore = useAuthStore();
  authStore.user = {
    id: 'u-1',
    email: 'a@example.test',
    display_name: 'A',
    role: 'user',
    locale: 'en',
    timezone: 'UTC',
    email_verified: true,
    created_at: '2026-01-01T00:00:00Z',
  };
  const calendarStore = useCalendarStore();
  calendarStore.calendars = [{ id: 'c-1', name: 'A-owned', participants: [] }] as never;
  calendarStore.calendarsForUser = 'u-1';
  calendarStore.currentCalendar = { id: 'c-settings', name: 'in settings' } as never;
  calendarStore.currentPublicCalendar = { id: 'c-public', name: 'Board games' } as never;
  const feedStore = useUnifiedFeedStore();
  feedStore.config = { configured: true } as never;
  return { authStore, calendarStore, feedStore };
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setActivePinia(createPinia());
  (apiClient.getFamilyId as ReturnType<typeof vi.fn>).mockReturnValue(null);
});

describe('remote sign-out', () => {
  it('resets the account-scoped stores when the event fires on a public route', () => {
    const { authStore, calendarStore, feedStore } = populateStores();

    const unsubscribe = registerRemoteSignoutListener();
    try {
      window.dispatchEvent(new CustomEvent(REMOTE_SIGNOUT_EVENT));
    } finally {
      unsubscribe();
    }

    expect(authStore.user).toBeNull();
    expect(calendarStore.calendars).toEqual([]);
    expect(calendarStore.calendarsForUser).toBeNull();
    expect(feedStore.config).toBeNull();
  });

  it('preserves the public calendar so a public-route sign-out does not blank the page', () => {
    const { authStore, calendarStore } = populateStores();

    const unsubscribe = registerRemoteSignoutListener();
    try {
      window.dispatchEvent(new CustomEvent(REMOTE_SIGNOUT_EVENT));
    } finally {
      unsubscribe();
    }

    // Owner/account state is gone — but the public payload on screen is not
    // authenticated account data, and there is no navigation on a public route to
    // reload it from. It must keep rendering after the account reset.
    expect(calendarStore.calendars).toEqual([]);
    expect(calendarStore.calendarsForUser).toBeNull();
    expect(calendarStore.currentCalendar).toBeNull();
    expect(authStore.user).toBeNull();
    expect(calendarStore.currentPublicCalendar?.id).toBe('c-public');
  });

  it('no longer resets anything after being unsubscribed', () => {
    const { authStore } = populateStores();
    const unsubscribe = registerRemoteSignoutListener();
    unsubscribe();

    window.dispatchEvent(new CustomEvent(REMOTE_SIGNOUT_EVENT));

    expect(authStore.user).not.toBeNull();
  });

  it('is safe to register twice (dedupes listeners is not required, but no error)', () => {
    const unsubscribeA = registerRemoteSignoutListener();
    const unsubscribeB = registerRemoteSignoutListener();
    unsubscribeA();
    unsubscribeB();
    expect(true).toBe(true);
  });
});

describe('remote session restored', () => {
  const ACCEPTED_FAMILY = 'family-b';

  function dispatchSessionEvent(): void {
    window.dispatchEvent(
      new CustomEvent(REMOTE_SESSION_EVENT, { detail: { family: ACCEPTED_FAMILY } })
    );
  }

  it('resets the previous account and hydrates /auth/me when the family is still current', async () => {
    const { authStore, calendarStore, feedStore } = populateStores();
    // The API client still belongs to the family the event announces.
    (apiClient.getFamilyId as ReturnType<typeof vi.fn>).mockReturnValue(ACCEPTED_FAMILY);
    authApi.getMe.mockResolvedValue(USER_B);

    const unsubscribe = registerRemoteSessionListener();
    try {
      dispatchSessionEvent();
      await vi.waitFor(() => expect(authStore.user?.id).toBe('u-b'));
    } finally {
      unsubscribe();
    }

    // The previous account's state is gone, and /auth/me hydrated B.
    expect(authApi.getMe).toHaveBeenCalled();
    expect(authStore.user).toEqual(USER_B);
    expect(calendarStore.calendars).toEqual([]);
    expect(calendarStore.calendarsForUser).toBeNull();
    expect(feedStore.config).toBeNull();
  });

  it('hydrates the confirmed identity with a single /auth/me call', async () => {
    (apiClient.getFamilyId as ReturnType<typeof vi.fn>).mockReturnValue(ACCEPTED_FAMILY);
    authApi.getMe.mockResolvedValue(USER_B);

    await handleRemoteSessionRestored(ACCEPTED_FAMILY);
    expect(authApi.getMe).toHaveBeenCalledTimes(1);
  });

  it('does not commit a /auth/me answer for a superseded family', async () => {
    const { authStore } = populateStores();
    // A newer session was accepted while /auth/me was on the wire.
    (apiClient.getFamilyId as ReturnType<typeof vi.fn>).mockReturnValue('family-c');
    authApi.getMe.mockResolvedValue({
      ...USER_B,
      id: 'u-c',
      email: 'c@example.test',
    });

    const unsubscribe = registerRemoteSessionListener();
    try {
      dispatchSessionEvent();
      await vi.waitFor(() => expect(authApi.getMe).toHaveBeenCalled());
      await Promise.resolve();
      await Promise.resolve();
    } finally {
      unsubscribe();
    }

    expect(authStore.user).toBeNull();
  });

  it('does not clear the current token when a superseded /auth/me fails', async () => {
    populateStores();
    (apiClient.getFamilyId as ReturnType<typeof vi.fn>).mockReturnValue('family-c');
    authApi.getMe.mockRejectedValue(new Error('gone'));

    const unsubscribe = registerRemoteSessionListener();
    try {
      dispatchSessionEvent();
      await vi.waitFor(() => expect(authApi.getMe).toHaveBeenCalled());
      await Promise.resolve();
    } finally {
      unsubscribe();
    }

    // A failure for an older family must not evict the newer family's token.
    expect(apiClient.clearToken).not.toHaveBeenCalled();
  });

  it('does not crash when the hydration itself fails for the current family', async () => {
    const { authStore, calendarStore } = populateStores();
    (apiClient.getFamilyId as ReturnType<typeof vi.fn>).mockReturnValue(ACCEPTED_FAMILY);
    // A *definitive* rejection: the backend refused the accepted session outright.
    authApi.getMe.mockRejectedValue({ code: 'UNAUTHORIZED', message: 'session dead' });

    const unsubscribe = registerRemoteSessionListener();
    try {
      dispatchSessionEvent();
      await vi.waitFor(() => expect(authApi.getMe).toHaveBeenCalled());
      await Promise.resolve();
    } finally {
      unsubscribe();
    }

    // The reset still happened; the failure did not throw, and the dead session is
    // cleared (the token it was issued for is genuinely gone).
    expect(authStore.user).toBeNull();
    expect(calendarStore.calendars).toEqual([]);
    expect(apiClient.clearToken).toHaveBeenCalled();
  });

  it('does not crash when the hydration fails transiently for the current family', async () => {
    const { authStore } = populateStores();
    (apiClient.getFamilyId as ReturnType<typeof vi.fn>).mockReturnValue(ACCEPTED_FAMILY);
    // A transient fault proves nothing; the token survives for a retry.
    authApi.getMe.mockRejectedValue({ code: 'INTERNAL', message: 'boom' });

    const unsubscribe = registerRemoteSessionListener();
    try {
      dispatchSessionEvent();
      await vi.waitFor(() => expect(authApi.getMe).toHaveBeenCalled());
      await Promise.resolve();
    } finally {
      unsubscribe();
    }

    expect(authStore.user).toBeNull();
    expect(apiClient.clearToken).not.toHaveBeenCalled();
  });

  it('is safe to register twice (dedupes listeners is not required, but no error)', () => {
    const unsubscribeA = registerRemoteSessionListener();
    const unsubscribeB = registerRemoteSessionListener();
    unsubscribeA();
    unsubscribeB();
    expect(true).toBe(true);
  });
});
