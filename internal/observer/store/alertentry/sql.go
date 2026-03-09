// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package alertentry

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const initializeTimeout = 30 * time.Second

type sqlStore struct {
	db      *sql.DB
	backend string
	dsn     string
	logger  *slog.Logger
}

func newSQLStore(backend, dsn string, logger *slog.Logger) (AlertEntryStore, error) {
	driver := "sqlite"
	if backend == BackendPostgreSQL {
		driver = "pgx"
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open alert entry store: %w", err)
	}
	return &sqlStore{
		db:      db,
		backend: backend,
		dsn:     dsn,
		logger:  logger,
	}, nil
}

func (s *sqlStore) Initialize(ctx context.Context) error {
	initCtx, cancel := context.WithTimeout(ctx, initializeTimeout)
	defer cancel()

	if s.backend == BackendSQLite {
		s.db.SetMaxOpenConns(1)
		if err := s.enableSQLiteWAL(initCtx); err != nil {
			return err
		}
	}

	if err := s.db.PingContext(initCtx); err != nil {
		return fmt.Errorf("failed to ping alert entry store: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createTableQuery); err != nil {
		return fmt.Errorf("failed to create alert_entries table: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createProjectEnvTimestampIndexQuery); err != nil {
		return fmt.Errorf("failed to create alert_entries index: %w", err)
	}
	return nil
}

func (s *sqlStore) WriteAlertEntry(ctx context.Context, entry *AlertEntry) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("alert entry is required")
	}

	id := uuid.NewString()
	timestamp := strings.TrimSpace(entry.Timestamp)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	} else {
		normalizedTimestamp, err := normalizeTimestamp(timestamp)
		if err != nil {
			return "", fmt.Errorf("invalid alert timestamp %q: %w", entry.Timestamp, err)
		}
		timestamp = normalizedTimestamp
	}

	var query string
	var args []any
	if s.backend == BackendPostgreSQL {
		query = insertAlertEntryPostgresQuery
		args = []any{
			id,
			timestamp,
			entry.AlertRuleName,
			entry.AlertRuleCRName,
			entry.AlertRuleCRNamespace,
			entry.AlertValue,
			entry.NamespaceName,
			entry.ComponentName,
			entry.EnvironmentName,
			entry.ProjectName,
			entry.ComponentID,
			entry.EnvironmentID,
			entry.ProjectID,
			entry.IncidentEnabled,
		}
	} else {
		query = insertAlertEntrySQLiteQuery
		args = []any{
			id,
			timestamp,
			entry.AlertRuleName,
			entry.AlertRuleCRName,
			entry.AlertRuleCRNamespace,
			entry.AlertValue,
			entry.NamespaceName,
			entry.ComponentName,
			entry.EnvironmentName,
			entry.ProjectName,
			entry.ComponentID,
			entry.EnvironmentID,
			entry.ProjectID,
			entry.IncidentEnabled,
		}
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return "", fmt.Errorf("failed to insert alert entry: %w", err)
	}
	return id, nil
}

func normalizeTimestamp(value string) (string, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UTC().Format(time.RFC3339Nano), nil
	}

	parsed, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.UTC().Format(time.RFC3339Nano), nil
	}

	return "", err
}

func (s *sqlStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *sqlStore) enableSQLiteWAL(ctx context.Context) error {
	if strings.Contains(strings.ToLower(s.dsn), "memory") {
		// In-memory SQLite does not support WAL; this path is expected in tests.
		return nil
	}

	if _, err := s.db.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("failed to enable sqlite WAL mode: %w", err)
	}
	return nil
}

const createTableQuery = `
CREATE TABLE IF NOT EXISTS alert_entries (
	id TEXT PRIMARY KEY,
	timestamp TEXT NOT NULL,
	alert_rule_name TEXT NOT NULL,
	alert_rule_cr_name TEXT,
	alert_rule_cr_namespace TEXT,
	alert_value TEXT,
	namespace_name TEXT,
	component_name TEXT,
	environment_name TEXT,
	project_name TEXT,
	component_id TEXT,
	environment_id TEXT,
	project_id TEXT,
	incident_enabled BOOLEAN NOT NULL
);`

const createProjectEnvTimestampIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_alert_entries_project_env_ts
ON alert_entries(project_id, environment_id, timestamp);`

const insertAlertEntrySQLiteQuery = `
INSERT INTO alert_entries (
	id, timestamp, alert_rule_name, alert_rule_cr_name, alert_rule_cr_namespace, alert_value,
	namespace_name, component_name, environment_name, project_name,
	component_id, environment_id, project_id,
	incident_enabled
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

const insertAlertEntryPostgresQuery = `
INSERT INTO alert_entries (
	id, timestamp, alert_rule_name, alert_rule_cr_name, alert_rule_cr_namespace, alert_value,
	namespace_name, component_name, environment_name, project_name,
	component_id, environment_id, project_id,
	incident_enabled
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);`
