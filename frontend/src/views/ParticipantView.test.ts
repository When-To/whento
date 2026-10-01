/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 *
 * The regression this file guards: an account reset on a public participant link — a
 * cross-tab sign-out, or a local refresh failure — must strip the previous account's
 * privileges from the participant view without blanking the page. The participant
 * route has no reload: navigation is skipped on purpose, so the public payload has to
 * keep rendering.
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { flushPromises } from '@vue/test-utils';
import { nextTick, reactive } from 'vue';
import type { User, PublicCalendar } from '@/types';

const routerReplace = vi.fn();
const routerPush = vi.fn();
// Reactive route so tests can navigate between calendars/participants on the same
// component instance, exactly as Vue Router does.
const routeState = reactive({ params: { token: 'tok', participantId: 'p-ada' }, query: {} });

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ replace: routerReplace, push: routerPush }),
}));

const calendarsApi = {
  getAll: vi.fn(),
  getPublic: vi.fn(),
};
vi.mock('@/api/calendars', () => ({ calendarsApi }));

const authApi = {
  login: vi.fn(),
  register: vi.fn(),
  bootstrap: vi.fn(),
  bootstrapStatus: vi.fn(),
  getMe: vi.fn(),
  updateProfile: vi.fn(),
  updatePassword: vi.fn(),
  logout: vi.fn(),
  forgotPassword: vi.fn(),
  resetPassword: vi.fn(),
  checkMagicLinkAvailable: vi.fn(async () => ({ available: false })),
};
const apiClient = {
  setToken: vi.fn(),
  clearToken: vi.fn(),
  hasSession: vi.fn(() => false),
  signOut: vi.fn(),
};
vi.mock('@/api/auth', () => ({ authApi }));
vi.mock('@/api/client', () => ({ apiClient }));

// The live stream composable opens an EventSource, which jsdom does not provide. It
// only drives reloads (reconciliation) — irrelevant to the reset-persistence behavior
// under test — so replace it with a no-op stream.
vi.mock('@/composables/calendar/useCalendarStream', () => ({
  useCalendarStream: vi.fn((_options: unknown) => ({ connected: false, close: vi.fn() })),
}));
vi.mock('@/api/availabilities', () => ({
  availabilitiesApi: {
    getByParticipant: vi.fn(async () => ({ availabilities: [] })),
    getRecurrences: vi.fn(async () => []),
    getRangeSummary: vi.fn(async () => []),
    create: vi.fn(async () => ({})),
    update: vi.fn(async () => ({})),
    delete: vi.fn(async () => {}),
    createRecurrence: vi.fn(async () => ({})),
    deleteRecurrence: vi.fn(async () => {}),
    createException: vi.fn(async () => {}),
    deleteException: vi.fn(async () => {}),
  },
}));
vi.mock('@/api/holidays', () => ({
  holidaysApi: {
    year: vi.fn(async () => ({ country_code: 'FR', supported: true, holidays: [] })),
  },
}));

// Dynamic imports once the mocks above are registered: a static import is hoisted
// past the vi.mock calls and would pull in the real API modules.
const { useAuthStore } = await import('@/stores/auth');
const { useCalendarStore } = await import('@/stores/calendar');
const { registerRemoteSignoutListener } = await import('@/stores/remoteSignout');
const { REMOTE_SIGNOUT_EVENT } = await import('@/sessionEvents');
const { mountWithI18n } = await import('@/test/harness');
const { default: ParticipantView } = await import('./ParticipantView.vue');

const PUBLIC_CALENDAR: PublicCalendar = {
  id: 'c-1',
  name: 'Board games',
  lock_participants: false,
  participants: [{ id: 'p-ada', name: 'Ada', email: 'ada@example.test' }],
  timezone: 'Europe/Paris',
  allowed_weekdays: [0, 1, 2, 3, 4, 5, 6],
  notify_participants: false,
  ics_token: 'ics',
  threshold: 1,
  min_duration_hours: 0,
  holidays_policy: 'ignore',
  allow_holiday_eves: false,
  allow_anonymous_participants: false,
} as never;

const USER_A: User = {
  id: 'u-a',
  email: 'a@example.test',
  display_name: 'A',
  role: 'user',
  locale: 'en',
  timezone: 'UTC',
  created_at: '2026-01-01T00:00:00Z',
} as User;

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  setActivePinia(createPinia());
  calendarsApi.getAll.mockResolvedValue([]);
  calendarsApi.getPublic.mockResolvedValue(PUBLIC_CALENDAR);
  // Several tests navigate across calendars/participants; reset the shared route.
  routeState.params = { token: 'tok', participantId: 'p-ada' };
  routeState.query = {};
});

