// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package alertentry

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
	if _, err := s.db.ExecContext(initCtx, createNamespaceTimestampIndexQuery); err != nil {
		return fmt.Errorf("failed to create alert_entries index: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createAlertRuleCRTimestampIndexQuery); err != nil {
		return fmt.Errorf("failed to create alert_entries cr index: %w", err)
	}
	return nil
}

func (s *sqlStore) WriteAlertEntry(ctx context.Context, entry *AlertEntry) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("alert entry is required")
	}

	id := uuid.NewString()
	timestamp := strings.TrimSpace(entry.Timestamp)
	var timestampNS int64
	if timestamp == "" {
		now := time.Now().UTC()
		timestamp = now.Format(time.RFC3339Nano)
		timestampNS = now.UnixNano()
	} else {
		normalizedTimestamp, err := normalizeTimestamp(timestamp)
		if err != nil {
			return "", fmt.Errorf("invalid alert timestamp %q: %w", entry.Timestamp, err)
		}
		timestamp = normalizedTimestamp
		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return "", fmt.Errorf("failed to parse normalized alert timestamp %q: %w", timestamp, err)
		}
		timestampNS = parsed.UnixNano()
	}
	// keep entry.Timestamp normalized for callers
	entry.Timestamp = timestamp

	var query string
	var args []any
	if s.backend == BackendPostgreSQL {
		query = insertAlertEntryPostgresQuery
		args = []any{
			id,
			timestampNS,
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
			entry.Severity,
			entry.Description,
			entry.NotificationChannels,
			entry.SourceType,
			entry.SourceQuery,
			entry.SourceMetric,
			entry.ConditionOperator,
			entry.ConditionThreshold,
			entry.ConditionWindow,
			entry.ConditionInterval,
		}
	} else {
		query = insertAlertEntrySQLiteQuery
		args = []any{
			id,
			timestampNS,
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
			entry.Severity,
			entry.Description,
			entry.NotificationChannels,
			entry.SourceType,
			entry.SourceQuery,
			entry.SourceMetric,
			entry.ConditionOperator,
			entry.ConditionThreshold,
			entry.ConditionWindow,
			entry.ConditionInterval,
		}
	}

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return "", fmt.Errorf("failed to insert alert entry: %w", err)
	}
	return id, nil
}

func (s *sqlStore) QueryAlertEntries(ctx context.Context, params QueryParams) ([]AlertEntry, int, error) {
	startTimeStr, err := normalizeTimestamp(strings.TrimSpace(params.StartTime))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid start time %q: %w", params.StartTime, err)
	}
	if startTimeStr == "" {
		return nil, 0, fmt.Errorf("start time is required")
	}

	endTimeStr, err := normalizeTimestamp(strings.TrimSpace(params.EndTime))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid end time %q: %w", params.EndTime, err)
	}
	if endTimeStr == "" {
		return nil, 0, fmt.Errorf("end time is required")
	}

	startTime, err := time.Parse(time.RFC3339Nano, startTimeStr)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse normalized start time %q: %w", startTimeStr, err)
	}
	endTime, err := time.Parse(time.RFC3339Nano, endTimeStr)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse normalized end time %q: %w", endTimeStr, err)
	}

	startNS := startTime.UnixNano()
	endNS := endTime.UnixNano()

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

	conditions := make([]string, 0, 6)
	args := make([]any, 0, 6)

	nextPlaceholder := func() string {
		if s.backend == BackendPostgreSQL {
			return "$" + strconv.Itoa(len(args)+1)
		}
		return "?"
	}

	conditions = append(conditions, "timestamp_ns >= "+nextPlaceholder())
	args = append(args, startNS)
	conditions = append(conditions, "timestamp_ns <= "+nextPlaceholder())
	args = append(args, endNS)

	if value := strings.TrimSpace(params.NamespaceName); value != "" {
		conditions = append(conditions, "namespace_name = "+nextPlaceholder())
		args = append(args, value)
	}
	if value := strings.TrimSpace(params.ProjectID); value != "" {
		conditions = append(conditions, "project_id = "+nextPlaceholder())
		args = append(args, value)
	}
	if value := strings.TrimSpace(params.ComponentID); value != "" {
		conditions = append(conditions, "component_id = "+nextPlaceholder())
		args = append(args, value)
	}
	if value := strings.TrimSpace(params.EnvironmentID); value != "" {
		conditions = append(conditions, "environment_id = "+nextPlaceholder())
		args = append(args, value)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	countQuery := "SELECT COUNT(*) FROM alert_entries" + whereClause

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count alert entries: %w", err)
	}

	limitPh := nextPlaceholder()
	limit := maxQueryLimit
	if params.Limit > 0 && params.Limit < limit {
		limit = params.Limit
	}
	args = append(args, limit)
	// #nosec G202 -- whereClause uses parameterized placeholders; orderClause is validated switch; limitPh is placeholder
	query := `SELECT
		id, timestamp_ns, alert_rule_name, alert_rule_cr_name, alert_rule_cr_namespace, alert_value,
		namespace_name, component_name, environment_name, project_name,
		component_id, environment_id, project_id, incident_enabled,
		severity, description, notification_channels,
		source_type, source_query, source_metric,
		condition_operator, condition_threshold, condition_window, condition_interval
	FROM alert_entries` + whereClause + " ORDER BY timestamp_ns " + orderClause + " LIMIT " + limitPh

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query alert entries: %w", err)
	}
	defer rows.Close()

	entries := make([]AlertEntry, 0, limit)
	for rows.Next() {
		var entry AlertEntry
		var tsNS int64
		if err := rows.Scan(
			&entry.ID,
			&tsNS,
			&entry.AlertRuleName,
			&entry.AlertRuleCRName,
			&entry.AlertRuleCRNamespace,
			&entry.AlertValue,
			&entry.NamespaceName,
			&entry.ComponentName,
			&entry.EnvironmentName,
			&entry.ProjectName,
			&entry.ComponentID,
			&entry.EnvironmentID,
			&entry.ProjectID,
			&entry.IncidentEnabled,
			&entry.Severity,
			&entry.Description,
			&entry.NotificationChannels,
			&entry.SourceType,
			&entry.SourceQuery,
			&entry.SourceMetric,
			&entry.ConditionOperator,
			&entry.ConditionThreshold,
			&entry.ConditionWindow,
			&entry.ConditionInterval,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan alert entry: %w", err)
		}
		entry.Timestamp = time.Unix(0, tsNS).UTC().Format(time.RFC3339Nano)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate alert entries: %w", err)
	}

	return entries, total, nil
}

