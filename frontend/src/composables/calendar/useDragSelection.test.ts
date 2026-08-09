/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, ref, type EffectScope } from 'vue';
import {
  useDragSelection,
  TOUCH_HOLD_MS,
  type DragCommit,
  type DragSelection,
} from './useDragSelection';

/**
 * A row of cells laid out left to right, each 10px wide, at y = 5.
 *
 * The composable resolves targets through `document.elementFromPoint`, which jsdom
 * does not implement, so the harness below stubs it against this synthetic geometry.
 * That keeps the tests about the state machine rather than about layout.
 */
interface Cell {
  readonly key: string;
  readonly index: number;
}

const CELL_WIDTH = 10;

interface Harness {
  readonly drag: DragSelection;
  readonly commits: DragCommit<Cell>[];
  readonly scope: EffectScope;
}

function setup(options: { disabled?: number[]; cellCount?: number } = {}): Harness {
  const disabled = new Set(options.disabled ?? []);
  const cellCount = options.cellCount ?? 7;

  const cells: Cell[] = Array.from({ length: cellCount }, (_, index) => ({
    key: `cell-${index}`,
    index,
  }));

  const container = document.createElement('div');
  document.body.appendChild(container);

  // One element per cell, tagged with its index so elementFromPoint can find it.
  const elements = cells.map(cell => {
    const element = document.createElement('div');
    element.dataset.index = String(cell.index);
    container.appendChild(element);
    return element;
  });

  // jsdom has no layout, so elementFromPoint does not exist at all — it cannot be
  // spied on, only defined. Restored in afterEach.
  (
    document as Document & { elementFromPoint: (x: number, y: number) => Element | null }
  ).elementFromPoint = (x: number) => elements[Math.floor(x / CELL_WIDTH)] ?? null;

  const commits: DragCommit<Cell>[] = [];
  const scope = effectScope();

  const drag = scope.run(() =>
    useDragSelection<Cell>({
      container: ref(container),
      resolve: element => {
        const raw = (element as HTMLElement).dataset?.index;
        return raw === undefined ? null : cells[Number(raw)];
      },
      canStart: cell => !disabled.has(cell.index),
      initialMode: () => 'add',
      rectangle: (anchor, focus) => {
        const min = Math.min(anchor.index, focus.index);
        const max = Math.max(anchor.index, focus.index);
        return cells.slice(min, max + 1);
      },
      commit: result => commits.push(result),
    })
  ) as DragSelection;

  return { drag, commits, scope };
}

/** x coordinate at the centre of a cell. */
function xOf(index: number): number {
  return index * CELL_WIDTH + CELL_WIDTH / 2;
}

function pointerDown(drag: DragSelection, index: number, pointerType = 'mouse') {
  drag.onPointerDown(
    new PointerEvent('pointerdown', {
      clientX: xOf(index),
      clientY: 5,
      button: 0,
      pointerType,
      bubbles: true,
    })
  );
}

function pointerMove(index: number) {
  document.querySelector('div')?.dispatchEvent(
    new PointerEvent('pointermove', {
      clientX: xOf(index),
      clientY: 5,
      bubbles: true,
    })
  );
}

function pointerUp() {
  window.dispatchEvent(new PointerEvent('pointerup', { bubbles: true }));
}

/** requestAnimationFrame is how pointermove updates are coalesced. */
async function flushFrame() {
  await new Promise(resolve => requestAnimationFrame(() => resolve(null)));
  await Promise.resolve();
}

