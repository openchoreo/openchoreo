// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"fmt"
	"log/slog"

	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/glebarez/sqlite"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"gorm.io/gorm"
)

// CasbinRule defines the custom schema for Casbin policy storage
type CasbinRule struct {
	ID    uint   `gorm:"primaryKey;autoIncrement"`
	Ptype string `gorm:"type:text"` // Policy type: p (policy) or g (grouping/role)
	V0    string `gorm:"type:text"` // p - subject/principal, g - role
	V1    string `gorm:"type:text"` // p - resource path, g - action
	V2    string `gorm:"type:text"` // p - role name
	V3    string `gorm:"type:text"` // p - effect (allow/deny)
	V4    string `gorm:"type:text"` // p - context info
	V5    string `gorm:"type:text"` // extra field
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
func newAdapter(dbPath string, logger *slog.Logger) (*gormadapter.Adapter, *gorm.DB, error) {
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
	adapter, err := gormadapter.NewAdapterByDBWithCustomTable(db, &CasbinRule{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gorm adapter: %w", err)
	}

	// Seed initial data (actions and roles)
	if err := seedInitialData(db, logger); err != nil {
		return nil, nil, fmt.Errorf("failed to seed initial data: %w", err)
	}

	return adapter, db, nil
}

func seedInitialData(db *gorm.DB, logger *slog.Logger) error {
	logger.Info("seeding initial authorization data")

	// Seed actions
	if err := seedActions(db, logger); err != nil {
		return fmt.Errorf("failed to seed actions: %w", err)
	}

	// Seed roles (grouping policies)
	if err := seedRoles(db, logger); err != nil {
		return fmt.Errorf("failed to seed roles: %w", err)
	}

	logger.Info("initial authorization data seeded successfully")
	return nil
}

// seedActions creates initial actions if they don't exist
// Actions are defined in internal/authz/default_data.go
func seedActions(db *gorm.DB, logger *slog.Logger) error {
	// Get all actions from authz package
	actions := authzcore.ListDefaultActions()

	for _, actionName := range actions {
		action := Action{Action: actionName}
		result := db.Where(Action{Action: actionName}).FirstOrCreate(&action)
		if result.Error != nil {
			return fmt.Errorf("failed to create action %s: %w", actionName, result.Error)
		}
		if result.RowsAffected > 0 {
			logger.Debug("created action", "action", actionName)
		}
	}

	logger.Info("actions seeded", "count", len(actions))
	return nil
}

// seedRoles creates initial role definitions (grouping policies)
// Roles are defined in internal/authz/default_data.go
func seedRoles(db *gorm.DB, logger *slog.Logger) error {
	// Get default roles from authz package
	roleDefinitions := authzcore.ListDefaultRoles()

	createdCount := 0
	for _, roleDef := range roleDefinitions {
		for _, action := range roleDef.Actions {
			rule := CasbinRule{
				Ptype: "g",
				V0:    roleDef.Name,
				V1:    action,
			}

			result := db.Where(CasbinRule{
				Ptype: "g",
				V0:    roleDef.Name,
				V1:    action,
			}).FirstOrCreate(&rule)

			if result.Error != nil {
				return fmt.Errorf("failed to create role mapping %s -> %s: %w", roleDef.Name, action, result.Error)
			}

			if result.RowsAffected > 0 {
				logger.Debug("created role mapping", "role", roleDef.Name, "action", action)
				createdCount++
			}
		}
	}

	logger.Info("roles seeded", "roles", len(roleDefinitions), "mappings_created", createdCount)
	return nil
}
