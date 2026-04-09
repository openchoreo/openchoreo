// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const (
	initializeTimeout = 30 * time.Second
	maxQueryLimit     = 10000
	sortOrderDesc     = "DESC"

	// timestampFormat is a fixed-width UTC timestamp layout so that TEXT
	// timestamps stored in SQLite/PostgreSQL are lexically comparable.
	timestampFormat = "2006-01-02T15:04:05.000Z"
)

type sqlStore struct {
	db      *sql.DB
	backend string
	dsn     string
	logger  *slog.Logger
}

func newSQLStore(backend, dsn string, logger *slog.Logger) (ReportStore, error) {
	driver := "sqlite"
	if backend == BackendPostgreSQL {
		driver = "pgx"
	}

	// Ensure parent directory exists for SQLite.
	if backend == BackendSQLite {
		dbPath := strings.TrimPrefix(dsn, "file:")
		if idx := strings.IndexByte(dbPath, '?'); idx != -1 {
			dbPath = dbPath[:idx]
		}
		if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create database directory %q: %w", dir, err)
			}
		}
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open report store: %w", err)
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
		return fmt.Errorf("failed to ping report store: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createTableQuery); err != nil {
		return fmt.Errorf("failed to create rca_reports table: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createAlertIDIndexQuery); err != nil {
		return fmt.Errorf("failed to create alert_id index: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createProjectEnvIndexQuery); err != nil {
		return fmt.Errorf("failed to create project_env index: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createTimestampIndexQuery); err != nil {
		return fmt.Errorf("failed to create timestamp index: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createStatusIndexQuery); err != nil {
		return fmt.Errorf("failed to create status index: %w", err)
	}
	return nil
}

func (s *sqlStore) UpsertReport(ctx context.Context, entry *ReportEntry) error {
	if entry == nil {
		return fmt.Errorf("report entry is required")
	}

	timestamp := strings.TrimSpace(entry.Timestamp)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(timestampFormat)
	} else {
		normalized, err := normalizeTimestamp(timestamp)
		if err != nil {
			return fmt.Errorf("invalid timestamp %q: %w", timestamp, err)
		}
		timestamp = normalized
	}

	var query string
	if s.backend == BackendPostgreSQL {
		query = upsertReportPostgresQuery
	} else {
		query = upsertReportSQLiteQuery
	}

	var summary sql.NullString
	if entry.Summary != nil {
		summary = sql.NullString{String: *entry.Summary, Valid: true}
	}

	var report sql.NullString
	if entry.Report != nil {
		report = sql.NullString{String: *entry.Report, Valid: true}
	}

	if _, err := s.db.ExecContext(ctx, query,
		entry.ReportID,
		entry.AlertID,
		entry.Status,
		summary,
		timestamp,
		entry.NamespaceName,
		entry.ProjectName,
		entry.EnvironmentName,
		entry.ComponentName,
		entry.EnvironmentUID,
		entry.ProjectUID,
		report,
	); err != nil {
		return fmt.Errorf("failed to upsert report: %w", err)
	}
	return nil
}

func (s *sqlStore) GetReport(ctx context.Context, reportID string) (*ReportEntry, error) {
	var query string
	if s.backend == BackendPostgreSQL {
		query = "SELECT report_id, alert_id, status, summary, timestamp, namespace_name, project_name, environment_name, component_name, environment_uid, project_uid, report FROM rca_reports WHERE report_id = $1"
	} else {
		query = "SELECT report_id, alert_id, status, summary, timestamp, namespace_name, project_name, environment_name, component_name, environment_uid, project_uid, report FROM rca_reports WHERE report_id = ?"
	}

	var entry ReportEntry
	var summary, report sql.NullString
	err := s.db.QueryRowContext(ctx, query, reportID).Scan(
		&entry.ReportID,
		&entry.AlertID,
		&entry.Status,
		&summary,
		&entry.Timestamp,
		&entry.NamespaceName,
		&entry.ProjectName,
		&entry.EnvironmentName,
		&entry.ComponentName,
		&entry.EnvironmentUID,
		&entry.ProjectUID,
		&report,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	if summary.Valid {
		entry.Summary = &summary.String
	}
	if report.Valid {
		entry.Report = &report.String
	}

	return &entry, nil
}

func (s *sqlStore) ListReports(ctx context.Context, params QueryParams) ([]ReportEntry, int, error) {
	startTime, err := normalizeTimestamp(strings.TrimSpace(params.StartTime))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid start time %q: %w", params.StartTime, err)
	}
	endTime, err := normalizeTimestamp(strings.TrimSpace(params.EndTime))
	if err != nil {
		return nil, 0, fmt.Errorf("invalid end time %q: %w", params.EndTime, err)
	}

	sortOrder := strings.ToUpper(strings.TrimSpace(params.SortOrder))
	if sortOrder == "" {
		sortOrder = sortOrderDesc
	}
	switch sortOrder {
	case "ASC", "DESC":
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

	conditions = append(conditions, "timestamp >= "+nextPlaceholder())
	args = append(args, startTime)
	conditions = append(conditions, "timestamp <= "+nextPlaceholder())
	args = append(args, endTime)

	if value := strings.TrimSpace(params.ProjectUID); value != "" {
		conditions = append(conditions, "project_uid = "+nextPlaceholder())
		args = append(args, value)
	}
	if value := strings.TrimSpace(params.EnvironmentUID); value != "" {
		conditions = append(conditions, "environment_uid = "+nextPlaceholder())
		args = append(args, value)
	}
	if value := strings.TrimSpace(params.Status); value != "" {
		conditions = append(conditions, "status = "+nextPlaceholder())
		args = append(args, value)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	// Count total
	countQuery := "SELECT COUNT(*) FROM rca_reports" + whereClause
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count reports: %w", err)
	}

	// Query with limit
	limit := maxQueryLimit
	if params.Limit > 0 && params.Limit < limit {
		limit = params.Limit
	}
	limitPh := nextPlaceholder()
	args = append(args, limit)

	// #nosec G202 -- whereClause uses parameterized placeholders; sortOrder is validated; limitPh is placeholder
	query := `SELECT report_id, alert_id, status, summary, timestamp
		FROM rca_reports` + whereClause + " ORDER BY timestamp " + sortOrder + " LIMIT " + limitPh

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query reports: %w", err)
	}
	defer rows.Close()

	entries := make([]ReportEntry, 0, min(limit, 64))
	for rows.Next() {
		var entry ReportEntry
		var summary sql.NullString
		if err := rows.Scan(
			&entry.ReportID,
			&entry.AlertID,
			&entry.Status,
			&summary,
			&entry.Timestamp,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan report: %w", err)
		}
		if summary.Valid {
			entry.Summary = &summary.String
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate reports: %w", err)
	}

	return entries, total, nil
}

func (s *sqlStore) UpdateActionStatuses(ctx context.Context, reportID string, report string) error {
	var query string
	if s.backend == BackendPostgreSQL {
		query = "UPDATE rca_reports SET report = $1 WHERE report_id = $2"
	} else {
		query = "UPDATE rca_reports SET report = ? WHERE report_id = ?"
	}

	result, err := s.db.ExecContext(ctx, query, report, reportID)
	if err != nil {
		return fmt.Errorf("failed to update report: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *sqlStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *sqlStore) enableSQLiteWAL(ctx context.Context) error {
	if strings.Contains(strings.ToLower(s.dsn), "memory") {
		return nil
	}

	if _, err := s.db.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("failed to enable sqlite WAL mode: %w", err)
	}
	return nil
}

func normalizeTimestamp(value string) (string, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UTC().Format(timestampFormat), nil
	}

	parsed, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.UTC().Format(timestampFormat), nil
	}

	return "", err
}

const createTableQuery = `
CREATE TABLE IF NOT EXISTS rca_reports (
	report_id TEXT PRIMARY KEY,
	alert_id TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	summary TEXT,
	timestamp TEXT NOT NULL,
	namespace_name TEXT,
	project_name TEXT,
	environment_name TEXT,
	component_name TEXT,
	environment_uid TEXT,
	project_uid TEXT,
	report TEXT
);`

const createAlertIDIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_rca_reports_alert_id
ON rca_reports(alert_id);`

const createProjectEnvIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_rca_reports_project_env
ON rca_reports(project_uid, environment_uid);`

const createTimestampIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_rca_reports_timestamp
ON rca_reports(timestamp);`

const createStatusIndexQuery = `
CREATE INDEX IF NOT EXISTS idx_rca_reports_status
ON rca_reports(status);`

const upsertReportSQLiteQuery = `
INSERT INTO rca_reports (report_id, alert_id, status, summary, timestamp, namespace_name, project_name, environment_name, component_name, environment_uid, project_uid, report)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(report_id) DO UPDATE SET
	status = excluded.status,
	summary = excluded.summary,
	timestamp = excluded.timestamp,
	namespace_name = excluded.namespace_name,
	project_name = excluded.project_name,
	environment_name = excluded.environment_name,
	component_name = excluded.component_name,
	environment_uid = excluded.environment_uid,
	project_uid = excluded.project_uid,
	report = COALESCE(excluded.report, rca_reports.report);`

const upsertReportPostgresQuery = `
INSERT INTO rca_reports (report_id, alert_id, status, summary, timestamp, namespace_name, project_name, environment_name, component_name, environment_uid, project_uid, report)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT(report_id) DO UPDATE SET
	status = EXCLUDED.status,
	summary = EXCLUDED.summary,
	timestamp = EXCLUDED.timestamp,
	namespace_name = EXCLUDED.namespace_name,
	project_name = EXCLUDED.project_name,
	environment_name = EXCLUDED.environment_name,
	component_name = EXCLUDED.component_name,
	environment_uid = EXCLUDED.environment_uid,
	project_uid = EXCLUDED.project_uid,
	report = COALESCE(EXCLUDED.report, rca_reports.report);`
