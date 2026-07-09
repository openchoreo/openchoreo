// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deliveryinsights

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const (
	initializeTimeout = 30 * time.Second
	defaultQueryLimit = 100
	maxQueryLimit     = 10000
	sortOrderAsc      = "ASC"
	sortOrderDesc     = "DESC"
)

// migration is one ordered schema change. Versions are applied exactly once and recorded
// in delivery_insights_schema_version, so later releases can evolve the schema safely.
type migration struct {
	version    int
	statements []string
}

// The DDL is deliberately restricted to portable SQL (TEXT/BIGINT/INTEGER columns, no
// dialect-specific functions or types) so it runs unchanged on SQLite and PostgreSQL.
// Time is epoch milliseconds; bucketing and percentiles are computed in Go.
var migrations = []migration{
	{
		version: 1,
		statements: []string{
			`CREATE TABLE IF NOT EXISTS deployment_fact (
				release_uid        TEXT PRIMARY KEY,
				org_namespace      TEXT NOT NULL,
				project_uid        TEXT NOT NULL,
				component_uid      TEXT NOT NULL,
				environment_uid    TEXT NOT NULL,
				project_name       TEXT NOT NULL DEFAULT '',
				component_name     TEXT NOT NULL DEFAULT '',
				environment_name   TEXT NOT NULL DEFAULT '',
				component_release  TEXT NOT NULL DEFAULT '',
				commit_sha         TEXT NOT NULL DEFAULT '',
				commit_authored_ms BIGINT,
				started_ms         BIGINT,
				ready_ms           BIGINT,
				outcome            TEXT NOT NULL DEFAULT 'in_progress',
				failed_by          TEXT NOT NULL DEFAULT '',
				failure_reason     TEXT NOT NULL DEFAULT '',
				incident_id        TEXT NOT NULL DEFAULT '',
				lead_time_ms       BIGINT,
				updated_at_ms      BIGINT NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_deployment_fact_component_env_ready
				ON deployment_fact(component_uid, environment_uid, ready_ms);`,
			`CREATE INDEX IF NOT EXISTS idx_deployment_fact_project_ready
				ON deployment_fact(project_uid, ready_ms);`,
			`CREATE INDEX IF NOT EXISTS idx_deployment_fact_org_ready
				ON deployment_fact(org_namespace, ready_ms);`,
			`CREATE TABLE IF NOT EXISTS recovery_fact (
				id                 TEXT PRIMARY KEY,
				org_namespace      TEXT NOT NULL,
				project_uid        TEXT NOT NULL DEFAULT '',
				component_uid      TEXT NOT NULL DEFAULT '',
				environment_uid    TEXT NOT NULL DEFAULT '',
				release_uid        TEXT NOT NULL DEFAULT '',
				incident_id        TEXT NOT NULL DEFAULT '',
				severity           TEXT NOT NULL DEFAULT '',
				source             TEXT NOT NULL,
				failure_started_ms BIGINT NOT NULL,
				recovered_ms       BIGINT,
				duration_ms        BIGINT,
				updated_at_ms      BIGINT NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_recovery_fact_component_env_started
				ON recovery_fact(component_uid, environment_uid, failure_started_ms);`,
			`CREATE TABLE IF NOT EXISTS delivery_metric_rollup (
				scope_type       TEXT NOT NULL,
				scope_uid        TEXT NOT NULL,
				environment_uid  TEXT NOT NULL DEFAULT '',
				granularity      TEXT NOT NULL,
				bucket_start_ms  BIGINT NOT NULL,
				deploy_total     INTEGER NOT NULL DEFAULT 0,
				deploy_success   INTEGER NOT NULL DEFAULT 0,
				deploy_failed    INTEGER NOT NULL DEFAULT 0,
				lead_time_p50_ms BIGINT,
				lead_time_p75_ms BIGINT,
				lead_time_p95_ms BIGINT,
				mttr_mean_ms     BIGINT,
				mttr_p50_ms      BIGINT,
				recovery_count   INTEGER NOT NULL DEFAULT 0,
				computed_at_ms   BIGINT NOT NULL,
				PRIMARY KEY (scope_type, scope_uid, environment_uid, granularity, bucket_start_ms)
			);`,
			`CREATE TABLE IF NOT EXISTS delivery_insights_watermark (
				source       TEXT PRIMARY KEY,
				watermark_ms BIGINT NOT NULL
			);`,
		},
	},
}

