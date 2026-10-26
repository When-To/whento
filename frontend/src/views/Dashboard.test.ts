/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, disposePinia, setActivePinia } from 'pinia';
import { enableAutoUnmount, flushPromises } from '@vue/test-utils';
import { nextTick } from 'vue';
import { bumpAccountGeneration } from '@/accountFence';
import type { CalendarWithParticipants, User } from '@/types';

import { mountWithI18n } from '@/test/harness';

enableAutoUnmount(afterEach);

const routerPush = vi.fn();

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush, currentRoute: { value: { meta: {} } } }),
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
  completeAccountRecovery: vi.fn(),
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
const mountedPinia: ReturnType<typeof createPinia>[] = [];
afterEach(() => {
  for (const pinia of mountedPinia.splice(0)) disposePinia(pinia);
});

async function mountDashboard(
  list: CalendarWithParticipants[],
  configureFeed: () => void = () =>
    unifiedFeedApi.getConfig.mockResolvedValue({ configured: false }),
  options: { user?: User | null; attach?: boolean } = {}
) {
  const pinia = createPinia();
  mountedPinia.push(pinia);
  setActivePinia(pinia);
  calendarsApi.getAll.mockResolvedValue(list);
  configureFeed();

  const authStore = useAuthStore();
  authStore.user = options.user === undefined ? (USER as User) : options.user;

  const wrapper = await mountWithI18n(Dashboard, {
    attachTo: options.attach ? document.body : undefined,
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

function byId(wrapper: Awaited<ReturnType<typeof mountDashboard>>, id: string) {
  return wrapper.findAllComponents(CalendarCard).find(card => card.props('calendar').id === id)!;
}

/**
 * The focus after a reorder, described by stable markers. Returns null when focus is
 * lost to BODY — the S3 regression this suite guards against.
 */
function focused() {
  const el = document.activeElement as HTMLElement | null;
  if (!el || el === document.body) return null;
  return {
    tag: el.tagName,
    action: el.getAttribute('data-action'),
    calendar: el.closest('[data-calendar-id]')?.getAttribute('data-calendar-id') ?? null,
    disabled: el instanceof HTMLButtonElement && el.disabled,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  document.body.innerHTML = '';
});

describe('Dashboard.vue — my calendars', () => {
  it('reloads each account-scoped resource exactly once after a replacement', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')]);
    calendarsApi.getAll.mockClear();
    unifiedFeedApi.getConfig.mockClear();

    bumpAccountGeneration();
    await flushPromises();

    expect(calendarsApi.getAll).toHaveBeenCalledTimes(1);
    expect(unifiedFeedApi.getConfig).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });

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
    const zebraCard = byId(wrapper, 'z');
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
    await byId(wrapper, 'z').find('button[title="Pin to top"]').trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['z', 'a', 'b']);

    // "Zebra" is at the bottom of the pinned group: move down is dead.
    const zebraDown = byId(wrapper, 'z').find('button[title="Move down"]');
    expect((zebraDown.element as HTMLButtonElement).disabled).toBe(true);
    await zebraDown.trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['z', 'a', 'b']);

    // "Alpha" is the first unpinned calendar: move up is dead and cannot lift it
    // above the pinned group.
    const alphaUp = byId(wrapper, 'a').find('button[title="Move up"]');
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

    // The grip must be a genuine draggable source: a real browser only fires
    // `dragstart` on an element carrying the `draggable` attribute.
    const grip = byId(wrapper, 'a').find('.drag-grip');
    expect(grip.attributes('draggable')).toBe('true');

    // Drive the actual DOM path — dragstart on the source grip, drop on the target
    // card root — rather than synthesising the component's internal events.
    await grip.trigger('dragstart');
    await byId(wrapper, 'c').find('article').trigger('drop');
    await nextTick();

    // The dropped card takes the target's slot, the last one included.
    expect(renderedIds(wrapper)).toEqual(['b', 'c', 'a']);

    // Onto the adjacent card is a swap, not a no-op; upwards works the same way.
    await byId(wrapper, 'b').find('.drag-grip').trigger('dragstart');
    await byId(wrapper, 'c').find('article').trigger('drop');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['c', 'b', 'a']);

    await byId(wrapper, 'a').find('.drag-grip').trigger('dragstart');
    await byId(wrapper, 'c').find('article').trigger('drop');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'c', 'b']);
  });

  it('does not let drag-and-drop cross the pinned/unpinned boundary', async () => {
    const wrapper = await mountDashboard([
      calendar('a', 'Alpha'),
      calendar('b', 'Bravo'),
      calendar('c', 'Charlie'),
    ]);
    await setSort(wrapper, 'custom');

    // Pin "Alpha": it leads the custom order, and its group is the pinned one.
    await byId(wrapper, 'a').find('button[title="Pin to top"]').trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);
    const dashboardStore = useDashboardStore();
    const persistedBefore = [...dashboardStore.customOrder];

    // Dragging the pinned "Alpha" onto the unpinned "Bravo" is a cross-group move:
    // regrouped rendering would show no change, so it must not silently rewrite the
    // persisted sequence either.
    await byId(wrapper, 'a').find('.drag-grip').trigger('dragstart');
    await byId(wrapper, 'b').find('article').trigger('drop');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);
    expect(dashboardStore.customOrder).toEqual(persistedBefore);

    // And the reverse direction — unpinned "Bravo" onto pinned "Alpha" — is rejected
    // the same way.
    await byId(wrapper, 'b').find('.drag-grip').trigger('dragstart');
    await byId(wrapper, 'a').find('article').trigger('drop');
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

    // Move "Charlie" up one slot.
    await byId(wrapper, 'c').find('button[title="Move up"]').trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'c', 'b']);

    // And back down.
    await byId(wrapper, 'c').find('button[title="Move down"]').trigger('click');
    await nextTick();
    expect(renderedIds(wrapper)).toEqual(['a', 'b', 'c']);

    // Boundaries are no-ops: "Alpha" is already first.
    await byId(wrapper, 'a').find('button[title="Move up"]').trigger('click');
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

    await byId(wrapper, 'b').find('button[title="Pin to top"]').trigger('click');
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

    // A fresh page (new pinia) reads the same preferences back from localStorage,
    // stored under the account's own key.
    setActivePinia(createPinia());
    useAuthStore().user = USER;
    const reloaded = useDashboardStore();
    expect(reloaded.sortDirection).toBe('desc');
    expect(reloaded.viewMode).toBe('compact');
    expect(localStorage.getItem('dashboard:u-1:prefs')).toContain('"compact"');
  });

  it('keeps the persisted pins and order across a logout or failed fetch', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')]);
    await setSort(wrapper, 'custom');
    await byId(wrapper, 'a').find('button[title="Pin to top"]').trigger('click');
    await nextTick();
    const dashboardStore = useDashboardStore();
    const calendarStore = useCalendarStore();
    const authStore = useAuthStore();
    expect(dashboardStore.pinnedIds).toEqual(['a']);
    const persisted = () =>
      JSON.parse(localStorage.getItem('dashboard:u-1:prefs') ?? '{}') as { pinnedIds?: unknown };

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

  it('announces an account-reload feed failure inline without duplicating its toast', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')]);
    unifiedFeedApi.create.mockRejectedValue(new Error('offline'));
    await wrapper
      .findAll('button')
      .find(b => b.text() === 'Enable Unified Feed')!
      .trigger('click');
    await flushPromises();
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    useToastStore().toasts = [];
    unifiedFeedApi.getConfig.mockRejectedValue(new Error('offline'));
    bumpAccountGeneration();
    await flushPromises();
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(1);
    expect(wrapper.find('[role="alert"]').text()).toBe('Failed to load the unified feed');
    expect(useToastStore().toasts.filter(t => t.type === 'error')).toEqual([]);
    wrapper.unmount();
  });

  it('renders a unified-feed load failure inline instead of swallowing it', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')], () =>
      unifiedFeedApi.getConfig.mockRejectedValue(new Error('offline'))
    );

    expect(wrapper.text()).toContain('Failed to load the unified feed');
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(1);
    expect(useToastStore().toasts.filter(t => t.type === 'error')).toEqual([]);
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
    expect(toastStore.toasts.filter(t => t.type === 'error')).toHaveLength(1);
    expect(wrapper.find('[role="alert"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it('announces a feed toggle failure inline without an error toast or unhandled rejection', async () => {
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

    expect(wrapper.findAll('[role="alert"]')).toHaveLength(1);
    expect(wrapper.find('[role="alert"]').text()).toBe(
      'Failed to update the unified feed calendars'
    );
    expect(toastStore.toasts.filter(t => t.type === 'error')).toEqual([]);
    expect((wrapper.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(
      false
    );
    wrapper.unmount();
  });

  it('announces a token regeneration failure inline without an error toast', async () => {
    const wrapper = await mountDashboard([calendar('a', 'Alpha')], () =>
      unifiedFeedApi.getConfig.mockResolvedValue({
        configured: true,
        ics_token: 't',
        included_calendar_ids: [],
      } as never)
    );
    unifiedFeedApi.regenerateToken.mockRejectedValue(new Error('offline'));
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    try {
      await wrapper.find('button[title="Regenerate"]').trigger('click');
      await flushPromises();
      expect(wrapper.findAll('[role="alert"]')).toHaveLength(1);
      expect(wrapper.find('[role="alert"]').text()).toContain('Failed to regenerate');
      expect(useToastStore().toasts.filter(t => t.type === 'error')).toEqual([]);
    } finally {
      confirm.mockRestore();
      wrapper.unmount();
    }
  });
});

