// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

import (
	"context"
	"database/sql"
	"errors"
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
	var timestampNS int64
	if timestamp == "" {
		now := time.Now().UTC()
		timestamp = now.Format(time.RFC3339Nano)
		timestampNS = now.UnixNano()
	} else {
		normalizedTimestamp, err := normalizeTimestamp(timestamp)
		if err != nil {
			return "", fmt.Errorf("invalid incident timestamp %q: %w", entry.Timestamp, err)
		}
		timestamp = normalizedTimestamp
		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return "", fmt.Errorf("failed to parse normalized incident timestamp %q: %w", timestamp, err)
		}
		timestampNS = parsed.UnixNano()
	}
	// keep entry.Timestamp normalized for callers
	entry.Timestamp = timestamp

	status := strings.TrimSpace(entry.Status)
	if status == "" {
		status = StatusActive
	}
	if status != StatusActive && status != StatusAcknowledged && status != StatusResolved {
		return "", fmt.Errorf("unsupported incident status %q", status)
	}

	var triggeredAtNS int64
	triggeredAt, err := normalizeTimestamp(entry.TriggeredAt)
	if err != nil {
		return "", fmt.Errorf("invalid incident triggeredAt %q: %w", entry.TriggeredAt, err)
	}
	if triggeredAt == "" {
		triggeredAt = timestamp
		triggeredAtNS = timestampNS
	} else {
		parsed, err := time.Parse(time.RFC3339Nano, triggeredAt)
		if err != nil {
			return "", fmt.Errorf("failed to parse normalized incident triggeredAt %q: %w", triggeredAt, err)
		}
		triggeredAtNS = parsed.UnixNano()
	}
	entry.TriggeredAt = triggeredAt

	acknowledgedAt, err := normalizeTimestamp(entry.AcknowledgedAt)
	if err != nil {
		return "", fmt.Errorf("invalid incident acknowledgedAt %q: %w", entry.AcknowledgedAt, err)
	}

	var acknowledgedAtNS any
	if acknowledgedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, acknowledgedAt)
		if err != nil {
			return "", fmt.Errorf("failed to parse normalized incident acknowledgedAt %q: %w", acknowledgedAt, err)
		}
		acknowledgedAtNS = parsed.UnixNano()
	}
	entry.AcknowledgedAt = acknowledgedAt

	resolvedAt, err := normalizeTimestamp(entry.ResolvedAt)
	if err != nil {
		return "", fmt.Errorf("invalid incident resolvedAt %q: %w", entry.ResolvedAt, err)
	}

	var resolvedAtNS any
	if resolvedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, resolvedAt)
		if err != nil {
			return "", fmt.Errorf("failed to parse normalized incident resolvedAt %q: %w", resolvedAt, err)
		}
		resolvedAtNS = parsed.UnixNano()
	}
	entry.ResolvedAt = resolvedAt

	var query string
	var args []any
	if s.backend == BackendPostgreSQL {
		query = insertIncidentEntryPostgresQuery
		args = []any{
			id,
			alertID,
			timestampNS,
			status,
			entry.TriggerAiRca,
			triggeredAtNS,
			acknowledgedAtNS,
			resolvedAtNS,
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
			timestampNS,
			status,
			entry.TriggerAiRca,
			triggeredAtNS,
			acknowledgedAtNS,
			resolvedAtNS,
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
	countQuery := "SELECT COUNT(*) FROM incident_entries" + whereClause

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count incident entries: %w", err)
	}

	limitPh := nextPlaceholder()
	limit := maxQueryLimit
	if params.Limit > 0 && params.Limit < limit {
		limit = params.Limit
	}
	args = append(args, limit)
	// #nosec G202 -- whereClause uses parameterized placeholders; orderClause is validated switch; limitPh is placeholder
	query := `SELECT
		id, alert_id, timestamp_ns, status, trigger_ai_rca,
		triggered_at_ns, acknowledged_at_ns, resolved_at_ns,
		notes, description,
		namespace_name, component_name, environment_name, project_name,
		component_id, environment_id, project_id
	FROM incident_entries` + whereClause + " ORDER BY timestamp_ns " + orderClause + " LIMIT " + limitPh

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query incident entries: %w", err)
	}
	defer rows.Close()

	entries := make([]IncidentEntry, 0, limit)
	for rows.Next() {
		var entry IncidentEntry
		var tsNS int64
		var triggeredNS int64
		var acknowledgedNS sql.NullInt64
		var resolvedNS sql.NullInt64
		var notes sql.NullString
		var description sql.NullString
		if err := rows.Scan(
			&entry.ID,
			&entry.AlertID,
			&tsNS,
			&entry.Status,
			&entry.TriggerAiRca,
			&triggeredNS,
			&acknowledgedNS,
			&resolvedNS,
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

		entry.Timestamp = time.Unix(0, tsNS).UTC().Format(time.RFC3339Nano)
		entry.TriggeredAt = time.Unix(0, triggeredNS).UTC().Format(time.RFC3339Nano)

		if acknowledgedNS.Valid {
			entry.AcknowledgedAt = time.Unix(0, acknowledgedNS.Int64).UTC().Format(time.RFC3339Nano)
		}
		if resolvedNS.Valid {
			entry.ResolvedAt = time.Unix(0, resolvedNS.Int64).UTC().Format(time.RFC3339Nano)
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

func (s *sqlStore) UpdateIncidentEntry(ctx context.Context, id string, status string, notes, description *string, now time.Time) (IncidentEntry, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return IncidentEntry{}, fmt.Errorf("incident id is required")
	}

	status = strings.TrimSpace(status)
	if status == "" {
		return IncidentEntry{}, fmt.Errorf("incident status is required")
	}
	if status != StatusActive && status != StatusAcknowledged && status != StatusResolved {
		return IncidentEntry{}, fmt.Errorf("unsupported incident status %q", status)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IncidentEntry{}, fmt.Errorf("failed to begin transaction for incident update: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	entry, ackOutNS, resolvedOutNS, err := s.loadAndPrepareIncidentEntryForUpdate(ctx, tx, id, status, notes, description, now)
	if err != nil {
		return IncidentEntry{}, err
	}

	updateQuery, args := buildUpdateIncidentEntryQuery(s.backend, entry, ackOutNS, resolvedOutNS)

	result, execErr := tx.ExecContext(ctx, updateQuery, args...)
	if execErr != nil {
		err = fmt.Errorf("failed to update incident entry %q: %w", id, execErr)
		return IncidentEntry{}, err
	}

	rowsAffected, raErr := result.RowsAffected()
	if raErr != nil {
		err = fmt.Errorf("failed to check rows affected for incident entry %q: %w", id, raErr)
		return IncidentEntry{}, err
	}
	if rowsAffected == 0 {
		err = fmt.Errorf("%w: %s", ErrIncidentNotFound, id)
		return IncidentEntry{}, err
	}

	if err = tx.Commit(); err != nil {
		err = fmt.Errorf("failed to commit incident entry update %q: %w", id, err)
		return IncidentEntry{}, err
	}

	return entry, nil
}

// loadAndPrepareIncidentEntryForUpdate loads the existing incident entry within the given transaction
// and applies status, timestamp, and notes/description changes. It returns the updated entry along with
// the nanosecond values to be written for acknowledged/resolved timestamps.
func (s *sqlStore) loadAndPrepareIncidentEntryForUpdate(
	ctx context.Context,
	tx *sql.Tx,
	id string,
	status string,
	notes, description *string,
	now time.Time,
) (IncidentEntry, int64, int64, error) {
	placeholder := "?"
	if s.backend == BackendPostgreSQL {
		placeholder = "$1"
	}

	var selectQuery string
	// #nosec G202 -- id value is always passed as a parameter via placeholder; query text concatenation is limited to backend-specific placeholder.
	if s.backend == BackendPostgreSQL {
		selectQuery = `SELECT
		id, alert_id, timestamp_ns, status, trigger_ai_rca,
		triggered_at_ns, acknowledged_at_ns, resolved_at_ns,
		notes, description,
		namespace_name, component_name, environment_name, project_name,
		component_id, environment_id, project_id
	FROM incident_entries WHERE id = ` + placeholder + ` FOR UPDATE`
	} else {
		selectQuery = `SELECT
		id, alert_id, timestamp_ns, status, trigger_ai_rca,
		triggered_at_ns, acknowledged_at_ns, resolved_at_ns,
		notes, description,
		namespace_name, component_name, environment_name, project_name,
		component_id, environment_id, project_id
	FROM incident_entries WHERE id = ` + placeholder
	}

	row := tx.QueryRowContext(ctx, selectQuery, id)

	var entry IncidentEntry
	var tsNS int64
	var triggeredNS int64
	var acknowledgedNS sql.NullInt64
	var resolvedNS sql.NullInt64
	var existingNotes sql.NullString
	var existingDescription sql.NullString

	if err := row.Scan(
		&entry.ID,
		&entry.AlertID,
		&tsNS,
		&entry.Status,
		&entry.TriggerAiRca,
		&triggeredNS,
		&acknowledgedNS,
		&resolvedNS,
		&existingNotes,
		&existingDescription,
		&entry.NamespaceName,
		&entry.ComponentName,
		&entry.EnvironmentName,
		&entry.ProjectName,
		&entry.ComponentID,
		&entry.EnvironmentID,
		&entry.ProjectID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return IncidentEntry{}, 0, 0, fmt.Errorf("%w: %s", ErrIncidentNotFound, id)
		}
		return IncidentEntry{}, 0, 0, fmt.Errorf("failed to load incident entry %q: %w", id, err)
	}

	// Enforce forward-only status transitions.
	oldStatus := entry.Status
	if !isValidStatusTransition(oldStatus, status) {
		return IncidentEntry{}, 0, 0, fmt.Errorf("%w: %q → %q", ErrInvalidStatusTransition, oldStatus, status)
	}

	// Base timestamps from existing row.
	entry.Timestamp = time.Unix(0, tsNS).UTC().Format(time.RFC3339Nano)
	entry.TriggeredAt = time.Unix(0, triggeredNS).UTC().Format(time.RFC3339Nano)

	var ackOutNS int64
	if acknowledgedNS.Valid {
		ackOutNS = acknowledgedNS.Int64
	}
	var resolvedOutNS int64
	if resolvedNS.Valid {
		resolvedOutNS = resolvedNS.Int64
	}

	now = now.UTC()
	nowNS := now.UnixNano()

	// Update status and manage acknowledged/resolved timestamps.
	entry.Status = status
	if status == StatusAcknowledged && ackOutNS == 0 {
		ackOutNS = nowNS
	}
	if status == StatusResolved && resolvedOutNS == 0 {
		resolvedOutNS = nowNS
	}

	// Preserve existing notes/description when omitted (nil); apply when provided.
	entry.Notes = ""
	if existingNotes.Valid {
		entry.Notes = existingNotes.String
	}
	entry.Description = ""
	if existingDescription.Valid {
		entry.Description = existingDescription.String
	}
	if notes != nil {
		entry.Notes = strings.TrimSpace(*notes)
	}
	if description != nil {
		entry.Description = strings.TrimSpace(*description)
	}

	if ackOutNS != 0 {
		entry.AcknowledgedAt = time.Unix(0, ackOutNS).UTC().Format(time.RFC3339Nano)
	}
	if resolvedOutNS != 0 {
		entry.ResolvedAt = time.Unix(0, resolvedOutNS).UTC().Format(time.RFC3339Nano)
	}

	return entry, ackOutNS, resolvedOutNS, nil
}

func buildUpdateIncidentEntryQuery(backend string, entry IncidentEntry, ackOutNS, resolvedOutNS int64) (string, []any) {
	ackParam := any(nil)
	if ackOutNS != 0 {
		ackParam = ackOutNS
	}
	resolvedParam := any(nil)
	if resolvedOutNS != 0 {
		resolvedParam = resolvedOutNS
	}

	if backend == BackendPostgreSQL {
		updateQuery := `
UPDATE incident_entries
SET status = $1,
    acknowledged_at_ns = $2,
    resolved_at_ns = $3,
    notes = $4,
    description = $5
WHERE id = $6;`
		args := []any{
			entry.Status,
			ackParam,
			resolvedParam,
			nullableString(entry.Notes),
			nullableString(entry.Description),
			entry.ID,
		}
		return updateQuery, args
	}

	updateQuery := `
UPDATE incident_entries
SET status = ?,
    acknowledged_at_ns = ?,
    resolved_at_ns = ?,
    notes = ?,
    description = ?
WHERE id = ?;`
	args := []any{
		entry.Status,
		ackParam,
		resolvedParam,
		nullableString(entry.Notes),
		nullableString(entry.Description),
		entry.ID,
	}
	return updateQuery, args
}

// isValidStatusTransition reports whether the transition from oldStatus to newStatus is allowed.
// Allowed transitions: active → acknowledged, active → resolved, acknowledged → resolved.
// Re-applying the same status is also permitted (idempotent).
func isValidStatusTransition(oldStatus, newStatus string) bool {
	if oldStatus == newStatus {
		return true
	}
	switch oldStatus {
	case StatusActive:
		return newStatus == StatusAcknowledged || newStatus == StatusResolved
	case StatusAcknowledged:
		return newStatus == StatusResolved
	default:
		// resolved is a terminal state; no forward transitions allowed.
		return false
	}
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
	timestamp_ns BIGINT NOT NULL,
	status TEXT NOT NULL,
	trigger_ai_rca BOOLEAN NOT NULL,
	triggered_at_ns BIGINT NOT NULL,
	acknowledged_at_ns BIGINT,
	resolved_at_ns BIGINT,
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
ON incident_entries(project_id, environment_id, timestamp_ns);`

const insertIncidentEntrySQLiteQuery = `
INSERT INTO incident_entries (
	id, alert_id, timestamp_ns, status, trigger_ai_rca,
	triggered_at_ns, acknowledged_at_ns, resolved_at_ns,
	notes, description,
	namespace_name, component_name, environment_name, project_name,
	component_id, environment_id, project_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

const insertIncidentEntryPostgresQuery = `
INSERT INTO incident_entries (
	id, alert_id, timestamp_ns, status, trigger_ai_rca,
	triggered_at_ns, acknowledged_at_ns, resolved_at_ns,
	notes, description,
	namespace_name, component_name, environment_name, project_name,
	component_id, environment_id, project_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);`
