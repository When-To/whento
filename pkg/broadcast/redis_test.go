// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package broadcast

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// The topic is the calendar's public token, and the token is the authorisation. It used
// to be the Redis channel name, where `PUBSUB CHANNELS` and `MONITOR` both show it.
//
// Hashing it is the easy half. The half that has to be proved is that publishing and
// subscribing still land on the same channel: a mismatch fails silently — no error, no
// log line, simply no browser ever hearing about a change again — and it would only show
// up in production, across two instances, which is precisely where no test looks. So
// these tests drive a broker through a stand-in Redis that fans a publish back out the
// way the real one does, and check both that the token is gone and that the notice
// arrives.

// fakeRedis stands in for the server. It records the channel of every publish and fans
// each one out to every broker attached to it — including the publisher, which is what
// real Redis does, and why the Redis broker has no separate local delivery.
type fakeRedis struct {
	mu       sync.Mutex
	channels []string
	err      error
	brokers  []*redisBroker
}

func (f *fakeRedis) Publish(ctx context.Context, channel string, _ any) *redis.IntCmd {
	f.mu.Lock()
	f.channels = append(f.channels, channel)
	err := f.err
	targets := append([]*redisBroker(nil), f.brokers...)
	f.mu.Unlock()

	cmd := redis.NewIntCmd(ctx)
	if err != nil {
		cmd.SetErr(err)

		return cmd
	}

	for _, broker := range targets {
		broker.deliver(channel)
	}

	return cmd
}

func (f *fakeRedis) published() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.channels...)
}

