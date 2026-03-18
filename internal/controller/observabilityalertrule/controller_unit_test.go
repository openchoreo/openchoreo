// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertrule

import (
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// formatMinutesHours
// ---------------------------------------------------------------------------

func TestFormatMinutesHours(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  string
	}{
		{name: "1 minute", input: 1 * time.Minute, want: "1m"},
		{name: "5 minutes", input: 5 * time.Minute, want: "5m"},
		{name: "30 minutes", input: 30 * time.Minute, want: "30m"},
		{name: "60 minutes becomes 1h", input: 60 * time.Minute, want: "1h"},
		{name: "1 hour", input: 1 * time.Hour, want: "1h"},
		{name: "2 hours", input: 2 * time.Hour, want: "2h"},
		{name: "90 minutes stays in minutes", input: 90 * time.Minute, want: "90m"},
		{name: "zero duration", input: 0, want: "0h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMinutesHours(tt.input)
			if got != tt.want {
				t.Errorf("formatMinutesHours(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildAlertRuleRequest
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool { return &b }

func validAlertRule(name string) *openchoreov1alpha1.ObservabilityAlertRule {
	return &openchoreov1alpha1.ObservabilityAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				"openchoreo.dev/component-uid":   "comp-uid-1",
				"openchoreo.dev/project-uid":     "proj-uid-1",
				"openchoreo.dev/environment-uid": "env-uid-1",
			},
		},
		Spec: openchoreov1alpha1.ObservabilityAlertRuleSpec{
			Name: name,
			Source: openchoreov1alpha1.ObservabilityAlertSource{
				Type:  openchoreov1alpha1.ObservabilityAlertSourceTypeLog,
				Query: "error",
			},
			Condition: openchoreov1alpha1.ObservabilityAlertCondition{
				Window:    metav1.Duration{Duration: 5 * time.Minute},
				Interval:  metav1.Duration{Duration: 1 * time.Minute},
				Operator:  openchoreov1alpha1.ObservabilityAlertConditionOperatorGt,
				Threshold: 10,
			},
			Actions: openchoreov1alpha1.ObservabilityAlertActions{
				Notifications: openchoreov1alpha1.ObservabilityAlertNotifications{
					Channels: []openchoreov1alpha1.NotificationChannelName{"ch1"},
				},
			},
		},
	}
}

func TestBuildAlertRuleRequest_MissingLabels(t *testing.T) {
	tests := []struct {
		name      string
		deleteKey string
		wantMsg   string
	}{
		{"missing component-uid", "openchoreo.dev/component-uid", "component UID is required"},
		{"missing project-uid", "openchoreo.dev/project-uid", "project UID is required"},
		{"missing environment-uid", "openchoreo.dev/environment-uid", "environment UID is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := validAlertRule("r-missing")
			delete(rule.Labels, tt.deleteKey)
			_, err := buildAlertRuleRequest(rule)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantMsg {
				t.Errorf("unexpected error message: got %q, want %q", err.Error(), tt.wantMsg)
			}
		})
	}

	t.Run("nil labels returns error for component-uid", func(t *testing.T) {
		rule := validAlertRule("r-nil-labels")
		rule.Labels = nil
		_, err := buildAlertRuleRequest(rule)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBuildAlertRuleRequest_HappyPath(t *testing.T) {
	rule := validAlertRule("my-rule")
	req, err := buildAlertRuleRequest(rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct{ got, want, field string }{
		{req.Metadata.Name, "my-rule", "name"},
		{req.Metadata.Namespace, "default", "namespace"},
		{req.Metadata.ComponentUID, "comp-uid-1", "componentUID"},
		{req.Metadata.ProjectUID, "proj-uid-1", "projectUID"},
		{req.Metadata.EnvironmentUID, "env-uid-1", "environmentUID"},
		{req.Source.Type, "log", "source.type"},
		{req.Source.Query, "error", "source.query"},
		{req.Condition.Window, "5m", "condition.window"},
		{req.Condition.Interval, "1m", "condition.interval"},
		{req.Condition.Operator, "gt", "condition.operator"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
		}
	}
	if req.Condition.Enabled != true {
		t.Error("expected enabled=true (default when nil)")
	}
	if req.Condition.Threshold != 10.0 {
		t.Errorf("threshold: got %v, want 10.0", req.Condition.Threshold)
	}
}

func TestBuildAlertRuleRequest_EnabledFlag(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"nil defaults to true", nil, true},
		{"explicit true", boolPtr(true), true},
		{"explicit false", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := validAlertRule("r-enabled")
			rule.Spec.Enabled = tt.enabled
			req, err := buildAlertRuleRequest(rule)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.Condition.Enabled != tt.want {
				t.Errorf("expected enabled=%v, got %v", tt.want, req.Condition.Enabled)
			}
		})
	}
}

func TestBuildAlertRuleRequest_SourceTypes(t *testing.T) {
	t.Run("metric source", func(t *testing.T) {
		rule := validAlertRule("r-metric")
		rule.Spec.Source = openchoreov1alpha1.ObservabilityAlertSource{
			Type:   openchoreov1alpha1.ObservabilityAlertSourceTypeMetric,
			Metric: "cpu_usage",
		}
		req, err := buildAlertRuleRequest(rule)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Source.Type != "metric" {
			t.Errorf("expected source type %q, got %q", "metric", req.Source.Type)
		}
		if req.Source.Metric != "cpu_usage" {
			t.Errorf("expected metric %q, got %q", "cpu_usage", req.Source.Metric)
		}
	})
}

func TestBuildAlertRuleRequest_ConditionFormatting(t *testing.T) {
	t.Run("1h window is formatted correctly", func(t *testing.T) {
		rule := validAlertRule("r-window")
		rule.Spec.Condition.Window = metav1.Duration{Duration: 1 * time.Hour}
		req, err := buildAlertRuleRequest(rule)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Condition.Window != "1h" {
			t.Errorf("expected window %q, got %q", "1h", req.Condition.Window)
		}
	})

	t.Run("all operators are passed through", func(t *testing.T) {
		operators := []openchoreov1alpha1.ObservabilityAlertConditionOperator{
			openchoreov1alpha1.ObservabilityAlertConditionOperatorGt,
			openchoreov1alpha1.ObservabilityAlertConditionOperatorLt,
			openchoreov1alpha1.ObservabilityAlertConditionOperatorGte,
			openchoreov1alpha1.ObservabilityAlertConditionOperatorLte,
			openchoreov1alpha1.ObservabilityAlertConditionOperatorEq,
		}
		for _, op := range operators {
			rule := validAlertRule("r-op")
			rule.Spec.Condition.Operator = op
			req, err := buildAlertRuleRequest(rule)
			if err != nil {
				t.Fatalf("unexpected error for operator %q: %v", op, err)
			}
			if req.Condition.Operator != string(op) {
				t.Errorf("expected operator %q, got %q", string(op), req.Condition.Operator)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// setStatusCondition
// ---------------------------------------------------------------------------

func TestSetStatusCondition(t *testing.T) {
	t.Run("appends condition when none exists", func(t *testing.T) {
		rule := &openchoreov1alpha1.ObservabilityAlertRule{
			ObjectMeta: metav1.ObjectMeta{Generation: 3},
		}
		setStatusCondition(rule, metav1.ConditionTrue, "SyncSucceeded", "all good")

		if len(rule.Status.Conditions) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(rule.Status.Conditions))
		}
		c := rule.Status.Conditions[0]
		if c.Type != conditionTypeSynced {
			t.Errorf("expected type %q, got %q", conditionTypeSynced, c.Type)
		}
		if c.Status != metav1.ConditionTrue {
			t.Errorf("expected status ConditionTrue, got %q", c.Status)
		}
		if c.Reason != "SyncSucceeded" {
			t.Errorf("expected reason %q, got %q", "SyncSucceeded", c.Reason)
		}
		if c.Message != "all good" {
			t.Errorf("expected message %q, got %q", "all good", c.Message)
		}
		if c.ObservedGeneration != 3 {
			t.Errorf("expected observedGeneration 3, got %d", c.ObservedGeneration)
		}
	})

	t.Run("updates existing condition of same type", func(t *testing.T) {
		rule := &openchoreov1alpha1.ObservabilityAlertRule{
			ObjectMeta: metav1.ObjectMeta{Generation: 5},
			Status: openchoreov1alpha1.ObservabilityAlertRuleStatus{
				Conditions: []metav1.Condition{
					{Type: conditionTypeSynced, Status: metav1.ConditionTrue, Reason: "OldReason", Message: "old msg"},
				},
			},
		}
		setStatusCondition(rule, metav1.ConditionFalse, "SyncFailed", "something broke")

		if len(rule.Status.Conditions) != 1 {
			t.Fatalf("expected 1 condition (updated in place), got %d", len(rule.Status.Conditions))
		}
		c := rule.Status.Conditions[0]
		if c.Status != metav1.ConditionFalse {
			t.Errorf("expected status ConditionFalse, got %q", c.Status)
		}
		if c.Reason != "SyncFailed" {
			t.Errorf("expected reason %q, got %q", "SyncFailed", c.Reason)
		}
		if c.Message != "something broke" {
			t.Errorf("expected message %q, got %q", "something broke", c.Message)
		}
		if c.ObservedGeneration != 5 {
			t.Errorf("expected observedGeneration 5, got %d", c.ObservedGeneration)
		}
	})

	t.Run("preserves other conditions when adding Synced", func(t *testing.T) {
		rule := &openchoreov1alpha1.ObservabilityAlertRule{
			Status: openchoreov1alpha1.ObservabilityAlertRuleStatus{
				Conditions: []metav1.Condition{
					{Type: "OtherCondition", Status: metav1.ConditionTrue, Reason: "OK", Message: "fine"},
				},
			},
		}
		setStatusCondition(rule, metav1.ConditionTrue, "SyncSucceeded", "synced")

		if len(rule.Status.Conditions) != 2 {
			t.Fatalf("expected 2 conditions, got %d", len(rule.Status.Conditions))
		}
		// Verify OtherCondition is still there
		found := false
		for _, c := range rule.Status.Conditions {
			if c.Type == "OtherCondition" {
				found = true
			}
		}
		if !found {
			t.Error("expected OtherCondition to be preserved")
		}
	})

	t.Run("condition type is always Synced", func(t *testing.T) {
		rule := &openchoreov1alpha1.ObservabilityAlertRule{}
		setStatusCondition(rule, metav1.ConditionFalse, "SyncFailed", "error")
		if rule.Status.Conditions[0].Type != "Synced" {
			t.Errorf("expected type \"Synced\", got %q", rule.Status.Conditions[0].Type)
		}
	})
}

// ---------------------------------------------------------------------------
// getObserverInternalBaseURL
// ---------------------------------------------------------------------------

func TestGetObserverInternalBaseURL(t *testing.T) {
	t.Run("returns default when no env vars set", func(t *testing.T) {
		t.Setenv("OBSERVER_INTERNAL_ENDPOINT", "")
		t.Setenv("OBSERVER_ENDPOINT", "")
		// Unset them completely
		os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
		os.Unsetenv("OBSERVER_ENDPOINT")

		got := getObserverInternalBaseURL()
		if got != defaultObserverInternalBaseURL {
			t.Errorf("expected default URL %q, got %q", defaultObserverInternalBaseURL, got)
		}
	})

	t.Run("returns OBSERVER_INTERNAL_ENDPOINT when set", func(t *testing.T) {
		t.Setenv("OBSERVER_INTERNAL_ENDPOINT", "http://custom-observer:9090")
		t.Setenv("OBSERVER_ENDPOINT", "")
		os.Unsetenv("OBSERVER_ENDPOINT")

		got := getObserverInternalBaseURL()
		if got != "http://custom-observer:9090" {
			t.Errorf("expected %q, got %q", "http://custom-observer:9090", got)
		}
	})

	t.Run("returns OBSERVER_ENDPOINT as legacy fallback", func(t *testing.T) {
		os.Unsetenv("OBSERVER_INTERNAL_ENDPOINT")
		t.Setenv("OBSERVER_ENDPOINT", "http://legacy-observer:8080")

		got := getObserverInternalBaseURL()
		if got != "http://legacy-observer:8080" {
			t.Errorf("expected %q, got %q", "http://legacy-observer:8080", got)
		}
	})

	t.Run("OBSERVER_INTERNAL_ENDPOINT takes priority over OBSERVER_ENDPOINT", func(t *testing.T) {
		t.Setenv("OBSERVER_INTERNAL_ENDPOINT", "http://internal:9090")
		t.Setenv("OBSERVER_ENDPOINT", "http://legacy:8080")

		got := getObserverInternalBaseURL()
		if got != "http://internal:9090" {
			t.Errorf("OBSERVER_INTERNAL_ENDPOINT should take priority; got %q", got)
		}
	})
}
