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
import { nextTick } from 'vue';
import type { CalendarWithParticipants, User } from '@/types';

import { mountWithI18n } from '@/test/harness';

const routerPush = vi.fn();

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
}));

const calendarsApi = {
  getAll: vi.fn(),
};
vi.mock('@/api/calendars', () => ({ calendarsApi }));

const unifiedFeedApi = {
  getConfig: vi.fn(),
  create: vi.fn(),
  updateCalendars: vi.fn(),
  regenerateToken: vi.fn(),
};
vi.mock('@/api/unifiedFeed', () => ({ unifiedFeedApi }));

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

// Dynamic imports once the mocks above are registered (a static import is hoisted
// past the vi.mock calls and would pull in the real API modules).
const { useAuthStore } = await import('@/stores/auth');
const { useCalendarStore } = await import('@/stores/calendar');
const { useDashboardStore } = await import('@/stores/dashboard');
const { useToastStore } = await import('@/stores/toast');
const { default: Dashboard } = await import('./Dashboard.vue');
const CalendarCard = (await import('@/components/dashboard/CalendarCard.vue')).default;

function calendar(id: string, name: string, extra: Partial<CalendarWithParticipants> = {}) {
  return {
    id,
    name,
    public_token: `tok-${id}`,
    participants: [],
    ...extra,
  } as CalendarWithParticipants;
}

const USER = { id: 'u-1', display_name: 'Owner' } as unknown as User;

async function mountDashboard(
  list: CalendarWithParticipants[],
  configureFeed: () => void = () =>
    unifiedFeedApi.getConfig.mockResolvedValue({ configured: false })
) {
  const pinia = createPinia();
  setActivePinia(pinia);
  calendarsApi.getAll.mockResolvedValue(list);
  configureFeed();

  const authStore = useAuthStore();
  authStore.user = USER;

  const wrapper = await mountWithI18n(Dashboard, {
    global: {
      plugins: [pinia],
      stubs: {
        QuotaUsage: true,
        RouterLink: { props: ['to'], template: '<a :data-to="to"><slot /></a>' },
      },
    },
  });
  await flushPromises();
  return wrapper;
}

/** The calendars in the order they are rendered on the page. */
function renderedIds(wrapper: Awaited<ReturnType<typeof mountDashboard>>) {
  return wrapper.findAllComponents(CalendarCard).map(card => card.props('calendar').id);
}

async function setSort(wrapper: Awaited<ReturnType<typeof mountDashboard>>, value: string) {
  await wrapper.find('#dashboard-sort').setValue(value);
  await nextTick();
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
});

