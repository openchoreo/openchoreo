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
	"sync/atomic"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/logsadapterclientgen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

// trackingOpenSearchClient records which methods were called on the AlertOpenSearchClient.
type trackingOpenSearchClient struct {
	searchCalled atomic.Int32
	getCalled    atomic.Int32
	createCalled atomic.Int32
	updateCalled atomic.Int32
	deleteCalled atomic.Int32
}

func (c *trackingOpenSearchClient) SearchMonitorByName(_ context.Context, _ string) (string, bool, error) {
	c.searchCalled.Add(1)
	return "mon-id", false, nil // not found by default (for create path)
}

func (c *trackingOpenSearchClient) GetMonitorByID(_ context.Context, _ string) (map[string]interface{}, error) {
	c.getCalled.Add(1)
	return map[string]interface{}{}, nil
}

func (c *trackingOpenSearchClient) CreateMonitor(_ context.Context, _ map[string]interface{}) (string, int64, error) {
	c.createCalled.Add(1)
	return "new-mon-id", 1700000000000, nil
}

func (c *trackingOpenSearchClient) UpdateMonitor(_ context.Context, _ string, _ map[string]interface{}) (int64, error) {
	c.updateCalled.Add(1)
	return 1700000000000, nil
}

func (c *trackingOpenSearchClient) DeleteMonitor(_ context.Context, _ string) error {
	c.deleteCalled.Add(1)
	return nil
}

// searchExistsOpenSearchClient returns exists=true for SearchMonitorByName so
// that get/update/delete OpenSearch paths succeed.
type searchExistsOpenSearchClient struct {
	trackingOpenSearchClient
}

func (c *searchExistsOpenSearchClient) SearchMonitorByName(_ context.Context, _ string) (string, bool, error) {
	c.searchCalled.Add(1)
	return "mon-id", true, nil
}

