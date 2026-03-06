// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labelcache

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// entry represents a cached set of labels with an expiration time.
type entry struct {
	labels   map[string]string
	notFound bool
	expireAt time.Time
}

// Cache is a thread-safe in-memory cache for Kubernetes object labels.
// Keys are in the format "namespace/kind/name".
type Cache struct {
	mu      sync.RWMutex
	entries map[string]entry
	ttl     time.Duration
	logger  *slog.Logger
}

// New creates a new labels cache with the given TTL.
func New(ttl time.Duration, logger *slog.Logger) *Cache {
	if logger == nil {
		logger = slog.Default()
	}

	return &Cache{
		entries: make(map[string]entry),
		ttl:     ttl,
		logger:  logger,
	}
}

// Key builds a cache key from namespace, kind, and name.
func Key(namespace, kind, name string) string {
	return namespace + "/" + kind + "/" + name
}

// Get retrieves labels from the cache. Returns the labels and true if found
// and not expired; nil and false otherwise.
func (c *Cache) Get(key string) (map[string]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(e.expireAt) {
		return nil, false
	}

	if e.notFound {
		// Return empty labels but indicate that we have a cached result
		// (the object was not found, so we don't need to look it up again)
		return nil, true
	}

	return e.labels, true
}

// Set stores labels in the cache with the configured TTL.
func (c *Cache) Set(key string, labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry{
		labels:   labels,
		expireAt: time.Now().Add(c.ttl),
	}
}

// SetNotFound caches a "not found" marker so we avoid repeated API calls
// for objects that no longer exist (e.g., deleted ReplicaSets).
func (c *Cache) SetNotFound(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry{
		notFound: true,
		expireAt: time.Now().Add(c.ttl),
	}
}

// StartEviction runs a periodic eviction loop to remove expired entries.
// It blocks until the context is cancelled.
func (c *Cache) StartEviction(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		if c.logger != nil {
			c.logger.Error("invalid label cache eviction interval", "interval", interval)
		}
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.evict()
		}
	}
}

func (c *Cache) evict() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	evicted := 0
	for key, e := range c.entries {
		if now.After(e.expireAt) {
			delete(c.entries, key)
			evicted++
		}
	}
	if evicted > 0 {
		c.logger.Debug("evicted expired label cache entries", "count", evicted)
	}
}
