// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

// Package broadcast delivers short notices to everyone currently watching a topic.
//
// It exists for the live calendar: when a participant answers, every other browser
// showing that calendar should hear about it. The notices carry no data, only the fact
// that something changed, so a listener refetches through the normal read path. That
// keeps one read model instead of two and means a notice can never leak a field the
// recipient is not allowed to see.
//
// Two implementations, chosen the way pkg/middleware chooses its rate-limit backend: a
// process-local hub that needs nothing, and a Redis-backed one that also reaches the
// other instances of a horizontally scaled deployment. A single-instance self-hosted
// install works fully with the local hub; Redis is only required once there is more
// than one instance, and its absence degrades to "live within this instance" rather
// than to an error.
package broadcast

import (
	"context"
	"sync"
)

// Broker delivers notices published on a topic to that topic's current subscribers.
//
// Delivery is best-effort by design. A subscriber that is not reading fast enough is
// skipped rather than allowed to block the publisher: the notices are invalidations, so
// a listener that misses one and receives the next is in the same state as one that
// received both.
type Broker interface {
	// Publish sends a notice to everyone subscribed to the topic. It never blocks on a
	// slow subscriber.
	Publish(ctx context.Context, topic string) error

	// Subscribe returns a channel of notices and a function that stops the
	// subscription. The channel is closed when the subscription stops, so a range over
	// it terminates. Cancelling the context also stops it.
	Subscribe(ctx context.Context, topic string) (<-chan struct{}, func())

	// Close releases whatever the broker holds. Subscribers see their channels close.
	Close() error
}

// hub is the process-local delivery mechanism, shared by both implementations: the
// Redis broker uses it to deliver what arrives from Redis.
type hub struct {
	mu     sync.RWMutex
	topics map[string]map[*subscriber]struct{}
	closed bool
}

type subscriber struct {
	// Capacity one, so a notice waiting to be read stands in for any number of further
	// notices. What a listener needs to know is "something changed since you last
	// looked", and one pending notice says exactly that.
	ch chan struct{}

	once sync.Once
}

func newHub() *hub {
	return &hub{topics: make(map[string]map[*subscriber]struct{})}
}

func (h *hub) publish(topic string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for sub := range h.topics[topic] {
		select {
		case sub.ch <- struct{}{}:
		default:
			// Already has a notice pending; a second would tell it nothing new.
		}
	}
}

func (h *hub) subscribe(ctx context.Context, topic string) (<-chan struct{}, func()) {
	sub := &subscriber{ch: make(chan struct{}, 1)}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(sub.ch)

		return sub.ch, func() {}
	}
	if h.topics[topic] == nil {
		h.topics[topic] = make(map[*subscriber]struct{})
	}
	h.topics[topic][sub] = struct{}{}
	h.mu.Unlock()

	stop := func() {
		h.mu.Lock()
		if peers := h.topics[topic]; peers != nil {
			delete(peers, sub)
			// Drop the topic once nobody is listening, so a long-lived process does not
			// accumulate an entry per calendar ever viewed.
			if len(peers) == 0 {
				delete(h.topics, topic)
			}
		}
		h.mu.Unlock()

		sub.once.Do(func() { close(sub.ch) })
	}

	// Cancelling the caller's context unsubscribes, so a handler that returns cannot
	// leave an entry behind even if it forgets to call stop.
	if ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			stop()
		}()
	}

	return sub.ch, stop
}

func (h *hub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.closed = true

	for topic, peers := range h.topics {
		for sub := range peers {
			sub.once.Do(func() { close(sub.ch) })
		}
		delete(h.topics, topic)
	}
}

// subscriberCount reports how many subscriptions a topic has. Used by the tests, and by
// nothing else — the brokers do not branch on it.
func (h *hub) subscriberCount(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.topics[topic])
}

// memoryBroker delivers within one process.
type memoryBroker struct{ hub *hub }

// NewMemoryBroker returns a broker that reaches the subscribers of this process only.
// That is the whole of a single-instance deployment, which is how self-hosted WhenTo
// normally runs.
func NewMemoryBroker() Broker {
	return &memoryBroker{hub: newHub()}
}

func (b *memoryBroker) Publish(_ context.Context, topic string) error {
	b.hub.publish(topic)

	return nil
}

func (b *memoryBroker) Subscribe(ctx context.Context, topic string) (<-chan struct{}, func()) {
	return b.hub.subscribe(ctx, topic)
}

func (b *memoryBroker) Close() error {
	b.hub.close()

	return nil
}
