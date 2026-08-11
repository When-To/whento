/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 */

import { onScopeDispose, ref, watch, type Ref } from 'vue';

/**
 * Keeps a calendar view in step with what other participants are doing.
 *
 * The server sends notices, not data: each one means "this calendar changed, look
 * again". The refetch goes through the ordinary read path, so there is one read model
 * rather than two, and a notice can never carry a field its recipient is not entitled
 * to see.
 *
 * EventSource is used rather than a WebSocket because the traffic is one-way. It also
 * reconnects on its own, which is most of what a hand-rolled socket would have to
 * reimplement — and reconnecting correctly is the part that decides whether this works
 * on a train.
 */

/** How long to wait after a notice before refetching. */
const COALESCE_MS = 250;

export interface UseCalendarStreamOptions {
  /** Public token of the calendar to watch; null suspends the stream. */
  readonly token: Ref<string | null | undefined>;
  /** Called when the calendar has changed and the view should reload its data. */
  readonly onChange: () => void | Promise<void>;
  /** Set false to keep the stream closed, e.g. behind a preference. */
  readonly enabled?: Ref<boolean>;
}

export interface CalendarStream {
  /** Whether the browser currently holds an open stream. */
  readonly connected: Ref<boolean>;
  /** Closes the stream; it reopens if the token changes. */
  close(): void;
}

export function useCalendarStream(options: UseCalendarStreamOptions): CalendarStream {
  const connected = ref(false);

  let source: EventSource | null = null;
  let coalesceTimer: ReturnType<typeof setTimeout> | null = null;
  // A first connection means the view has just loaded its data; a later one means the
  // stream was interrupted, and anything that changed meanwhile was missed.
  let hasConnectedBefore = false;

  function requestReload() {
    if (coalesceTimer !== null) clearTimeout(coalesceTimer);
    // One person dragging across a week produces a write per day. The server already
    // collapses notices per connection, but a burst still arrives as several; without
    // this the grid would refetch once per day dragged.
    coalesceTimer = setTimeout(() => {
      coalesceTimer = null;
      void options.onChange();
    }, COALESCE_MS);
  }

  function close() {
    if (coalesceTimer !== null) {
      clearTimeout(coalesceTimer);
      coalesceTimer = null;
    }
    source?.close();
    source = null;
    connected.value = false;
  }

  function open(token: string) {
    close();

    // Same origin as the rest of the API. EventSource cannot go through the axios client,
    // which is why this is the one place a URL is built by hand.
    source = new EventSource(`/api/v1/availabilities/calendar/${encodeURIComponent(token)}/events`);

    source.addEventListener('open', () => {
      connected.value = true;
      if (hasConnectedBefore) {
        // Reconnected. Whatever happened while the stream was down produced no notice
        // this browser will ever see, so the only safe assumption is that it missed
        // something.
        requestReload();
      }
      hasConnectedBefore = true;
    });

    source.addEventListener('update', () => requestReload());

    source.addEventListener('error', () => {
      // EventSource reconnects on its own, using the retry delay the server sent. The
      // stream is only reported as down so the UI can say so; it is not torn down here,
      // because closing it would cancel the automatic reconnection.
      connected.value = false;
    });
  }

  watch(
    () => [options.token.value, options.enabled?.value ?? true] as const,
    ([token, enabled]) => {
      if (!token || !enabled) {
        close();
        return;
      }
      open(token);
    },
    { immediate: true }
  );

  // Vue's scope, not onUnmounted: the composable is used from a view, but this way it
  // also cleans up when called inside an effectScope that is stopped.
  onScopeDispose(close);

  return { connected, close };
}
