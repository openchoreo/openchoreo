// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package sqlschema holds schema helpers shared by the observer's SQL stores.
package sqlschema

import (
	"context"
	"database/sql"
	"fmt"
)

// AddColumnIfMissing adds a nullable column to an existing table, for tables created before the
// column existed. Table, column and columnType are trusted identifiers, never user input.
func AddColumnIfMissing(ctx context.Context, db *sql.DB, postgres bool, table, column, columnType string) error {
	if postgres {
		// IF NOT EXISTS keeps concurrent observer replicas from racing on the same column.
		// #nosec G201 -- identifiers are constants supplied by the stores
		query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s;", table, column, columnType)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to add %s.%s column: %w", table, column, err)
		}
		return nil
	}

	var exists bool
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) > 0 FROM pragma_table_info(?) WHERE name = ?;", table, column).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check for %s.%s column: %w", table, column, err)
	}
	if exists {
		return nil
	}
	// #nosec G201 -- identifiers are constants supplied by the stores
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", table, column, columnType)
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to add %s.%s column: %w", table, column, err)
	}
	return nil
}
