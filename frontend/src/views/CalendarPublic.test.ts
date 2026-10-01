/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 *
 * The regression this file guards: the owner of a calendar with `lock_participants`
 * enabled used to be shown the "direct participant link required" wall on their own
 * public link — even though they are already authenticated and managing the calendar
 * from the same session. The lock is a door for *anonymous* readers; the owner must be
 * able to reach the participant picker (and, through it, any participant's view) the
 * same way they can when the calendar is unlocked.
 *
 * It also pins the two account-isolation rules that make the owner shortcut safe:
 *
 *   - the owner list is only trusted when it was fetched for the *current* user (and
 *     not for a previous account on the same browser), and
 *   - the ownership decision is awaited before the saved participant is validated or
 *     cleared, so a cold cache cannot delete a valid saved participant by racing the
 *     ownership fetch.
 */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { flushPromises } from '@vue/test-utils';
import { nextTick, reactive } from 'vue';
import type { User } from '@/types';

import { mountWithI18n } from '@/test/harness';

const routerReplace = vi.fn();
const routerPush = vi.fn();
// Reactive route so tests can drive the component across /c/<token-a> → /c/<token-b>
// while the router reuses the same instance, exactly as Vue Router does.
const routeState = reactive({ params: { token: 'tok' }, query: {} });

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ replace: routerReplace, push: routerPush }),
}));

const calendarsApi = {
  getAll: vi.fn(),
  getPublic: vi.fn(),
  addAnonymousParticipant: vi.fn(),
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
  checkMagicLinkAvailable: vi.fn(),
};

const apiClient = {
  setToken: vi.fn(),
  clearToken: vi.fn(),
  hasSession: vi.fn(() => false),
  signOut: vi.fn(),
};

vi.mock('@/api/auth', () => ({ authApi }));
vi.mock('@/api/client', () => ({ apiClient }));

// Dynamic imports once the mocks above are registered: a static import is hoisted
// past the vi.mock calls, so the component would load the real (unmocked) API modules.
const { useCalendarStore } = await import('@/stores/calendar');
const { useAuthStore } = await import('@/stores/auth');
const { useCalendarHistoryStore } = await import('@/stores/calendarHistory');
const { default: CalendarPublic } = await import('./CalendarPublic.vue');

const RouterLinkStub = { props: ['to'], template: '<a :data-to="to"><slot /></a>' };

const USER_A: User = {
  id: 'u-a',
  email: 'a@example.test',
  display_name: 'A',
  role: 'user',
  locale: 'en',
  timezone: 'UTC',
  email_verified: true,
  created_at: '2026-01-01T00:00:00Z',
} as User;

const USER_B: User = {
  id: 'u-b',
  email: 'b@example.test',
  display_name: 'B',
  role: 'user',
  locale: 'en',
  timezone: 'UTC',
  email_verified: true,
  created_at: '2026-01-01T00:00:00Z',
} as User;

/** Public payload for `tok`: locked, participants whose ids are masked (no ids). */
function lockedPublicCalendar() {
  return {
    id: 'c-1',
    name: 'Board games',
    lock_participants: true,
    allow_anonymous_participants: true,
    participants: [{ name: 'Ada', email_verified: false }],
  };
}

function unlockedPublicCalendar() {
  return {
    id: 'c-1',
    name: 'Board games',
    lock_participants: false,
    allow_anonymous_participants: true,
    participants: [
      { id: 'p-ada', name: 'Ada', email_verified: false },
      { id: 'p-lin', name: 'Lin', email_verified: false },
    ],
  };
}

/** The owner-side calendar for `c-1`, whose participants keep their real ids. */
function ownedCalendar(
  participants: Array<{ id: string; name: string }> = [
    { id: 'p-ada', name: 'Ada' },
    { id: 'p-lin', name: 'Lin' },
  ]
) {
  return {
    id: 'c-1',
    name: 'Board games',
    public_token: 'tok',
    lock_participants: true,
    participants,
  };
}

