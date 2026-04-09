// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

const (
	// BackendSQLite is the SQLite backend identifier.
	BackendSQLite = "sqlite"
	// BackendPostgreSQL is the PostgreSQL backend identifier.
	BackendPostgreSQL = "postgresql"
)

// ReportEntry represents a stored RCA report.
type ReportEntry struct {
	ReportID        string  `json:"reportId"`
	AlertID         string  `json:"alertId"`
	Status          string  `json:"status"`
	Summary         *string `json:"summary,omitempty"`
	Timestamp       string  `json:"timestamp"`
	NamespaceName   string  `json:"namespaceName,omitempty"`
	ProjectName     string  `json:"projectName,omitempty"`
	EnvironmentName string  `json:"environmentName,omitempty"`
	ComponentName   string  `json:"componentName,omitempty"`
	EnvironmentUID  string  `json:"environmentUid,omitempty"`
	ProjectUID      string  `json:"projectUid,omitempty"`
	Report          *string `json:"report,omitempty"`
}

// QueryParams defines the query parameters for listing reports.
type QueryParams struct {
	ProjectUID     string
	EnvironmentUID string
	StartTime      string // RFC3339
	EndTime        string // RFC3339
	Limit          int
	SortOrder      string // "ASC" or "DESC"
	Status         string // optional filter: pending, completed, failed
}

// ReportStore defines the interface for RCA report persistence.
type ReportStore interface {
	Initialize(ctx context.Context) error
	UpsertReport(ctx context.Context, entry *ReportEntry) error
	GetReport(ctx context.Context, reportID string) (*ReportEntry, error)
	ListReports(ctx context.Context, params QueryParams) ([]ReportEntry, int, error)
	UpdateActionStatuses(ctx context.Context, reportID string, report string) error
	Close() error
}

// New creates a new ReportStore for the given backend.
func New(backend, dsn string, logger *slog.Logger) (ReportStore, error) {
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		backend = BackendSQLite
	}

	switch backend {
	case BackendSQLite, BackendPostgreSQL:
		return newSQLStore(backend, dsn, logger)
	default:
		return nil, fmt.Errorf("unsupported report store backend: %q (supported: %s, %s)",
			backend, BackendSQLite, BackendPostgreSQL)
	}
}
