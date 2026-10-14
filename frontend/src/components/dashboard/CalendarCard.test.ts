/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, describe, expect, it } from 'vitest';
import { mountWithI18n, lastEmit } from '@/test/harness';
import type { CalendarWithParticipants } from '@/types';
import CalendarCard from './CalendarCard.vue';

const mounted: { unmount(): void }[] = [];
function track<T extends { unmount(): void }>(wrapper: T): T {
  mounted.push(wrapper);
  return wrapper;
}
afterEach(() => {
  for (const wrapper of mounted.splice(0)) wrapper.unmount();
  document.body.innerHTML = '';
});

function calendar(overrides: Partial<CalendarWithParticipants> = {}): CalendarWithParticipants {
  return {
    id: 'c-1',
    name: 'Board games',
    public_token: 'tok',
    participants: [{ id: 'p-1', name: 'Ada' }],
    description: 'Every Friday, board games in the cellar.',
    ...overrides,
  } as CalendarWithParticipants;
}

function mountCard(props: Record<string, unknown> = {}) {
  return track(
    mountWithI18n(CalendarCard, {
      props: {
        calendar: calendar(),
        view: 'card',
        pinned: false,
        draggable: false,
        canMoveUp: true,
        canMoveDown: true,
        unifiedFeedConfigured: false,
        feedIncluded: false,
        ...props,
      },
    })
  );
}

/**
 * Focus assertions rely on jsdom's `document.activeElement`, which only updates for
 * elements actually in `<body>` — the wrapper must be attached there.
 */
function mountCardAttached(props: Record<string, unknown> = {}) {
  return track(
    mountWithI18n(CalendarCard, {
      attachTo: document.body,
      props: {
        calendar: calendar(),
        view: 'card',
        pinned: false,
        draggable: false,
        canMoveUp: true,
        canMoveDown: true,
        unifiedFeedConfigured: false,
        feedIncluded: false,
        ...props,
      },
    })
  );
}

type CardVm = {
  focusMoveControl(direction: 'up' | 'down' | 'auto'): boolean;
  focusOpen(): boolean;
};

function vmOf(wrapper: ReturnType<typeof mountCard>): CardVm {
  return wrapper.vm as unknown as CardVm;
}

/** The currently focused element, with stable markers, or null. */
function focused() {
  const el = document.activeElement as HTMLElement | null;
  if (el === document.body) return null;
  return {
    tag: el?.tagName,
    action: el?.getAttribute('data-action'),
    calendar: el?.closest('[data-calendar-id]')?.getAttribute('data-calendar-id'),
  };
}

