// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
)

func newLogAlertRoutingRequest(ruleName string) gen.AlertRuleRequest {
	query := "level=error"
	return gen.AlertRuleRequest{
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{
			Type:  gen.AlertRuleRequestSourceTypeLog,
			Query: &query,
		},
		//nolint:revive,staticcheck
		Metadata: struct {
			ComponentUid   openapi_types.UUID `json:"componentUid"`
			EnvironmentUid openapi_types.UUID `json:"environmentUid"`
			Name           string             `json:"name"`
			Namespace      string             `json:"namespace"`
			ProjectUid     openapi_types.UUID `json:"projectUid"`
		}{
			Name:      ruleName,
			Namespace: "test-ns",
		},
		Condition: struct {
			Enabled   bool                                  `json:"enabled"`
			Interval  string                                `json:"interval"`
			Operator  gen.AlertRuleRequestConditionOperator `json:"operator"`
			Threshold float32                               `json:"threshold"`
			Window    string                                `json:"window"`
		}{
			Enabled:   true,
			Interval:  "1m",
			Operator:  gen.AlertRuleRequestConditionOperatorGt,
			Threshold: float32(10),
			Window:    "5m",
		},
	}
}

// TestLogAlertRoutesViaAdapter verifies that log alert CRUD operations route to the logs adapter.
func TestLogAlertRoutesViaAdapter(t *testing.T) {
	t.Parallel()

	ruleName := "test-rule"

	type operation struct {
		name string
		call func(svc *AlertService) error
	}

	operations := []operation{
		{name: "Create", call: func(svc *AlertService) error {
			_, err := svc.CreateAlertRule(context.Background(), newLogAlertRoutingRequest(ruleName))
			return err
		}},
		{name: "Get", call: func(svc *AlertService) error {
			_, err := svc.GetAlertRule(context.Background(), ruleName, sourceTypeLog)
			return err
		}},
		{name: "Update", call: func(svc *AlertService) error {
			_, err := svc.UpdateAlertRule(context.Background(), ruleName, newLogAlertRoutingRequest(ruleName))
			return err
		}},
		{name: "Delete", call: func(svc *AlertService) error {
			_, err := svc.DeleteAlertRule(context.Background(), ruleName, sourceTypeLog)
			return err
		}},
	}

	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			t.Parallel()

			var adapterCallCount atomic.Int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				adapterCallCount.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_ = json.NewEncoder(w).Encode(gen.AlertRuleResponse{})
				} else {
					_ = json.NewEncoder(w).Encode(gen.AlertingRuleSyncResponse{})
				}
			}))
			defer ts.Close()

			adapterClient, err := logsadapterclientgen.NewClient(ts.URL)
			if err != nil {
				t.Fatalf("failed to create adapter client: %v", err)
			}
			logsAdapter := &LogsAdapter{
				baseURL:       ts.URL,
				httpClient:    ts.Client(),
				adapterClient: adapterClient,
			}

			svc := &AlertService{
				config:      &config.Config{},
				logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
				logsAdapter: logsAdapter,
			}

			if err := op.call(svc); err != nil {
				t.Fatalf("expected adapter call to succeed, got error: %v", err)
			}
			if adapterCallCount.Load() == 0 {
				t.Fatalf("expected logs adapter to be called, but it was not")
			}
		})
	}
}

// TestLogAlertWithoutAdapter verifies that log alert CRUD fails when the logs adapter is not configured.
func TestLogAlertWithoutAdapter(t *testing.T) {
	t.Parallel()

	ruleName := "test-rule"
	svc := &AlertService{
		config: &config.Config{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		// logsAdapter intentionally nil
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"Create", func() error {
			_, err := svc.CreateAlertRule(context.Background(), newLogAlertRoutingRequest(ruleName))
			return err
		}},
		{"Get", func() error {
			_, err := svc.GetAlertRule(context.Background(), ruleName, sourceTypeLog)
			return err
		}},
		{"Update", func() error {
			_, err := svc.UpdateAlertRule(context.Background(), ruleName, newLogAlertRoutingRequest(ruleName))
			return err
		}},
		{"Delete", func() error {
			_, err := svc.DeleteAlertRule(context.Background(), ruleName, sourceTypeLog)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("expected error when logs adapter is not configured")
			}
			if !strings.Contains(err.Error(), "logs adapter is required") {
				t.Fatalf("expected error about missing logs adapter, got: %v", err)
			}
		})
	}
}

