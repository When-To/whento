// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package broadcast

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

// channelPrefix namespaces the pub/sub channels, so a Redis shared with something else
// does not collide.
const channelPrefix = "whento:broadcast:"

// redisBroker reaches the subscribers of every instance.
//
// One pattern subscription rather than one per topic: a deployment serving a thousand
// calendars would otherwise hold a thousand Redis subscriptions, and the traffic here is
// small enough that filtering locally costs nothing. Everything arriving on the pattern
// is handed to the same process-local hub the memory broker uses, so delivery semantics
// are identical whichever broker is in play.
type redisBroker struct {
	client *redis.Client
	hub    *hub
	pubsub *redis.PubSub
	logger *slog.Logger

	closeOnce sync.Once
	done      chan struct{}
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
		client: client,
		hub:    newHub(),
		logger: logger,
		done:   make(chan struct{}),
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
			b.hub.publish(strings.TrimPrefix(message.Channel, channelPrefix))
		}
	}
}

func (b *redisBroker) Publish(ctx context.Context, topic string) error {
	// The payload is unused: the channel name carries the topic and the notice itself
	// says only "something changed".
	if err := b.client.Publish(ctx, channelPrefix+topic, "").Err(); err != nil {
		// A Redis outage must not fail the write that triggered this. Deliver locally so
		// this instance's viewers still update, and say so once rather than silently.
		b.logger.Warn("broadcast: publishing through Redis failed, delivering locally only",
			"topic", topic, "error", err)
		b.hub.publish(topic)

		return fmt.Errorf("failed to publish: %w", err)
	}

	return nil
}

func (b *redisBroker) Subscribe(ctx context.Context, topic string) (<-chan struct{}, func()) {
	return b.hub.subscribe(ctx, topic)
}

func (b *redisBroker) Close() error {
	var err error
	b.closeOnce.Do(func() {
		close(b.done)
		err = b.pubsub.Close()
		b.hub.close()
	})

	return err
}