const createSchemaVersionTableQuery = `CREATE TABLE IF NOT EXISTS delivery_insights_schema_version (
	version       INTEGER PRIMARY KEY,
	applied_at_ms BIGINT NOT NULL
);`

// Write/read statements below use '?' placeholders; rebind converts them to $N for
// PostgreSQL. 'excluded' upsert references are supported by both SQLite (>=3.24) and
// PostgreSQL, so a single statement serves both dialects.
const upsertDeploymentFactQuery = `INSERT INTO deployment_fact (
	release_uid, org_namespace, project_uid, component_uid, environment_uid,
	project_name, component_name, environment_name, component_release,
	commit_sha, commit_authored_ms, started_ms, ready_ms,
	outcome, failed_by, failure_reason, incident_id, lead_time_ms, updated_at_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (release_uid) DO UPDATE SET
	org_namespace = excluded.org_namespace,
	project_uid = excluded.project_uid,
	component_uid = excluded.component_uid,
	environment_uid = excluded.environment_uid,
	project_name = excluded.project_name,
	component_name = excluded.component_name,
	environment_name = excluded.environment_name,
	component_release = CASE WHEN excluded.component_release <> ''
		THEN excluded.component_release ELSE deployment_fact.component_release END,
	commit_sha = CASE WHEN excluded.commit_sha <> ''
		THEN excluded.commit_sha ELSE deployment_fact.commit_sha END,
	commit_authored_ms = COALESCE(excluded.commit_authored_ms, deployment_fact.commit_authored_ms),
	started_ms = COALESCE(excluded.started_ms, deployment_fact.started_ms),
	ready_ms = COALESCE(excluded.ready_ms, deployment_fact.ready_ms),
	outcome = CASE WHEN deployment_fact.outcome = 'failed'
		THEN deployment_fact.outcome ELSE excluded.outcome END,
	failed_by = CASE WHEN excluded.failed_by <> ''
		THEN excluded.failed_by ELSE deployment_fact.failed_by END,
	failure_reason = CASE WHEN excluded.failure_reason <> ''
		THEN excluded.failure_reason ELSE deployment_fact.failure_reason END,
	incident_id = CASE WHEN excluded.incident_id <> ''
		THEN excluded.incident_id ELSE deployment_fact.incident_id END,
	lead_time_ms = COALESCE(excluded.lead_time_ms, deployment_fact.lead_time_ms),
	updated_at_ms = excluded.updated_at_ms;`

const upsertRecoveryFactQuery = `INSERT INTO recovery_fact (
	id, org_namespace, project_uid, component_uid, environment_uid,
	release_uid, incident_id, severity, source,
	failure_started_ms, recovered_ms, duration_ms, updated_at_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
	release_uid = CASE WHEN excluded.release_uid <> ''
		THEN excluded.release_uid ELSE recovery_fact.release_uid END,
	incident_id = CASE WHEN excluded.incident_id <> ''
		THEN excluded.incident_id ELSE recovery_fact.incident_id END,
	severity = CASE WHEN excluded.severity <> ''
		THEN excluded.severity ELSE recovery_fact.severity END,
	recovered_ms = COALESCE(excluded.recovered_ms, recovery_fact.recovered_ms),
	duration_ms = COALESCE(excluded.duration_ms, recovery_fact.duration_ms),
	updated_at_ms = excluded.updated_at_ms;`