describe('useDragSelection', () => {
  let harness: Harness | null = null;

  beforeEach(() => {
    document.body.innerHTML = '';
  });

  afterEach(() => {
    harness?.scope.stop();
    harness = null;
    delete (document as Partial<Document>).elementFromPoint;
    vi.restoreAllMocks();
  });

  describe('dragging across a disabled cell', () => {
    it('spans the whole range rather than stopping at the disabled cell', async () => {
      // Cell 3 is closed. Dragging 1 -> 5 must still reach 5.
      harness = setup({ disabled: [3] });

      pointerDown(harness.drag, 1);
      pointerMove(3);
      await flushFrame();
      pointerMove(5);
      await flushFrame();
      pointerUp();

      expect(harness.commits).toHaveLength(1);
      const keys = harness.commits[0].targets.map(t => t.key);
      expect(keys).toEqual(['cell-1', 'cell-2', 'cell-4', 'cell-5']);
    });

    it('commits the enabled cells when the gesture ends on a disabled one', async () => {
      // The reported defect: releasing over a disabled cell truncated the selection
      // back to the last enabled cell the pointer had crossed.
      harness = setup({ disabled: [4] });

      pointerDown(harness.drag, 1);
      pointerMove(4);
      await flushFrame();
      pointerUp();

      expect(harness.commits).toHaveLength(1);
      const keys = harness.commits[0].targets.map(t => t.key);
      expect(keys).toEqual(['cell-1', 'cell-2', 'cell-3']);
      // The focus reached the disabled cell even though it is not committed.
      expect(harness.commits[0].focus.index).toBe(4);
    });

    it('spans several consecutive disabled cells', async () => {
      harness = setup({ disabled: [2, 3, 4] });

      pointerDown(harness.drag, 1);
      pointerMove(5);
      await flushFrame();
      pointerUp();

      const keys = harness.commits[0].targets.map(t => t.key);
      expect(keys).toEqual(['cell-1', 'cell-5']);
    });

    it('does not highlight disabled cells during the drag', async () => {
      harness = setup({ disabled: [3] });

      pointerDown(harness.drag, 1);
      pointerMove(5);
      await flushFrame();

      // The preview must match what will be committed, or the user sees a cell light
      // up and then not take.
      expect([...harness.drag.selected.value].sort()).toEqual([
        'cell-1',
        'cell-2',
        'cell-4',
        'cell-5',
      ]);

      pointerUp();
    });

    it('still refuses to start on a disabled cell', async () => {
      harness = setup({ disabled: [2] });

      pointerDown(harness.drag, 2);
      pointerMove(5);
      await flushFrame();
      pointerUp();

      expect(harness.drag.isDragging.value).toBe(false);
      expect(harness.commits).toHaveLength(0);
    });

    it('drags right to left across a disabled cell', async () => {
      harness = setup({ disabled: [3] });

      pointerDown(harness.drag, 5);
      pointerMove(1);
      await flushFrame();
      pointerUp();

      const keys = harness.commits[0].targets.map(t => t.key);
      expect(keys).toEqual(['cell-1', 'cell-2', 'cell-4', 'cell-5']);
    });
  });

  describe('taps', () => {
    it('commits a click that never moved, as a one-cell drag', () => {
      harness = setup();

      pointerDown(harness.drag, 2);
      pointerUp();

      expect(harness.commits).toHaveLength(1);
      expect(harness.commits[0].targets.map(t => t.key)).toEqual(['cell-2']);
      // isTap means "never became a drag", and a mouse press starts one immediately —
      // only touch defers behind the hold timer. So a plain click is a one-cell drag,
      // not a tap. Consumers must not use isTap to mean "was a click".
      expect(harness.commits[0].isTap).toBe(false);
    });

    it('commits a touch released before the hold delay', () => {
      // The regression this composable was written to fix: sub-threshold taps used to
      // do nothing at all on mobile.
      vi.useFakeTimers();
      harness = setup();

      pointerDown(harness.drag, 2, 'touch');
      vi.advanceTimersByTime(TOUCH_HOLD_MS - 1);
      pointerUp();

      expect(harness.commits).toHaveLength(1);
      expect(harness.commits[0].isTap).toBe(true);
      vi.useRealTimers();
    });

    it('becomes a drag once the touch is held', async () => {
      vi.useFakeTimers();
      harness = setup();

      pointerDown(harness.drag, 1, 'touch');
      vi.advanceTimersByTime(TOUCH_HOLD_MS + 1);
      expect(harness.drag.isDragging.value).toBe(true);

      vi.useRealTimers();
      pointerMove(3);
      await flushFrame();
      pointerUp();

      expect(harness.commits[0].isTap).toBe(false);
      expect(harness.commits[0].targets).toHaveLength(3);
    });
  });

  describe('gesture guards', () => {
    it('ignores a secondary mouse button', () => {
      harness = setup();

      harness.drag.onPointerDown(
        new PointerEvent('pointerdown', {
          clientX: xOf(2),
          clientY: 5,
          button: 2,
          pointerType: 'mouse',
          bubbles: true,
        })
      );
      pointerUp();

      expect(harness.commits).toHaveLength(0);
    });

    it('cancels without committing', async () => {
      harness = setup();

      pointerDown(harness.drag, 1);
      pointerMove(4);
      await flushFrame();
      harness.drag.cancel();
      pointerUp();

      expect(harness.commits).toHaveLength(0);
      expect(harness.drag.selected.value.size).toBe(0);
    });

    it('reports no selection once the gesture ends', async () => {
      harness = setup();

      pointerDown(harness.drag, 1);
      pointerMove(4);
      await flushFrame();
      expect(harness.drag.selected.value.size).toBe(4);

      pointerUp();
      expect(harness.drag.selected.value.size).toBe(0);
      expect(harness.drag.isDragging.value).toBe(false);
    });
  });
});
