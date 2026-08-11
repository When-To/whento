// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package broadcast

import (
	"context"
	"sync"
	"testing"
	"time"
)

// The broker sits between a write and every browser watching the calendar, so the
// failure modes that matter are the ones that are invisible: a notice that reaches the
// wrong calendar, a subscriber that blocks a write, and a subscription that outlives the
// request that made it.

// waitForNotice reports whether a notice arrives within a short window. Long enough not
// to be flaky on a loaded machine, short enough that a missing notice fails quickly.
func waitForNotice(t *testing.T, ch <-chan struct{}) bool {
	t.Helper()

	select {
	case _, ok := <-ch:
		return ok
	case <-time.After(2 * time.Second):
		return false
	}
}

func TestASubscriberHearsItsOwnTopic(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	notices, stop := broker.Subscribe(context.Background(), "calendar-a")
	t.Cleanup(stop)

	if err := broker.Publish(context.Background(), "calendar-a"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !waitForNotice(t, notices) {
		t.Error("no notice arrived")
	}
}

// TestTopicsAreIsolated is the one that matters for privacy. Calendars are addressed by
// their public token, and a notice crossing topics would tell the holder of one link
// that a different calendar just changed.
func TestTopicsAreIsolated(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	mine, stopMine := broker.Subscribe(context.Background(), "calendar-a")
	t.Cleanup(stopMine)
	theirs, stopTheirs := broker.Subscribe(context.Background(), "calendar-b")
	t.Cleanup(stopTheirs)

	if err := broker.Publish(context.Background(), "calendar-b"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if !waitForNotice(t, theirs) {
		t.Error("the subscriber to calendar-b heard nothing")
	}
	select {
	case <-mine:
		t.Error("a notice for calendar-b reached a subscriber of calendar-a")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestEverySubscriberToATopicIsNotified(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	const watchers = 25
	channels := make([]<-chan struct{}, watchers)
	for i := range channels {
		ch, stop := broker.Subscribe(context.Background(), "calendar-a")
		t.Cleanup(stop)
		channels[i] = ch
	}

	if err := broker.Publish(context.Background(), "calendar-a"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i, ch := range channels {
		if !waitForNotice(t, ch) {
			t.Errorf("watcher %d heard nothing", i)
		}
	}
}

// TestAStalledSubscriberDoesNotBlockThePublisher covers the failure that would turn a
// browser left open on a locked screen into a stalled write for everyone else.
func TestAStalledSubscriberDoesNotBlockThePublisher(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	// Subscribed and never read from.
	_, stop := broker.Subscribe(context.Background(), "calendar-a")
	t.Cleanup(stop)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			_ = broker.Publish(context.Background(), "calendar-a")
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publishing blocked on a subscriber that never reads")
	}
}

// TestNoticesCoalesce records the design: the channel holds one pending notice, because
// "something changed since you last looked" is all a listener acts on. Ten writes while
// a browser is busy must not queue ten refetches.
func TestNoticesCoalesce(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	notices, stop := broker.Subscribe(context.Background(), "calendar-a")
	t.Cleanup(stop)

	for range 10 {
		if err := broker.Publish(context.Background(), "calendar-a"); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	if !waitForNotice(t, notices) {
		t.Fatal("no notice arrived")
	}
	select {
	case <-notices:
		t.Error("a second notice was queued; ten writes should collapse into one refetch")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStopEndsTheSubscription(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	notices, stop := broker.Subscribe(context.Background(), "calendar-a")
	stop()

	// The channel closes, so a range over it terminates rather than hanging.
	if _, open := <-notices; open {
		t.Error("the channel yielded a value after the subscription stopped")
	}

	// And stopping twice is harmless: the SSE handler calls stop in a defer and again on
	// the disconnect path.
	stop()
}

// TestCancellingTheContextUnsubscribes is what stops a leak. Every subscription is made
// from a request context, and a browser closing its tab cancels it without the handler
// getting a chance to tidy up.
func TestCancellingTheContextUnsubscribes(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	local, ok := broker.(*memoryBroker)
	if !ok {
		t.Fatalf("unexpected broker type %T", broker)
	}

	ctx, cancel := context.WithCancel(context.Background())
	notices, _ := broker.Subscribe(ctx, "calendar-a")

	if got := local.hub.subscriberCount("calendar-a"); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}

	cancel()

	if _, open := <-notices; open {
		t.Error("the channel stayed open after the context was cancelled")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if local.hub.subscriberCount("calendar-a") == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("the subscription survived its context: %d still registered",
		local.hub.subscriberCount("calendar-a"))
}

// TestTopicsAreForgotten guards against a slow leak that no functional test would show:
// a process that has served a million calendars holding a million empty maps.
func TestTopicsAreForgotten(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	local := broker.(*memoryBroker)

	for i := range 100 {
		_, stop := broker.Subscribe(context.Background(), string(rune('a'+i%26))+"-calendar")
		stop()
	}

	local.hub.mu.RLock()
	remaining := len(local.hub.topics)
	local.hub.mu.RUnlock()

	if remaining != 0 {
		t.Errorf("%d topics kept after every subscriber left", remaining)
	}
}

func TestCloseEndsEverySubscription(t *testing.T) {
	broker := NewMemoryBroker()

	first, _ := broker.Subscribe(context.Background(), "calendar-a")
	second, _ := broker.Subscribe(context.Background(), "calendar-b")

	if err := broker.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for name, ch := range map[string]<-chan struct{}{"calendar-a": first, "calendar-b": second} {
		if _, open := <-ch; open {
			t.Errorf("the subscription to %s survived Close", name)
		}
	}

	// Subscribing after Close yields a closed channel rather than one that never
	// delivers, so a listener started during shutdown terminates instead of hanging.
	late, stop := broker.Subscribe(context.Background(), "calendar-c")
	defer stop()
	if _, open := <-late; open {
		t.Error("subscribing after Close produced a live channel")
	}

	// And Close is idempotent; the server shutdown path may reach it twice.
	if err := broker.Close(); err != nil {
		t.Errorf("Close twice: %v", err)
	}
}

// TestConcurrentUseIsSafe is worth running under -race, which CI does. Subscribing,
// publishing and unsubscribing all touch the same map.
func TestConcurrentUseIsSafe(t *testing.T) {
	broker := NewMemoryBroker()
	t.Cleanup(func() { _ = broker.Close() })

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ch, stop := broker.Subscribe(context.Background(), "calendar-a")
			_ = broker.Publish(context.Background(), "calendar-a")
			select {
			case <-ch:
			case <-time.After(time.Second):
			}
			stop()
			_ = i
		}()
	}
	wg.Wait()
}
