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
				IncidentEnabled:      true,
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
	for _, expected := range []string{"high-errors", "email-main", "critical", "\"incidentEnabled\":true", "\"total\":1"} {
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
				ID:                    "inc-1",
				AlertID:               "a-1",
				Timestamp:             "2026-03-07T10:20:30Z",
				Status:                incidententry.StatusActive,
				TriggerAiRca:          true,
				TriggerAiCostAnalysis: false,
				TriggeredAt:           "2026-03-07T10:20:30Z",
				Description:           "Investigate error spike",
				NamespaceName:         "team-a",
				ProjectName:           "project-a",
				ComponentName:         "component-a",
				EnvironmentName:       "dev",
				ProjectID:             "b2c3d4e5-6789-01bc-def0-234567890abc",
				ComponentID:           "a1b2c3d4-5678-90ab-cdef-1234567890ab",
				EnvironmentID:         "d4e5f6a7-8901-23de-f012-4567890abcde",
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
	for _, expected := range []string{"inc-1", "a-1", "active", "\"total\":1"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in response: %s", expected, out)
		}
	}
}

func TestAlertServiceUpdateIncident(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	updatedEntry := incidententry.IncidentEntry{
		ID:                    "inc-1",
		AlertID:               "a-1",
		Timestamp:             "2026-03-07T10:20:30Z",
		Status:                incidententry.StatusAcknowledged,
		TriggerAiRca:          true,
		TriggerAiCostAnalysis: false,
		TriggeredAt:           "2026-03-07T10:20:30Z",
		AcknowledgedAt:        "2026-03-07T10:21:00Z",
		Description:           "Updated description",
		Notes:                 "Updated notes",
		NamespaceName:         "team-a",
		ProjectName:           "project-a",
		ComponentName:         "component-a",
		EnvironmentName:       "dev",
		ProjectID:             "b2c3d4e5-6789-01bc-def0-234567890abc",
		ComponentID:           "a1b2c3d4-5678-90ab-cdef-1234567890ab",
		EnvironmentID:         "d4e5f6a7-8901-23de-f012-4567890abcde",
	}

	fakeStore := &fakeIncidentEntryStore{
		updateEntry: updatedEntry,
	}

	svc := &AlertService{
		incidentEntryStore: fakeStore,
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	note := "Updated notes"
	desc := "Updated description"
	req := gen.IncidentPutRequest{
		Status:      gen.IncidentPutRequestStatusAcknowledged,
		Notes:       &note,
		Description: &desc,
	}

	resp, err := svc.UpdateIncident(ctx, "inc-1", req)
	if err != nil {
		t.Fatalf("UpdateIncident failed: %v", err)
	}

	if fakeStore.lastUpdateID != "inc-1" {
		t.Fatalf("expected lastUpdateID=inc-1, got %s", fakeStore.lastUpdateID)
	}
	if fakeStore.lastUpdateStatus != string(gen.IncidentPutRequestStatusAcknowledged) {
		t.Fatalf("expected lastUpdateStatus=%s, got %s", gen.IncidentPutRequestStatusAcknowledged, fakeStore.lastUpdateStatus)
	}
	if fakeStore.lastUpdateNotes == nil || *fakeStore.lastUpdateNotes != note {
		t.Fatalf("expected lastUpdateNotes=%q, got %v", note, fakeStore.lastUpdateNotes)
	}
	if fakeStore.lastUpdateDesc == nil || *fakeStore.lastUpdateDesc != desc {
		t.Fatalf("expected lastUpdateDesc=%q, got %v", desc, fakeStore.lastUpdateDesc)
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}
	out := string(raw)
	for _, expected := range []string{`"incidentId":"inc-1"`, `"alertId":"a-1"`, `"status":"acknowledged"`, `"incidentTriggerAiRca":true`, `"notes":"Updated notes"`, `"description":"Updated description"`} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in response: %s", expected, out)
		}
	}
}

func TestUpdateIncident_PreservesOmittedFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Fake returns entry with existing notes/description (simulating preserve semantics)
	preservedNotes := "existing-notes"
	preservedDesc := "existing-description"
	fakeStore := &fakeIncidentEntryStore{
		updateEntry: incidententry.IncidentEntry{
			ID:                    "inc-1",
			AlertID:               "a-1",
			Timestamp:             "2026-03-07T10:20:30Z",
			Status:                incidententry.StatusAcknowledged,
			TriggerAiRca:          true,
			TriggerAiCostAnalysis: false,
			TriggeredAt:           "2026-03-07T10:20:30Z",
			AcknowledgedAt:        "2026-03-07T10:21:00Z",
			Notes:                 preservedNotes,
			Description:           preservedDesc,
			NamespaceName:         "team-a",
			ProjectName:           "project-a",
			ComponentName:         "component-a",
			EnvironmentName:       "dev",
			ProjectID:             "b2c3d4e5-6789-01bc-def0-234567890abc",
			ComponentID:           "a1b2c3d4-5678-90ab-cdef-1234567890ab",
			EnvironmentID:         "d4e5f6a7-8901-23de-f012-4567890abcde",
		},
	}

	svc := &AlertService{
		incidentEntryStore: fakeStore,
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// Request with only status - no Notes, no Description
	req := gen.IncidentPutRequest{
		Status: gen.IncidentPutRequestStatusAcknowledged,
	}

	resp, err := svc.UpdateIncident(ctx, "inc-1", req)
	if err != nil {
		t.Fatalf("UpdateIncident failed: %v", err)
	}

	// Store should have been called with nil for notes and description
	if fakeStore.lastUpdateNotes != nil {
		t.Fatalf("expected lastUpdateNotes=nil when omitted, got %q", *fakeStore.lastUpdateNotes)
	}
	if fakeStore.lastUpdateDesc != nil {
		t.Fatalf("expected lastUpdateDesc=nil when omitted, got %q", *fakeStore.lastUpdateDesc)
	}

	// Response should contain preserved values from store
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}
	out := string(raw)
	for _, expected := range []string{`"notes":"existing-notes"`, `"description":"existing-description"`} {
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
func (f *fakeAlertEntryStore) HasRecentAlert(context.Context, string, string, string, time.Time) (bool, error) {
	return false, nil
}
func (f *fakeAlertEntryStore) Close() error { return nil }

type fakeIncidentEntryStore struct {
	entries          []incidententry.IncidentEntry
	total            int
	lastQueryParams  incidententry.QueryParams
	updateEntry      incidententry.IncidentEntry
	updateErr        error
	lastUpdateID     string
	lastUpdateStatus string
	lastUpdateNotes  *string
	lastUpdateDesc   *string
}

func (f *fakeIncidentEntryStore) Initialize(context.Context) error { return nil }
func (f *fakeIncidentEntryStore) WriteIncidentEntry(context.Context, *incidententry.IncidentEntry) (string, error) {
	return "", nil
}
func (f *fakeIncidentEntryStore) QueryIncidentEntries(_ context.Context, params incidententry.QueryParams) ([]incidententry.IncidentEntry, int, error) {
	f.lastQueryParams = params
	return f.entries, f.total, nil
}
func (f *fakeIncidentEntryStore) UpdateIncidentEntry(_ context.Context, id string, status string, notes, description *string, _ time.Time) (incidententry.IncidentEntry, error) {
	f.lastUpdateID = id
	f.lastUpdateStatus = status
	f.lastUpdateNotes = notes
	f.lastUpdateDesc = description
	if f.updateErr != nil {
		return incidententry.IncidentEntry{}, f.updateErr
	}
	return f.updateEntry, nil
}
func (f *fakeIncidentEntryStore) Close() error { return nil }
