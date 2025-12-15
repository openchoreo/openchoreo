// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestNewAdapter verifies that the adapter is created successfully,
// performs database migrations, and seeds initial data
func TestNewAdapter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Verify database doesn't exist yet
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("database file should not exist before creation")
	}

	adapter, db, err := newAdapter(dbPath, "", logger)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	if adapter == nil {
		t.Fatal("adapter is nil")
	}
	if db == nil {
		t.Fatal("db is nil")
	}

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file should exist after creation")
	}

	// Verify table exists and has seeded data
	var count int64
	result := db.Model(&CasbinRule{}).Where("ptype = ?", "g").Count(&count)
	if result.Error != nil {
		t.Fatalf("failed to query casbin_rules table: %v", result.Error)
	}

	// Count should be > 0 because seeding happens automatically
	if count == 0 {
		t.Fatal("expected at least one role to be seeded, got 0")
	}
}

// TestSeedInitialData_Idempotent verifies that seeding is idempotent
// Running seed multiple times should not create duplicate records
func TestSeedInitialData_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// First seeding (happens during adapter creation)
	_, db, err := newAdapter(dbPath, "", logger)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	// Count records after first seed
	var countAfterFirst int64
	db.Model(&CasbinRule{}).Where("ptype = ?", "g").Count(&countAfterFirst)

	// Run seeding again manually
	if err := seedInitialData(db, "", logger); err != nil {
		t.Fatalf("failed to seed data second time: %v", err)
	}

	// Count records after second seed
	var countAfterSecond int64
	db.Model(&CasbinRule{}).Where("ptype = ?", "g").Count(&countAfterSecond)

	// Counts should be identical (no duplicates)
	if countAfterFirst != countAfterSecond {
		t.Errorf("seeding is not idempotent: first=%d, second=%d", countAfterFirst, countAfterSecond)
	}
}
