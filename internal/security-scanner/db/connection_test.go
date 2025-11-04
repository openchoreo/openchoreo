// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"os"
	"testing"
)

func TestInitDB_SQLite(t *testing.T) {
	tmpFile := "/tmp/test-security-scanner.db"
	defer os.Remove(tmpFile)

	cfg := Config{
		Backend: SQLite,
		DSN:     tmpFile,
	}

	conn, err := InitDB(cfg)
	if err != nil {
		t.Fatalf("failed to initialize SQLite DB: %v", err)
	}
	defer conn.Close()

	if conn.Backend() != SQLite {
		t.Errorf("expected backend SQLite, got %v", conn.Backend())
	}

	if conn.Querier() == nil {
		t.Error("expected non-nil querier")
	}
}

func TestInitDB_UnsupportedBackend(t *testing.T) {
	cfg := Config{
		Backend: DBBackend("invalid"),
		DSN:     "test.db",
	}

	_, err := InitDB(cfg)
	if err == nil {
		t.Error("expected error for unsupported backend, got nil")
	}
}

func TestDBBackend_Values(t *testing.T) {
	if SQLite != "sqlite" {
		t.Errorf("expected SQLite to be 'sqlite', got %s", SQLite)
	}
	if Postgres != "postgres" {
		t.Errorf("expected Postgres to be 'postgres', got %s", Postgres)
	}
}