const upsertRollupQuery = `INSERT INTO delivery_metric_rollup (
	scope_type, scope_uid, environment_uid, granularity, bucket_start_ms,
	deploy_total, deploy_success, deploy_failed,
	lead_time_p50_ms, lead_time_p75_ms, lead_time_p95_ms,
	mttr_mean_ms, mttr_p50_ms, recovery_count, computed_at_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (scope_type, scope_uid, environment_uid, granularity, bucket_start_ms) DO UPDATE SET
	deploy_total = excluded.deploy_total,
	deploy_success = excluded.deploy_success,
	deploy_failed = excluded.deploy_failed,
	lead_time_p50_ms = excluded.lead_time_p50_ms,
	lead_time_p75_ms = excluded.lead_time_p75_ms,
	lead_time_p95_ms = excluded.lead_time_p95_ms,
	mttr_mean_ms = excluded.mttr_mean_ms,
	mttr_p50_ms = excluded.mttr_p50_ms,
	recovery_count = excluded.recovery_count,
	computed_at_ms = excluded.computed_at_ms;`

const setWatermarkQuery = `INSERT INTO delivery_insights_watermark (source, watermark_ms)
VALUES (?, ?)
ON CONFLICT (source) DO UPDATE SET watermark_ms = excluded.watermark_ms;`

type sqlStore struct {
	db      *sql.DB
	backend string
	dsn     string
	logger  *slog.Logger
}

func newSQLStore(backend, dsn string, logger *slog.Logger) (Store, error) {
	driver := "sqlite"
	if backend == BackendPostgreSQL {
		driver = "pgx"
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open delivery insights store: %w", err)
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
		return fmt.Errorf("failed to ping delivery insights store: %w", err)
	}
	if _, err := s.db.ExecContext(initCtx, createSchemaVersionTableQuery); err != nil {
		return fmt.Errorf("failed to create delivery insights schema version table: %w", err)
	}

	return s.applyMigrations(initCtx)
}

func (s *sqlStore) enableSQLiteWAL(ctx context.Context) error {
	// In-memory databases (used by tests) do not support WAL.
	if strings.Contains(s.dsn, "memory") {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("failed to enable WAL mode on delivery insights store: %w", err)
	}
	return nil
}

// applyMigrations runs every migration with a version greater than the recorded maximum,
// each inside its own transaction so a failure leaves the schema at a known version.
func (s *sqlStore) applyMigrations(ctx context.Context) error {
	var current sql.NullInt64
	row := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM delivery_insights_schema_version;")
	if err := row.Scan(&current); err != nil {
		return fmt.Errorf("failed to read delivery insights schema version: %w", err)
	}

	for _, m := range migrations {
		if current.Valid && m.version <= int(current.Int64) {
			continue
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin migration %d: %w", m.version, err)
		}
		if err := s.applyMigration(ctx, tx, m); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				s.logger.Error("Failed to roll back delivery insights migration",
					"version", m.version, "error", rbErr)
			}
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", m.version, err)
		}
		s.logger.Info("Applied delivery insights schema migration", "version", m.version)
	}

	return nil
}

func (s *sqlStore) applyMigration(ctx context.Context, tx *sql.Tx, m migration) error {
	for _, stmt := range m.statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", m.version, err)
		}
	}
	record := s.rebind("INSERT INTO delivery_insights_schema_version (version, applied_at_ms) VALUES (?, ?);")
	if _, err := tx.ExecContext(ctx, record, m.version, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("failed to record migration %d: %w", m.version, err)
	}
	return nil
}

