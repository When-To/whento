// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package broadcast

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/logger"
)

// channelPrefix namespaces the pub/sub channels, so a Redis shared with something else
// does not collide.
const channelPrefix = "whento:broadcast:"

// channelName is the only place a topic becomes a Redis channel, and the only place a
// channel becomes a hub key.
//
// The topic is the calendar's public token, which in WhenTo *is* the authorisation.
// A channel name is never stored — it exists for the duration of a publish — but
// `PUBSUB CHANNELS` lists the live ones and `MONITOR` shows every publish, so a read-only
// Redis console was a list of the token of every calendar currently being watched. Only
// the digest goes out now; the prefix stays readable so an operator can still tell what
// the channel is for.
//
// Publish, Subscribe and the receive loop all reach the wire through this function, and
// the local hub is keyed by the full channel name rather than by the topic. That is what
// makes the two sides provably agree: a subscriber's hub key is the very string Redis
// delivers on, so there is no second place where a prefix could be stripped differently
// or a digest computed from something else. A divergence here would not fail loudly — it
// would simply mean no browser ever hears about a change again.
//
// The digest comes from cache.HashKeyPart, whose salt is deterministic across instances
// and restarts. That is a requirement, not a convenience: under logger.Fingerprint's
// per-process salt each instance would publish on a channel none of its peers subscribes
// to, live updates would stop crossing the cluster, and every single-instance test would
// still pass.
func channelName(topic string) string {
	return channelPrefix + cache.HashKeyPart(topic)
}

// redisBroker reaches the subscribers of every instance.
//
// One pattern subscription rather than one per topic: a deployment serving a thousand
// calendars would otherwise hold a thousand Redis subscriptions, and the traffic here is
// small enough that filtering locally costs nothing. Everything arriving on the pattern
// is handed to the same process-local hub the memory broker uses, so delivery semantics
// are identical whichever broker is in play.
type redisBroker struct {
	publisher channelPublisher
	hub       *hub
	pubsub    *redis.PubSub
	logger    *slog.Logger

	closeOnce sync.Once
	done      chan struct{}
}

// channelPublisher is the single method this broker needs of a Redis client. Narrowing
// it here lets a test observe the channel a publish actually goes to — which is the one
// thing that has to match what a subscriber listens on — without a live Redis.
// *redis.Client satisfies it structurally, so no call site changes.
type channelPublisher interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// NewRedisBroker returns a broker that reaches every instance sharing the Redis.
//
// Returns the memory broker when client is nil, which is how a deployment without Redis
// still gets live updates within its own instance instead of none at all.
func NewRedisBroker(client *redis.Client, logger *slog.Logger) Broker {
	if client == nil {
		return NewMemoryBroker()
	}

	broker := &redisBroker{
		publisher: client,
		hub:       newHub(),
		logger:    logger,
		done:      make(chan struct{}),
	}

	broker.pubsub = client.PSubscribe(context.Background(), channelPrefix+"*")
	go broker.receive()

	return broker
}

// receive forwards everything published on the pattern into the local hub. Redis
// delivers to every instance including the one that published, so a publisher needs no
// separate local delivery.
func (b *redisBroker) receive() {
	messages := b.pubsub.Channel()

	for {
		select {
		case <-b.done:
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			b.deliver(message.Channel)
		}
	}
}

// deliver hands a notice that arrived on a Redis channel to that channel's local
// subscribers. Split out of receive so the delivery path can be exercised with exactly
// the string Redis puts on the wire, without standing up a Redis.
func (b *redisBroker) deliver(channel string) {
	b.hub.publish(channel)
}

func (b *redisBroker) Publish(ctx context.Context, topic string) error {
	// The payload is unused: the channel name carries the topic and the notice itself
	// says only "something changed". Keeping it empty is also what keeps the fan-out
	// free of personal data — there is no field for a name or an availability to creep
	// into later.
	channel := channelName(topic)

	if err := b.publisher.Publish(ctx, channel, "").Err(); err != nil {
		// A Redis outage must not fail the write that triggered this. Deliver locally so
		// this instance's viewers still update, and say so once rather than silently.
		//
		// The topic is the calendar token, so the line carries a per-process fingerprint
		// of it instead: enough to see that a run of failures is about one calendar,
		// useless to anyone reading the log.
		b.logger.Warn("broadcast: publishing through Redis failed, delivering locally only",
			"calendar_ref", logger.Fingerprint(topic), "error", err)
		b.deliver(channel)

		return fmt.Errorf("failed to publish: %w", err)
	}

	return nil
}

func (b *redisBroker) Subscribe(ctx context.Context, topic string) (<-chan struct{}, func()) {
	return b.hub.subscribe(ctx, channelName(topic))
}

func (b *redisBroker) Close() error {
	var err error
	b.closeOnce.Do(func() {
		close(b.done)
		if b.pubsub != nil {
			err = b.pubsub.Close()
		}
		b.hub.close()
	})

	return err
}