describe('ParticipantView.vue — account reset on a public route', () => {
  it('keeps the public calendar rendering after a session reset', async () => {
    // A healthy load must not surface caught failures (missing mock methods used to
    // make this test pass through console noise). Any unexpected console output fails
    // the test instead of being normalized.
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

    try {
      const pinia = createPinia();
      setActivePinia(pinia);

      const calendarStore = useCalendarStore();
      calendarStore.calendars = [{ id: 'c-1', name: 'Board games', public_token: 'tok' }] as never;
      calendarStore.calendarsForUser = USER_A.id;
      calendarStore.currentPublicCalendar = PUBLIC_CALENDAR;
      const authStore = useAuthStore();
      authStore.user = USER_A;

      const unsubscribe = registerRemoteSignoutListener();
      const wrapper = await mountWithI18n(ParticipantView, {
        global: { plugins: [pinia], stubs: { RouterLink: true } },
      });
      await flushPromises();
      await nextTick();

      // The load landed as a normal participant page: calendar, participant, and —
      // critically — no swallowed failures along the way.
      expect(wrapper.text()).toContain('Board games');
      expect(wrapper.text()).toContain('Ada');
      expect(errorSpy).not.toHaveBeenCalled();
      expect(warnSpy).not.toHaveBeenCalled();

      try {
        window.dispatchEvent(new CustomEvent(REMOTE_SIGNOUT_EVENT));
        await flushPromises();
        await nextTick();
      } finally {
        unsubscribe();
      }

      // The participant page is still there — the reset must not blank it.
      expect(wrapper.text()).toContain('Board games');
      expect(wrapper.text()).toContain('Ada');
      expect(authStore.user).toBeNull();
      expect(calendarStore.currentPublicCalendar).not.toBeNull();
      // And the reset itself is still a clean render.
      expect(errorSpy).not.toHaveBeenCalled();
      expect(warnSpy).not.toHaveBeenCalled();
      wrapper.unmount();
    } finally {
      errorSpy.mockRestore();
      warnSpy.mockRestore();
    }
  });
});

