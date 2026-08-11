/*
 * WhenTo - Collaborative event calendar for self-hosted environments
 * Copyright (C) 2025 WhenTo Contributors
 * SPDX-License-Identifier: BSL-1.1
 *
 * @vitest-environment jsdom
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, nextTick, ref } from 'vue';

import { useCalendarStream } from './useCalendarStream';

/**
 * The two ways a live view fails are opposites, and both are quiet: it refetches far too
 * often, turning one person's drag into a burst of requests from every open browser; or
 * it stops refetching after a network blip and shows stale availability that looks
 * perfectly current.
 *
 * jsdom has no EventSource, so this stands one in for it and drives the events by hand.
 */

class FakeEventSource {
  static instances: FakeEventSource[] = [];

  readonly url: string;
  closed = false;
  private listeners = new Map<string, Set<() => void>>();

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, handler: () => void) {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)!.add(handler);
  }

  close() {
    this.closed = true;
  }

  /** Plays a server event. */
  emit(type: string) {
    for (const handler of this.listeners.get(type) ?? []) handler();
  }

  static get latest() {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1];
  }

  static reset() {
    FakeEventSource.instances = [];
  }
}

beforeEach(() => {
  FakeEventSource.reset();
  vi.stubGlobal('EventSource', FakeEventSource);
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

/** Runs the composable inside a scope the test can stop, as a component would. */
function withScope<T>(fn: () => T): { result: T; stop: () => void } {
  const scope = effectScope();
  const result = scope.run(fn)!;

  return { result, stop: () => scope.stop() };
}

describe('opening the stream', () => {
  it('subscribes to the calendar named by the token', () => {
    const { stop } = withScope(() =>
      useCalendarStream({ token: ref('abc123'), onChange: () => {} })
    );

    expect(FakeEventSource.latest.url).toBe('/api/v1/availabilities/calendar/abc123/events');
    stop();
  });

  it('escapes the token rather than interpolating it raw', () => {
    // The token comes from the URL the user pasted. It is normally hex, but building a
    // path by concatenation is how a stray slash becomes a request somewhere else.
    const { stop } = withScope(() =>
      useCalendarStream({ token: ref('a/b?c'), onChange: () => {} })
    );

    expect(FakeEventSource.latest.url).toBe('/api/v1/availabilities/calendar/a%2Fb%3Fc/events');
    stop();
  });

  it('stays closed without a token', () => {
    const { stop } = withScope(() => useCalendarStream({ token: ref(null), onChange: () => {} }));

    expect(FakeEventSource.instances).toHaveLength(0);
    stop();
  });

  it('stays closed when disabled', () => {
    const { stop } = withScope(() =>
      useCalendarStream({ token: ref('abc123'), onChange: () => {}, enabled: ref(false) })
    );

    expect(FakeEventSource.instances).toHaveLength(0);
    stop();
  });
});

describe('reacting to notices', () => {
  it('reloads when the calendar changes', async () => {
    const onChange = vi.fn();
    const { stop } = withScope(() => useCalendarStream({ token: ref('abc123'), onChange }));

    FakeEventSource.latest.emit('update');
    expect(onChange).not.toHaveBeenCalled(); // still inside the coalescing window

    await vi.advanceTimersByTimeAsync(300);
    expect(onChange).toHaveBeenCalledTimes(1);
    stop();
  });

  // One person dragging across a week writes a day at a time. Without coalescing, every
  // other open browser would refetch the whole range once per day dragged.
  it('collapses a burst into a single reload', async () => {
    const onChange = vi.fn();
    const { stop } = withScope(() => useCalendarStream({ token: ref('abc123'), onChange }));

    for (let i = 0; i < 7; i += 1) FakeEventSource.latest.emit('update');

    await vi.advanceTimersByTimeAsync(300);
    expect(onChange).toHaveBeenCalledTimes(1);
    stop();
  });

  it('reloads again for a change that arrives after the window', async () => {
    const onChange = vi.fn();
    const { stop } = withScope(() => useCalendarStream({ token: ref('abc123'), onChange }));

    FakeEventSource.latest.emit('update');
    await vi.advanceTimersByTimeAsync(300);
    FakeEventSource.latest.emit('update');
    await vi.advanceTimersByTimeAsync(300);

    expect(onChange).toHaveBeenCalledTimes(2);
    stop();
  });
});

describe('connection state', () => {
  it('does not reload on the first connection', async () => {
    // The view has just fetched its data; refetching immediately would double every
    // page load.
    const onChange = vi.fn();
    const { result, stop } = withScope(() => useCalendarStream({ token: ref('abc123'), onChange }));

    FakeEventSource.latest.emit('open');
    await vi.advanceTimersByTimeAsync(300);

    expect(onChange).not.toHaveBeenCalled();
    expect(result.connected.value).toBe(true);
    stop();
  });

  // The case that decides whether this works on a train. Anything that changed while the
  // stream was down produced no notice this browser will ever receive, so a reconnection
  // has to assume it missed something.
  it('reloads after reconnecting', async () => {
    const onChange = vi.fn();
    const { result, stop } = withScope(() => useCalendarStream({ token: ref('abc123'), onChange }));

    FakeEventSource.latest.emit('open');
    FakeEventSource.latest.emit('error');
    expect(result.connected.value).toBe(false);

    FakeEventSource.latest.emit('open');
    await vi.advanceTimersByTimeAsync(300);

    expect(onChange).toHaveBeenCalledTimes(1);
    stop();
  });

  it('leaves the stream open on error so EventSource can reconnect', () => {
    const { stop } = withScope(() =>
      useCalendarStream({ token: ref('abc123'), onChange: () => {} })
    );

    const source = FakeEventSource.latest;
    source.emit('open');
    source.emit('error');

    // Closing here would cancel the browser's own reconnection and the view would never
    // come back.
    expect(source.closed).toBe(false);
    stop();
  });
});

describe('lifecycle', () => {
  it('reopens on a different calendar when the token changes', async () => {
    const token = ref('first');
    const { stop } = withScope(() => useCalendarStream({ token, onChange: () => {} }));

    const first = FakeEventSource.latest;
    token.value = 'second';
    await nextTick();

    expect(first.closed).toBe(true);
    expect(FakeEventSource.latest.url).toContain('second');
    stop();
  });

  it('closes when its scope is stopped', () => {
    const { stop } = withScope(() =>
      useCalendarStream({ token: ref('abc123'), onChange: () => {} })
    );

    const source = FakeEventSource.latest;
    stop();

    // A view left behind holding an open stream is a connection the server keeps alive
    // for a tab nobody is looking at.
    expect(source.closed).toBe(true);
  });

  it('does not reload after being closed', async () => {
    const onChange = vi.fn();
    const { result, stop } = withScope(() => useCalendarStream({ token: ref('abc123'), onChange }));

    FakeEventSource.latest.emit('update');
    result.close();
    await vi.advanceTimersByTimeAsync(300);

    // A refetch landing after the view is gone writes into a store nobody is showing.
    expect(onChange).not.toHaveBeenCalled();
    stop();
  });
});
