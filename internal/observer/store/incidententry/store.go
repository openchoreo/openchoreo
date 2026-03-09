// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package incidententry

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

const (
	StatusTriggered    = "triggered"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
)

// IncidentEntry represents one incident persisted by the observer.
type IncidentEntry struct {
	ID              string
	AlertID         string
	Timestamp       string
	Status          string
	TriggerAiRca    bool
	TriggeredAt     string
	AcknowledgedAt  string
	ResolvedAt      string
	Notes           string
	Description     string
	NamespaceName   string
	ComponentName   string
	EnvironmentName string
	ProjectName     string
	ComponentID     string
	EnvironmentID   string
	ProjectID       string
}

// QueryParams contains filters and pagination for querying incident entries.
type QueryParams struct {
	StartTime     string
	EndTime       string
	NamespaceName string
	ProjectID     string
	ComponentID   string
	EnvironmentID string
	Limit         int
	SortOrder     string
}

// IncidentEntryStore defines lifecycle and write operations for incident persistence.
type IncidentEntryStore interface {
	Initialize(ctx context.Context) error
	WriteIncidentEntry(ctx context.Context, entry *IncidentEntry) (id string, err error)
	QueryIncidentEntries(ctx context.Context, params QueryParams) ([]IncidentEntry, int, error)
	Close() error
}

// New creates a concrete incident entry store for the configured backend.
func New(backend, dsn string, logger *slog.Logger) (IncidentEntryStore, error) {
	selected := strings.ToLower(strings.TrimSpace(backend))
	if selected == "" {
		selected = BackendSQLite
	}

	switch selected {
	case BackendSQLite, BackendPostgreSQL:
		return newSQLStore(selected, dsn, logger)
	default:
		return nil, fmt.Errorf("unsupported incident store backend %q: use %q or %q", selected, BackendSQLite, BackendPostgreSQL)
	}
}