describe('ParticipantView.vue — route-scoped loads and stale participants', () => {
  function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (reason: unknown) => void;
    const promise = new Promise<T>((res, rej) => {
      resolve = res;
      reject = rej;
    });
    return { promise, resolve, reject };
  }

  function calendarFor(token: string, name: string, participantId: string): PublicCalendar {
    return {
      ...PUBLIC_CALENDAR,
      id: token,
      name,
      participants: [{ id: participantId, name: 'Ada', email: 'ada@example.test' }],
    } as never;
  }

  it('never redirects after a deferred old-token success when a newer route is already on screen', async () => {
    const { useCalendarHistoryStore } = await import('@/stores/calendarHistory');

    // The tok load stays pending; navigating to tok-b starts a newer load that settles.
    const aLoad = deferred<PublicCalendar>();
    calendarsApi.getPublic
      .mockReturnValueOnce(aLoad.promise)
      .mockResolvedValue(calendarFor('tok-b', 'Beta', 'p-b1'));

    const pinia = createPinia();
    setActivePinia(pinia);
    const historyStore = useCalendarHistoryStore();
    const wrapper = await mountWithI18n(ParticipantView, {
      global: { plugins: [pinia], stubs: { RouterLink: true } },
    });
    await flushPromises();
    await nextTick();

    routeState.params.token = 'tok-b';
    routeState.params.participantId = 'p-b1';
    await nextTick();
    await flushPromises();
    await nextTick();
    expect(wrapper.text()).toContain('Beta');

    // The old token's deferred success lands afterwards.
    aLoad.resolve(calendarFor('tok', 'Alpha', 'p-a1'));
    await flushPromises();
    await nextTick();

    // The old continuation did not inspect the newer route, decide its participant is
    // missing, wipe the newer calendar from history, or redirect home.
    expect(routerPush).not.toHaveBeenCalled();
    expect(routerReplace).not.toHaveBeenCalled();
    expect(historyStore.calendars.some(c => c.token === 'tok-b')).toBe(true);
    expect(wrapper.text()).toContain('Beta');
    wrapper.unmount();
  });

  it('does not show an error or eject from the page when a superseded load fails', async () => {
    const { useCalendarHistoryStore } = await import('@/stores/calendarHistory');
    const { useToastStore } = await import('@/stores/toast');

    // tok stays pending, then fails; tok-b resolves and renders.
    const aFailure = deferred<PublicCalendar>();
    calendarsApi.getPublic
      .mockReturnValueOnce(aFailure.promise)
      .mockResolvedValue(calendarFor('tok-b', 'Beta', 'p-b1'));

    const pinia = createPinia();
    setActivePinia(pinia);
    const historyStore = useCalendarHistoryStore();
    const toastStore = useToastStore();
    const wrapper = await mountWithI18n(ParticipantView, {
      global: { plugins: [pinia], stubs: { RouterLink: true } },
    });
    await flushPromises();
    await nextTick();

    routeState.params.token = 'tok-b';
    routeState.params.participantId = 'p-b1';
    await nextTick();
    await flushPromises();
    await nextTick();
    expect(wrapper.text()).toContain('Beta');

    aFailure.reject(new Error('boom'));
    await flushPromises();
    await nextTick();

    // A stale failure produced no error, no history removal and no eject.
    expect(toastStore.toasts.length).toBe(0);
    expect(routerPush).not.toHaveBeenCalled();
    expect(historyStore.calendars.some(c => c.token === 'tok-b')).toBe(true);
    expect(wrapper.text()).toContain('Beta');
    wrapper.unmount();
  });

  it('keeps the calendar in history when only the saved participant id is stale', async () => {
    const { useCalendarHistoryStore } = await import('@/stores/calendarHistory');

    // A locked calendar masks every participant id; the visitor followed an old direct
    // link whose participant id no longer matches anything on it. Only the backend can
    // validate a masked id — the calendar itself is fine, so the whole history entry
    // must not be deleted with it.
    calendarsApi.getPublic.mockResolvedValue({
      ...PUBLIC_CALENDAR,
      lock_participants: true,
      participants: [{ name: 'Lin', email_verified: false }],
    } as never);

    const pinia = createPinia();
    setActivePinia(pinia);
    const historyStore = useCalendarHistoryStore();
    historyStore.addCalendar('tok', 'Board games', 'p-ada');

    const wrapper = await mountWithI18n(ParticipantView, {
      global: { plugins: [pinia], stubs: { RouterLink: true } },
    });
    await flushPromises();
    await nextTick();

    // The stale capability was dropped and the visitor returned to the calendar page —
    // not thrown home, and the calendar/history settings survive.
    expect(routerReplace).toHaveBeenCalledWith('/c/tok');
    expect(routerPush).not.toHaveBeenCalled();
    expect(historyStore.getParticipantId('tok')).toBeUndefined();
    expect(historyStore.calendars.some(c => c.token === 'tok')).toBe(true);
    wrapper.unmount();
  });

  it('does not let a stale recurrence/range response overwrite a newer route\u2019s data', async () => {
    const { useCalendarHistoryStore } = await import('@/stores/calendarHistory');
    const { availabilitiesApi } = await import('@/api/availabilities');

    const pinia = createPinia();
    setActivePinia(pinia);
    const historyStore = useCalendarHistoryStore();

    // A's availability for route /c/tok/p-ada stays on the wire.
    const aOwn = deferred<{ availabilities: Array<{ date: string }> }>();
    (availabilitiesApi.getByParticipant as ReturnType<typeof vi.fn>).mockReturnValueOnce(
      aOwn.promise
    );
    (availabilitiesApi.getByParticipant as ReturnType<typeof vi.fn>).mockImplementation(
      async () => ({ availabilities: [] })
    );
    (availabilitiesApi.getRangeSummary as ReturnType<typeof vi.fn>).mockResolvedValue([]);
    (availabilitiesApi.getRecurrences as ReturnType<typeof vi.fn>).mockResolvedValue([]);

    calendarsApi.getPublic.mockResolvedValue(calendarFor('tok', 'Alpha', 'p-a1'));
    // The default test route targets p-ada; this scenario starts on p-a1.
    routeState.params = { token: 'tok', participantId: 'p-a1' };
    const wrapper = await mountWithI18n(ParticipantView, {
      global: { plugins: [pinia], stubs: { RouterLink: true } },
    });
    await flushPromises();
    await nextTick();

    // Navigate to a second participant on the same component.
    calendarsApi.getPublic.mockResolvedValue(calendarFor('tok-b', 'Beta', 'p-b1'));
    routeState.params.token = 'tok-b';
    routeState.params.participantId = 'p-b1';
    await nextTick();
    await flushPromises();
    await nextTick();
    expect(wrapper.text()).toContain('Beta');

    // A's stale response lands now. The sub-loaders fenced by the route version must
    // neither commit A's recurrences/counts over B's nor throw.
    aOwn.resolve({ availabilities: [{ date: '2099-01-01' }] });
    await flushPromises();
    await nextTick();

    // The page still renders B, and B's participant was not perturbed.
    expect(wrapper.text()).toContain('Beta');
    expect(historyStore.calendars.some(c => c.token === 'tok-b')).toBe(true);
    wrapper.unmount();
  });
});
