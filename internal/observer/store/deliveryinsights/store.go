// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package deliveryinsights persists normalized delivery facts and pre-computed DORA metric
// rollups for the Delivery Insights feature. Facts are derived from data-plane delivery
// events and incident data by the DORA aggregator and survive beyond the raw-event
// retention window; rollups serve the Insights read API.
package deliveryinsights

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

const (
	BackendSQLite     = "sqlite"
	BackendPostgreSQL = "postgresql"
)

// Deployment outcome values.
const (
	OutcomeInProgress = "in_progress"
	OutcomeSuccess    = "success"
	OutcomeFailed     = "failed"
)

// Failure attribution values.
const (
	FailedByRollout  = "rollout"
	FailedByIncident = "incident"
)

// Recovery fact sources.
const (
	RecoverySourceIncident = "incident"
	RecoverySourceHealth   = "health"
)

// Rollup scope types.
const (
	ScopeTypeOrg       = "org"
	ScopeTypeProject   = "project"
	ScopeTypeComponent = "component"
)

// Rollup granularities.
const (
	GranularityDaily   = "daily"
	GranularityWeekly  = "weekly"
	GranularityMonthly = "monthly"
)

// DeploymentFact is one normalized deployment: the unit DORA metrics are computed from.
// Exactly one row exists per rendered-release UID; retries and reconcile churn collapse
// into it via upsert. All timestamps are epoch milliseconds (UTC); nil means unknown.
type DeploymentFact struct {
	ReleaseUID       string
	OrgNamespace     string
	ProjectUID       string
	ComponentUID     string
	EnvironmentUID   string
	ProjectName      string
	ComponentName    string
	EnvironmentName  string
	ComponentRelease string
	CommitSHA        string
	CommitAuthoredMs *int64
	StartedMs        *int64
	ReadyMs          *int64
	Outcome          string
	FailedBy         string
	FailureReason    string
	IncidentID       string
	LeadTimeMs       *int64
	UpdatedAtMs      int64
}

// OccurredMs is the best-known moment the deployment happened: ready time for successful
// rollouts, start time for rollouts that never became ready, last update as a fallback.
func (f *DeploymentFact) OccurredMs() int64 {
	if f.ReadyMs != nil {
		return *f.ReadyMs
	}
	if f.StartedMs != nil {
		return *f.StartedMs
	}
	return f.UpdatedAtMs
}

// RecoveryFact is one failure→recovery episode, sourced from an incident lifecycle or
// from workload health transitions. RecoveredMs/DurationMs are nil while still failing.
type RecoveryFact struct {
	ID               string
	OrgNamespace     string
	ProjectUID       string
	ComponentUID     string
	EnvironmentUID   string
	ReleaseUID       string
	IncidentID       string
	Severity         string
	Source           string
	FailureStartedMs int64
	RecoveredMs      *int64
	DurationMs       *int64
	UpdatedAtMs      int64
}

// MetricRollup is one pre-computed metrics bucket for a scope. EnvironmentUID slices the
// scope per environment; empty string is the all-environments rollup. Count metrics
// (deployments, failures) are authoritative here; distribution metrics (lead time, MTTR)
// are cached snapshots — exact-window percentiles are recomputed from facts at read time.
type MetricRollup struct {
	ScopeType      string
	ScopeUID       string
	EnvironmentUID string
	Granularity    string
	BucketStartMs  int64
	DeployTotal    int
	DeploySuccess  int
	DeployFailed   int
	LeadTimeP50Ms  *int64
	LeadTimeP75Ms  *int64
	LeadTimeP95Ms  *int64
	MTTRMeanMs     *int64
	MTTRP50Ms      *int64
	RecoveryCount  int
	ComputedAtMs   int64
}

// RollupQuery selects rollup rows for one scope, granularity, and time range.
// StartMs is inclusive and EndMs exclusive, both against bucket_start_ms.
type RollupQuery struct {
	ScopeType      string
	ScopeUID       string
	EnvironmentUID string
	Granularity    string
	StartMs        int64
	EndMs          int64
}

// FactQuery filters fact rows by scope and time range. Empty scope fields are not
// filtered on; StartMs is inclusive and EndMs exclusive. Time filtering applies to the
// deployment moment for deployment facts and to failure start for recovery facts.
type FactQuery struct {
	OrgNamespace   string
	ProjectUID     string
	ComponentUID   string
	EnvironmentUID string
	StartMs        int64
	EndMs          int64
	Limit          int
	SortOrder      string
}

// AttributionResult reports the deployment an incident was matched against.
type AttributionResult struct {
	// ReleaseUID of the deployment live when the incident triggered; empty when no
	// deployment matched within the attribution window.
	ReleaseUID string
	// OccurredMs is that deployment's moment — the rollup bucket it lives in.
	OccurredMs int64
	// Attributed is true when this call marked the fact failed-by-incident. False
	// when there was no match or the fact already carried a failure attribution.
	Attributed bool
}

// Store persists delivery facts and metric rollups behind a pluggable SQL backend.
type Store interface {
	Initialize(ctx context.Context) error
	UpsertDeploymentFacts(ctx context.Context, facts []DeploymentFact) error
	UpsertRecoveryFacts(ctx context.Context, facts []RecoveryFact) error
	UpsertRollups(ctx context.Context, rollups []MetricRollup) error
	// AttributeIncident marks the deployment live in (componentUID, environmentUID)
	// at triggeredMs as failed-by-incident, if one deployed within windowMs before
	// the trigger and no failure is attributed to it yet (rollout failures win).
	AttributeIncident(
		ctx context.Context, componentUID, environmentUID, incidentID string,
		triggeredMs, windowMs int64) (AttributionResult, error)
	QueryRollups(ctx context.Context, q RollupQuery) ([]MetricRollup, error)
	QueryDeploymentFacts(ctx context.Context, q FactQuery) ([]DeploymentFact, int, error)
	QueryRecoveryFacts(ctx context.Context, q FactQuery) ([]RecoveryFact, error)
	QueryLeadTimes(ctx context.Context, q FactQuery) ([]int64, error)
	QueryRecoveryDurations(ctx context.Context, q FactQuery) ([]int64, error)
	Watermark(ctx context.Context, source string) (int64, error)
	SetWatermark(ctx context.Context, source string, watermarkMs int64) error
	Close() error
}

// New creates a delivery insights store for the configured backend.
func New(backend, dsn string, logger *slog.Logger) (Store, error) {
	selected := strings.ToLower(strings.TrimSpace(backend))
	if selected == "" {
		selected = BackendSQLite
	}

	switch selected {
	case BackendSQLite, BackendPostgreSQL:
		return newSQLStore(selected, dsn, logger)
	default:
		return nil, fmt.Errorf("unsupported delivery insights store backend %q: use %q or %q",
			selected, BackendSQLite, BackendPostgreSQL)
	}
}