// rebind converts '?' placeholders to PostgreSQL's positional '$N' form. Statements in
// this package never contain a literal '?', so a plain scan is safe.
func (s *sqlStore) rebind(query string) string {
	if s.backend != BackendPostgreSQL {
		return query
	}

	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

func (s *sqlStore) UpsertDeploymentFacts(ctx context.Context, facts []DeploymentFact) error {
	if len(facts) == 0 {
		return nil
	}
	for i := range facts {
		if err := validateDeploymentFact(&facts[i]); err != nil {
			return err
		}
	}

	query := s.rebind(upsertDeploymentFactQuery)
	return s.execInTx(ctx, func(tx *sql.Tx) error {
		for i := range facts {
			f := &facts[i]
			_, err := tx.ExecContext(ctx, query,
				f.ReleaseUID, f.OrgNamespace, f.ProjectUID, f.ComponentUID, f.EnvironmentUID,
				f.ProjectName, f.ComponentName, f.EnvironmentName, f.ComponentRelease,
				f.CommitSHA, nullableInt64(f.CommitAuthoredMs), nullableInt64(f.StartedMs),
				nullableInt64(f.ReadyMs), f.Outcome, f.FailedBy, f.FailureReason,
				f.IncidentID, nullableInt64(f.LeadTimeMs), f.UpdatedAtMs,
			)
			if err != nil {
				return fmt.Errorf("failed to upsert deployment fact %q: %w", f.ReleaseUID, err)
			}
		}
		return nil
	})
}

func (s *sqlStore) UpsertRecoveryFacts(ctx context.Context, facts []RecoveryFact) error {
	if len(facts) == 0 {
		return nil
	}
	for i := range facts {
		if err := validateRecoveryFact(&facts[i]); err != nil {
			return err
		}
	}

	query := s.rebind(upsertRecoveryFactQuery)
	return s.execInTx(ctx, func(tx *sql.Tx) error {
		for i := range facts {
			f := &facts[i]
			_, err := tx.ExecContext(ctx, query,
				f.ID, f.OrgNamespace, f.ProjectUID, f.ComponentUID, f.EnvironmentUID,
				f.ReleaseUID, f.IncidentID, f.Severity, f.Source,
				f.FailureStartedMs, nullableInt64(f.RecoveredMs), nullableInt64(f.DurationMs),
				f.UpdatedAtMs,
			)
			if err != nil {
				return fmt.Errorf("failed to upsert recovery fact %q: %w", f.ID, err)
			}
		}
		return nil
	})
}

func (s *sqlStore) UpsertRollups(ctx context.Context, rollups []MetricRollup) error {
	if len(rollups) == 0 {
		return nil
	}
	for i := range rollups {
		if err := validateRollup(&rollups[i]); err != nil {
			return err
		}
	}

	query := s.rebind(upsertRollupQuery)
	return s.execInTx(ctx, func(tx *sql.Tx) error {
		for i := range rollups {
			r := &rollups[i]
			_, err := tx.ExecContext(ctx, query,
				r.ScopeType, r.ScopeUID, r.EnvironmentUID, r.Granularity, r.BucketStartMs,
				r.DeployTotal, r.DeploySuccess, r.DeployFailed,
				nullableInt64(r.LeadTimeP50Ms), nullableInt64(r.LeadTimeP75Ms),
				nullableInt64(r.LeadTimeP95Ms), nullableInt64(r.MTTRMeanMs),
				nullableInt64(r.MTTRP50Ms), r.RecoveryCount, r.ComputedAtMs,
			)
			if err != nil {
				return fmt.Errorf("failed to upsert rollup %s/%s/%s@%d: %w",
					r.ScopeType, r.ScopeUID, r.Granularity, r.BucketStartMs, err)
			}
		}
		return nil
	})
}

func (s *sqlStore) QueryRollups(ctx context.Context, q RollupQuery) ([]MetricRollup, error) {
	if err := validateRollupQuery(&q); err != nil {
		return nil, err
	}

	query := s.rebind(`SELECT scope_type, scope_uid, environment_uid, granularity, bucket_start_ms,
	deploy_total, deploy_success, deploy_failed,
	lead_time_p50_ms, lead_time_p75_ms, lead_time_p95_ms,
	mttr_mean_ms, mttr_p50_ms, recovery_count, computed_at_ms
FROM delivery_metric_rollup
WHERE scope_type = ? AND scope_uid = ? AND environment_uid = ? AND granularity = ?
	AND bucket_start_ms >= ? AND bucket_start_ms < ?
ORDER BY bucket_start_ms ASC;`)

	rows, err := s.db.QueryContext(ctx, query,
		q.ScopeType, q.ScopeUID, q.EnvironmentUID, q.Granularity, q.StartMs, q.EndMs)
	if err != nil {
		return nil, fmt.Errorf("failed to query delivery metric rollups: %w", err)
	}
	defer closeRows(rows, s.logger)

	var rollups []MetricRollup
	for rows.Next() {
		var r MetricRollup
		var p50, p75, p95, mttrMean, mttrP50 sql.NullInt64
		if err := rows.Scan(&r.ScopeType, &r.ScopeUID, &r.EnvironmentUID, &r.Granularity,
			&r.BucketStartMs, &r.DeployTotal, &r.DeploySuccess, &r.DeployFailed,
			&p50, &p75, &p95, &mttrMean, &mttrP50, &r.RecoveryCount, &r.ComputedAtMs); err != nil {
			return nil, fmt.Errorf("failed to scan delivery metric rollup: %w", err)
		}
		r.LeadTimeP50Ms = int64Ptr(p50)
		r.LeadTimeP75Ms = int64Ptr(p75)
		r.LeadTimeP95Ms = int64Ptr(p95)
		r.MTTRMeanMs = int64Ptr(mttrMean)
		r.MTTRP50Ms = int64Ptr(mttrP50)
		rollups = append(rollups, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate delivery metric rollups: %w", err)
	}
	return rollups, nil
}

// occurredMsExpr orders and filters deployment facts by the best-known deployment moment.
const occurredMsExpr = "COALESCE(ready_ms, started_ms, updated_at_ms)"

func (s *sqlStore) QueryDeploymentFacts(ctx context.Context, q FactQuery) ([]DeploymentFact, int, error) {
	orderClause, err := normalizeSortOrder(q.SortOrder)
	if err != nil {
		return nil, 0, err
	}
	limit := normalizeLimit(q.Limit)

	conditions, args := s.factScopeConditions(q)
	conditions = append(conditions, occurredMsExpr+" >= ?", occurredMsExpr+" < ?")
	args = append(args, q.StartMs, q.EndMs)
	where := " WHERE " + strings.Join(conditions, " AND ")

	countQuery := s.rebind("SELECT COUNT(*) FROM deployment_fact" + where + ";")
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count deployment facts: %w", err)
	}

	query := s.rebind(`SELECT release_uid, org_namespace, project_uid, component_uid, environment_uid,
	project_name, component_name, environment_name, component_release,
	commit_sha, commit_authored_ms, started_ms, ready_ms,
	outcome, failed_by, failure_reason, incident_id, lead_time_ms, updated_at_ms
FROM deployment_fact` + where +
		" ORDER BY " + occurredMsExpr + " " + orderClause +
		" LIMIT " + strconv.Itoa(limit) + ";")

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query deployment facts: %w", err)
	}
	defer closeRows(rows, s.logger)

	var facts []DeploymentFact
	for rows.Next() {
		var f DeploymentFact
		var authored, started, ready, leadTime sql.NullInt64
		if err := rows.Scan(&f.ReleaseUID, &f.OrgNamespace, &f.ProjectUID, &f.ComponentUID,
			&f.EnvironmentUID, &f.ProjectName, &f.ComponentName, &f.EnvironmentName,
			&f.ComponentRelease, &f.CommitSHA, &authored, &started, &ready,
			&f.Outcome, &f.FailedBy, &f.FailureReason, &f.IncidentID, &leadTime,
			&f.UpdatedAtMs); err != nil {
			return nil, 0, fmt.Errorf("failed to scan deployment fact: %w", err)
		}
		f.CommitAuthoredMs = int64Ptr(authored)
		f.StartedMs = int64Ptr(started)
		f.ReadyMs = int64Ptr(ready)
		f.LeadTimeMs = int64Ptr(leadTime)
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate deployment facts: %w", err)
	}
	return facts, total, nil
}

