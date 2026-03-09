// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const (
	initializeTimeout = 30 * time.Second
	maxQueryLimit     = 10000
	sortOrderDesc     = "DESC"
)

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
	} else {
		normalizedTimestamp, err := normalizeTimestamp(timestamp)
		if err != nil {
			return "", fmt.Errorf("invalid incident timestamp %q: %w", entry.Timestamp, err)
		}
		timestamp = normalizedTimestamp
	}

	status := strings.TrimSpace(entry.Status)
	if status == "" {
		status = StatusTriggered
	}
	if status != StatusTriggered && status != StatusAcknowledged && status != StatusResolved {
		return "", fmt.Errorf("unsupported incident status %q", status)
	}

	triggeredAt, err := normalizeTimestamp(entry.TriggeredAt)
	if err != nil {
		return "", fmt.Errorf("invalid incident triggeredAt %q: %w", entry.TriggeredAt, err)
	}
	if triggeredAt == "" {
		triggeredAt = timestamp
	}

	acknowledgedAt, err := normalizeTimestamp(entry.AcknowledgedAt)
	if err != nil {
		return "", fmt.Errorf("invalid incident acknowledgedAt %q: %w", entry.AcknowledgedAt, err)
	}

	resolvedAt, err := normalizeTimestamp(entry.ResolvedAt)
	if err != nil {
		return "", fmt.Errorf("invalid incident resolvedAt %q: %w", entry.ResolvedAt, err)
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
			nullableString(acknowledgedAt),
			nullableString(resolvedAt),
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
			nullableString(acknowledgedAt),
			nullableString(resolvedAt),
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

func (s *sqlStore) QueryIncidentEntries(ctx context.Context, params QueryParams) ([]IncidentEntry, int, error) {
	startTime, err := normalizeTimestamp(strings.TrimSpace(params.StartTime))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid start time %q: %w", params.StartTime, err)
	}
	if startTime == "" {
		return nil, 0, fmt.Errorf("start time is required")
	}

	endTime, err := normalizeTimestamp(strings.TrimSpace(params.EndTime))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid end time %q: %w", params.EndTime, err)
	}
	if endTime == "" {
		return nil, 0, fmt.Errorf("end time is required")
	}

	sortOrder := strings.ToUpper(strings.TrimSpace(params.SortOrder))
	if sortOrder == "" {
		sortOrder = sortOrderDesc
	}
	var orderClause string
	switch sortOrder {
	case "ASC":
		orderClause = "ASC"
	case sortOrderDesc:
		orderClause = sortOrderDesc
	default:
		return nil, 0, fmt.Errorf("invalid sort order %q", params.SortOrder)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxQueryLimit {
		limit = maxQueryLimit
	}

	conditions := make([]string, 0, 6)
	args := make([]any, 0, 6)

	nextPlaceholder := func() string {
		if s.backend == BackendPostgreSQL {
			return "$" + strconv.Itoa(len(args)+1)
		}
		return "?"
	}

	conditions = append(conditions, "timestamp >= "+nextPlaceholder())
	args = append(args, startTime)
	conditions = append(conditions, "timestamp <= "+nextPlaceholder())
	args = append(args, endTime)

	if value := strings.TrimSpace(params.NamespaceName); value != "" {
		conditions = append(conditions, "namespace_name = "+nextPlaceholder())
		args = append(args, value)
	}
	if value := strings.TrimSpace(params.ProjectName); value != "" {
		conditions = append(conditions, "project_name = "+nextPlaceholder())
		args = append(args, value)
	}
	if value := strings.TrimSpace(params.ComponentName); value != "" {
		conditions = append(conditions, "component_name = "+nextPlaceholder())
		args = append(args, value)
	}
	if value := strings.TrimSpace(params.EnvironmentName); value != "" {
		conditions = append(conditions, "environment_name = "+nextPlaceholder())
		args = append(args, value)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	countQuery := "SELECT COUNT(*) FROM incident_entries" + whereClause

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count incident entries: %w", err)
	}

	limitPh := nextPlaceholder()
	args = append(args, limit)
	// #nosec G202 -- whereClause uses parameterized placeholders; orderClause is validated switch; limitPh is placeholder
	query := `SELECT
		id, alert_id, timestamp, status, trigger_ai_rca, triggered_at,
		acknowledged_at, resolved_at, notes, description,
		namespace_name, component_name, environment_name, project_name,
		component_id, environment_id, project_id
	FROM incident_entries` + whereClause + " ORDER BY timestamp " + orderClause + " LIMIT " + limitPh

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query incident entries: %w", err)
	}
	defer rows.Close()

	entries := make([]IncidentEntry, 0, limit)
	for rows.Next() {
		var entry IncidentEntry
		var acknowledgedAt sql.NullString
		var resolvedAt sql.NullString
		var notes sql.NullString
		var description sql.NullString
		if err := rows.Scan(
			&entry.ID,
			&entry.AlertID,
			&entry.Timestamp,
			&entry.Status,
			&entry.TriggerAiRca,
			&entry.TriggeredAt,
			&acknowledgedAt,
			&resolvedAt,
			&notes,
			&description,
			&entry.NamespaceName,
			&entry.ComponentName,
			&entry.EnvironmentName,
			&entry.ProjectName,
			&entry.ComponentID,
			&entry.EnvironmentID,
			&entry.ProjectID,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan incident entry: %w", err)
		}
		if acknowledgedAt.Valid {
			entry.AcknowledgedAt = acknowledgedAt.String
		}
		if resolvedAt.Valid {
			entry.ResolvedAt = resolvedAt.String
		}
		if notes.Valid {
			entry.Notes = notes.String
		}
		if description.Valid {
			entry.Description = description.String
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate incident entries: %w", err)
	}

	return entries, total, nil
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

func normalizeTimestamp(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil
	}

	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err == nil {
		return parsed.UTC().Format(time.RFC3339Nano), nil
	}

	parsed, err = time.Parse(time.RFC3339, trimmed)
	if err == nil {
		return parsed.UTC().Format(time.RFC3339Nano), nil
	}

	return "", err
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
