// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"path/filepath"
	"testing"
)

// setupTestActionRepository creates a test ActionRepository with seeded data
func setupTestActionRepository(t *testing.T) *ActionRepository {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize database
	db, err := initializeSQLite(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}

	// Auto-migrate the actions table
	if err := db.AutoMigrate(&Action{}); err != nil {
		t.Fatalf("failed to migrate actions table: %v", err)
	}

	// Seed some test actions
	testActions := []Action{
		{Action: "component:create", IsInternal: false},
		{Action: "component:read", IsInternal: false},
		{Action: "component:update", IsInternal: false},
		{Action: "component:delete", IsInternal: false},
		{Action: "component:*", IsInternal: false},
		{Action: "project:view", IsInternal: false},
		{Action: "project:create", IsInternal: false},
		{Action: "*", IsInternal: false},
		{Action: "internal:secret", IsInternal: true},
		{Action: "internal:*", IsInternal: true},
		{Action: "namespace:view", IsInternal: false},
	}

	for _, action := range testActions {
		if err := db.Create(&action).Error; err != nil {
			t.Fatalf("failed to seed test action %s: %v", action.Action, err)
		}
	}

	return NewActionRepository(db)
}

// TestActionRepository_ListPublicActions tests listing public actions
func TestActionRepository_ListPublicActions(t *testing.T) {
	repo := setupTestActionRepository(t)

	t.Run("list all public actions", func(t *testing.T) {
		actions, err := repo.ListPublicActions()
		if err != nil {
			t.Fatalf("ListPublicActions() error = %v", err)
		}

		if len(actions) != 9 {
			t.Errorf("ListPublicActions() returned %d actions, want 10", len(actions))
		}

		// Verify internal actions are excluded
		for _, action := range actions {
			if action.IsInternal {
				t.Errorf("ListPublicActions() returned internal action: %s", action.Action)
			}
		}
	})
}

// TestActionRepository_ListConcretePublicActions tests listing concrete public actions
func TestActionRepository_ListConcretePublicActions(t *testing.T) {
	repo := setupTestActionRepository(t)

	t.Run("list all concrete public actions", func(t *testing.T) {
		actions, err := repo.ListConcretePublicActions()
		if err != nil {
			t.Fatalf("ListConcretePublicActions() error = %v", err)
		}

		if len(actions) != 7 {
			t.Errorf("ListConcretePublicActions() returned %d actions, want 8", len(actions))
		}

		// Verify no wildcarded actions are included
		for _, action := range actions {
			if action.Action == "*" || action.Action == "component:*" || action.Action == "internal:*" {
				t.Errorf("ListConcretePublicActions() returned wildcarded action: %s", action.Action)
			}
		}

		// Verify internal actions are excluded
		for _, action := range actions {
			if action.IsInternal {
				t.Errorf("ListConcretePublicActions() returned internal action: %s", action.Action)
			}
		}
	})
}
