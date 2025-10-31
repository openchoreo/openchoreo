// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/exp/slog"
)

// FeatureFlags contains feature flag configuration
type FeatureFlags struct {
	CursorPaginationEnabled bool `json:"cursor_pagination_enabled"`
	// Add other feature flags here as needed
}

// Config contains the complete API configuration
type Config struct {
	Features FeatureFlags `json:"features"`
	// Add other configuration sections here
}

var (
	globalConfig     *Config
	configMutex      sync.RWMutex
	lastLoadTime     time.Time
	cacheTTL         = 5 * time.Minute // Cache config for 5 minutes
	reloadInProgress atomic.Bool
	reloadWaitGroup  sync.WaitGroup
)

// LoadFeatureFlags loads the feature flags configuration
func LoadFeatureFlags() (*Config, error) {
	// Fast path: Check cache with read lock
	configMutex.RLock()
	cachedConfig := globalConfig
	loadTime := lastLoadTime
	configMutex.RUnlock()

	// Return cached config if still fresh
	if cachedConfig != nil && time.Since(loadTime) < cacheTTL {
		return cachedConfig, nil
	}

	// SECURITY: Use atomic CAS to ensure only ONE reload happens
	// This prevents thundering herd DoS during cache expiration
	if !reloadInProgress.CompareAndSwap(false, true) {
		// Another goroutine is reloading, wait for it
		reloadWaitGroup.Wait()

		// Re-check cache after waiting
		configMutex.RLock()
		reloadedConfig := globalConfig
		configMutex.RUnlock()
		return reloadedConfig, nil
	}

	// We won the race, we're responsible for reloading
	reloadWaitGroup.Add(1)
	defer reloadWaitGroup.Done()        // CRITICAL: Defer immediately after Add to prevent deadlock on panic
	defer reloadInProgress.Store(false) // Execute before Done() due to LIFO defer order

	// Acquire write lock for reload
	configMutex.Lock()
	defer configMutex.Unlock()

	// Final check after acquiring write lock (might have been updated)
	if globalConfig != nil && time.Since(lastLoadTime) < cacheTTL {
		return globalConfig, nil
	}

	config := &Config{
		Features: FeatureFlags{
			CursorPaginationEnabled: false, // Default to safe legacy mode
		},
	}

	// SECURITY: Add timeout for file operations to prevent DoS
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Load from config file if it exists
	if err := loadFromFileWithContext(ctx, "config/flags.json", config); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Warn("config file not loaded, using defaults/env vars",
				"error", err,
				"file", "config/flags.json")
		} else if errors.Is(err, context.DeadlineExceeded) {
			slog.Error("config file load timeout, using defaults/env vars",
				"error", err,
				"file", "config/flags.json")
		} else {
			return config, fmt.Errorf("load feature flags from file: %w", err)
		}
	} else {
		slog.Info("feature flags loaded from file", "file", "config/flags.json")
	}

	// Environment variables override file configuration
	if envValue, ok := os.LookupEnv("CURSOR_PAGINATION_ENABLED"); ok {
		config.Features.CursorPaginationEnabled = envValue == "true"
	}

	globalConfig = config
	lastLoadTime = time.Now()

	return config, nil
}

// loadFromFileWithContext loads configuration from a JSON file with timeout protection
func loadFromFileWithContext(ctx context.Context, filename string, config *Config) error {
	type result struct {
		data []byte
		err  error
	}

	resultChan := make(chan result, 1)

	go func() {
		// Check context before expensive operation to avoid unnecessary work
		select {
		case <-ctx.Done():
			resultChan <- result{err: ctx.Err()}
			return
		default:
		}

		data, err := os.ReadFile(filename)

		// Use select to prevent goroutine leak if context is cancelled
		select {
		case resultChan <- result{data: data, err: err}:
		case <-ctx.Done():
			// Don't block if context cancelled while sending
		}
	}()

	select {
	case res := <-resultChan:
		if res.err != nil {
			return res.err
		}
		return json.Unmarshal(res.data, config)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetCursorPaginationEnabled returns the current state of the cursor pagination flag
func GetCursorPaginationEnabled() bool {
	config, err := LoadFeatureFlags()
	if err != nil {
		// Fail safe to disabled if config loading fails
		return false
	}
	return config.Features.CursorPaginationEnabled
}

// InvalidateCache forces a reload of the configuration on next access
func InvalidateCache() {
	configMutex.Lock()
	defer configMutex.Unlock()

	// Invalidate timestamp first to prevent readers from seeing nil config with valid timestamp
	lastLoadTime = time.Time{}
	globalConfig = nil
}