describe('Dashboard.vue — keyboard focus after reorder (S3 regression)', () => {
  async function mountFocusable() {
    return mountDashboard(
      [
        calendar('a', 'Alpha'),
        calendar('b', 'Bravo'),
        calendar('c', 'Charlie'),
        calendar('d', 'Delta'),
      ],
      undefined,
      { attach: true }
    );
  }

  it('keeps keyboard focus on the moved card move control after a middle move', async () => {
    const wrapper = await mountFocusable();
    await setSort(wrapper, 'custom');
    // Visible order: [a, b, c, d]. Move "Charlie" (index 2) up to index 1. Its
    // move-up control stays enabled, so focus must land back on it — not BODY.
    await byId(wrapper, 'c').find('button[data-action="move-up"]').trigger('click');
    await flushPromises();

    expect(renderedIds(wrapper)).toEqual(['a', 'c', 'b', 'd']);
    expect(focused()).toEqual({ tag: 'BUTTON', action: 'move-up', calendar: 'c', disabled: false });
  });

  it('focuses the opposite enabled control when a move lands on the group edge', async () => {
    const wrapper = await mountFocusable();
    await setSort(wrapper, 'custom');
    // Move "Bravo" up: it becomes the first of its group, where move-up is dead, so
    // focus must transfer to the opposite enabled control (move-down).
    await byId(wrapper, 'b').find('button[data-action="move-up"]').trigger('click');
    await flushPromises();

    expect(renderedIds(wrapper)).toEqual(['b', 'a', 'c', 'd']);
    expect(focused()).toEqual({
      tag: 'BUTTON',
      action: 'move-down',
      calendar: 'b',
      disabled: false,
    });
  });

  it('keeps focus across repeated moves', async () => {
    const wrapper = await mountFocusable();
    await setSort(wrapper, 'custom');

    // Move "Charlie" up twice: [a, c, b, d] then [c, a, b, d]. Each time the moved
    // card must carry focus; after the second move "Charlie" is first (move-up dead,
    // so focus lands on move-down).
    await byId(wrapper, 'c').find('button[data-action="move-up"]').trigger('click');
    await flushPromises();
    expect(focused()?.calendar).toBe('c');

    const again = byId(wrapper, 'c').find('button[data-action="move-up"]');
    await again.trigger('click');
    await flushPromises();
    expect(renderedIds(wrapper)).toEqual(['c', 'a', 'b', 'd']);
    expect(focused()).toEqual({
      tag: 'BUTTON',
      action: 'move-down',
      calendar: 'c',
      disabled: false,
    });
  });

  it('holds focus after a drag-and-drop reorder too', async () => {
    const wrapper = await mountFocusable();
    await setSort(wrapper, 'custom');
    await byId(wrapper, 'a').find('.drag-grip').trigger('dragstart');
    await byId(wrapper, 'c').find('article').trigger('drop');
    await flushPromises();

    expect(renderedIds(wrapper)).toEqual(['b', 'c', 'a', 'd']);
    // The dragged calendar ("a") keeps focus on one of its enabled move controls.
    expect(focused()?.calendar).toBe('a');
    expect(focused()?.disabled).toBe(false);
  });

  it('does not lose focus in list, expanded, card or compact layouts', async () => {
    for (const view of ['list', 'expanded', 'card', 'compact']) {
      document.body.innerHTML = '';
      localStorage.clear();
      const wrapper = await mountFocusable();
      await wrapper.find(`button[data-action="view-${view}"]`).trigger('click');
      await setSort(wrapper, 'custom');

      await byId(wrapper, 'c').find('button[data-action="move-up"]').trigger('click');
      await flushPromises();

      expect(renderedIds(wrapper)).toEqual(['a', 'c', 'b', 'd']);
      const f = focused();
      expect(f, `focus lost to BODY in ${view} view`).not.toBeNull();
      expect(f).toEqual({ tag: 'BUTTON', action: 'move-up', calendar: 'c', disabled: false });
      wrapper.unmount();
    }
  });

  it('announces the new position in a polite live region', async () => {
    const wrapper = await mountFocusable();
    await setSort(wrapper, 'custom');
    await byId(wrapper, 'c').find('button[data-action="move-up"]').trigger('click');
    await flushPromises();

    const live = wrapper.find('#dashboard-move-live');
    expect(live.attributes('aria-live')).toBe('polite');
    expect(live.attributes('role')).toBe('status');
    expect(live.text()).toContain('Charlie');
    expect(live.text()).toContain('position 2');
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });
});