describe('Dashboard.vue — my calendars', () => {
  it('renders calendars alphabetically ascending by default', async () => {
    const wrapper = await mountDashboard([
      calendar('z', 'Zebra'),
      calendar('b', 'Bravo'),
      calendar('a', 'Alpha'),
    ]);

    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'z']);
  });

  it('sorts alphabetically descending when chosen', async () => {
    const wrapper = await mountDashboard([
      calendar('z', 'Zebra'),
      calendar('b', 'Bravo'),
      calendar('a', 'Alpha'),
    ]);

    await setSort(wrapper, 'name-desc');
    expect(renderedIds(wrapper)).toEqual(['z', 'b', 'a']);
  });

  it('pins a calendar to the top', async () => {
    const wrapper = await mountDashboard([
      calendar('z', 'Zebra'),
      calendar('b', 'Bravo'),
      calendar('a', 'Alpha'),
    ]);

    // Pin "Zebra": the alphabetical default puts it last, so pinning should float it up.
    const zebraCard = wrapper
      .findAllComponents(CalendarCard)
      .find(card => card.props('calendar').id === 'z')!;
    await zebraCard.find('button[title="Pin to top"]').trigger('click');
    await nextTick();

    expect(renderedIds(wrapper)).toEqual(['z', 'a', 'b']);
    expect(wrapper.text()).toContain('Pinned');
  });

  it('does not let the move buttons cross the pinned/unpinned boundary', async () => {
    // Pin "Zebra": custom order now renders [z, a, b]. "Zebra" is the only (and so
    // the last) pinned calendar, and "Alpha" is the first unpinned one. Moving
    // "Zebra" down over "Alpha", or "Alpha" up over "Zebra", must be impossible —
    // the buttons are disabled and clicking them must not change the order.
    const wrapper = await mountDashboard([
      calendar('z', 'Zebra'),
      calendar('b', 'Bravo'),
      calendar('a', 'Alpha'),
    ]);
    await setSort(wrapper, 'custom');
    await wrapper
      .findAllComponents(CalendarCard)
      .find(card => card.props('calendar').id === 'z')!
      .find('button[title="Pin to top"]')
      .trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['z', 'a', 'b']);

    const byId = (id: string) =>
      wrapper.findAllComponents(CalendarCard).find(card => card.props('calendar').id === id)!;

    // "Zebra" is at the bottom of the pinned group: move down is dead.
    const zebraDown = byId('z').find('button[title="Move down"]');
    expect((zebraDown.element as HTMLButtonElement).disabled).toBe(true);
    await zebraDown.trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['z', 'a', 'b']);

    // "Alpha" is the first unpinned calendar: move up is dead and cannot lift it
    // above the pinned group.
    const alphaUp = byId('a').find('button[title="Move up"]');
    expect((alphaUp.element as HTMLButtonElement).disabled).toBe(true);
    await alphaUp.trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['z', 'a', 'b']);
  });

  it('switches between card, list, expanded and compact views', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')]);

    const clickView = async (label: string) => {
      await wrapper.find(`button[aria-label="${label}"]`).trigger('click');
      await nextTick();
    };

    await clickView('List');
    expect(wrapper.findAllComponents(CalendarCard)[0].props('view')).toBe('list');

    await clickView('Expanded list');
    expect(wrapper.findAllComponents(CalendarCard)[0].props('view')).toBe('expanded');

    await clickView('Compact list');
    expect(wrapper.findAllComponents(CalendarCard)[0].props('view')).toBe('compact');

    await clickView('Cards');
    expect(wrapper.findAllComponents(CalendarCard)[0].props('view')).toBe('card');
  });

  it('reorders calendars by drag and drop in custom order, through the real DOM', async () => {
    const wrapper = await mountDashboard([
      calendar('a', 'Alpha'),
      calendar('b', 'Bravo'),
      calendar('c', 'Charlie'),
    ]);

    // Switch to custom order: this seeds the manual order from the visible sort.
    await setSort(wrapper, 'custom');
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);
    expect(wrapper.text()).toContain('own order');

    const byId = (id: string) =>
      wrapper.findAllComponents(CalendarCard).find(card => card.props('calendar').id === id)!;

    // The grip must be a genuine draggable source: a real browser only fires
    // `dragstart` on an element carrying the `draggable` attribute.
    const grip = byId('a').find('.drag-grip');
    expect(grip.attributes('draggable')).toBe('true');

    // Drive the actual DOM path — dragstart on the source grip, drop on the target
    // card root — rather than synthesising the component's internal events.
    await grip.trigger('dragstart');
    await byId('c').trigger('drop');
    await nextTick();

    expect(renderedIds(wrapper)).toEqual(['b', 'a', 'c']);
  });

  it('does not let drag-and-drop cross the pinned/unpinned boundary', async () => {
    const wrapper = await mountDashboard([
      calendar('a', 'Alpha'),
      calendar('b', 'Bravo'),
      calendar('c', 'Charlie'),
    ]);
    await setSort(wrapper, 'custom');

    const byId = (id: string) =>
      wrapper.findAllComponents(CalendarCard).find(card => card.props('calendar').id === id)!;

    // Pin "Alpha": it leads the custom order, and its group is the pinned one.
    await byId('a').find('button[title="Pin to top"]').trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);
    const dashboardStore = useDashboardStore();
    const persistedBefore = [...dashboardStore.customOrder];

    // Dragging the pinned "Alpha" onto the unpinned "Bravo" is a cross-group move:
    // regrouped rendering would show no change, so it must not silently rewrite the
    // persisted sequence either.
    await byId('a').find('.drag-grip').trigger('dragstart');
    await byId('b').trigger('drop');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);
    expect(dashboardStore.customOrder).toEqual(persistedBefore);

    // And the reverse direction — unpinned "Bravo" onto pinned "Alpha" — is rejected
    // the same way.
    await byId('b').find('.drag-grip').trigger('dragstart');
    await byId('a').trigger('drop');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);
    expect(dashboardStore.customOrder).toEqual(persistedBefore);
  });

  it('reorders calendars with the accessible move up/down buttons', async () => {
    const wrapper = await mountDashboard([
      calendar('a', 'Alpha'),
      calendar('b', 'Bravo'),
      calendar('c', 'Charlie'),
    ]);
    await setSort(wrapper, 'custom');

    const byId = (id: string) =>
      wrapper.findAllComponents(CalendarCard).find(card => card.props('calendar').id === id)!;

    // Move "Charlie" up one slot.
    await byId('c').find('button[title="Move up"]').trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'c', 'b']);

    // And back down.
    await byId('c').find('button[title="Move down"]').trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);

    // Boundaries are no-ops: "Alpha" is already first.
    await byId('a').find('button[title="Move up"]').trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);
  });

  it('seeds the custom order from the visible alphabetical order, not the API order', async () => {
    // The API returns the calendars in a scrambled order; alphabetical sorting shows
    // them [Alpha, Bravo, Zebra]. Switching to custom order must keep that visible
    // order instead of snapping to the API's sequence.
    const wrapper = await mountDashboard([
      calendar('z', 'Zebra'),
      calendar('b', 'Bravo'),
      calendar('a', 'Alpha'),
    ]);
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'z']);

    await setSort(wrapper, 'custom');
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'z']);
  });

  it('seeds the custom order from the visible descending order', async () => {
    const wrapper = await mountDashboard([
      calendar('a', 'Alpha'),
      calendar('b', 'Bravo'),
      calendar('z', 'Zebra'),
    ]);
    await setSort(wrapper, 'name-desc');
    expect(renderedIds(wrapper)).toEqual(['z', 'b', 'a']);

    // Switching to custom order must freeze what is on screen, descending included.
    await setSort(wrapper, 'custom');
    expect(renderedIds(wrapper)).toEqual(['z', 'b', 'a']);
  });

  it('does not pre-seed the manual order while alphabetical sorting is active', async () => {
    await mountDashboard([calendar('a', 'Alpha'), calendar('b', 'Bravo')]);

    // The page only ever used alphabetical sorting, so no manual order exists yet.
    const store = useDashboardStore();
    expect(store.sortMode).toBe('alphabetical');
    expect(store.customOrder).toEqual([]);
  });

  it('keeps pinned calendars above the rest in custom order', async () => {
    const wrapper = await mountDashboard([
      calendar('a', 'Alpha'),
      calendar('b', 'Bravo'),
      calendar('c', 'Charlie'),
    ]);
    await setSort(wrapper, 'custom');

    const byId = (id: string) =>
      wrapper.findAllComponents(CalendarCard).find(card => card.props('calendar').id === id)!;
    await byId('b').find('button[title="Pin to top"]').trigger('click');
    await nextTick();

    // "Bravo" is pinned, so it leads even though custom order had it second.
    expect(renderedIds(wrapper)).toEqual(['b', 'a', 'c']);
  });

  it('opens the selected calendar in the participant flow', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')]);
    const card = wrapper.findAllComponents(CalendarCard)[0];
    // The open affordance is the calendar-name link (the card root is a plain,
    // non-interactive container around its controls).
    await card.find('a.card-open-area[aria-label="Open calendar Alpha"]').trigger('click');
    expect(routerPush).toHaveBeenCalledWith('/c/tok-a');
  });

  it('persists the chosen sort and view across reloads', async () => {
    const first = await mountDashboard([calendar('a', 'Alpha'), calendar('b', 'Bravo')]);
    await setSort(first, 'name-desc');
    await first.find('button[aria-label="Compact list"]').trigger('click');
    await nextTick();

    // A fresh page (new pinia) reads the same preferences back from localStorage.
    const store = useDashboardStore();
    expect(store.sortDirection).toBe('desc');
    expect(store.viewMode).toBe('compact');
  });

  it('keeps the persisted pins and order across a logout or failed fetch', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')]);
    await setSort(wrapper, 'custom');
    const byId = (id: string) =>
      wrapper.findAllComponents(CalendarCard).find(card => card.props('calendar').id === id)!;
    await byId('a').find('button[title="Pin to top"]').trigger('click');
    await nextTick();
    const dashboardStore = useDashboardStore();
    const calendarStore = useCalendarStore();
    const authStore = useAuthStore();
    expect(dashboardStore.pinnedIds).toEqual(['a']);
    const persisted = () => JSON.parse(localStorage.getItem('whento_dashboard_prefs:u-1') ?? '{}');

    // A failed fetch empties the list and its user marker while the account stays;
    // the dashboard watcher must not treat that as a deletion set.
    calendarStore.calendars = [];
    calendarStore.calendarsForUser = null;
    await nextTick();
    expect(dashboardStore.pinnedIds).toEqual(['a']);
    expect(persisted()).toMatchObject({ pinnedIds: ['a'] });

    // Logout clears the user; in-memory prefs drop to the anonymous session's (none),
    // but the account's saved pins and order are untouched in storage.
    authStore.user = null;
    await nextTick();
    expect(persisted()).toMatchObject({ pinnedIds: ['a'] });
    wrapper.unmount();
  });

  it('renders a unified-feed load failure inline instead of swallowing it', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')], () =>
      unifiedFeedApi.getConfig.mockRejectedValue(new Error('offline'))
    );

    expect(wrapper.text()).toContain('Failed to load the unified feed');
    wrapper.unmount();
  });

  it('toasts a unified-feed enable failure', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')], () =>
      unifiedFeedApi.getConfig.mockRejectedValue(new Error('offline'))
    );
    unifiedFeedApi.create.mockRejectedValue(new Error('offline'));
    const toastStore = useToastStore();

    const enable = wrapper.findAll('button').find(b => b.text() === 'Enable Unified Feed');
    expect(enable).toBeTruthy();
    await enable!.trigger('click');
    await flushPromises();

    expect(
      toastStore.toasts.some(
        t => t.type === 'error' && t.message === 'Failed to create the unified feed'
      )
    ).toBe(true);
    wrapper.unmount();
  });

  it('toasts a unified-feed toggle failure instead of an unhandled rejection', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')], () =>
      unifiedFeedApi.getConfig.mockResolvedValue({
        configured: true,
        ics_token: 't',
        included_calendar_ids: [],
      } as never)
    );
    unifiedFeedApi.updateCalendars.mockRejectedValue(new Error('offline'));
    const toastStore = useToastStore();

    await wrapper.find('input[type="checkbox"]').setValue(true);
    await flushPromises();

    expect(
      toastStore.toasts.some(
        t => t.type === 'error' && t.message === 'Failed to update the unified feed calendars'
      )
    ).toBe(true);
    wrapper.unmount();
  });
});
