/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Keyboard navigation for the calendar grids.
 *
 * The grids were entirely unreachable by keyboard: cells were `<div>`s, and the project
 * contained no `role`, no `tabindex` and no arrow-key handling anywhere. This adds the
 * standard grid pattern — one cell in the tab order, arrows to move, Space or Enter to
 * act, Shift+arrows to select a range — using the same commit path as the pointer drag,
 * so the two cannot diverge.
 */

import { computed, ref, watch, type ComputedRef, type Ref } from 'vue';
import { useEventListener, useMediaQuery } from '@vueuse/core';

export interface GridNavOptions {
  readonly container: Ref<HTMLElement | null>;
  /** Number of items in the grid. */
  readonly count: ComputedRef<number>;
  /** Whether an index can receive focus at all. */
  isFocusable(index: number): boolean;
  /** Space or Enter on a single cell. */
  activate(index: number): void;
  /** Shift+arrow released, or Space with a range extended. */
  commitRange(anchorIndex: number, focusIndex: number): void;
  /** Left/right at the edge of the grid, and PageUp/PageDown. */
  shiftPeriod(delta: number): void;
  /** Media query for the transposed compact layout. */
  readonly compactQuery?: string;
}

export interface GridNav {
  /** Where focus last landed, or -1 before the grid has been entered. */
  readonly focusedIndex: Ref<number>;
  /**
   * The one cell in the tab order. Falls back to the first focusable cell so the grid
   * is reachable by Tab before it has ever been focused — without it the roving
   * tabindex leaves every cell at -1 and the grid stays unreachable.
   */
  readonly tabStopIndex: ComputedRef<number>;
  /** Indices covered by an in-progress Shift+arrow selection. */
  readonly rangeIndices: ComputedRef<ReadonlySet<number>>;
  onKeydown(event: KeyboardEvent): void;
  focus(index: number): void;
}

const COMPACT_QUERY = '(max-width: 48rem)';

export function useCalendarGridNav(options: GridNavOptions): GridNav {
  const focusedIndex = ref(-1);
  const anchorIndex = ref<number | null>(null);

  // The compact layout transposes the grid, so "right" moves by one week there and by
  // one day in the wide layout. Reading the media query keeps the arrows matching what
  // the user actually sees.
  const isCompact = useMediaQuery(options.compactQuery ?? COMPACT_QUERY);
  const horizontalStep = computed(() => (isCompact.value ? 7 : 1));
  const verticalStep = computed(() => (isCompact.value ? 1 : 7));

  const rangeIndices = computed<ReadonlySet<number>>(() => {
    if (anchorIndex.value === null || focusedIndex.value < 0) return new Set<number>();
    const from = Math.min(anchorIndex.value, focusedIndex.value);
    const to = Math.max(anchorIndex.value, focusedIndex.value);
    const indices = new Set<number>();
    for (let index = from; index <= to; index++) {
      if (options.isFocusable(index)) indices.add(index);
    }
    return indices;
  });

  /** First focusable index at or after `from`, walking in `direction`. */
  function seek(from: number, direction: number): number {
    for (let index = from; index >= 0 && index < options.count.value; index += direction || 1) {
      if (options.isFocusable(index)) return index;
      if (direction === 0) break;
    }
    return -1;
  }

  function focus(index: number) {
    if (index < 0 || index >= options.count.value) return;
    focusedIndex.value = index;
    const cell = options.container.value?.querySelectorAll<HTMLElement>('[role="gridcell"]')[index];
    cell?.focus();
  }

  /** Move by `delta`, skipping unfocusable cells, and paging when we run off the end. */
  function move(delta: number, extend: boolean) {
    const current = focusedIndex.value < 0 ? seek(0, 1) : focusedIndex.value;
    const target = current + delta;

    if (target < 0 || target >= options.count.value) {
      if (!extend) options.shiftPeriod(delta > 0 ? 1 : -1);
      return;
    }

    const next = seek(target, delta > 0 ? 1 : -1);
    if (next < 0) return;

    if (extend && anchorIndex.value === null) anchorIndex.value = current;
    if (!extend) anchorIndex.value = null;

    focus(next);
  }

  function onKeydown(event: KeyboardEvent) {
    const extend = event.shiftKey;

    switch (event.key) {
      case 'ArrowRight':
        move(horizontalStep.value, extend);
        break;
      case 'ArrowLeft':
        move(-horizontalStep.value, extend);
        break;
      case 'ArrowDown':
        move(verticalStep.value, extend);
        break;
      case 'ArrowUp':
        move(-verticalStep.value, extend);
        break;
      case 'Home':
        focus(seek(focusedIndex.value - (focusedIndex.value % 7), 1));
        break;
      case 'End':
        focus(seek(focusedIndex.value - (focusedIndex.value % 7) + 6, -1));
        break;
      case 'PageDown':
        options.shiftPeriod(1);
        break;
      case 'PageUp':
        options.shiftPeriod(-1);
        break;
      case ' ':
      case 'Enter':
        if (focusedIndex.value < 0) return;
        if (anchorIndex.value !== null && anchorIndex.value !== focusedIndex.value) {
          options.commitRange(anchorIndex.value, focusedIndex.value);
        } else {
          options.activate(focusedIndex.value);
        }
        anchorIndex.value = null;
        break;
      default:
        return;
    }

    event.preventDefault();
  }

  // Releasing Shift ends a range selection without committing it, matching how letting
  // go of the pointer outside the grid abandons a drag.
  useEventListener(window, 'keyup', (event: KeyboardEvent) => {
    if (event.key === 'Shift' && anchorIndex.value === focusedIndex.value) {
      anchorIndex.value = null;
    }
  });

  // The grid re-renders on every navigation; keep the tab stop on a real cell.
  watch(options.count, () => {
    if (focusedIndex.value >= 0 && !options.isFocusable(focusedIndex.value)) {
      focusedIndex.value = seek(0, 1);
    }
  });

  const tabStopIndex = computed(() =>
    focusedIndex.value >= 0 && options.isFocusable(focusedIndex.value)
      ? focusedIndex.value
      : seek(0, 1)
  );

  return { focusedIndex, tabStopIndex, rangeIndices, onKeydown, focus };
}
