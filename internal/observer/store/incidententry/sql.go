// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

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

func newSQLStore(backend, dsn string, logger *slog.Logger) (IncidentEntryStore, error) {
	driver := "sqlite"
	if backend == BackendPostgreSQL {
		driver = "pgx"
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open incident entry store: %w", err)
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
		return fmt.Errorf("failed to ping incident entry store: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createTableQuery); err != nil {
		return fmt.Errorf("failed to create incident_entries table: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createProjectEnvTimestampIndexQuery); err != nil {
		return fmt.Errorf("failed to create incident_entries index: %w", err)
	}
	return nil
}

func (s *sqlStore) WriteIncidentEntry(ctx context.Context, entry *IncidentEntry) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("incident entry is required")
	}

	alertID := strings.TrimSpace(entry.AlertID)
	if alertID == "" {
		return "", fmt.Errorf("alert id is required")
	}

	id := uuid.NewString()
	timestamp := strings.TrimSpace(entry.Timestamp)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	status := strings.TrimSpace(entry.Status)
	if status == "" {
		status = StatusTriggered
	}
	if status != StatusTriggered && status != StatusAcknowledged && status != StatusResolved {
		return "", fmt.Errorf("unsupported incident status %q", status)
	}

	triggeredAt := strings.TrimSpace(entry.TriggeredAt)
	if triggeredAt == "" {
		triggeredAt = timestamp
	}

	var query string
	var args []any
	if s.backend == BackendPostgreSQL {
		query = insertIncidentEntryPostgresQuery
		args = []any{
			id,
			alertID,
			timestamp,
			status,
			entry.TriggerAiRca,
			triggeredAt,
			nullableString(entry.AcknowledgedAt),
			nullableString(entry.ResolvedAt),
			nullableString(entry.Notes),
			nullableString(entry.Description),
			entry.NamespaceName,
			entry.ComponentName,
			entry.EnvironmentName,
			entry.ProjectName,
			entry.ComponentID,
			entry.EnvironmentID,
			entry.ProjectID,
		}
	} else {
		query = insertIncidentEntrySQLiteQuery
		args = []any{
			id,
			alertID,
			timestamp,
			status,
			entry.TriggerAiRca,
			triggeredAt,
			nullableString(entry.AcknowledgedAt),
			nullableString(entry.ResolvedAt),
			nullableString(entry.Notes),
			nullableString(entry.Description),
			entry.NamespaceName,
			entry.ComponentName,
			entry.EnvironmentName,
			entry.ProjectName,
			entry.ComponentID,
			entry.EnvironmentID,
			entry.ProjectID,
		}
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return "", fmt.Errorf("failed to insert incident entry: %w", err)
	}

	return id, nil
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

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

const createTableQuery = `
CREATE TABLE IF NOT EXISTS incident_entries (
	id TEXT PRIMARY KEY,
	alert_id TEXT NOT NULL,
	timestamp TEXT NOT NULL,
	status TEXT NOT NULL,
	trigger_ai_rca BOOLEAN NOT NULL,
	triggered_at TEXT NOT NULL,
	acknowledged_at TEXT,
	resolved_at TEXT,
	notes TEXT,
	description TEXT,
	namespace_name TEXT,
	component_name TEXT,
	environment_name TEXT,
	project_name TEXT,
	component_id TEXT,
	environment_id TEXT,
	project_id TEXT
);`

const createProjectEnvTimestampIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_incident_entries_project_env_ts
ON incident_entries(project_id, environment_id, timestamp);`

const insertIncidentEntrySQLiteQuery = `
INSERT INTO incident_entries (
	id, alert_id, timestamp, status, trigger_ai_rca, triggered_at,
	acknowledged_at, resolved_at, notes, description,
	namespace_name, component_name, environment_name, project_name,
	component_id, environment_id, project_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

const insertIncidentEntryPostgresQuery = `
INSERT INTO incident_entries (
	id, alert_id, timestamp, status, trigger_ai_rca, triggered_at,
	acknowledged_at, resolved_at, notes, description,
	namespace_name, component_name, environment_name, project_name,
	component_id, environment_id, project_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);`
