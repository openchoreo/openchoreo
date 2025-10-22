// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"container/list"
	"sort"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
)

// EngineOption configures cache behavior for the template engine.
// Primarily used for testing and benchmarking different cache strategies.
type EngineOption func(*EngineCache)

// DisableCache disables all caching (both environment and program caches).
// Use this for benchmarking to measure the cost of caching vs compilation.
//
// Example:
//
//	engine := template.NewEngineWithOptions(template.DisableCache())
func DisableCache() EngineOption {
	return func(cache *EngineCache) {
		cache.envCacheDisabled = true
		cache.progCacheDisabled = true
	}
}

// DisableProgramCacheOnly disables only the program cache, keeping environment cache enabled.
// Use this to measure the impact of program compilation caching separately from environment caching.
//
// Example:
//
//	engine := template.NewEngineWithOptions(template.DisableProgramCacheOnly())
func DisableProgramCacheOnly() EngineOption {
	return func(cache *EngineCache) {
		cache.progCacheDisabled = true
	}
}

// Default cache sizes - limits memory usage while providing good hit rates
const (
	defaultEnvCacheSize = 100 // CEL environments (typically 2-10 unique contexts)
	// Compiled programs: ~875 expected for typical deployment
	// Calculation: (5 CTDs × ~25 expressions) + (50 addons × ~15 expressions) = ~875
	// 2000 limit provides 2.3x headroom
	defaultProgramCacheSize = 2000
)

// lruCache implements a simple thread-safe generic LRU cache with a maximum size.
type lruCache[T any] struct {
	mu        sync.Mutex
	maxSize   int
	items     map[string]*list.Element
	evictList *list.List
}

type cacheEntry[T any] struct {
	key   string
	value T
}

func newLRUCache[T any](maxSize int) *lruCache[T] {
	return &lruCache[T]{
		maxSize:   maxSize,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}
}

func (c *lruCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evictList.MoveToFront(elem)
		return elem.Value.(*cacheEntry[T]).value, true
	}
	var zero T
	return zero, false
}

func (c *lruCache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if elem, ok := c.items[key]; ok {
		c.evictList.MoveToFront(elem)
		elem.Value.(*cacheEntry[T]).value = value
		return
	}

	// Add new entry
	entry := &cacheEntry[T]{key: key, value: value}
	elem := c.evictList.PushFront(entry)
	c.items[key] = elem

	// Evict oldest if over capacity
	if c.evictList.Len() > c.maxSize {
		oldest := c.evictList.Back()
		if oldest != nil {
			c.evictList.Remove(oldest)
			oldEntry := oldest.Value.(*cacheEntry[T])
			delete(c.items, oldEntry.key)
		}
	}
}

// EngineCache provides caching for CEL environments and compiled programs.
// It maintains two levels of caching:
// - Environment cache: LRU cache of CEL environments by variable names
// - Program cache: LRU cache of compiled programs by (env, expression)
//
// Cache Architecture:
//
//	Level 1: ENV Cache (LRU, max 100 entries)
//	  └─ envKey: ["addon", "metadata", ..."] → CEL Environment
//
//	Level 2: PROGRAM Cache (LRU, max 2000 entries)
//	  └─ (envKey + expression) → Compiled CEL Program
//
// For a deployment with 5 CTDs and 50 addons, expect ~875 cached programs.
// The 2000 entry limit provides 2x headroom and protects against edge cases
// like dynamic template updates or multi-tenancy scenarios.
type EngineCache struct {
	envCache          *lruCache[*cel.Env]
	programCache      *lruCache[cel.Program]
	envCacheDisabled  bool
	progCacheDisabled bool
}

// NewEngineCache creates a new cache with the default cache sizes.
func NewEngineCache() *EngineCache {
	return &EngineCache{
		envCache:     newLRUCache[*cel.Env](defaultEnvCacheSize),
		programCache: newLRUCache[cel.Program](defaultProgramCacheSize),
	}
}

// NewEngineCacheWithOptions creates a new cache with custom options.
// This is primarily used for benchmarking different cache strategies.
func NewEngineCacheWithOptions(opts ...EngineOption) *EngineCache {
	cache := &EngineCache{}

	// Apply options
	for _, opt := range opts {
		opt(cache)
	}

	// Only create caches if they're not disabled
	if !cache.envCacheDisabled {
		cache.envCache = newLRUCache[*cel.Env](defaultEnvCacheSize)
	}
	if !cache.progCacheDisabled {
		cache.programCache = newLRUCache[cel.Program](defaultProgramCacheSize)
	}

	return cache
}

// GetEnv retrieves a cached CEL environment by its cache key.
// Returns (nil, false) if caching is disabled.
func (c *EngineCache) GetEnv(key string) (*cel.Env, bool) {
	if c.envCacheDisabled || c.envCache == nil {
		return nil, false
	}
	return c.envCache.Get(key)
}

// SetEnv stores a CEL environment in the cache.
// No-op if caching is disabled.
func (c *EngineCache) SetEnv(key string, env *cel.Env) {
	if c.envCacheDisabled || c.envCache == nil {
		return
	}
	c.envCache.Set(key, env)
}

// GetProgram retrieves a cached compiled CEL program.
// Returns (nil, false) if caching is disabled.
func (c *EngineCache) GetProgram(envKey, expression string) (cel.Program, bool) {
	if c.progCacheDisabled || c.programCache == nil {
		return nil, false
	}
	key := programCacheKey(envKey, expression)
	return c.programCache.Get(key)
}

// SetProgram stores a compiled CEL program in the cache.
// No-op if caching is disabled.
func (c *EngineCache) SetProgram(envKey, expression string, program cel.Program) {
	if c.progCacheDisabled || c.programCache == nil {
		return
	}
	key := programCacheKey(envKey, expression)
	c.programCache.Set(key, program)
}

// ProgramCacheSize returns the number of entries in the program cache.
// Returns 0 if caching is disabled. Useful for testing and monitoring cache effectiveness.
func (c *EngineCache) ProgramCacheSize() int {
	if c.progCacheDisabled || c.programCache == nil {
		return 0
	}
	c.programCache.mu.Lock()
	defer c.programCache.mu.Unlock()
	return len(c.programCache.items)
}

// programCacheKey creates a composite key for caching compiled CEL programs.
// Combines the environment cache key with the expression to ensure programs
// are only reused when both the variable declarations and expression match.
func programCacheKey(envKey, expression string) string {
	return envKey + "\x1e" + expression
}

// envCacheKey generates a cache key based on the top-level variable names in the input map.
// The key is independent of variable values, allowing environments to be reused across
// different input data as long as the variable structure matches.
//
// For example, both of these inputs produce the same cache key:
//   - {"metadata": {"name": "app1"}, "parameters": {"replicas": 1}}
//   - {"metadata": {"name": "app2"}, "parameters": {"replicas": 3}}
//
// This enables high cache hit rates in controller scenarios where the same CTD/addon
// templates are applied to different components with varying values.
func envCacheKey(inputs map[string]any) string {
	if len(inputs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x1f")
}