describe('Dashboard.vue — per-user and resilience', () => {
  it('never bleeds one account saved order into another, nor into a signed-out visitor', async () => {
    // Account u-1 has pinned "Bravo", custom order [b,a,c], compact view — persisted
    // under its own key before anyone visits.
    localStorage.setItem(
      'dashboard:u-1:prefs',
      JSON.stringify({
        sortMode: 'custom',
        sortDirection: 'asc',
        pinnedIds: ['b'],
        customOrder: ['b', 'a', 'c'],
        viewMode: 'compact',
      })
    );

    // Account u-2 on the same browser must see none of it.
    const wrapperB = await mountDashboard(
      [calendar('a', 'Alpha'), calendar('b', 'Bravo'), calendar('c', 'Charlie')],
      undefined,
      { user: { id: 'u-2', display_name: 'Other' } as User }
    );
    expect(renderedIds(wrapperB)).toEqual(['a', 'b', 'c']);
    expect(wrapperB.text()).not.toContain('Pinned');
    expect(wrapperB.findAllComponents(CalendarCard)[0].props('view')).toBe('card');

    // And a signed-out visitor must not see it either.
    const wrapperAnon = await mountDashboard(
      [calendar('a', 'Alpha'), calendar('b', 'Bravo'), calendar('c', 'Charlie')],
      undefined,
      { user: null }
    );
    expect(renderedIds(wrapperAnon)).toEqual(['a', 'b', 'c']);
    expect(wrapperAnon.text()).not.toContain('Pinned');
    expect(useDashboardStore().pinnedIds).toEqual([]);
    wrapperB.unmount();
    wrapperAnon.unmount();
  });

  it('prunes stale order ids only against a successful authoritative load', async () => {
    // A stale pin for a deleted calendar sits in storage for u-1.
    localStorage.setItem(
      'dashboard:u-1:prefs',
      JSON.stringify({
        sortMode: 'custom',
        pinnedIds: ['ghost', 'b'],
        customOrder: ['ghost', 'b', 'a'],
      })
    );
    const wrapper = await mountDashboard([calendar('a', 'Alpha'), calendar('b', 'Bravo')]);

    // The successful load gave an authoritative list, so the stale id is pruned from
    // memory and persisted, while the live pin survives.
    const store = useDashboardStore();
    expect(store.pinnedIds).toEqual(['b']);
    expect(store.customOrder).toEqual(['b', 'a']);
    const saved = JSON.parse(localStorage.getItem('dashboard:u-1:prefs') ?? '{}');
    expect(saved.pinnedIds).toEqual(['b']);
    wrapper.unmount();
  });

  it('survives malformed persisted JSON at the view level', async () => {
    localStorage.setItem('dashboard:u-1:prefs', '{not valid json!!');
    const wrapper = await mountDashboard([calendar('a', 'Alpha')]);
    expect(renderedIds(wrapper)).toEqual(['a']);
    expect(useDashboardStore().pinnedIds).toEqual([]);
    wrapper.unmount();
  });

  it('keeps working when localStorage is denied at the view level', async () => {
    const getItem = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('denied', 'SecurityError');
    });
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('denied', 'SecurityError');
    });
    try {
      const wrapper = await mountDashboard([calendar('a', 'Alpha'), calendar('b', 'Bravo')]);
      await setSort(wrapper, 'custom');
      await byId(wrapper, 'b').find('button[data-action="move-up"]').trigger('click');
      await flushPromises();

      // The reorder still works in memory even though nothing can be persisted.
      expect(renderedIds(wrapper)).toEqual(['b', 'a']);
      wrapper.unmount();
    } finally {
      getItem.mockRestore();
      setItem.mockRestore();
    }
  });

  it('does not erase saved choices when a later fetch fails', async () => {
    localStorage.setItem(
      'dashboard:u-1:prefs',
      JSON.stringify({
        sortMode: 'custom',
        pinnedIds: ['a'],
        customOrder: ['a', 'b'],
        viewMode: 'card',
      })
    );
    const wrapper = await mountDashboard([calendar('a', 'Alpha'), calendar('b', 'Bravo')]);

    // The next load fails: the store empties the list and the dashboard must not
    // prune the saved pins/order away.
    useCalendarStore().calendars = [];
    useCalendarStore().calendarsForUser = null;
    await nextTick();

    const store = useDashboardStore();
    expect(store.pinnedIds).toEqual(['a']);
    expect(store.customOrder).toEqual(['a', 'b']);
    const saved = JSON.parse(localStorage.getItem('dashboard:u-1:prefs') ?? '{}');
    expect(saved.pinnedIds).toEqual(['a']);
    wrapper.unmount();
  });

  it('ignores missing/deleted calendars without unstable jumps while rendering', async () => {
    // customOrder references ids that are not in the list; rendering must be stable
    // (simply those that exist, in order), not churn.
    localStorage.setItem(
      'dashboard:u-1:prefs',
      JSON.stringify({ sortMode: 'custom', customOrder: ['x', 'a', 'y', 'b'] })
    );
    const wrapper = await mountDashboard([calendar('b', 'Bravo'), calendar('a', 'Alpha')]);
    expect(renderedIds(wrapper)).toEqual(['a', 'b']);
    wrapper.unmount();
  });
});
