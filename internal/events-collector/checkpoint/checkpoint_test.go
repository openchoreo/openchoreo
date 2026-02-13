// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer store.Close()

	// Verify the database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("database file was not created at %s", dbPath)
	}

	// Verify the events table exists by attempting an insert
	_, err = store.Add("test-uid")
	if err != nil {
		t.Errorf("Add() failed after New(), table may not exist: %v", err)
	}
}

func TestNew_InvalidPath(t *testing.T) {
	// Try to create a DB in a non-existent directory
	dbPath := "/nonexistent/path/to/db.sqlite"

	_, err := New(dbPath)
	if err == nil {
		t.Errorf("New() should return error for invalid path")
	}
}

func TestStore_Add(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	inserted, err := store.Add("event-1")
	if err != nil {
		t.Fatalf("Add() returned error: %v", err)
	}
	if !inserted {
		t.Errorf("Add() returned false for new event, expected true")
	}
}

func TestStore_Add_Duplicate(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// First insert
	inserted, err := store.Add("event-1")
	if err != nil {
		t.Fatalf("First Add() returned error: %v", err)
	}
	if !inserted {
		t.Errorf("First Add() should return true")
	}

	// Second insert of same UID
	inserted, err = store.Add("event-1")
	if err != nil {
		t.Fatalf("Second Add() returned error: %v", err)
	}
	if inserted {
		t.Errorf("Second Add() should return false for duplicate")
	}
}

func TestStore_Add_MultipleEvents(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	eventUIDs := []string{"event-1", "event-2", "event-3", "event-4", "event-5"}

	for _, uid := range eventUIDs {
		inserted, err := store.Add(uid)
		if err != nil {
			t.Fatalf("Add(%q) returned error: %v", uid, err)
		}
		if !inserted {
			t.Errorf("Add(%q) returned false, expected true", uid)
		}
	}

	// Verify all events exist
	for _, uid := range eventUIDs {
		exists, err := store.Exists(uid)
		if err != nil {
			t.Fatalf("Exists(%q) returned error: %v", uid, err)
		}
		if !exists {
			t.Errorf("Exists(%q) returned false, expected true", uid)
		}
	}
}

func TestStore_Exists(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Add an event
	_, err := store.Add("existing-event")
	if err != nil {
		t.Fatalf("Add() returned error: %v", err)
	}

	tests := []struct {
		name     string
		eventUID string
		want     bool
	}{
		{
			name:     "existing event",
			eventUID: "existing-event",
			want:     true,
		},
		{
			name:     "non-existing event",
			eventUID: "non-existing-event",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := store.Exists(tt.eventUID)
			if err != nil {
				t.Fatalf("Exists() returned error: %v", err)
			}
			if exists != tt.want {
				t.Errorf("Exists(%q) = %v, want %v", tt.eventUID, exists, tt.want)
			}
		})
	}
}

func TestStore_Exists_Empty(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	exists, err := store.Exists("any-event")
	if err != nil {
		t.Fatalf("Exists() returned error: %v", err)
	}
	if exists {
		t.Errorf("Exists() on empty DB returned true, expected false")
	}
}

func TestStore_CleanupBefore(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	// Add events at different times by directly inserting with controlled timestamps
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)
	recentTime := now.Add(-30 * time.Minute)

	// Insert old events directly
	_, err := store.db.Exec("INSERT INTO events (event_uid, timestamp) VALUES (?, ?)", "old-event-1", oldTime.Unix())
	if err != nil {
		t.Fatalf("Failed to insert old event: %v", err)
	}
	_, err = store.db.Exec("INSERT INTO events (event_uid, timestamp) VALUES (?, ?)", "old-event-2", oldTime.Unix())
	if err != nil {
		t.Fatalf("Failed to insert old event: %v", err)
	}

	// Insert recent events
	_, err = store.db.Exec("INSERT INTO events (event_uid, timestamp) VALUES (?, ?)", "recent-event-1", recentTime.Unix())
	if err != nil {
		t.Fatalf("Failed to insert recent event: %v", err)
	}

	// Cleanup events older than 1 hour ago
	cutoff := now.Add(-1 * time.Hour)
	deleted, err := store.CleanupBefore(cutoff)
	if err != nil {
		t.Fatalf("CleanupBefore() returned error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("CleanupBefore() deleted %d records, expected 2", deleted)
	}

	// Verify old events are gone
	exists, _ := store.Exists("old-event-1")
	if exists {
		t.Errorf("old-event-1 should have been deleted")
	}
	exists, _ = store.Exists("old-event-2")
	if exists {
		t.Errorf("old-event-2 should have been deleted")
	}

	// Verify recent event still exists
	exists, _ = store.Exists("recent-event-1")
	if !exists {
		t.Errorf("recent-event-1 should still exist")
	}
}

func TestStore_CleanupBefore_Empty(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	deleted, err := store.CleanupBefore(time.Now())
	if err != nil {
		t.Fatalf("CleanupBefore() returned error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupBefore() on empty DB returned %d, expected 0", deleted)
	}
}

func TestStore_Close(t *testing.T) {
	store := newTestStore(t)

	err := store.Close()
	if err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	// Operations should fail after close
	_, err = store.Add("test-event")
	if err == nil {
		t.Errorf("Add() should fail after Close()")
	}
}

func TestStore_Close_NilDB(t *testing.T) {
	// Test Close() when db is nil
	store := &Store{db: nil}

	err := store.Close()
	if err != nil {
		t.Errorf("Close() with nil db should not return error, got: %v", err)
	}
}

// newTestStore creates a new Store with an in-memory SQLite database for testing.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	return store
}
