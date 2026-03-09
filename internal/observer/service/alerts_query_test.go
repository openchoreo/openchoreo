// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

func TestAlertServiceQueryAlerts(t *testing.T) {
	t.Parallel()

	notificationChannelsJSON := `["email-main"]`

	fakeStore := &fakeAlertEntryStore{
		entries: []alertentry.AlertEntry{
			{
				ID:                   "a-1",
				Timestamp:            "2026-03-07T10:20:30Z",
				AlertRuleName:        "high-errors",
				AlertRuleCRName:      "rule-cr",
				AlertRuleCRNamespace: "obs-ns",
				AlertValue:           "12",
				NamespaceName:        "team-a",
				ProjectName:          "project-a",
				ComponentName:        "component-a",
				EnvironmentName:      "dev",
				ProjectID:            "b2c3d4e5-6789-01bc-def0-234567890abc",
				ComponentID:          "a1b2c3d4-5678-90ab-cdef-1234567890ab",
				EnvironmentID:        "d4e5f6a7-8901-23de-f012-4567890abcde",
				Severity:             "critical",
				Description:          "Errors too high",
				NotificationChannels: notificationChannelsJSON,
				SourceType:           "log",
				SourceQuery:          "level=error",
				ConditionOperator:    "gt",
				ConditionThreshold:   10,
				ConditionWindow:      "5m0s",
				ConditionInterval:    "1m0s",
			},
		},
		total: 1,
	}

	svc := &AlertService{
		alertEntryStore: fakeStore,
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := gen.AlertsQueryRequest{
		StartTime: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC),
		SearchScope: gen.ComponentSearchScope{
			Namespace: "team-a",
		},
	}
	resp, err := svc.QueryAlerts(context.Background(), req)
	if err != nil {
		t.Fatalf("query alerts failed: %v", err)
	}

	// Assert that query parameters were translated correctly
	gotParams := fakeStore.lastQueryParams
	if gotParams.NamespaceName != "team-a" {
		t.Fatalf("expected namespace %q, got %q", "team-a", gotParams.NamespaceName)
	}
	if gotParams.StartTime != "2026-03-07T10:00:00Z" {
		t.Fatalf("expected startTime %q, got %q", "2026-03-07T10:00:00Z", gotParams.StartTime)
	}
	if gotParams.EndTime != "2026-03-07T11:00:00Z" {
		t.Fatalf("expected endTime %q, got %q", "2026-03-07T11:00:00Z", gotParams.EndTime)
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}
	out := string(raw)
	for _, expected := range []string{"high-errors", "email-main", "critical", "\"total\":1"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in response: %s", expected, out)
		}
	}
}

func TestAlertServiceQueryIncidents(t *testing.T) {
	t.Parallel()

	fakeStore := &fakeIncidentEntryStore{
		entries: []incidententry.IncidentEntry{
			{
				ID:              "inc-1",
				AlertID:         "a-1",
				Timestamp:       "2026-03-07T10:20:30Z",
				Status:          incidententry.StatusTriggered,
				TriggerAiRca:    true,
				TriggeredAt:     "2026-03-07T10:20:30Z",
				Description:     "Investigate error spike",
				NamespaceName:   "team-a",
				ProjectName:     "project-a",
				ComponentName:   "component-a",
				EnvironmentName: "dev",
				ProjectID:       "b2c3d4e5-6789-01bc-def0-234567890abc",
				ComponentID:     "a1b2c3d4-5678-90ab-cdef-1234567890ab",
				EnvironmentID:   "d4e5f6a7-8901-23de-f012-4567890abcde",
			},
		},
		total: 1,
	}

	svc := &AlertService{
		incidentEntryStore: fakeStore,
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := gen.IncidentsQueryRequest{
		StartTime: time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC),
		SearchScope: gen.ComponentSearchScope{
			Namespace: "team-a",
		},
	}
	resp, err := svc.QueryIncidents(context.Background(), req)
	if err != nil {
		t.Fatalf("query incidents failed: %v", err)
	}

	// Assert that query parameters were translated correctly
	gotParams := fakeStore.lastQueryParams
	if gotParams.NamespaceName != "team-a" {
		t.Fatalf("expected namespace %q, got %q", "team-a", gotParams.NamespaceName)
	}
	if gotParams.StartTime != "2026-03-07T10:00:00Z" {
		t.Fatalf("expected startTime %q, got %q", "2026-03-07T10:00:00Z", gotParams.StartTime)
	}
	if gotParams.EndTime != "2026-03-07T11:00:00Z" {
		t.Fatalf("expected endTime %q, got %q", "2026-03-07T11:00:00Z", gotParams.EndTime)
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}
	out := string(raw)
	for _, expected := range []string{"inc-1", "a-1", "triggered", "\"total\":1"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in response: %s", expected, out)
		}
	}
}

type fakeAlertEntryStore struct {
	entries         []alertentry.AlertEntry
	total           int
	lastQueryParams alertentry.QueryParams
}

func (f *fakeAlertEntryStore) Initialize(context.Context) error { return nil }
func (f *fakeAlertEntryStore) WriteAlertEntry(context.Context, *alertentry.AlertEntry) (string, error) {
	return "", nil
}
func (f *fakeAlertEntryStore) QueryAlertEntries(_ context.Context, params alertentry.QueryParams) ([]alertentry.AlertEntry, int, error) {
	f.lastQueryParams = params
	return f.entries, f.total, nil
}
func (f *fakeAlertEntryStore) Close() error { return nil }

type fakeIncidentEntryStore struct {
	entries         []incidententry.IncidentEntry
	total           int
	lastQueryParams incidententry.QueryParams
}

func (f *fakeIncidentEntryStore) Initialize(context.Context) error { return nil }
func (f *fakeIncidentEntryStore) WriteIncidentEntry(context.Context, *incidententry.IncidentEntry) (string, error) {
	return "", nil
}
func (f *fakeIncidentEntryStore) QueryIncidentEntries(_ context.Context, params incidententry.QueryParams) ([]incidententry.IncidentEntry, int, error) {
	f.lastQueryParams = params
	return f.entries, f.total, nil
}
func (f *fakeIncidentEntryStore) Close() error { return nil }
