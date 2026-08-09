/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

/**
 * Rectangle drag selection over a calendar grid, for mouse, pen and touch.
 *
 * Replaces three near-identical state machines (month cells, week slots, week day
 * headers) that each carried their own timers, their own throttle constants and their
 * own listener bookkeeping. Two consequences are structural rather than incidental:
 *
 * - Every global listener goes through `useEventListener`, so it is bound to the
 *   component scope and removed on unmount. The week grid previously registered
 *   `mouseup`/`touchend` on `window` at setup and had no `onUnmounted` at all, and its
 *   header drag could stay stuck "on" because its pointer-up handler was only bound to
 *   the headers themselves.
 * - A quick tap always commits. The previous handlers returned early when neither the
 *   drag nor the hold timer had started, which made the "treat as a single click"
 *   branch unreachable — so on mobile, taps shorter than the 100 ms hold threshold
 *   silently did nothing.
 */

import { computed, ref, type ComputedRef, type Ref } from 'vue';
import { useEventListener } from '@vueuse/core';

/** Anything addressable in the grid. `key` must be stable and unique. */
export interface DragTarget {
  readonly key: string;
}

export type DragMode = 'add' | 'remove';

export interface DragCommit<T extends DragTarget> {
  readonly mode: DragMode;
  readonly anchor: T;
  readonly focus: T;
  /** Every target in the rectangle, already filtered by `canStart`. */
  readonly targets: readonly T[];
  /** True when the gesture never became a drag — a click or a quick tap. */
  readonly isTap: boolean;
}

export interface UseDragSelectionOptions<T extends DragTarget> {
  /** Element the pointer coordinates are resolved against. */
  readonly container: Ref<HTMLElement | null>;
  /** Map a DOM element under the pointer to a target, or null if it is not one. */
  resolve(element: Element): T | null;
  /** Whether a gesture may start on, or extend to, this target. */
  canStart(target: T): boolean;
  /** Whether the gesture adds or removes, decided from the anchor. */
  initialMode(target: T): DragMode;
  /** Every target inside the rectangle spanned by two corners, in view coordinates. */
  rectangle(anchor: T, focus: T): readonly T[];
  /** Called once per gesture, on release. */
  commit(result: DragCommit<T>): void;
  /** How long a touch must be held, without moving, before it becomes a drag. */
  readonly holdDelayMs?: number;
}

/** The returned surface deals only in keys, so it needs no target type. */
export interface DragSelection {
  readonly isDragging: ComputedRef<boolean>;
  readonly mode: ComputedRef<DragMode>;
  /** Keys currently inside the rectangle. Empty when no gesture is running. */
  readonly selected: ComputedRef<ReadonlySet<string>>;
  /** Bind to `@pointerdown` on the container. */
  onPointerDown(event: PointerEvent): void;
  /** Abort without committing. */
  cancel(): void;
}

/**
 * A touch must be held this long, without moving, before it is treated as a drag
 * rather than a scroll. Below it the gesture is a tap, and still commits.
 */
export const TOUCH_HOLD_MS = 100;

export function useDragSelection<T extends DragTarget>(
  options: UseDragSelectionOptions<T>
): DragSelection {
  const holdDelay = options.holdDelayMs ?? TOUCH_HOLD_MS;

  const anchor = ref<T | null>(null) as Ref<T | null>;
  const focus = ref<T | null>(null) as Ref<T | null>;
  const mode = ref<DragMode>('add');
  const dragging = ref(false);
  const pointerType = ref<string>('mouse');
  const startPoint = ref<{ x: number; y: number } | null>(null);

  let holdTimer: ReturnType<typeof setTimeout> | null = null;
  let rafHandle: number | null = null;

  function clearHoldTimer() {
    if (holdTimer !== null) {
      clearTimeout(holdTimer);
      holdTimer = null;
    }
  }

  function reset() {
    clearHoldTimer();
    if (rafHandle !== null) {
      cancelAnimationFrame(rafHandle);
      rafHandle = null;
    }
    anchor.value = null;
    focus.value = null;
    dragging.value = false;
    startPoint.value = null;
  }

  const selected = computed<ReadonlySet<string>>(() => {
    if (!dragging.value || !anchor.value || !focus.value) return new Set<string>();
    return new Set(options.rectangle(anchor.value, focus.value).map(target => target.key));
  });

  /** Resolve the target under a point, so touch drags can leave the origin element. */
  function targetAt(x: number, y: number): T | null {
    const element = document.elementFromPoint(x, y);
    return element ? options.resolve(element) : null;
  }

  function onPointerDown(event: PointerEvent) {
    // Secondary buttons and modified clicks are not selections.
    if (event.button !== 0 && event.pointerType === 'mouse') return;

    const target = targetAt(event.clientX, event.clientY);
    if (!target || !options.canStart(target)) return;

    anchor.value = target;
    focus.value = target;
    mode.value = options.initialMode(target);
    pointerType.value = event.pointerType;
    startPoint.value = { x: event.clientX, y: event.clientY };

    if (event.pointerType === 'touch') {
      // Wait: a touch that moves before the delay is the user scrolling the page.
      clearHoldTimer();
      holdTimer = setTimeout(() => {
        holdTimer = null;
        if (anchor.value) dragging.value = true;
      }, holdDelay);
    } else {
      dragging.value = true;
      event.preventDefault();
    }
  }

  useEventListener(options.container, 'pointermove', (event: PointerEvent) => {
    if (!anchor.value) return;

    if (!dragging.value) {
      // Still deciding. Any real movement before the hold fires means "scroll".
      if (pointerType.value === 'touch' && startPoint.value) {
        const dx = Math.abs(event.clientX - startPoint.value.x);
        const dy = Math.abs(event.clientY - startPoint.value.y);
        if (dx > 8 || dy > 8) reset();
      }
      return;
    }

    // Coalesce to one update per frame, replacing a hand-rolled 16 ms throttle that
    // called Date.now() on every move event.
    if (rafHandle !== null) return;
    const { clientX, clientY } = event;
    rafHandle = requestAnimationFrame(() => {
      rafHandle = null;
      if (!dragging.value) return;
      const target = targetAt(clientX, clientY);
      if (target && options.canStart(target)) focus.value = target;
    });
  });

  // Once a touch drag is confirmed, stop the page from scrolling under it.
  useEventListener(
    options.container,
    'touchmove',
    (event: TouchEvent) => {
      if (dragging.value && pointerType.value === 'touch') event.preventDefault();
    },
    { passive: false }
  );

  function finish() {
    const start = anchor.value;
    const end = focus.value ?? anchor.value;
    if (!start || !end) {
      reset();
      return;
    }

    const wasDragging = dragging.value;
    const targets = wasDragging ? options.rectangle(start, end).filter(options.canStart) : [start];

    reset();

    // A gesture that had a valid anchor always commits, whether or not it ever became
    // a drag. This is what makes sub-threshold taps work.
    options.commit({ mode: mode.value, anchor: start, focus: end, targets, isTap: !wasDragging });
  }

  // Bound to the window, so releasing outside the grid still ends the gesture. The
  // week grid's header drag used to stay stuck highlighted in exactly this case.
  useEventListener(window, 'pointerup', finish);
  useEventListener(window, 'pointercancel', reset);

  return {
    isDragging: computed(() => dragging.value),
    mode: computed(() => mode.value),
    selected,
    onPointerDown,
    cancel: reset,
  };
}