func (s *sqlStore) QueryLeadTimes(ctx context.Context, q FactQuery) ([]int64, error) {
	conditions, args := s.factScopeConditions(q)
	conditions = append(conditions,
		"ready_ms IS NOT NULL", "ready_ms >= ?", "ready_ms < ?",
		"lead_time_ms IS NOT NULL", "lead_time_ms >= 0")
	args = append(args, q.StartMs, q.EndMs)

	query := s.rebind("SELECT lead_time_ms FROM deployment_fact WHERE " +
		strings.Join(conditions, " AND ") + ";")
	return s.queryInt64s(ctx, query, args, "lead times")
}

func (s *sqlStore) QueryRecoveryDurations(ctx context.Context, q FactQuery) ([]int64, error) {
	conditions, args := s.factScopeConditions(q)
	conditions = append(conditions,
		"failure_started_ms >= ?", "failure_started_ms < ?", "duration_ms IS NOT NULL")
	args = append(args, q.StartMs, q.EndMs)

	query := s.rebind("SELECT duration_ms FROM recovery_fact WHERE " +
		strings.Join(conditions, " AND ") + ";")
	return s.queryInt64s(ctx, query, args, "recovery durations")
}

func (s *sqlStore) Watermark(ctx context.Context, source string) (int64, error) {
	query := s.rebind("SELECT watermark_ms FROM delivery_insights_watermark WHERE source = ?;")
	var watermark int64
	err := s.db.QueryRowContext(ctx, query, source).Scan(&watermark)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read delivery insights watermark %q: %w", source, err)
	}
	return watermark, nil
}