/** A promise with its resolvers exposed, so a test can control when it settles. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

/** Mount with explicit store state; `cache` is what the calendar store currently holds. */
async function mountWithStore(options: {
  calendar: unknown;
  cache: unknown[];
  cacheForUser: string | null;
  signedInUser: User | null;
}) {
  const pinia = createPinia();
  setActivePinia(pinia);
  calendarsApi.getPublic.mockResolvedValue(options.calendar);
  // The owner list is now refetched on every visit, so the refetch must land on the
  // same cache the tests preload — otherwise it would wipe it before `isOwner` runs.
  calendarsApi.getAll.mockResolvedValue(options.cache);

  const calendarStore = useCalendarStore();
  calendarStore.calendars = options.cache as never;
  calendarStore.calendarsForUser = options.cacheForUser;

  const authStore = useAuthStore();
  authStore.user = options.signedInUser;

  const wrapper = await mountWithI18n(CalendarPublic, {
    global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
  });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  // Several tests navigate across public tokens; reset the shared route between them.
  routeState.params = { token: 'tok' };
  routeState.query = {};
});

describe('CalendarPublic.vue — participant lock vs the owner', () => {
  it('shows the participant-lock wall to an anonymous reader of a locked calendar', async () => {
    const wrapper = await mountWithStore({
      calendar: lockedPublicCalendar(),
      cache: [],
      cacheForUser: null,
      signedInUser: null,
    });

    expect(wrapper.text()).toContain('requires a direct participant link');
    expect(wrapper.text()).not.toContain('You own this calendar');
    // Masked participants must not produce clickable participant links.
    expect(wrapper.findAll('a[data-to]').length).toBe(0);
  });

  it('shows the participant-lock wall to an authenticated non-owner', async () => {
    // The reader is authenticated and their own (fresh) list simply has no such
    // calendar — a cache bound to them that does not contain it.
    const wrapper = await mountWithStore({
      calendar: lockedPublicCalendar(),
      cache: [],
      cacheForUser: USER_B.id,
      signedInUser: USER_B,
    });

    expect(wrapper.text()).toContain('requires a direct participant link');
    expect(wrapper.findAll('a[data-to]').length).toBe(0);
  });

  it('never unlocks a stale previous-account cache for the current account, even mid-refresh', async () => {
    // Account A loaded its dashboard on this browser: the store holds A's calendars,
    // tracked as belonging to A. B then signs in without a page reload. The ownership
    // refresh starts and is held pending — it must not matter: B may never see A's
    // participant ids, not even while the fresh list is still loading.
    const refresh = deferred<unknown[]>();
    calendarsApi.getAll.mockReturnValue(refresh.promise);

    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(lockedPublicCalendar());

    const calendarStore = useCalendarStore();
    calendarStore.calendars = [ownedCalendar()] as never;
    calendarStore.calendarsForUser = USER_A.id;
    const authStore = useAuthStore();
    authStore.user = USER_B;

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    // The stale cache is bound to A, so it must not lift the lock for B — the link
    // check and owner view stay off the screen even while the refresh is pending.
    expect(wrapper.findAll('a[data-to]').length).toBe(0);
    expect(wrapper.text()).not.toContain('You own this calendar');

    // B owns nothing: once the refresh lands, the wall shows.
    refresh.resolve([]);
    await flushPromises();
    expect(wrapper.text()).toContain('requires a direct participant link');
    expect(wrapper.findAll('a[data-to]').length).toBe(0);
    wrapper.unmount();
  });

  it('lifts the lock for the authenticated owner of the calendar', async () => {
    const wrapper = await mountWithStore({
      calendar: lockedPublicCalendar(),
      cache: [ownedCalendar()],
      cacheForUser: USER_A.id,
      signedInUser: USER_A,
    });

    expect(wrapper.text()).not.toContain('requires a direct participant link');
    expect(wrapper.text()).toContain('You own this calendar');
    // The owner can reach any participant through their real ids.
    expect(wrapper.findAll('a[data-to="/c/tok/p/p-ada"]').length).toBe(1);
    expect(wrapper.findAll('a[data-to="/c/tok/p/p-lin"]').length).toBe(1);
  });

  it('does not offer the anonymous join form to the owner', async () => {
    const wrapper = await mountWithStore({
      calendar: lockedPublicCalendar(),
      cache: [ownedCalendar()],
      cacheForUser: USER_A.id,
      signedInUser: USER_A,
    });

    expect(wrapper.find('form').exists()).toBe(false);
  });

  it('keeps the anonymous join form for a non-owner of an unlocked, open calendar', async () => {
    const wrapper = await mountWithStore({
      calendar: unlockedPublicCalendar(),
      cache: [],
      cacheForUser: null,
      signedInUser: null,
    });

    expect(wrapper.find('form').exists()).toBe(true);
    expect(wrapper.findAll('a[data-to="/c/tok/p/p-ada"]').length).toBe(1);
  });

  it('tells the owner of an empty locked calendar to add participants from settings', async () => {
    const wrapper = await mountWithStore({
      calendar: lockedPublicCalendar(),
      cache: [ownedCalendar([])],
      cacheForUser: USER_A.id,
      signedInUser: USER_A,
    });

    expect(wrapper.text()).toContain('no participants yet');
    expect(wrapper.text()).toContain('calendar settings');
  });

  it('resumes the saved participant of the owner on a locked calendar', async () => {
    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(lockedPublicCalendar());
    // F4: the owner list is refetched on every visit, so get it to resolve the same
    // owner roster the test preloads below.
    calendarsApi.getAll.mockResolvedValue([ownedCalendar()]);

    const calendarStore = useCalendarStore();
    calendarStore.calendars = [ownedCalendar()] as never;
    calendarStore.calendarsForUser = USER_A.id;
    const authStore = useAuthStore();
    authStore.user = USER_A;

    const history = useCalendarHistoryStore();
    history.addCalendar('tok', 'Board games', 'p-ada');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    expect(routerReplace).toHaveBeenCalledWith('/c/tok/p/p-ada');
    // The saved participant survives: it is still on record for next time.
    expect(history.getParticipantId('tok')).toBe('p-ada');
    wrapper.unmount();
  });

  it('retains and resumes the saved participant on a cold cache, after the ownership fetch lands', async () => {
    // Cold cache: the store has never loaded the owner list for this user, so the
    // ownership decision must wait for the fetch instead of judging on empty data.
    const getAll = deferred<unknown[]>();
    calendarsApi.getAll.mockReturnValue(getAll.promise);

    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(lockedPublicCalendar());

    const calendarStore = useCalendarStore();
    calendarStore.calendars = [];
    calendarStore.calendarsForUser = null;
    const authStore = useAuthStore();
    authStore.user = USER_A;

    const history = useCalendarHistoryStore();
    history.addCalendar('tok', 'Board games', 'p-ada');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    // The decision is not made before the ownership fetch settles: nothing has been
    // cleared or routed yet.
    expect(history.getParticipantId('tok')).toBe('p-ada');
    expect(routerReplace).not.toHaveBeenCalled();

    getAll.resolve([ownedCalendar()]);
    await flushPromises();

    expect(routerReplace).toHaveBeenCalledWith('/c/tok/p/p-ada');
    expect(history.getParticipantId('tok')).toBe('p-ada');
    wrapper.unmount();
  });

  it('resumes a saved participant for an anonymous reader of a locked calendar', async () => {
    // The locked public payload masks every participant id, so the page can never
    // prove the saved capability is stale. It must hand off to the participant route
    // for backend validation instead of erasing a valid direct link.
    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(lockedPublicCalendar());

    const history = useCalendarHistoryStore();
    history.addCalendar('tok', 'Board games', 'p-ada');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    expect(routerReplace).toHaveBeenCalledWith('/c/tok/p/p-ada');
    expect(history.getParticipantId('tok')).toBe('p-ada');
    wrapper.unmount();
  });

  it('resumes a saved participant for an authenticated non-owner of a locked calendar', async () => {
    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(lockedPublicCalendar());
    calendarsApi.getAll.mockResolvedValue([]);

    const calendarStore = useCalendarStore();
    calendarStore.calendars = [];
    calendarStore.calendarsForUser = USER_B.id;
    const authStore = useAuthStore();
    authStore.user = USER_B;

    const history = useCalendarHistoryStore();
    history.addCalendar('tok', 'Board games', 'p-ada');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    expect(routerReplace).toHaveBeenCalledWith('/c/tok/p/p-ada');
    expect(history.getParticipantId('tok')).toBe('p-ada');
    wrapper.unmount();
  });

  it('resumes a saved participant when the owner roster fetch fails', async () => {
    // The owner lookup is best-effort; when it fails there is no id-bearing roster to
    // disprove the saved capability, so it must survive for the backend to validate.
    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(lockedPublicCalendar());
    calendarsApi.getAll.mockRejectedValue(new Error('offline'));

    const calendarStore = useCalendarStore();
    calendarStore.calendars = [ownedCalendar()] as never;
    calendarStore.calendarsForUser = USER_A.id;
    const authStore = useAuthStore();
    authStore.user = USER_A;

    const history = useCalendarHistoryStore();
    history.addCalendar('tok', 'Board games', 'p-ada');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    expect(routerReplace).toHaveBeenCalledWith('/c/tok/p/p-ada');
    expect(history.getParticipantId('tok')).toBe('p-ada');
    wrapper.unmount();
  });

  it('clears a saved participant only when the roster authoritatively proves it is gone', async () => {
    // An unlocked roster carries every real id, so a saved id that is not among them is
    // genuinely stale and can be dropped in place.
    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(unlockedPublicCalendar());

    const history = useCalendarHistoryStore();
    history.addCalendar('tok', 'Board games', 'p-gone');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    expect(history.getParticipantId('tok')).toBeUndefined();
    expect(routerReplace).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('keeps the public calendar rendering, without owner powers, after a session reset', async () => {
    // A tab stays open on a public calendar while another tab signs the session out.
    // The account reset must strip the owner privileges but must NOT blank the page:
    // there is no navigation on a public route to reload the public payload from.
    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(lockedPublicCalendar());
    // Keep the owner list resolvable on the always-refetch, independent of any earlier
    // test's mock implementation (clearAllMocks does not reset implementations).
    calendarsApi.getAll.mockResolvedValue([ownedCalendar()]);

    const calendarStore = useCalendarStore();
    calendarStore.calendars = [ownedCalendar()] as never;
    calendarStore.calendarsForUser = USER_A.id;
    const authStore = useAuthStore();
    authStore.user = USER_A;

    const { registerRemoteSignoutListener } = await import('@/stores/remoteSignout');
    const { REMOTE_SIGNOUT_EVENT } = await import('@/sessionEvents');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();
    // The owner had the lock lifted; the calendar is on screen.
    expect(wrapper.text()).toContain('You own this calendar');

    const unsubscribe = registerRemoteSignoutListener();
    try {
      window.dispatchEvent(new CustomEvent(REMOTE_SIGNOUT_EVENT));
      await flushPromises();
    } finally {
      unsubscribe();
    }

    // The public page still renders its content, but ownership evidence is gone.
    expect(wrapper.text()).toContain('Board games');
    expect(wrapper.text()).not.toContain('You own this calendar');
    wrapper.unmount();
  });

  it('treats an unlocked empty roster as authoritative and clears a stale saved participant', async () => {
    // An unlocked calendar's roster is complete even when empty: a saved id that is
    // not in it is genuinely stale, so it is cleared in place — not routed to a dead
    // participant that would wipe the whole history entry.
    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue({
      id: 'c-1',
      name: 'Board games',
      lock_participants: false,
      allow_anonymous_participants: true,
      participants: [],
    });

    const history = useCalendarHistoryStore();
    history.addCalendar('tok', 'Board games', 'p-gone');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    expect(history.getParticipantId('tok')).toBeUndefined();
    expect(routerReplace).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("treats the owner's empty locked roster as authoritative and clears a stale saved participant", async () => {
    // The owner lookup succeeds and returns the owned calendar with zero participants:
    // that roster is complete for the owner, so a saved id not in it is stale.
    const pinia = createPinia();
    setActivePinia(pinia);
    calendarsApi.getPublic.mockResolvedValue(lockedPublicCalendar());
    calendarsApi.getAll.mockResolvedValue([ownedCalendar([])]);

    const calendarStore = useCalendarStore();
    calendarStore.calendars = [ownedCalendar([])] as never;
    calendarStore.calendarsForUser = USER_A.id;
    const authStore = useAuthStore();
    authStore.user = USER_A;

    const history = useCalendarHistoryStore();
    history.addCalendar('tok', 'Board games', 'p-gone');

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    expect(history.getParticipantId('tok')).toBeUndefined();
    expect(routerReplace).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('follows the route token: re-renders and targets the new calendar, not the old one', async () => {
    // The router reuses this component instance when navigating between public links;
    // the page must reload for the new token and never join the old calendar.
    const pinia = createPinia();
    setActivePinia(pinia);
    const calendarFor = (token: string, name: string, participantId: string) => ({
      id: token,
      name,
      lock_participants: false,
      allow_anonymous_participants: true,
      participants: [{ id: participantId, name: 'Participant', email_verified: false }],
    });
    calendarsApi.getPublic.mockResolvedValue(calendarFor('tok', 'Alpha', 'p-a1'));
    calendarsApi.getAll.mockResolvedValue([]);

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    expect(wrapper.text()).toContain('Alpha');
    expect(wrapper.findAll('a[data-to="/c/tok/p/p-a1"]').length).toBe(1);

    // Navigate to a different calendar: only the token param changes.
    calendarsApi.getPublic.mockResolvedValue(calendarFor('tok-b', 'Beta', 'p-b1'));
    routeState.params.token = 'tok-b';
    await nextTick();
    await flushPromises();

    // Rendered data and generated links now target the *new* calendar.
    expect(wrapper.text()).toContain('Beta');
    expect(wrapper.findAll('a[data-to="/c/tok-b/p/p-b1"]').length).toBe(1);
    expect(wrapper.findAll('a[data-to="/c/tok/p/p-a1"]').length).toBe(0);
    wrapper.unmount();
  });

  it('does not commit a join that settles after navigating to another public calendar', async () => {
    // A deferred A-join must not save its participant id under B's history entry, push
    // itself into B's roster, or route B to a (dead) participant URL.
    const pinia = createPinia();
    setActivePinia(pinia);
    const calendarFor = (token: string, name: string, participantId: string) => ({
      id: token,
      name,
      lock_participants: false,
      allow_anonymous_participants: true,
      participants: [{ id: participantId, name: 'Participant', email_verified: false }],
    });
    calendarsApi.getPublic.mockResolvedValue(calendarFor('tok', 'Alpha', 'p-a1'));
    calendarsApi.getAll.mockResolvedValue([]);

    const history = useCalendarHistoryStore();

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();
    expect(wrapper.findAll('form').length).toBe(1);

    // Submit the anonymous join on /c/tok; the create request stays in flight.
    const join = deferred<{ id: string; name: string; email_verified: boolean }>();
    calendarsApi.addAnonymousParticipant.mockReturnValue(join.promise);
    await wrapper.find('form input').setValue('Newcomer');
    await wrapper.find('form').trigger('submit');
    await flushPromises();
    expect(calendarsApi.addAnonymousParticipant).toHaveBeenCalledWith('tok', {
      name: 'Newcomer',
    });

    // Navigate to another public calendar before the join settles.
    calendarsApi.getPublic.mockResolvedValue(calendarFor('tok-b', 'Beta', 'p-b1'));
    routeState.params.token = 'tok-b';
    await nextTick();
    await flushPromises();
    expect(wrapper.text()).toContain('Beta');

    // The deferred A-join settles after B is already on screen.
    join.resolve({ id: 'p-new', name: 'Newcomer', email_verified: false });
    await flushPromises();

    // No continuation targeted B: no participant history entry, no navigation to a dead
    // route, and B's roster never picked up A's participant.
    expect(routerPush).not.toHaveBeenCalled();
    expect(routerReplace).not.toHaveBeenCalled();
    expect(history.getParticipantId('tok-b')).toBeUndefined();
    expect(wrapper.findAll('a[data-to="/c/tok-b/p/p-new"]').length).toBe(0);
    wrapper.unmount();
  });

  it('does not let a stale A-join re-enable B\u2019s form while B\u2019s join is still in flight', async () => {
    // The join's success/error effects are fenced, but its `finally` used to clear the
    // shared busy flag unconditionally: A settling first would flip B's form back to
    // idle mid-submit, inviting duplicate B submissions.
    const pinia = createPinia();
    setActivePinia(pinia);
    const calendarFor = (token: string, name: string, participantId: string) => ({
      id: token,
      name,
      lock_participants: false,
      allow_anonymous_participants: true,
      participants: [{ id: participantId, name: 'Participant', email_verified: false }],
    });
    calendarsApi.getPublic.mockResolvedValue(calendarFor('tok', 'Alpha', 'p-a1'));
    calendarsApi.getAll.mockResolvedValue([]);

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    // A's join stays on the wire.
    const joinA = deferred<{ id: string; name: string; email_verified: boolean }>();
    calendarsApi.addAnonymousParticipant.mockReturnValueOnce(joinA.promise);
    await wrapper.find('form input').setValue('Newcomer');
    await wrapper.find('form').trigger('submit');
    await flushPromises();
    expect(calendarsApi.addAnonymousParticipant).toHaveBeenCalledTimes(1);

    // Navigate to B (the route watcher resets the flag), then B starts its own join.
    calendarsApi.getPublic.mockResolvedValue(calendarFor('tok-b', 'Beta', 'p-b1'));
    routeState.params.token = 'tok-b';
    await nextTick();
    await flushPromises();
    expect(wrapper.text()).toContain('Beta');

    const joinB = deferred<{ id: string; name: string; email_verified: boolean }>();
    calendarsApi.addAnonymousParticipant.mockReturnValueOnce(joinB.promise);
    await wrapper.find('form input').setValue('Newcomer');
    await wrapper.find('form').trigger('submit');
    await flushPromises();
    expect(calendarsApi.addAnonymousParticipant).toHaveBeenCalledTimes(2);

    // A settles first: B's form must stay busy (its own join is still pending).
    joinA.resolve({ id: 'p-new-a', name: 'Newcomer', email_verified: false });
    await flushPromises();
    expect(wrapper.find('button[type=submit]').attributes('disabled')).toBeDefined();

    // B settles: only now is the busy state cleared for the current operation.
    joinB.resolve({ id: 'p-new-b', name: 'Newcomer', email_verified: false });
    await flushPromises();
    expect(wrapper.find('button[type=submit]').attributes('disabled')).toBeUndefined();
    wrapper.unmount();
  });

  it('does not show an error or eject from the page when a superseded load fails', async () => {
    // A slow failure for token A must not surface after token B has loaded: no stale
    // error toast, no history removal, and no eject from the valid B page.
    const pinia = createPinia();
    setActivePinia(pinia);
    const calendarFor = (token: string, name: string, participantId: string) => ({
      id: token,
      name,
      lock_participants: false,
      allow_anonymous_participants: true,
      participants: [{ id: participantId, name: 'Participant', email_verified: false }],
    });
    // First (tok) load stays pending, then fails; the tok-b load resolves.
    const aFailure = deferred<unknown>();
    calendarsApi.getPublic
      .mockReturnValueOnce(aFailure.promise)
      .mockResolvedValue(calendarFor('tok-b', 'Beta', 'p-b1'));
    calendarsApi.getAll.mockResolvedValue([]);

    const history = useCalendarHistoryStore();
    const { useToastStore } = await import('@/stores/toast');
    const toastStore = useToastStore();

    const wrapper = await mountWithI18n(CalendarPublic, {
      global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
    });
    await flushPromises();

    // Navigate to tok-b; its load resolves and renders.
    routeState.params.token = 'tok-b';
    await nextTick();
    await flushPromises();
    expect(wrapper.text()).toContain('Beta');

    // Now tok's deferred failure lands.
    aFailure.reject(new Error('boom'));
    await flushPromises();

    // The superseded failure was ignored: no error toast, no eject, and B's history
    // entry survived.
    expect(toastStore.toasts.length).toBe(0);
    expect(routerPush).not.toHaveBeenCalled();
    expect(history.calendars.some(c => c.token === 'tok-b')).toBe(true);
    expect(wrapper.text()).toContain('Beta');
    wrapper.unmount();
  });
});
