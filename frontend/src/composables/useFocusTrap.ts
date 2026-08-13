/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { nextTick, onBeforeUnmount, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue';

/**
 * The four things every modal owes a keyboard user, extracted from ConfirmDialog.vue
 * so the rest of the app's dialogs stop reinventing — or, more usually, skipping — them:
 *
 *  1. focus moves *into* the dialog when it opens, otherwise a screen reader and a Tab
 *     key both stay parked behind the backdrop on a page the user can no longer see;
 *  2. Tab and Shift+Tab wrap inside the panel, and focus that has escaped is pulled
 *     back in — that is the whole point of a trap;
 *  3. Escape closes;
 *  4. the element that had focus before the dialog opened gets it back on close, so a
 *     keyboard user carries on where they were rather than at the top of the document.
 *
 * The keydown listener is on `document` in the capture phase, not on the panel: Escape
 * has to work even when focus has somehow leaked out, which is precisely the situation
 * a trap exists to recover from.
 *
 * Backdrop clicks are deliberately not handled here. Whether an accidental click outside
 * counts as an answer is a per-dialog decision, not an accessibility one.
 */

/** Everything the browser will hand focus to via Tab, in document order. */
const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'textarea:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ');

export interface FocusTrapOptions {
  /** The dialog panel. Everything focusable inside it is in the trap. */
  container: Ref<HTMLElement | null>;
  /** Called on Escape. Omit to make the dialog non-dismissable by keyboard. */
  onEscape?: () => void;
  /**
   * What to focus on open. Defaults to the first focusable element in the panel.
   * Return `null` to fall back to that default.
   */
  initialFocus?: () => HTMLElement | null | undefined;
}

/**
 * Traps focus inside `options.container` for as long as `isOpen` is truthy.
 *
 * Call from `<script setup>`; the trap detaches itself on close and on unmount, so
 * there is nothing to clean up at the call site.
 */
export function useFocusTrap(isOpen: MaybeRefOrGetter<boolean>, options: FocusTrapOptions): void {
  /** Where focus was before we took it, so it can be handed back. */
  let previouslyFocused: HTMLElement | null = null;
  let attached = false;

  function focusableElements(): HTMLElement[] {
    const root = options.container.value;
    if (!root) return [];
    return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
      element => element.offsetParent !== null || element === document.activeElement
    );
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      if (!options.onEscape) return;
      event.preventDefault();
      event.stopPropagation();
      options.onEscape();
      return;
    }

    if (event.key !== 'Tab') return;

    const focusable = focusableElements();
    if (focusable.length === 0) {
      // Nothing to land on: keep focus on the panel rather than letting Tab escape.
      event.preventDefault();
      options.container.value?.focus();
      return;
    }

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = document.activeElement;
    const inside = options.container.value?.contains(active) ?? false;

    // Wrap at both ends, and pull focus back in if it has left the panel entirely.
    if (event.shiftKey && (active === first || !inside)) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && (active === last || !inside)) {
      event.preventDefault();
      first.focus();
    }
  }

  function attach() {
    if (attached) return;
    document.addEventListener('keydown', onKeydown, true);
    attached = true;
  }

  function detach() {
    if (!attached) return;
    document.removeEventListener('keydown', onKeydown, true);
    attached = false;
  }

  watch(
    () => toValue(isOpen),
    async open => {
      if (open) {
        previouslyFocused =
          document.activeElement instanceof HTMLElement ? document.activeElement : null;
        attach();
        await nextTick();
        const target =
          options.initialFocus?.() ?? focusableElements()[0] ?? options.container.value;
        target?.focus();
      } else {
        detach();
        // Guard against focusing a node the close has already removed from the document.
        if (previouslyFocused?.isConnected) previouslyFocused.focus();
        previouslyFocused = null;
      }
    },
    { immediate: true }
  );

  onBeforeUnmount(detach);
}