func (s *sqlStore) SetWatermark(ctx context.Context, source string, watermarkMs int64) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("watermark source is required")
	}
	query := s.rebind(setWatermarkQuery)
	if _, err := s.db.ExecContext(ctx, query, source, watermarkMs); err != nil {
		return fmt.Errorf("failed to set delivery insights watermark %q: %w", source, err)
	}
	return nil
}

func (s *sqlStore) Close() error {
	return s.db.Close()
}

// factScopeConditions builds the WHERE fragment shared by all fact queries. Empty scope
// fields are not filtered on, so one query shape serves org, project, component, and
// per-environment reads.
func (s *sqlStore) factScopeConditions(q FactQuery) ([]string, []any) {
	conditions := make([]string, 0, 6)
	args := make([]any, 0, 6)
	scopeFilters := []struct {
		column string
		value  string
	}{
		{"org_namespace", q.OrgNamespace},
		{"project_uid", q.ProjectUID},
		{"component_uid", q.ComponentUID},
		{"environment_uid", q.EnvironmentUID},
	}
	for _, f := range scopeFilters {
		if value := strings.TrimSpace(f.value); value != "" {
			conditions = append(conditions, f.column+" = ?")
			args = append(args, value)
		}
	}
	return conditions, args
}

func (s *sqlStore) queryInt64s(ctx context.Context, query string, args []any, what string) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s: %w", what, err)
	}
	defer closeRows(rows, s.logger)

	var values []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("failed to scan %s: %w", what, err)
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate %s: %w", what, err)
	}
	return values, nil
}

func (s *sqlStore) execInTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin delivery insights transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			s.logger.Error("Failed to roll back delivery insights transaction", "error", rbErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delivery insights transaction: %w", err)
	}
	return nil
}

