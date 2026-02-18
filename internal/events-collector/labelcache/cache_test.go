// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labelcache

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const testKeyDefaultPod = "default/Pod/my-pod"

func TestKey(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		kind      string
		objName   string
		want      string
	}{
		{
			name:      "standard pod",
			namespace: "default",
			kind:      "Pod",
			objName:   "my-pod",
			want:      "default/Pod/my-pod",
		},
		{
			name:      "deployment in custom namespace",
			namespace: "production",
			kind:      "Deployment",
			objName:   "web-server",
			want:      "production/Deployment/web-server",
		},
		{
			name:      "empty namespace (cluster-scoped)",
			namespace: "",
			kind:      "Node",
			objName:   "node-1",
			want:      "/Node/node-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Key(tt.namespace, tt.kind, tt.objName)
			if got != tt.want {
				t.Errorf("Key(%q, %q, %q) = %q, want %q", tt.namespace, tt.kind, tt.objName, got, tt.want)
			}
		})
	}
}

func TestCache_SetAndGet(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	labels := map[string]string{
		"openchoreo.dev/component": "my-component",
		"openchoreo.dev/project":   "my-project",
	}

	key := testKeyDefaultPod
	cache.Set(key, labels)

	got, found := cache.Get(key)
	if !found {
		t.Errorf("Get(%q) found = false, want true", key)
	}
	if diff := cmp.Diff(labels, got); diff != "" {
		t.Errorf("Get(%q) mismatch (-want +got):\n%s", key, diff)
	}
}

func TestCache_Get_Miss(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	got, found := cache.Get("nonexistent/Pod/unknown")
	if found {
		t.Errorf("Get() found = true for unknown key, want false")
	}
	if got != nil {
		t.Errorf("Get() = %v for unknown key, want nil", got)
	}
}

func TestCache_Get_Expired(t *testing.T) {
	// Use very short TTL
	cache := newTestCache(t, 10*time.Millisecond)

	key := testKeyDefaultPod
	cache.Set(key, map[string]string{"key": "value"})

	// Wait for entry to expire
	time.Sleep(20 * time.Millisecond)

	got, found := cache.Get(key)
	if found {
		t.Errorf("Get() found = true for expired entry, want false")
	}
	if got != nil {
		t.Errorf("Get() = %v for expired entry, want nil", got)
	}
}

func TestCache_SetNotFound(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	key := "default/Pod/deleted-pod"
	cache.SetNotFound(key)

	// Get should return nil labels but found=true (indicating we have a cached result)
	got, found := cache.Get(key)
	if !found {
		t.Errorf("Get() found = false after SetNotFound, want true")
	}
	if got != nil {
		t.Errorf("Get() = %v after SetNotFound, want nil", got)
	}
}

func TestCache_SetNotFound_Expired(t *testing.T) {
	cache := newTestCache(t, 10*time.Millisecond)

	key := "default/Pod/deleted-pod"
	cache.SetNotFound(key)

	// Wait for entry to expire
	time.Sleep(20 * time.Millisecond)

	_, found := cache.Get(key)
	if found {
		t.Errorf("Get() found = true for expired not-found marker, want false")
	}
}

func TestCache_Set_Overwrite(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	key := testKeyDefaultPod

	// Set initial labels
	cache.Set(key, map[string]string{"version": "v1"})

	// Overwrite with new labels
	newLabels := map[string]string{"version": "v2"}
	cache.Set(key, newLabels)

	got, found := cache.Get(key)
	if !found {
		t.Errorf("Get() found = false after overwrite, want true")
	}
	if diff := cmp.Diff(newLabels, got); diff != "" {
		t.Errorf("Get() after overwrite mismatch (-want +got):\n%s", diff)
	}
}

func TestCache_Set_NilLabels(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	key := "default/Pod/no-labels"
	cache.Set(key, nil)

	got, found := cache.Get(key)
	if !found {
		t.Errorf("Get() found = false for nil labels, want true")
	}
	if got != nil {
		t.Errorf("Get() = %v for nil labels, want nil", got)
	}
}

func TestCache_Evict(t *testing.T) {
	cache := newTestCache(t, 10*time.Millisecond)

	// Add some entries
	cache.Set("key1", map[string]string{"a": "1"})
	cache.Set("key2", map[string]string{"b": "2"})

	// Wait for entries to expire
	time.Sleep(20 * time.Millisecond)

	// Add a fresh entry
	cache.Set("key3", map[string]string{"c": "3"})

	// Run eviction
	cache.evict()

	// Expired entries should be gone
	if _, found := cache.Get("key1"); found {
		t.Errorf("key1 should have been evicted")
	}
	if _, found := cache.Get("key2"); found {
		t.Errorf("key2 should have been evicted")
	}

	// Fresh entry should still exist
	if _, found := cache.Get("key3"); !found {
		t.Errorf("key3 should still exist")
	}
}

func TestCache_Evict_Empty(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	// Should not panic on empty cache
	cache.evict()
}

func TestCache_StartEviction_InvalidInterval(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	tests := []struct {
		name     string
		interval time.Duration
	}{
		{
			name:     "zero interval",
			interval: 0,
		},
		{
			name:     "negative interval",
			interval: -1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			done := make(chan struct{})
			go func() {
				cache.StartEviction(ctx, tt.interval)
				close(done)
			}()

			select {
			case <-done:
				// Expected - should return immediately
			case <-time.After(200 * time.Millisecond):
				t.Errorf("StartEviction did not return immediately for invalid interval")
			}
		})
	}
}

func TestCache_StartEviction_ContextCancellation(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		cache.StartEviction(ctx, 100*time.Millisecond)
		close(done)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Errorf("StartEviction did not stop after context cancellation")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cache := newTestCache(t, 1*time.Hour)

	// Run multiple goroutines reading and writing concurrently
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := Key("ns", "Pod", "pod")
				labels := map[string]string{"id": "value"}

				// Mix of operations
				switch j % 4 {
				case 0:
					cache.Set(key, labels)
				case 1:
					cache.Get(key)
				case 2:
					cache.SetNotFound(key)
				case 3:
					cache.evict()
				}
			}
		}()
	}

	wg.Wait()
	// If we get here without a race or panic, the test passes
}

func TestCache_New_NilLogger(t *testing.T) {
	// Test that New() handles nil logger gracefully by using default
	cache := New(1*time.Hour, nil)

	if cache == nil {
		t.Fatal("New() returned nil")
	}
	if cache.logger == nil {
		t.Error("logger should be set to default, not nil")
	}

	// Test basic functionality works
	cache.Set("test-key", map[string]string{"k": "v"})
	labels, found := cache.Get("test-key")
	if !found {
		t.Error("Get() should find the key")
	}
	if labels == nil || labels["k"] != "v" {
		t.Error("Get() returned wrong labels")
	}
}

// newTestCache creates a new Cache for testing with the given TTL.
func newTestCache(t *testing.T, ttl time.Duration) *Cache {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return New(ttl, logger)
}
