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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	choreoapis "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

func TestAlertServiceQueryAlerts(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := choreoapis.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding choreo api scheme: %v", err)
	}

	alertRule := &choreoapis.ObservabilityAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rule-cr",
			Namespace: "obs-ns",
			Labels: map[string]string{
				labels.LabelKeyNamespaceName:   "team-a",
				labels.LabelKeyProjectName:     "project-a",
				labels.LabelKeyComponentName:   "component-a",
				labels.LabelKeyEnvironmentName: "dev",
				labels.LabelKeyProjectUID:      "11111111-1111-1111-1111-111111111111",
				labels.LabelKeyComponentUID:    "22222222-2222-2222-2222-222222222222",
				labels.LabelKeyEnvironmentUID:  "33333333-3333-3333-3333-333333333333",
			},
		},
		Spec: choreoapis.ObservabilityAlertRuleSpec{
			Name:        "high-errors",
			Description: "Errors too high",
			Severity:    choreoapis.ObservabilityAlertSeverityCritical,
			Source: choreoapis.ObservabilityAlertSource{
				Type:  choreoapis.ObservabilityAlertSourceTypeLog,
				Query: "level=error",
			},
			Condition: choreoapis.ObservabilityAlertCondition{
				Operator:  choreoapis.ObservabilityAlertConditionOperatorGt,
				Threshold: 10,
				Window:    metav1.Duration{Duration: 5 * time.Minute},
				Interval:  metav1.Duration{Duration: time.Minute},
			},
			Actions: choreoapis.ObservabilityAlertActions{
				Notifications: choreoapis.ObservabilityAlertNotifications{
					Channels: []choreoapis.NotificationChannelName{"email-main"},
				},
			},
		},
	}

	svc := &AlertService{
		alertEntryStore: &fakeAlertEntryStore{
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
					ProjectID:            "11111111-1111-1111-1111-111111111111",
					ComponentID:          "22222222-2222-2222-2222-222222222222",
					EnvironmentID:        "33333333-3333-3333-3333-333333333333",
				},
			},
			total: 1,
		},
		k8sClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(alertRule).Build(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
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

	svc := &AlertService{
		incidentEntryStore: &fakeIncidentEntryStore{
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
					ProjectID:       "11111111-1111-1111-1111-111111111111",
					ComponentID:     "22222222-2222-2222-2222-222222222222",
					EnvironmentID:   "33333333-3333-3333-3333-333333333333",
				},
			},
			total: 1,
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
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
	entries []alertentry.AlertEntry
	total   int
}

func (f *fakeAlertEntryStore) Initialize(context.Context) error { return nil }
func (f *fakeAlertEntryStore) WriteAlertEntry(context.Context, *alertentry.AlertEntry) (string, error) {
	return "", nil
}
func (f *fakeAlertEntryStore) QueryAlertEntries(context.Context, alertentry.QueryParams) ([]alertentry.AlertEntry, int, error) {
	return f.entries, f.total, nil
}
func (f *fakeAlertEntryStore) Close() error { return nil }

type fakeIncidentEntryStore struct {
	entries []incidententry.IncidentEntry
	total   int
}

func (f *fakeIncidentEntryStore) Initialize(context.Context) error { return nil }
func (f *fakeIncidentEntryStore) WriteIncidentEntry(context.Context, *incidententry.IncidentEntry) (string, error) {
	return "", nil
}
func (f *fakeIncidentEntryStore) QueryIncidentEntries(context.Context, incidententry.QueryParams) ([]incidententry.IncidentEntry, int, error) {
	return f.entries, f.total, nil
}
func (f *fakeIncidentEntryStore) Close() error { return nil }