describe('CalendarCard', () => {
  it('renders the calendar name and participant count', () => {
    const wrapper = mountCard();
    expect(wrapper.text()).toContain('Board games');
    expect(wrapper.text()).toContain('1');
  });

  it('is a non-interactive card whose name is the single genuine open link (valid a11y tree)', () => {
    const wrapper = mountCard();
    // The root must not claim interactive semantics around its focusable controls.
    expect(wrapper.attributes('role')).toBeUndefined();
    expect(wrapper.attributes('tabindex')).toBeUndefined();

    const openLink = wrapper.find('a.card-open-area');
    expect(openLink.exists()).toBe(true);
    expect(openLink.attributes('href')).toBe('/c/tok');
    expect(openLink.text()).toBe('Board games');
    // Exactly one accessible "Open calendar" target per card: the name link itself.
    expect(wrapper.findAll('[aria-label="Open calendar Board games"]')).toHaveLength(1);
  });

  it('opens the calendar from the accessible name link', async () => {
    const wrapper = mountCard();
    const openLink = wrapper.find('a.card-open-area');

    // A plain click is routed through the open event; the Dashboard navigates in-place.
    await openLink.trigger('click');
    expect(wrapper.emitted('open')).toHaveLength(1);
  });

  it('opens the calendar from the whole card surface via the stretched name link', async () => {
    const wrapper = mountCard();
    // The name link carries a real href and a pseudo-element stretch (see style.css),
    // so pointer users open a calendar by clicking anywhere on the card surface. It
    // stays a normal focusable link — the keyboard open control — with no tabindex
    // removal and no duplicate sibling control in the tree.
    const surface = wrapper.find('a.card-open-area');
    expect(surface.attributes('href')).toBe('/c/tok');
    expect(surface.attributes('tabindex')).toBeUndefined();
    expect(wrapper.findAll('[aria-label="Open calendar Board games"]')).toHaveLength(1);

    await surface.trigger('click');
    expect(lastEmit(wrapper, 'open')).toBeTruthy();
  });

  it('leaves modified clicks to the browser instead of swallowing them', async () => {
    // Ctrl/Cmd/Shift/middle-click is the native "open in a new tab" gesture against the
    // link's real href: the component must not prevent it, and must not emit either,
    // because the navigation belongs to the browser.
    const wrapper = mountCard();
    const openLink = wrapper.find('a.card-open-area');

    await openLink.trigger('click', { metaKey: true });
    expect(wrapper.emitted('open')).toBeUndefined();

    await openLink.trigger('click', { ctrlKey: true });
    expect(wrapper.emitted('open')).toBeUndefined();
  });

  it('does not open when a control inside is activated', async () => {
    const wrapper = mountCard();
    await wrapper.find('button[title="Settings"]').trigger('click');
    expect(wrapper.emitted('open')).toBeUndefined();
    expect(lastEmit(wrapper, 'settings')).toBeTruthy();
  });

  it('copies the link from the copy action', async () => {
    const wrapper = mountCard();
    await wrapper.find('button[title="Copy link"]').trigger('click');
    expect(wrapper.emitted('open')).toBeUndefined();
    expect(lastEmit(wrapper, 'copy-link')).toBeTruthy();
  });

  it('emits toggle-pin and reflects the pinned state', async () => {
    const wrapper = mountCard({ pinned: true });
    const pinButton = wrapper.find('button[title="Unpin"]');
    expect(pinButton.exists()).toBe(true);
    await pinButton.trigger('click');
    expect(lastEmit(wrapper, 'toggle-pin')).toBeTruthy();
  });

  it.each(['card', 'expanded', 'list', 'compact'])(
    'shows one labeled feed toggle in %s only when configured',
    async view => {
      const hidden = mountCard({ view });
      expect(hidden.find('input[type=checkbox]').exists()).toBe(false);

      const shown = mountCard({ view, unifiedFeedConfigured: true, feedIncluded: true });
      expect(shown.findAll('[data-action="feed"]')).toHaveLength(1);
      const input = shown.find('input[type=checkbox]');
      expect(shown.find('label').attributes('for')).toBe(input.attributes('id'));
      expect((input.element as HTMLInputElement).checked).toBe(true);
      await input.setValue(false);
      expect(lastEmit(shown, 'toggle-feed')).toBeTruthy();
      expect(shown.emitted('open')).toBeUndefined();
    }
  );

  it.each(['card', 'expanded', 'list', 'compact'])(
    'keeps the pointer drag grip out of the %s tab order',
    view => {
      const wrapper = mountCard({ view, draggable: true });
      const grip = wrapper.find('[data-action="drag"]');
      expect(grip.attributes('tabindex')).toBe('-1');
      expect(grip.attributes('aria-hidden')).toBe('true');
      expect(grip.attributes('draggable')).toBe('true');
      expect(wrapper.find('[data-action="move-up"]').attributes('aria-label')).toBeTruthy();
    }
  );

  it('emits drop-on with the calendar id', async () => {
    const wrapper = mountCard();
    await wrapper.find('article').trigger('drop');
    expect(lastEmit(wrapper, 'drop-on')).toEqual(['c-1']);
  });

  it('emits start-drag with the calendar id, but only from the handle', async () => {
    const wrapper = mountCard({ draggable: true });
    const handle = wrapper.find('.drag-grip');
    await handle.trigger('dragstart');
    expect(lastEmit(wrapper, 'start-drag')).toEqual(['c-1']);
  });

  it('makes the drag grip a real draggable source in custom order mode', () => {
    const wrapper = mountCard({ draggable: true });
    // A real browser only starts a drag on an element with the `draggable` attribute;
    // the JavaSript dragstart listener alone is not enough to begin a pointer gesture.
    expect(wrapper.find('.drag-grip').attributes('draggable')).toBe('true');
  });

  it('hides the reorder controls outside custom order mode', () => {
    const wrapper = mountCard({ draggable: false });
    expect(wrapper.find('.drag-grip').exists()).toBe(false);
    expect(wrapper.find('button[title="Move up"]').exists()).toBe(false);
    expect(wrapper.find('button[title="Move down"]').exists()).toBe(false);
  });

  it('emits move-up and move-down from the accessible move buttons', async () => {
    const wrapper = mountCard({ draggable: true });
    await wrapper.find('button[title="Move up"]').trigger('click');
    expect(wrapper.emitted('open')).toBeUndefined();
    expect(lastEmit(wrapper, 'move-up')).toBeTruthy();

    await wrapper.find('button[title="Move down"]').trigger('click');
    expect(lastEmit(wrapper, 'move-down')).toBeTruthy();
  });

  it('disables the move button at the edge of its reorder group', async () => {
    // canMoveUp=false means this is the first of its group: Moving up is impossible.
    const wrapper = mountCard({ draggable: true, canMoveUp: false, canMoveDown: true });
    const up = wrapper.find('button[title="Move up"]');
    expect((up.element as HTMLButtonElement).disabled).toBe(true);
    const down = wrapper.find('button[title="Move down"]');
    expect((down.element as HTMLButtonElement).disabled).toBe(false);

    await up.trigger('click');
    expect(wrapper.emitted('move-up')).toBeUndefined();
  });

  it('renders a compact row without the description', () => {
    const wrapper = mountCard({ view: 'compact' });
    expect(wrapper.text()).toContain('Board games');
    expect(wrapper.text()).not.toContain('Every Friday');
  });

  it('renders an expanded row with the description', () => {
    const wrapper = mountCard({ view: 'expanded' });
    expect(wrapper.text()).toContain('Every Friday');
  });

  // ---------------------------------------------------------------------------
  // focusMoveControl: the S3 keyboard-accessibility regression. After a move the
  // moved card's control must receive focus (never BODY). A programmatic focus in
  // jsdom moves document.activeElement, so these assert exactly that.
  // ---------------------------------------------------------------------------

  it('focuses the asked-for move control when it is enabled (middle of a group)', () => {
    const wrapper = mountCardAttached({ draggable: true });
    expect(vmOf(wrapper).focusMoveControl('up')).toBe(true);
    expect(focused()).toEqual({ tag: 'BUTTON', action: 'move-up', calendar: 'c-1' });
    expect((document.activeElement as HTMLButtonElement).disabled).toBe(false);
  });

  it('focuses the opposite enabled control at a group edge', () => {
    // canMoveUp=false: moving up is dead, so focus falls through to "move down".
    const wrapper = mountCardAttached({ draggable: true, canMoveUp: false, canMoveDown: true });
    expect(vmOf(wrapper).focusMoveControl('up')).toBe(true);
    expect(focused()).toEqual({ tag: 'BUTTON', action: 'move-down', calendar: 'c-1' });
  });

  it('falls back to the calendar link when neither move control is enabled', () => {
    // A single-calendar group has no move at all.
    const wrapper = mountCardAttached({ draggable: true, canMoveUp: false, canMoveDown: false });
    expect(vmOf(wrapper).focusMoveControl('up')).toBe(true);
    expect(focused()).toEqual({ tag: 'A', action: 'open', calendar: 'c-1' });
  });

  it('prefers an enabled control for the direction-agnostic auto focus', () => {
    const wrapper = mountCardAttached({ draggable: true, canMoveUp: false, canMoveDown: true });
    expect(vmOf(wrapper).focusMoveControl('auto')).toBe(true);
    expect(focused()).toEqual({ tag: 'BUTTON', action: 'move-down', calendar: 'c-1' });
  });

  it('focuses the current view controls, never a hidden sibling layout', () => {
    // Card view is the only rendered branch; compact/list/expanded controls do not
    // exist in the DOM, so there is exactly one "move-up" button to land on.
    const wrapper = mountCardAttached({ draggable: true, view: 'card' });
    expect(wrapper.findAll('[data-action="move-up"]')).toHaveLength(1);
    expect(vmOf(wrapper).focusMoveControl('up')).toBe(true);
    expect(focused()).toEqual({ tag: 'BUTTON', action: 'move-up', calendar: 'c-1' });
  });

  it('focuses the newly mounted view after a view switch, not the unmounted one', async () => {
    // Every view shares one ref per role, so a switch must leave it on the new element.
    const wrapper = mountCardAttached({ draggable: true, view: 'card' });
    for (const view of ['expanded', 'list', 'card', 'compact']) {
      await wrapper.setProps({ view });
      expect(vmOf(wrapper).focusMoveControl('down')).toBe(true);
      expect(focused()).toEqual({ tag: 'BUTTON', action: 'move-down', calendar: 'c-1' });
      expect(wrapper.element.contains(document.activeElement)).toBe(true);
    }
  });

  it('focuses the row controls in compact view', () => {
    const wrapper = mountCardAttached({ draggable: true, view: 'compact' });
    expect(vmOf(wrapper).focusMoveControl('down')).toBe(true);
    expect(focused()).toEqual({ tag: 'BUTTON', action: 'move-down', calendar: 'c-1' });
  });

  it('focuses the row controls in list view and never BODY', () => {
    const wrapper = mountCardAttached({ draggable: true, view: 'list' });
    const link = wrapper.find('a[data-action="open"]').element as HTMLAnchorElement;
    link.focus();
    expect(document.activeElement).toBe(link);
    expect(vmOf(wrapper).focusMoveControl('up')).toBe(true);
    expect(focused()).toEqual({ tag: 'BUTTON', action: 'move-up', calendar: 'c-1' });
  });

  it('keeps a visible focus ring on the focused control', () => {
    const wrapper = mountCardAttached({ draggable: true });
    vmOf(wrapper).focusMoveControl('up');
    const button = document.activeElement as HTMLElement;
    // Either the browser reports :focus-visible natively (jsdom does), or the
    // component's fallback class that mirrors the ring is applied.
    const viaFallback = button.classList.contains('focus-visible-fallback');
    expect(viaFallback || button.matches(':focus-visible')).toBe(true);

    // The ring does not stay painted on a blurred control: the fallback class is
    // removed with focus, and a native focus-visible ring leaves with focus too.
    if (viaFallback) {
      button.dispatchEvent(new Event('blur'));
      expect(button.classList.contains('focus-visible-fallback')).toBe(false);
    } else {
      button.blur();
      expect(document.activeElement).toBe(document.body);
    }
  });

  it('focuses the open link via focusOpen()', () => {
    const wrapper = mountCardAttached({ draggable: true });
    expect(vmOf(wrapper).focusOpen()).toBe(true);
    expect(focused()).toEqual({ tag: 'A', action: 'open', calendar: 'c-1' });
  });
});
