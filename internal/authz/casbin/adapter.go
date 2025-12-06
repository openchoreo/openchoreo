// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"fmt"
	"log/slog"

	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openchoreo/openchoreo/internal/authz/data"
)

// CasbinRule defines the custom schema for Casbin policy storage
// The unique index ensures no duplicate rules can be created, enabling atomic conflict resolution
type CasbinRule struct {
	ID    uint   `gorm:"primaryKey;autoIncrement"`
	Ptype string `gorm:"type:text;uniqueIndex:idx_casbin_rule"` // Policy type: p (policy) or g (grouping/role)
	V0    string `gorm:"type:text;uniqueIndex:idx_casbin_rule"` // p - entitlement, g - role
	V1    string `gorm:"type:text;uniqueIndex:idx_casbin_rule"` // p - resource path, g - action
	V2    string `gorm:"type:text;uniqueIndex:idx_casbin_rule"` // p - role name
	V3    string `gorm:"type:text;uniqueIndex:idx_casbin_rule"` // p - effect (allow/deny)
	V4    string `gorm:"type:text;uniqueIndex:idx_casbin_rule"` // p - context info
	V5    string `gorm:"type:text;uniqueIndex:idx_casbin_rule"` // extra field
}

// Action defines the schema for storing available actions
type Action struct {
	ID     uint   `gorm:"primaryKey;autoIncrement"`
	Action string `gorm:"type:text;uniqueIndex;not null"`
}

// InitializeSQLite initializes a SQLite database connection
func initializeSQLite(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	return db, nil
}

// NewAdapter creates a new gorm adapter with custom schema for Casbin
// It initializes SQLite and auto-migrates the casbin_rule and actions tables
// Returns the adapter and DB instance for use with ActionRepository
func newAdapter(dbPath string, rolesFilePath string, logger *slog.Logger) (*gormadapter.Adapter, *gorm.DB, error) {
	// Initialize SQLite database
	db, err := initializeSQLite(dbPath)
	if err != nil {
		return nil, nil, err
	}

	// Auto-migrate the actions table only
	// The gorm adapter will handle the casbin_rule table migration
	if err := db.AutoMigrate(&Action{}); err != nil {
		return nil, nil, fmt.Errorf("failed to auto-migrate tables: %w", err)
	}

	// Create the gorm adapter with custom CasbinRule schema
	adapter, err := gormadapter.NewAdapterByDBWithCustomTable(db, &CasbinRule{}, "casbin_rules")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gorm adapter: %w", err)
	}

	// Seed initial data (actions and roles)
	if err := seedInitialData(db, rolesFilePath, logger); err != nil {
		return nil, nil, fmt.Errorf("failed to seed initial data: %w", err)
	}

	return adapter, db, nil
}

func seedInitialData(db *gorm.DB, rolesFilePath string, logger *slog.Logger) error {
	logger.Info("seeding initial authorization data")

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := seedActions(tx, logger); err != nil {
			return fmt.Errorf("failed to seed actions: %w", err)
		}

		if err := seedRoles(tx, rolesFilePath, logger); err != nil {
			return fmt.Errorf("failed to seed roles: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	logger.Info("initial authorization data seeded successfully")
	return nil
}

// seedActions creates initial actions if they don't exist
func seedActions(db *gorm.DB, logger *slog.Logger) error {
	// Load actions from embedded file
	actions, err := data.LoadActions()
	if err != nil {
		return fmt.Errorf("failed to load actions: %w", err)
	}

	// Prepare action records for batch insert
	actionRecords := make([]Action, 0, len(actions))
	for _, actionName := range actions {
		actionRecords = append(actionRecords, Action{Action: actionName})
	}

	// This is idempotent and safe for concurrent execution
	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "action"}},
		DoNothing: true, // Ignore conflicts on unique constraint
	}).Create(&actionRecords)

	if result.Error != nil {
		return fmt.Errorf("failed to seed actions: %w", result.Error)
	}

	logger.Info("actions seeded", "total_actions", len(actions), "inserted", result.RowsAffected)
	return nil
}

// seedRoles creates initial role definitions from external file
func seedRoles(db *gorm.DB, rolesFilePath string, logger *slog.Logger) error {
	// Load roles from external file
	roleDefinitions, err := data.LoadRolesFromFile(rolesFilePath)
	if err != nil {
		return fmt.Errorf("failed to load roles: %w", err)
	}

	// Prepare all role mapping records for batch insert
	ruleRecords := make([]CasbinRule, 0)
	for _, roleDef := range roleDefinitions {
		for _, action := range roleDef.Actions {
			ruleRecords = append(ruleRecords, CasbinRule{
				Ptype: "g",
				V0:    roleDef.Name,
				V1:    action,
				V2:    "",
				V3:    "",
				V4:    "",
				V5:    "",
			})
		}
	}

	// This is idempotent and safe for concurrent execution
	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ptype"}, {Name: "v0"}, {Name: "v1"}, {Name: "v2"}, {Name: "v3"}, {Name: "v4"}, {Name: "v5"}},
		DoNothing: true, // Ignore conflicts on unique constraint
	}).Create(&ruleRecords)

	if result.Error != nil {
		return fmt.Errorf("failed to seed roles: %w", result.Error)
	}

	logger.Info("roles seeded", "file", rolesFilePath, "roles", len(roleDefinitions), "total_mappings", len(ruleRecords), "inserted", result.RowsAffected)
	return nil
}