// attach builds a broker that publishes through this fake and receives everything it
// fans out — one instance of a horizontally scaled deployment.
func (f *fakeRedis) attach(t *testing.T) *redisBroker {
	t.Helper()

	broker := &redisBroker{
		publisher: f,
		hub:       newHub(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		done:      make(chan struct{}),
	}
	t.Cleanup(func() { _ = broker.Close() })

	f.mu.Lock()
	f.brokers = append(f.brokers, broker)
	f.mu.Unlock()

	return broker
}

// expectNoNotice is the negative half of waitForNotice: it waits long enough that a
// notice on its way would have arrived.
func expectNoNotice(t *testing.T, ch <-chan struct{}, message string) {
	t.Helper()

	select {
	case <-ch:
		t.Error(message)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestTheChannelNameCarriesNoToken is the leak itself: a channel name is not stored, but
// it is visible to anyone with a Redis console, and it used to be a list of the token of
// every calendar being watched.
func TestTheChannelNameCarriesNoToken(t *testing.T) {
	for _, tt := range []struct {
		name  string
		topic string
	}{
		{"a calendar token", "8f14e45fceea167a5a36dedd4bea2543"},
		{"a short token", "abc"},
		{"a token with punctuation", "tok_en-with.chars"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			channel := channelName(tt.topic)

			if !strings.HasPrefix(channel, channelPrefix) {
				t.Errorf("channel %q lost the %q prefix an operator reads it by", channel, channelPrefix)
			}
			if strings.Contains(channel, tt.topic) {
				t.Errorf("channel %q still contains the topic", channel)
			}
			if digest := strings.TrimPrefix(channel, channelPrefix); digest == "" {
				t.Error("the digest is empty, so every calendar would share one channel")
			}
			// Deterministic: two instances, and the same instance after a restart, must
			// derive the same name or the fan-out stops crossing the cluster.
			if again := channelName(tt.topic); again != channel {
				t.Errorf("channelName is not deterministic: %q then %q", channel, again)
			}
		})
	}
}

// TestDifferentTopicsGetDifferentChannels is the other half of the isolation guarantee:
// a collision would deliver one calendar's notices to the holder of another's link.
func TestDifferentTopicsGetDifferentChannels(t *testing.T) {
	seen := map[string]string{}

	for _, topic := range []string{"calendar-a", "calendar-b", "calendar-c", "calendar-A"} {
		channel := channelName(topic)
		if previous, clash := seen[channel]; clash {
			t.Errorf("%q and %q share the channel %q", previous, topic, channel)
		}
		seen[channel] = topic
	}
}

// TestPublishAndSubscribeUseTheSameChannel is the regression that matters. It asserts on
// the string that actually reached the wire, not on a recomputed one, and then feeds that
// same string back the way Redis would.
func TestPublishAndSubscribeUseTheSameChannel(t *testing.T) {
	for _, tt := range []struct {
		name           string
		topic, another string
	}{
		{"a hex token", "8f14e45fceea167a5a36dedd4bea2543", "0cc175b9c0f1b6a831c399e269772661"},
		{"topics that differ by one character", "calendar-a", "calendar-b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := &fakeRedis{}
			broker := server.attach(t)

			mine, stopMine := broker.Subscribe(context.Background(), tt.topic)
			t.Cleanup(stopMine)
			theirs, stopTheirs := broker.Subscribe(context.Background(), tt.another)
			t.Cleanup(stopTheirs)

			if err := broker.Publish(context.Background(), tt.topic); err != nil {
				t.Fatalf("Publish: %v", err)
			}

			published := server.published()
			if len(published) != 1 {
				t.Fatalf("%d publishes reached Redis, want 1", len(published))
			}
			if strings.Contains(published[0], tt.topic) {
				t.Errorf("the channel on the wire, %q, still carries the topic", published[0])
			}

			// The subscriber hears it, which is only possible if Subscribe derived the
			// channel the publish went to.
			if !waitForNotice(t, mine) {
				t.Error("the subscriber heard nothing: publish and subscribe disagree on the channel")
			}
			expectNoNotice(t, theirs, "a notice crossed to a subscriber of another calendar")
		})
	}
}

// TestTwoInstancesShareTheChannel is why the digest may not use logger.Fingerprint. With
// a per-process salt this passes on one instance and silently stops working on two.
func TestTwoInstancesShareTheChannel(t *testing.T) {
	server := &fakeRedis{}
	writer := server.attach(t)
	reader := server.attach(t)

	watching, stopWatching := reader.Subscribe(context.Background(), "calendar-a")
	t.Cleanup(stopWatching)
	elsewhere, stopElsewhere := reader.Subscribe(context.Background(), "calendar-b")
	t.Cleanup(stopElsewhere)

	if err := writer.Publish(context.Background(), "calendar-a"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if !waitForNotice(t, watching) {
		t.Error("a notice published by one instance never reached the other")
	}
	expectNoNotice(t, elsewhere, "a notice reached a subscriber of a different calendar on another instance")
}

// TestAFailedPublishStillDeliversLocally covers the fallback path, which builds no key of
// its own: a Redis outage must not cost this instance's viewers their live updates, and
// it must not deliver to the wrong topic either.
func TestAFailedPublishStillDeliversLocally(t *testing.T) {
	server := &fakeRedis{err: errors.New("redis is down")}
	broker := server.attach(t)

	mine, stopMine := broker.Subscribe(context.Background(), "calendar-a")
	t.Cleanup(stopMine)
	theirs, stopTheirs := broker.Subscribe(context.Background(), "calendar-b")
	t.Cleanup(stopTheirs)

	if err := broker.Publish(context.Background(), "calendar-a"); err == nil {
		t.Error("Publish reported success while Redis was down")
	}

	if !waitForNotice(t, mine) {
		t.Error("the local subscriber heard nothing after the Redis publish failed")
	}
	expectNoNotice(t, theirs, "the local fallback delivered to the wrong topic")
}

// TestTheSubscriptionPatternStillMatches guards the one thing the digest could have
// broken without any test noticing: the receive loop subscribes to a pattern, and a
// channel name that no longer sits under the prefix would never be delivered at all.
func TestTheSubscriptionPatternStillMatches(t *testing.T) {
	pattern := channelPrefix + "*"
	channel := channelName("calendar-a")

	if !strings.HasPrefix(channel, strings.TrimSuffix(pattern, "*")) {
		t.Errorf("channel %q does not match the subscription pattern %q", channel, pattern)
	}
}

// TestNewRedisBrokerWithoutAClientFallsBackToMemory keeps the no-Redis deployment on the
// path that still delivers within its own instance.
func TestNewRedisBrokerWithoutAClientFallsBackToMemory(t *testing.T) {
	broker := NewRedisBroker(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { _ = broker.Close() })

	if _, ok := broker.(*memoryBroker); !ok {
		t.Fatalf("NewRedisBroker(nil) returned %T, want *memoryBroker", broker)
	}

	notices, stop := broker.Subscribe(context.Background(), "calendar-a")
	t.Cleanup(stop)
	if err := broker.Publish(context.Background(), "calendar-a"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !waitForNotice(t, notices) {
		t.Error("the memory fallback delivered nothing")
	}
}
