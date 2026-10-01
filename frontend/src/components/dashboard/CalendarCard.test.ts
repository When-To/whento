/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { describe, expect, it } from 'vitest';
import { mountWithI18n, lastEmit } from '@/test/harness';
import type { CalendarWithParticipants } from '@/types';
import CalendarCard from './CalendarCard.vue';

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
  return mountWithI18n(CalendarCard, {
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
  });
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

  it('shows the feed toggle only when the unified feed is configured', async () => {
    const hidden = mountCard();
    expect(hidden.find('input[type=checkbox]').exists()).toBe(false);

    const shown = mountCard({ unifiedFeedConfigured: true });
    await shown.find('input[type=checkbox]').setValue(true);
    expect(lastEmit(shown, 'toggle-feed')).toBeTruthy();
  });

  it('emits drop-on with the calendar id', async () => {
    const wrapper = mountCard();
    await wrapper.trigger('drop');
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
});
