/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { computed, effectScope, ref, type EffectScope } from 'vue';
import { useCalendarGridNav, type GridNav } from './useCalendarGridNav';

/**
 * jsdom does not implement matchMedia, which @vueuse/core's useMediaQuery needs. The
 * stub below lets each test choose the layout, since the arrow steps depend on it: the
 * wide grid moves one day horizontally and a week vertically, the compact one is
 * transposed and moves the other way round.
 */
function stubMatchMedia(matches: boolean) {
  window.matchMedia = ((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

interface Harness {
  readonly nav: GridNav;
  readonly activated: number[];
  readonly ranges: [number, number][];
  readonly periods: number[];
  readonly scope: EffectScope;
}

/**
 * A 35-cell grid, five rows of seven, matching a month view.
 *
 * `blocked` marks indices that cannot take focus — the filler days of an adjacent
 * month, or days the calendar has closed.
 */
function setup(options: { blocked?: number[]; count?: number; compact?: boolean } = {}): Harness {
  stubMatchMedia(options.compact ?? false);

  const blocked = new Set(options.blocked ?? []);
  const count = options.count ?? 35;

  const container = document.createElement('div');
  for (let index = 0; index < count; index++) {
    const cell = document.createElement('div');
    cell.setAttribute('role', 'gridcell');
    cell.tabIndex = -1;
    container.appendChild(cell);
  }
  document.body.appendChild(container);

  const activated: number[] = [];
  const ranges: [number, number][] = [];
  const periods: number[] = [];
  const scope = effectScope();

  const nav = scope.run(() =>
    useCalendarGridNav({
      container: ref(container),
      count: computed(() => count),
      isFocusable: index => index >= 0 && index < count && !blocked.has(index),
      activate: index => activated.push(index),
      commitRange: (anchor, focus) => ranges.push([anchor, focus]),
      shiftPeriod: delta => periods.push(delta),
    })
  ) as GridNav;

  return { nav, activated, ranges, periods, scope };
}

function press(nav: GridNav, key: string, shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key, shiftKey, bubbles: true, cancelable: true });
  nav.onKeydown(event);
  return event;
}

describe('useCalendarGridNav', () => {
  let harness: Harness | null = null;

  beforeEach(() => {
    document.body.innerHTML = '';
  });

  afterEach(() => {
    harness?.scope.stop();
    harness = null;
    vi.restoreAllMocks();
  });

  describe('tabStopIndex', () => {
    it('points at the first focusable cell before the grid is entered', () => {
      // Without this the roving tabindex leaves every cell at -1 and the grid cannot
      // be reached by Tab at all.
      harness = setup({ blocked: [0, 1, 2] });

      expect(harness.nav.focusedIndex.value).toBe(-1);
      expect(harness.nav.tabStopIndex.value).toBe(3);
    });

    it('follows the focused cell once the grid has been entered', () => {
      harness = setup();

      harness.nav.focus(10);

      expect(harness.nav.tabStopIndex.value).toBe(10);
    });

    it('falls back when the focused cell stops being focusable', () => {
      harness = setup({ blocked: [5] });

      harness.nav.focus(5);
      // focus() itself does not filter, so the index is set but unfocusable; the tab
      // stop must not follow it off the grid.
      expect(harness.nav.tabStopIndex.value).toBe(0);
    });

    it('is -1 when nothing in the grid can be focused', () => {
      harness = setup({ count: 3, blocked: [0, 1, 2] });

      expect(harness.nav.tabStopIndex.value).toBe(-1);
    });
  });

  describe('arrow movement in the wide layout', () => {
    it('moves one day horizontally and one week vertically', () => {
      harness = setup();

      harness.nav.focus(15);

      press(harness.nav, 'ArrowRight');
      expect(harness.nav.focusedIndex.value).toBe(16);

      press(harness.nav, 'ArrowLeft');
      expect(harness.nav.focusedIndex.value).toBe(15);

      press(harness.nav, 'ArrowDown');
      expect(harness.nav.focusedIndex.value).toBe(22);

      press(harness.nav, 'ArrowUp');
      expect(harness.nav.focusedIndex.value).toBe(15);
    });

    it('enters at the first focusable cell when nothing is focused yet', () => {
      harness = setup({ blocked: [0, 1] });

      press(harness.nav, 'ArrowRight');

      // Starts from index 2, then moves one right.
      expect(harness.nav.focusedIndex.value).toBe(3);
    });

    it('skips over cells that cannot take focus', () => {
      harness = setup({ blocked: [16, 17] });

      harness.nav.focus(15);
      press(harness.nav, 'ArrowRight');

      expect(harness.nav.focusedIndex.value).toBe(18);
    });

    it('skips backwards too', () => {
      harness = setup({ blocked: [13, 14] });

      harness.nav.focus(15);
      press(harness.nav, 'ArrowLeft');

      expect(harness.nav.focusedIndex.value).toBe(12);
    });

    it('stays put when every cell in that direction is blocked', () => {
      harness = setup({ count: 7, blocked: [4, 5, 6] });

      harness.nav.focus(3);
      press(harness.nav, 'ArrowRight');

      expect(harness.nav.focusedIndex.value).toBe(3);
    });
  });

  describe('arrow movement in the compact layout', () => {
    it('transposes the steps', () => {
      // The compact grid is rotated, so horizontal movement crosses weeks.
      harness = setup({ compact: true });

      harness.nav.focus(8);

      press(harness.nav, 'ArrowRight');
      expect(harness.nav.focusedIndex.value).toBe(15);

      press(harness.nav, 'ArrowDown');
      expect(harness.nav.focusedIndex.value).toBe(16);
    });
  });

  describe('paging at the edges', () => {
    it('moves to the next period when running off the end', () => {
      harness = setup();

      harness.nav.focus(34);
      press(harness.nav, 'ArrowRight');

      expect(harness.periods).toEqual([1]);
      expect(harness.nav.focusedIndex.value).toBe(34);
    });

    it('moves to the previous period when running off the start', () => {
      harness = setup();

      harness.nav.focus(0);
      press(harness.nav, 'ArrowLeft');

      expect(harness.periods).toEqual([-1]);
    });

    it('pages vertically as well', () => {
      harness = setup();

      harness.nav.focus(30);
      press(harness.nav, 'ArrowDown');

      expect(harness.periods).toEqual([1]);
    });

    it('does not page while extending a selection', () => {
      // Shift+arrow at the boundary must not navigate away and abandon the range.
      harness = setup();

      harness.nav.focus(34);
      press(harness.nav, 'ArrowRight', true);

      expect(harness.periods).toEqual([]);
    });

    it('pages on PageDown and PageUp regardless of position', () => {
      harness = setup();

      harness.nav.focus(15);
      press(harness.nav, 'PageDown');
      press(harness.nav, 'PageUp');

      expect(harness.periods).toEqual([1, -1]);
    });
  });

  describe('Home and End', () => {
    it('go to the ends of the current row', () => {
      harness = setup();

      harness.nav.focus(17); // row 2, column 3
      press(harness.nav, 'Home');
      expect(harness.nav.focusedIndex.value).toBe(14);

      press(harness.nav, 'End');
      expect(harness.nav.focusedIndex.value).toBe(20);
    });

    it('skip blocked cells at the row edges', () => {
      harness = setup({ blocked: [14, 20] });

      harness.nav.focus(17);
      press(harness.nav, 'Home');
      expect(harness.nav.focusedIndex.value).toBe(15);

      press(harness.nav, 'End');
      expect(harness.nav.focusedIndex.value).toBe(19);
    });

    /**
     * Home and End divide by a hard-coded 7 rather than by the layout's horizontal
     * step. In the wide grid that is the row, which is right. In the compact grid a
     * "row" is a column and the step is 7, so the same arithmetic no longer describes
     * the visual row — Home lands on whatever index happens to share a residue.
     *
     * Pinned as documentation rather than fixed: correcting it means deciding what
     * Home should mean in a transposed grid, which is a design question.
     */
    it('use the same arithmetic in the compact layout, where it no longer means the row', () => {
      harness = setup({ compact: true });

      harness.nav.focus(17);
      press(harness.nav, 'Home');

      expect(harness.nav.focusedIndex.value).toBe(14);
    });
  });

  describe('activation', () => {
    it('activates the focused cell on Enter and Space', () => {
      harness = setup();

      harness.nav.focus(12);
      press(harness.nav, 'Enter');
      press(harness.nav, ' ');

      expect(harness.activated).toEqual([12, 12]);
    });

    it('does nothing when no cell is focused', () => {
      harness = setup();

      press(harness.nav, 'Enter');

      expect(harness.activated).toEqual([]);
      expect(harness.ranges).toEqual([]);
    });

    it('commits a range instead of a single cell when one is extended', () => {
      harness = setup();

      harness.nav.focus(10);
      press(harness.nav, 'ArrowRight', true);
      press(harness.nav, 'ArrowRight', true);
      press(harness.nav, 'Enter');

      expect(harness.ranges).toEqual([[10, 12]]);
      expect(harness.activated).toEqual([]);
    });

    it('clears the range after committing it', () => {
      harness = setup();

      harness.nav.focus(10);
      press(harness.nav, 'ArrowRight', true);
      press(harness.nav, 'Enter');
      press(harness.nav, 'Enter');

      expect(harness.ranges).toHaveLength(1);
      // The second Enter is a plain activation again.
      expect(harness.activated).toEqual([11]);
    });
  });

  describe('range selection', () => {
    it('reports every focusable index between the anchor and the focus', () => {
      harness = setup();

      harness.nav.focus(10);
      press(harness.nav, 'ArrowRight', true);
      press(harness.nav, 'ArrowRight', true);

      expect([...harness.nav.rangeIndices.value].sort((a, b) => a - b)).toEqual([10, 11, 12]);
    });

    it('excludes blocked cells from inside the range', () => {
      harness = setup({ blocked: [11] });

      harness.nav.focus(10);
      press(harness.nav, 'ArrowRight', true);

      expect([...harness.nav.rangeIndices.value].sort((a, b) => a - b)).toEqual([10, 12]);
    });

    it('works backwards from the anchor', () => {
      harness = setup();

      harness.nav.focus(12);
      press(harness.nav, 'ArrowLeft', true);
      press(harness.nav, 'ArrowLeft', true);

      expect([...harness.nav.rangeIndices.value].sort((a, b) => a - b)).toEqual([10, 11, 12]);
    });

    it('is empty without an anchor', () => {
      harness = setup();

      harness.nav.focus(10);

      expect(harness.nav.rangeIndices.value.size).toBe(0);
    });

    it('drops the anchor when an arrow is pressed without Shift', () => {
      harness = setup();

      harness.nav.focus(10);
      press(harness.nav, 'ArrowRight', true);
      press(harness.nav, 'ArrowRight');

      expect(harness.nav.rangeIndices.value.size).toBe(0);
    });
  });

  describe('event handling', () => {
    it('prevents the default action for keys it handles', () => {
      harness = setup();

      harness.nav.focus(10);
      const handled = press(harness.nav, 'ArrowRight');

      expect(handled.defaultPrevented).toBe(true);
    });

    it('leaves other keys alone', () => {
      harness = setup();

      harness.nav.focus(10);
      const ignored = press(harness.nav, 'a');

      expect(ignored.defaultPrevented).toBe(false);
      expect(harness.nav.focusedIndex.value).toBe(10);
    });
  });

  describe('focus', () => {
    it('moves DOM focus to the matching cell', () => {
      harness = setup();

      harness.nav.focus(7);

      const cells = document.querySelectorAll('[role="gridcell"]');
      expect(document.activeElement).toBe(cells[7]);
    });

    it('ignores an index outside the grid', () => {
      harness = setup();

      harness.nav.focus(10);
      harness.nav.focus(-1);
      harness.nav.focus(999);

      expect(harness.nav.focusedIndex.value).toBe(10);
    });
  });
});