func (s *sqlStore) HasRecentAlert(ctx context.Context, alertRuleCRName, alertRuleCRNamespace, componentUID string, since time.Time) (bool, error) {
	sinceNS := since.UnixNano()
	var query string
	if s.backend == BackendPostgreSQL {
		query = hasRecentAlertPostgresQuery
	} else {
		query = hasRecentAlertSQLiteQuery
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, query, alertRuleCRName, alertRuleCRNamespace, componentUID, sinceNS).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check recent alert: %w", err)
	}
	return exists, nil
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
	timestamp_ns BIGINT NOT NULL,
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
	incident_enabled BOOLEAN NOT NULL,
	severity TEXT,
	description TEXT,
	notification_channels TEXT,
	source_type TEXT,
	source_query TEXT,
	source_metric TEXT,
	condition_operator TEXT,
	condition_threshold REAL,
	condition_window TEXT,
	condition_interval TEXT
);`

const createProjectEnvTimestampIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_alert_entries_project_env_ts
ON alert_entries(project_id, environment_id, timestamp_ns);`

const createNamespaceTimestampIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_alert_entries_ns_ts
ON alert_entries(namespace_name, timestamp_ns);`

const insertAlertEntrySQLiteQuery = `
INSERT INTO alert_entries (
	id, timestamp_ns, alert_rule_name, alert_rule_cr_name, alert_rule_cr_namespace, alert_value,
	namespace_name, component_name, environment_name, project_name,
	component_id, environment_id, project_id,
	incident_enabled,
	severity, description, notification_channels,
	source_type, source_query, source_metric,
	condition_operator, condition_threshold, condition_window, condition_interval
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

const createAlertRuleCRTimestampIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_alert_entries_cr_ts
ON alert_entries(alert_rule_cr_name, alert_rule_cr_namespace, component_id, timestamp_ns);`

const hasRecentAlertSQLiteQuery = `
SELECT EXISTS(SELECT 1 FROM alert_entries
WHERE alert_rule_cr_name = ? AND alert_rule_cr_namespace = ? AND component_id = ? AND timestamp_ns >= ?
LIMIT 1);`

const hasRecentAlertPostgresQuery = `
SELECT EXISTS(SELECT 1 FROM alert_entries
WHERE alert_rule_cr_name = $1 AND alert_rule_cr_namespace = $2 AND component_id = $3 AND timestamp_ns >= $4
LIMIT 1);`

const insertAlertEntryPostgresQuery = `
INSERT INTO alert_entries (
	id, timestamp_ns, alert_rule_name, alert_rule_cr_name, alert_rule_cr_namespace, alert_value,
	namespace_name, component_name, environment_name, project_name,
	component_id, environment_id, project_id,
	incident_enabled,
	severity, description, notification_channels,
	source_type, source_query, source_metric,
	condition_operator, condition_threshold, condition_window, condition_interval
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24);`
