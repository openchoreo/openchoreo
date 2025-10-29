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

	if err := sqliteConn.Querier().InsertScannedPod(ctx, "test-pod"); err != nil {
		panic(err)
	}

	pods, err := sqliteConn.Querier().ListScannedPods(ctx)
	if err != nil {
		panic(err)
	}

	for _, pod := range pods {
		fmt.Printf("Pod: %s\n", pod.PodName)
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
