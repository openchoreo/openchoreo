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

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/authz/data"
)

// CasbinRule defines the custom schema for Casbin policy storage
// The unique index ensures no duplicate rules can be created, enabling atomic conflict resolution
type CasbinRule struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	Ptype      string `gorm:"type:text;uniqueIndex:idx_casbin_rule"`  // Policy type: p (policy) or g (grouping/role)
	V0         string `gorm:"type:text;uniqueIndex:idx_casbin_rule"`  // p - entitlement, g - role
	V1         string `gorm:"type:text;uniqueIndex:idx_casbin_rule"`  // p - resource path, g - action
	V2         string `gorm:"type:text;uniqueIndex:idx_casbin_rule"`  // p - role name
	V3         string `gorm:"type:text;uniqueIndex:idx_casbin_rule"`  // p - role namespace
	V4         string `gorm:"type:text;uniqueIndex:idx_casbin_rule"`  // p - effect (allow/deny)
	V5         string `gorm:"type:text;uniqueIndex:idx_casbin_rule"`  // p - context info
	IsInternal bool   `gorm:"column:internal;default:false;not null"` // Filter flag
}

// Action defines the schema for storing available actions
type Action struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	Action     string `gorm:"type:text;uniqueIndex;not null"`
	IsInternal bool   `gorm:"column:internal;default:false;not null"`
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
func newAdapter(dbPath string, authzDataFilePath string, logger *slog.Logger) (*gormadapter.Adapter, *gorm.DB, error) {
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

	// Seed initial data (actions, roles, and mappings)
	if err := seedInitialData(db, authzDataFilePath, logger); err != nil {
		return nil, nil, fmt.Errorf("failed to seed initial data: %w", err)
	}

	return adapter, db, nil
}

func seedInitialData(db *gorm.DB, authzDataFilePath string, logger *slog.Logger) error {
	logger.Info("seeding initial authorization data")

	// Load authz data from file
	authzData, err := data.LoadDefaultAuthzDataFromFile(authzDataFilePath)
	if err != nil {
		return fmt.Errorf("failed to load authz data: %w", err)
	}

	actionRecords, err := prepareActionRecords()
	if err != nil {
		return fmt.Errorf("failed to prepare actions: %w", err)
	}
	roleRecords := prepareRoleRecords(authzData.Roles)
	mappingRecords, err := prepareMappingRecords(authzData.Mappings)
	if err != nil {
		return fmt.Errorf("failed to prepare mappings: %w", err)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := insertActions(tx, actionRecords, logger); err != nil {
			return fmt.Errorf("failed to seed actions: %w", err)
		}

		if err := insertRoles(tx, roleRecords, len(authzData.Roles), logger); err != nil {
			return fmt.Errorf("failed to seed roles: %w", err)
		}

		if err := insertMappings(tx, mappingRecords, len(authzData.Mappings), logger); err != nil {
			return fmt.Errorf("failed to seed mappings: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	logger.Info("initial authorization data seeded successfully")
	return nil
}

// prepareActionRecords loads and prepares action records for insertion
func prepareActionRecords() ([]Action, error) {
	actions, err := data.LoadActions()
	if err != nil {
		return nil, fmt.Errorf("failed to load actions: %w", err)
	}

	actionRecords := make([]Action, 0, len(actions))
	for _, actionData := range actions {
		actionRecords = append(actionRecords, Action{
			Action:     actionData.Name,
			IsInternal: actionData.IsInternal,
		})
	}

	return actionRecords, nil
}

// insertActions inserts action records into the database
func insertActions(db *gorm.DB, actionRecords []Action, logger *slog.Logger) error {
	if len(actionRecords) == 0 {
		logger.Info("no actions to seed")
		return nil
	}

	// This is idempotent and safe for concurrent execution
	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "action"}},
		DoNothing: true, // Ignore conflicts on unique constraint
	}).Create(&actionRecords)

	if result.Error != nil {
		return fmt.Errorf("failed to insert actions: %w", result.Error)
	}

	logger.Info("actions seeded", "total_actions", len(actionRecords), "inserted", result.RowsAffected)
	return nil
}

// prepareRoleRecords prepares role-to-action mapping records for insertion
// All roles use g grouping with format: g, roleName, action, namespace
// Cluster roles use "*" as namespace, namespace roles use their actual namespace
func prepareRoleRecords(roleDefinitions []authzcore.Role) []CasbinRule {
	ruleRecords := make([]CasbinRule, 0)
	for _, roleDef := range roleDefinitions {
		namespace := roleDef.Namespace
		if namespace == "" {
			namespace = "*"
		}

		for _, action := range roleDef.Actions {
			ruleRecords = append(ruleRecords, CasbinRule{
				Ptype:      "g",
				V0:         roleDef.Name,
				V1:         action,
				V2:         namespace,
				V3:         "",
				V4:         "",
				V5:         "",
				IsInternal: roleDef.IsInternal,
			})
		}
	}
	return ruleRecords
}

// insertRoles inserts role records into the database
func insertRoles(db *gorm.DB, ruleRecords []CasbinRule, roleCount int, logger *slog.Logger) error {
	if len(ruleRecords) == 0 {
		logger.Info("no roles to seed")
		return nil
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ptype"}, {Name: "v0"}, {Name: "v1"}, {Name: "v2"}, {Name: "v3"}, {Name: "v4"}, {Name: "v5"}},
		DoNothing: true, // Ignore conflicts on unique constraint
	}).Create(&ruleRecords)

	if result.Error != nil {
		return fmt.Errorf("failed to insert roles: %w", result.Error)
	}

	logger.Info("roles seeded", "roles", roleCount, "total_mappings", len(ruleRecords), "inserted", result.RowsAffected)
	return nil
}

// prepareMappingRecords prepares role-entitlement mapping records for insertion
func prepareMappingRecords(mappingDefinitions []authzcore.RoleEntitlementMapping) ([]CasbinRule, error) {
	policyRecords := make([]CasbinRule, 0, len(mappingDefinitions))
	for _, mappingDef := range mappingDefinitions {
		entitlement, err := formatSubject(mappingDef.Entitlement.Claim, mappingDef.Entitlement.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to format entitlement: %w", err)
		}
		resourcePath := hierarchyToResourcePath(mappingDef.Hierarchy)

		// Determine role_ns: "*" for cluster roles, namespace for namespace-scoped roles
		roleNs := normalizeNamespace(mappingDef.RoleRef.Namespace)

		policyRecords = append(policyRecords, CasbinRule{
			Ptype:      "p",
			V0:         entitlement,
			V1:         resourcePath,
			V2:         mappingDef.RoleRef.Name,
			V3:         roleNs,
			V4:         string(mappingDef.Effect),
			V5:         emptyContextJSON,
			IsInternal: mappingDef.IsInternal,
		})
	}
	return policyRecords, nil
}

// insertMappings inserts mapping records into the database
func insertMappings(db *gorm.DB, policyRecords []CasbinRule, mappingCount int, logger *slog.Logger) error {
	if len(policyRecords) == 0 {
		logger.Info("no mappings to seed")
		return nil
	}

	// This is idempotent and safe for concurrent execution
	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ptype"}, {Name: "v0"}, {Name: "v1"}, {Name: "v2"}, {Name: "v3"}, {Name: "v4"}, {Name: "v5"}},
		DoNothing: true,
	}).Create(&policyRecords)

	if result.Error != nil {
		return fmt.Errorf("failed to insert mappings: %w", result.Error)
	}

	logger.Info("mappings seeded", "total_mappings", mappingCount, "inserted", result.RowsAffected)
	return nil
}
