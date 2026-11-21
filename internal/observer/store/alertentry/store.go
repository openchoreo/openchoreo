// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package alertentry

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	BackendSQLite     = "sqlite"
	BackendPostgreSQL = "postgresql"
)

// AlertEntry represents one fired alert event persisted by the observer.
type AlertEntry struct {
	ID                   string
	Timestamp            string
	AlertRuleName        string
	AlertRuleCRName      string
	AlertRuleCRNamespace string
	AlertValue           string
	NamespaceName        string
	ComponentName        string
	EnvironmentName      string
	ProjectName          string
	ComponentID          string
	EnvironmentID        string
	ProjectID            string
	ReleaseLineName      string
	ReleaseLineID        string
	IncidentEnabled      bool
	Severity             string
	Description          string
	NotificationChannels string // JSON array of channel names
	SourceType           string
	SourceQuery          string
	SourceMetric         string
	ConditionOperator    string
	ConditionThreshold   float64
	ConditionWindow      string
	ConditionInterval    string
}

// QueryParams contains filters and pagination for querying alert entries.
type QueryParams struct {
	StartTime     string
	EndTime       string
	NamespaceName string
	ProjectID     string
	ComponentID   string
	EnvironmentID string
	ReleaseLineID string
	// AllReleaseLines covers every line of the component. Without it, a component covers its
	// main line.
	AllReleaseLines bool
	Limit           int
	SortOrder       string
}

// AlertEntryStore defines lifecycle and write operations for alert entry persistence.
type AlertEntryStore interface {
	Initialize(ctx context.Context) error
	WriteAlertEntry(ctx context.Context, entry *AlertEntry) (id string, err error)
	QueryAlertEntries(ctx context.Context, params QueryParams) ([]AlertEntry, int, error)
	HasRecentAlert(ctx context.Context, alertRuleCRName, alertRuleCRNamespace, componentUID string, since time.Time) (bool, error)
	// RecordNotificationDelivery inserts or updates the delivery result of one channel.
	RecordNotificationDelivery(ctx context.Context, delivery NotificationDelivery) error
	// QueryNotificationDeliveries returns the delivery results of the given alerts, by alert ID.
	QueryNotificationDeliveries(ctx context.Context, alertIDs []string) (map[string][]NotificationDelivery, error)
	// FailStaleNotificationDeliveries marks deliveries pending since before cutoff as failed.
	FailStaleNotificationDeliveries(ctx context.Context, cutoff time.Time, reason string) (int64, error)
	Close() error
}

// New creates a concrete alert entry store for the configured backend.
func New(backend, dsn string, logger *slog.Logger) (AlertEntryStore, error) {
	selected := strings.ToLower(strings.TrimSpace(backend))
	if selected == "" {
		selected = BackendSQLite
	}

	switch selected {
	case BackendSQLite, BackendPostgreSQL:
		return newSQLStore(selected, dsn, logger)
	default:
		return nil, fmt.Errorf("unsupported alert store backend %q: use %q or %q", selected, BackendSQLite, BackendPostgreSQL)
	}
}