func TestAlertServiceRouting(t *testing.T) {
	t.Parallel()

	logSourceType := gen.AlertRuleRequestSourceTypeLog
	ruleName := "test-rule"
	query := "level=error"
	ns := "test-ns"
	condEnabled := true
	condInterval := "1m"
	condOperator := gen.AlertRuleRequestConditionOperatorGt
	condThreshold := float32(10)
	condWindow := "5m"

	logAlertReq := gen.AlertRuleRequest{
		Source: &struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   *gen.AlertRuleRequestSourceType   `json:"type,omitempty"`
		}{
			Type:  &logSourceType,
			Query: &query,
		},
		//nolint:revive,staticcheck // field names match generated code (e.g. ComponentUid not ComponentUID)
		Metadata: &struct {
			ComponentUid   *openapi_types.UUID `json:"componentUid,omitempty"`
			EnvironmentUid *openapi_types.UUID `json:"environmentUid,omitempty"`
			Name           *string             `json:"name,omitempty"`
			Namespace      *string             `json:"namespace,omitempty"`
			ProjectUid     *openapi_types.UUID `json:"projectUid,omitempty"`
		}{
			Name:      &ruleName,
			Namespace: &ns,
		},
		Condition: &struct {
			Enabled   *bool                                  `json:"enabled,omitempty"`
			Interval  *string                                `json:"interval,omitempty"`
			Operator  *gen.AlertRuleRequestConditionOperator `json:"operator,omitempty"`
			Threshold *float32                               `json:"threshold,omitempty"`
			Window    *string                                `json:"window,omitempty"`
		}{
			Enabled:   &condEnabled,
			Interval:  &condInterval,
			Operator:  &condOperator,
			Threshold: &condThreshold,
			Window:    &condWindow,
		},
	}

	type operation struct {
		name string
		// call invokes the operation and returns an error.
		call func(svc *AlertService) error
		// checkOpenSearch verifies that the OpenSearch client received the request.
		checkOpenSearch func(os *trackingOpenSearchClient) bool
	}

	operations := []operation{
		{
			name: "Create",
			call: func(svc *AlertService) error {
				_, err := svc.CreateAlertRule(context.Background(), logAlertReq)
				return err
			},
			checkOpenSearch: func(os *trackingOpenSearchClient) bool {
				return os.createCalled.Load() > 0
			},
		},
		{
			name: "Get",
			call: func(svc *AlertService) error {
				_, err := svc.GetAlertRule(context.Background(), ruleName, sourceTypeLog)
				return err
			},
			checkOpenSearch: func(os *trackingOpenSearchClient) bool {
				return os.searchCalled.Load() > 0
			},
		},
		{
			name: "Update",
			call: func(svc *AlertService) error {
				_, err := svc.UpdateAlertRule(context.Background(), ruleName, logAlertReq)
				return err
			},
			checkOpenSearch: func(os *trackingOpenSearchClient) bool {
				return os.searchCalled.Load() > 0
			},
		},
		{
			name: "Delete",
			call: func(svc *AlertService) error {
				_, err := svc.DeleteAlertRule(context.Background(), ruleName, sourceTypeLog)
				return err
			},
			checkOpenSearch: func(os *trackingOpenSearchClient) bool {
				return os.searchCalled.Load() > 0
			},
		},
	}

	type routingCase struct {
		name           string
		adapterEnabled bool
		adapterNil     bool
		expectAdapter  bool
	}

	routingCases := []routingCase{
		{
			name:           "adapter enabled and non-nil",
			adapterEnabled: true,
			adapterNil:     false,
			expectAdapter:  true,
		},
		{
			name:           "adapter disabled",
			adapterEnabled: false,
			adapterNil:     false,
			expectAdapter:  false,
		},
		{
			name:           "adapter nil",
			adapterEnabled: true,
			adapterNil:     true,
			expectAdapter:  false,
		},
	}

	for _, op := range operations {
		for _, rc := range routingCases {
			t.Run(op.name+"/"+rc.name, func(t *testing.T) {
				t.Parallel()

				// Track adapter HTTP calls.
				var adapterCallCount atomic.Int32
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					adapterCallCount.Add(1)
					// Return a valid sync/rule response.
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet {
						_ = json.NewEncoder(w).Encode(gen.AlertRuleResponse{})
					} else {
						_ = json.NewEncoder(w).Encode(gen.AlertingRuleSyncResponse{})
					}
				}))
				defer ts.Close()

				var logsAdapter *LogsAdapter
				if !rc.adapterNil {
					adapterClient, err := logsadapterclientgen.NewClient(ts.URL)
					if err != nil {
						t.Fatalf("failed to create adapter client: %v", err)
					}
					logsAdapter = &LogsAdapter{
						baseURL:       ts.URL,
						httpClient:    ts.Client(),
						adapterClient: adapterClient,
					}
				}

				// For OpenSearch path, use the "exists" variant for get/update/delete.
				var osClient AlertOpenSearchClient
				if op.name == "Create" {
					osClient = &trackingOpenSearchClient{}
				} else {
					osClient = &searchExistsOpenSearchClient{}
				}

				cfg := &config.Config{}
				cfg.Adapters.LogsAdapterEnabled = rc.adapterEnabled

				svc := &AlertService{
					osClient:     osClient,
					queryBuilder: &opensearch.QueryBuilder{},
					config:       cfg,
					logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
					logsAdapter:  logsAdapter,
				}

				err := op.call(svc)

				if rc.expectAdapter {
					// Should have called the adapter; no error expected.
					if err != nil {
						t.Fatalf("expected adapter call to succeed, got error: %v", err)
					}
					if adapterCallCount.Load() == 0 {
						t.Fatalf("expected adapter to be called, but it was not")
					}
				} else {
					// Should have used OpenSearch path.
					var tracker *trackingOpenSearchClient
					switch v := osClient.(type) {
					case *searchExistsOpenSearchClient:
						tracker = &v.trackingOpenSearchClient
					case *trackingOpenSearchClient:
						tracker = v
					}
					if !op.checkOpenSearch(tracker) {
						t.Fatalf("expected OpenSearch client to be called, but it was not")
					}
					// Adapter should NOT have been called.
					if adapterCallCount.Load() > 0 {
						t.Fatalf("adapter should not have been called when routing to OpenSearch")
					}
				}
			})
		}
	}
}
