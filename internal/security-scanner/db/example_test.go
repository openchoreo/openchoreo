// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/security-scanner/db"
)

func Example() {
	ctx := context.Background()

	sqliteConn, err := db.InitDB(db.Config{
		Backend:      db.SQLite,
		DSN:          "file::memory:?cache=shared",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	})
	if err != nil {
		panic(err)
	}
	defer sqliteConn.Close()

	resourceID, err := sqliteConn.Querier().UpsertResource(ctx, "Pod", "default", "test-pod", "uid-123", "version-1")
	if err != nil {
		panic(err)
	}

	if err := sqliteConn.Querier().InsertResourceLabel(ctx, resourceID, "app", "test"); err != nil {
		panic(err)
	}

	scanned, err := sqliteConn.Querier().GetPostureScannedResource(ctx, "Pod", "default", "test-pod")
	if err != nil {
		fmt.Printf("Resource not yet scanned: %v\n", err)
	} else {
		fmt.Printf("Resource scanned at version: %s\n", scanned.ResourceVersion)
	}

	pgConn, err := db.InitDB(db.Config{
		Backend: db.Postgres,
		DSN:     "postgres://user:pass@localhost:5432/dbname?sslmode=disable",
	})
	if err != nil {
		panic(err)
	}
	_ = pgConn.Close()
}