func validateDeploymentFact(f *DeploymentFact) error {
	if strings.TrimSpace(f.ReleaseUID) == "" {
		return fmt.Errorf("deployment fact release UID is required")
	}
	if strings.TrimSpace(f.OrgNamespace) == "" {
		return fmt.Errorf("deployment fact %q: org namespace is required", f.ReleaseUID)
	}
	if f.Outcome == "" {
		f.Outcome = OutcomeInProgress
	}
	switch f.Outcome {
	case OutcomeInProgress, OutcomeSuccess, OutcomeFailed:
	default:
		return fmt.Errorf("deployment fact %q: unsupported outcome %q", f.ReleaseUID, f.Outcome)
	}
	switch f.FailedBy {
	case "", FailedByRollout, FailedByIncident:
	default:
		return fmt.Errorf("deployment fact %q: unsupported failedBy %q", f.ReleaseUID, f.FailedBy)
	}
	if f.UpdatedAtMs == 0 {
		f.UpdatedAtMs = time.Now().UnixMilli()
	}
	return nil
}

func validateRecoveryFact(f *RecoveryFact) error {
	if strings.TrimSpace(f.ID) == "" {
		return fmt.Errorf("recovery fact ID is required")
	}
	if strings.TrimSpace(f.OrgNamespace) == "" {
		return fmt.Errorf("recovery fact %q: org namespace is required", f.ID)
	}
	switch f.Source {
	case RecoverySourceIncident, RecoverySourceHealth:
	default:
		return fmt.Errorf("recovery fact %q: unsupported source %q", f.ID, f.Source)
	}
	if f.FailureStartedMs <= 0 {
		return fmt.Errorf("recovery fact %q: failure start time is required", f.ID)
	}
	if f.DurationMs == nil && f.RecoveredMs != nil {
		duration := *f.RecoveredMs - f.FailureStartedMs
		f.DurationMs = &duration
	}
	if f.UpdatedAtMs == 0 {
		f.UpdatedAtMs = time.Now().UnixMilli()
	}
	return nil
}

func validateRollup(r *MetricRollup) error {
	if err := validateScopeType(r.ScopeType); err != nil {
		return err
	}
	if strings.TrimSpace(r.ScopeUID) == "" {
		return fmt.Errorf("rollup scope UID is required")
	}
	if err := validateGranularity(r.Granularity); err != nil {
		return err
	}
	if r.ComputedAtMs == 0 {
		r.ComputedAtMs = time.Now().UnixMilli()
	}
	return nil
}

func validateRollupQuery(q *RollupQuery) error {
	if err := validateScopeType(q.ScopeType); err != nil {
		return err
	}
	if strings.TrimSpace(q.ScopeUID) == "" {
		return fmt.Errorf("rollup query scope UID is required")
	}
	if err := validateGranularity(q.Granularity); err != nil {
		return err
	}
	if q.EndMs <= q.StartMs {
		return fmt.Errorf("rollup query end time must be after start time")
	}
	return nil
}

func validateScopeType(scopeType string) error {
	switch scopeType {
	case ScopeTypeOrg, ScopeTypeProject, ScopeTypeComponent:
		return nil
	default:
		return fmt.Errorf("unsupported rollup scope type %q", scopeType)
	}
}

func validateGranularity(granularity string) error {
	switch granularity {
	case GranularityDaily, GranularityWeekly, GranularityMonthly:
		return nil
	default:
		return fmt.Errorf("unsupported rollup granularity %q", granularity)
	}
}

func normalizeSortOrder(sortOrder string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(sortOrder)) {
	case "", sortOrderDesc:
		return sortOrderDesc, nil
	case sortOrderAsc:
		return sortOrderAsc, nil
	default:
		return "", fmt.Errorf("invalid sort order %q", sortOrder)
	}
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultQueryLimit
	}
	if limit > maxQueryLimit {
		return maxQueryLimit
	}
	return limit
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func int64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

func closeRows(rows *sql.Rows, logger *slog.Logger) {
	if err := rows.Close(); err != nil {
		logger.Error("Failed to close delivery insights rows", "error", err)
	}
}
