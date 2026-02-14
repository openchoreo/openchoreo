// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkpoint

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManager_Start_PeriodicCleanup(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Insert an old event (2 hours ago)
	oldTime := time.Now().Add(-2 * time.Hour)
	_, err := store.db.Exec("INSERT INTO events (event_uid, timestamp) VALUES (?, ?)", "old-event", oldTime.Unix())
	if err != nil {
		t.Fatalf("Failed to insert old event: %v", err)
	}

	// Insert a recent event (30 minutes ago)
	recentTime := time.Now().Add(-30 * time.Minute)
	_, err = store.db.Exec("INSERT INTO events (event_uid, timestamp) VALUES (?, ?)", "recent-event", recentTime.Unix())
	if err != nil {
		t.Fatalf("Failed to insert recent event: %v", err)
	}

	// Create manager with short intervals for testing
	// TTL of 1 hour means events older than 1 hour will be cleaned up
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(store, 50*time.Millisecond, 1*time.Hour, logger)

	// Start manager in background
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manager.Start(ctx)
		close(done)
	}()

	// Wait for at least one cleanup cycle
	time.Sleep(100 * time.Millisecond)

	// Cancel context and wait for manager to stop
	cancel()
	<-done

	// Verify old event was cleaned up
	exists, err := store.Exists("old-event")
	if err != nil {
		t.Fatalf("Exists() returned error: %v", err)
	}
	if exists {
		t.Errorf("old-event should have been cleaned up")
	}

	// Verify recent event still exists
	exists, err = store.Exists("recent-event")
	if err != nil {
		t.Fatalf("Exists() returned error: %v", err)
	}
	if !exists {
		t.Errorf("recent-event should still exist")
	}
}

func TestManager_Start_InvalidConfig(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name     string
		interval time.Duration
		ttl      time.Duration
	}{
		{
			name:     "zero interval",
			interval: 0,
			ttl:      1 * time.Hour,
		},
		{
			name:     "negative interval",
			interval: -1 * time.Second,
			ttl:      1 * time.Hour,
		},
		{
			name:     "zero ttl",
			interval: 1 * time.Minute,
			ttl:      0,
		},
		{
			name:     "negative ttl",
			interval: 1 * time.Minute,
			ttl:      -1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(store, tt.interval, tt.ttl, logger)

			// Start should return immediately for invalid config
			ctx := context.Background()
			done := make(chan struct{})
			go func() {
				manager.Start(ctx)
				close(done)
			}()

			// Should return quickly (within 100ms)
			select {
			case <-done:
				// Expected - manager returned immediately
			case <-time.After(200 * time.Millisecond):
				t.Errorf("Manager.Start() did not return immediately for invalid config")
			}
		})
	}
}

func TestManager_Start_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(store, 1*time.Second, 1*time.Hour, logger)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		manager.Start(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	// Manager should stop quickly
	select {
	case <-done:
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Manager.Start() did not stop after context cancellation")
	}
}