// TestBudgetAlertServiceRouting verifies that budget alerts are routed to the metrics adapter.
func TestBudgetAlertServiceRouting(t *testing.T) {
	t.Parallel()

	ruleName := "budget-rule"

	budgetAlertReq := gen.AlertRuleRequest{
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{
			Type: gen.AlertRuleRequestSourceTypeBudget,
		},
		//nolint:revive,staticcheck
		Metadata: struct {
			ComponentUid   openapi_types.UUID `json:"componentUid"`
			EnvironmentUid openapi_types.UUID `json:"environmentUid"`
			Name           string             `json:"name"`
			Namespace      string             `json:"namespace"`
			ProjectUid     openapi_types.UUID `json:"projectUid"`
		}{
			Name:      ruleName,
			Namespace: "test-ns",
		},
		Condition: struct {
			Enabled   bool                                  `json:"enabled"`
			Interval  string                                `json:"interval"`
			Operator  gen.AlertRuleRequestConditionOperator `json:"operator"`
			Threshold float32                               `json:"threshold"`
			Window    string                                `json:"window"`
		}{
			Enabled:   true,
			Interval:  "1h",
			Operator:  gen.AlertRuleRequestConditionOperatorGt,
			Threshold: float32(5),
			Window:    "24h",
		},
	}

	// Track metrics adapter HTTP calls
	var adapterCallCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adapterCallCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(gen.AlertRuleResponse{})
		} else {
			status := gen.Synced
			action := gen.Created
			if r.Method == http.MethodPut {
				action = gen.Updated
			} else if r.Method == http.MethodDelete {
				action = gen.Deleted
			}
			logicalID := ruleName
			backendID := "backend-id"
			lastSynced := "2024-01-01T00:00:00Z"
			_ = json.NewEncoder(w).Encode(gen.AlertingRuleSyncResponse{
				Status:        &status,
				Action:        &action,
				RuleLogicalId: &logicalID,
				RuleBackendId: &backendID,
				LastSyncedAt:  &lastSynced,
			})
		}
	}))
	defer ts.Close()

	cfg := &config.Config{}
	svc := &AlertService{
		config:               cfg,
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		metricsAdapterURL:    ts.URL,
		metricsAdapterClient: ts.Client(),
	}

	resp, err := svc.CreateAlertRule(context.Background(), budgetAlertReq)
	if err != nil {
		t.Fatalf("CreateAlertRule failed: %v", err)
	}
	if resp == nil || resp.Action == nil || string(*resp.Action) != "created" {
		t.Fatalf("CreateAlertRule response invalid: %+v", resp)
	}

	resp, err = svc.UpdateAlertRule(context.Background(), ruleName, budgetAlertReq)
	if err != nil {
		t.Fatalf("UpdateAlertRule failed: %v", err)
	}
	if resp == nil || resp.Action == nil || string(*resp.Action) != "updated" {
		t.Fatalf("UpdateAlertRule response invalid: %+v", resp)
	}

	if _, err = svc.GetAlertRule(context.Background(), ruleName, sourceTypeBudget); err != nil {
		t.Fatalf("GetAlertRule failed: %v", err)
	}

	resp, err = svc.DeleteAlertRule(context.Background(), ruleName, sourceTypeBudget)
	if err != nil {
		t.Fatalf("DeleteAlertRule failed: %v", err)
	}
	if resp == nil || resp.Action == nil || string(*resp.Action) != "deleted" {
		t.Fatalf("DeleteAlertRule response invalid: %+v", resp)
	}

	if adapterCallCount.Load() == 0 {
		t.Fatalf("expected metrics adapter to be called, but it was not")
	}
}

// TestBudgetAlertWithoutAdapter verifies that budget alerts fail when metrics adapter is not configured.
func TestBudgetAlertWithoutAdapter(t *testing.T) {
	t.Parallel()

	ruleName := "budget-rule"
	budgetAlertReq := gen.AlertRuleRequest{
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{
			Type: gen.AlertRuleRequestSourceTypeBudget,
		},
		//nolint:revive,staticcheck
		Metadata: struct {
			ComponentUid   openapi_types.UUID `json:"componentUid"`
			EnvironmentUid openapi_types.UUID `json:"environmentUid"`
			Name           string             `json:"name"`
			Namespace      string             `json:"namespace"`
			ProjectUid     openapi_types.UUID `json:"projectUid"`
		}{
			Name:      ruleName,
			Namespace: "test-ns",
		},
		Condition: struct {
			Enabled   bool                                  `json:"enabled"`
			Interval  string                                `json:"interval"`
			Operator  gen.AlertRuleRequestConditionOperator `json:"operator"`
			Threshold float32                               `json:"threshold"`
			Window    string                                `json:"window"`
		}{
			Enabled:   true,
			Interval:  "1h",
			Operator:  gen.AlertRuleRequestConditionOperatorGt,
			Threshold: float32(100),
			Window:    "24h",
		},
	}

	cfg := &config.Config{}
	svc := &AlertService{
		config:               cfg,
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		metricsAdapterClient: nil,
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"Create", func() error {
			_, err := svc.CreateAlertRule(context.Background(), budgetAlertReq)
			return err
		}},
		{"Update", func() error {
			_, err := svc.UpdateAlertRule(context.Background(), ruleName, budgetAlertReq)
			return err
		}},
		{"Get", func() error {
			_, err := svc.GetAlertRule(context.Background(), ruleName, sourceTypeBudget)
			return err
		}},
		{"Delete", func() error {
			_, err := svc.DeleteAlertRule(context.Background(), ruleName, sourceTypeBudget)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("expected error when metrics adapter is not configured for budget alerts")
			}
			if !strings.Contains(err.Error(), "metrics adapter is required for budget alert rules") {
				t.Fatalf("expected error about missing metrics adapter, got: %v", err)
			}
		})
	}
}
