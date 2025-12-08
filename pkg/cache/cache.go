// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache defines the interface for caching operations
type Cache interface {
	// Get retrieves a value from cache and unmarshals it into dest
	Get(ctx context.Context, key string, dest interface{}) error

	// Set stores a value in cache with the given TTL
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	// Delete removes a value from cache
	Delete(ctx context.Context, keys ...string) error

	// Exists checks if a key exists in cache
	Exists(ctx context.Context, key string) (bool, error)

	// IsEnabled returns true if caching is enabled
	IsEnabled() bool
}

// RedisCache implements Cache using Redis
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis-backed cache
func NewRedisCache(client *redis.Client) Cache {
	if client == nil {
		return &NoOpCache{}
	}
	return &RedisCache{client: client}
}

// Get retrieves a value from Redis and unmarshals it
func (c *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(val), dest)
}

// Set stores a value in Redis with the given TTL
func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, ttl).Err()
}

// Delete removes values from Redis
func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists in Redis
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	return n > 0, err
}

// IsEnabled returns true for RedisCache
func (c *RedisCache) IsEnabled() bool {
	return true
}

// NoOpCache is a cache that does nothing (when Redis is not available)
type NoOpCache struct{}

// Get always returns an error indicating cache miss
func (c *NoOpCache) Get(ctx context.Context, key string, dest interface{}) error {
	return redis.Nil // Return cache miss error
}

// Set does nothing
func (c *NoOpCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}

// Delete does nothing
func (c *NoOpCache) Delete(ctx context.Context, keys ...string) error {
	return nil
}

// Exists always returns false
func (c *NoOpCache) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}

// IsEnabled returns false for NoOpCache
func (c *NoOpCache) IsEnabled() bool {
	return false
}
