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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

const (
	testMetricRuleName = "test-metric-rule"
)

func TestAlertService_MetricsAdapter_CreateAlertRule(t *testing.T) {
	t.Parallel()

	ruleName := testMetricRuleName
	metricName := gen.AlertRuleRequestSourceMetric("cpu_usage")

	metricAlertReq := gen.AlertRuleRequest{
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{
			Type:   gen.AlertRuleRequestSourceTypeMetric,
			Metric: &metricName,
		},
		//nolint:revive,staticcheck // field names match generated code
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

	t.Run("routes to metrics adapter when client is set", func(t *testing.T) {
		t.Parallel()

		var adapterCalled atomic.Bool
		var capturedRequest gen.AlertRuleRequest

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adapterCalled.Store(true)

			// Verify request
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/api/v1alpha1/alerts/rules", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Capture request body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			err = json.Unmarshal(body, &capturedRequest)
			require.NoError(t, err)

			// Return success response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			status := gen.AlertingRuleSyncResponseStatus("synced")
			action := gen.AlertingRuleSyncResponseAction("created")
			syncResp := gen.AlertingRuleSyncResponse{
				Status:        &status,
				Action:        &action,
				RuleLogicalId: &ruleName,
			}
			err = json.NewEncoder(w).Encode(syncResp)
			require.NoError(t, err)
		}))
		defer server.Close()

		svc := &AlertService{
			metricsAdapterURL:    server.URL,
			metricsAdapterClient: server.Client(),
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		resp, err := svc.CreateAlertRule(context.Background(), metricAlertReq)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, adapterCalled.Load())
		assert.Equal(t, ruleName, capturedRequest.Metadata.Name)
		require.NotNil(t, capturedRequest.Source.Metric)
		assert.Equal(t, metricName, *capturedRequest.Source.Metric)
	})

	t.Run("routes to prometheus when metrics adapter client is nil", func(t *testing.T) {
		t.Parallel()

		svc := &AlertService{
			metricsAdapterURL:    "http://should-not-be-called",
			metricsAdapterClient: nil, // nil client should route to Prometheus
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		// This will fail because we haven't mocked Prometheus, but we're testing routing logic
		_, err := svc.CreateAlertRule(context.Background(), metricAlertReq)

		// We expect an error since Prometheus is not mocked, but the important thing is
		// that it attempted the Prometheus path (not the adapter path)
		require.Error(t, err)
	})
}

func TestAlertService_MetricsAdapter_GetAlertRule(t *testing.T) {
	t.Parallel()

	ruleName := testMetricRuleName

	t.Run("routes to metrics adapter when client is set", func(t *testing.T) {
		t.Parallel()

		var adapterCalled atomic.Bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adapterCalled.Store(true)

			// Verify request
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/api/v1alpha1/alerts/rules/"+ruleName, r.URL.Path)

			// Return success response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			//nolint:revive,staticcheck // field names match generated code
			ruleResp := gen.AlertRuleResponse{
				Metadata: &struct {
					ComponentUid   *openapi_types.UUID `json:"componentUid,omitempty"`
					EnvironmentUid *openapi_types.UUID `json:"environmentUid,omitempty"`
					Name           *string             `json:"name,omitempty"`
					Namespace      *string             `json:"namespace,omitempty"`
					ProjectUid     *openapi_types.UUID `json:"projectUid,omitempty"`
				}{
					Name: &ruleName,
				},
			}
			err := json.NewEncoder(w).Encode(ruleResp)
			require.NoError(t, err)
		}))
		defer server.Close()

		svc := &AlertService{
			metricsAdapterURL:    server.URL,
			metricsAdapterClient: server.Client(),
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		resp, err := svc.GetAlertRule(context.Background(), ruleName, sourceTypeMetric)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, adapterCalled.Load())
		require.NotNil(t, resp.Metadata)
		require.NotNil(t, resp.Metadata.Name)
		assert.Equal(t, ruleName, *resp.Metadata.Name)
	})

	t.Run("routes to prometheus when metrics adapter client is nil", func(t *testing.T) {
		t.Parallel()

		svc := &AlertService{
			metricsAdapterURL:    "http://should-not-be-called",
			metricsAdapterClient: nil,
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		_, err := svc.GetAlertRule(context.Background(), ruleName, sourceTypeMetric)

		// Should attempt Prometheus path and fail (since not mocked)
		require.Error(t, err)
	})
}

func TestAlertService_MetricsAdapter_UpdateAlertRule(t *testing.T) {
	t.Parallel()

	ruleName := testMetricRuleName
	metricName := gen.AlertRuleRequestSourceMetric("cpu_usage")

	metricAlertReq := gen.AlertRuleRequest{
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{
			Type:   gen.AlertRuleRequestSourceTypeMetric,
			Metric: &metricName,
		},
		//nolint:revive,staticcheck // field names match generated code
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
			Threshold: float32(20),
			Window:    "5m",
		},
	}

	t.Run("routes to metrics adapter when client is set", func(t *testing.T) {
		t.Parallel()

		var adapterCalled atomic.Bool
		var capturedRequest gen.AlertRuleRequest

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adapterCalled.Store(true)

			// Verify request
			assert.Equal(t, http.MethodPut, r.Method)
			assert.Equal(t, "/api/v1alpha1/alerts/rules/"+ruleName, r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Capture request body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			err = json.Unmarshal(body, &capturedRequest)
			require.NoError(t, err)

			// Return success response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			status := gen.AlertingRuleSyncResponseStatus("synced")
			action := gen.AlertingRuleSyncResponseAction("updated")
			syncResp := gen.AlertingRuleSyncResponse{
				Status:        &status,
				Action:        &action,
				RuleLogicalId: &ruleName,
			}
			err = json.NewEncoder(w).Encode(syncResp)
			require.NoError(t, err)
		}))
		defer server.Close()

		svc := &AlertService{
			metricsAdapterURL:    server.URL,
			metricsAdapterClient: server.Client(),
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		resp, err := svc.UpdateAlertRule(context.Background(), ruleName, metricAlertReq)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, adapterCalled.Load())
		assert.Equal(t, float32(20), capturedRequest.Condition.Threshold)
	})

	t.Run("routes to prometheus when metrics adapter client is nil", func(t *testing.T) {
		t.Parallel()

		svc := &AlertService{
			metricsAdapterURL:    "http://should-not-be-called",
			metricsAdapterClient: nil,
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		_, err := svc.UpdateAlertRule(context.Background(), ruleName, metricAlertReq)

		// Should attempt Prometheus path and fail (since not mocked)
		require.Error(t, err)
	})
}

func TestAlertService_MetricsAdapter_DeleteAlertRule(t *testing.T) {
	t.Parallel()

	ruleName := testMetricRuleName

	t.Run("routes to metrics adapter when client is set", func(t *testing.T) {
		t.Parallel()

		var adapterCalled atomic.Bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adapterCalled.Store(true)

			// Verify request
			assert.Equal(t, http.MethodDelete, r.Method)
			assert.Equal(t, "/api/v1alpha1/alerts/rules/"+ruleName, r.URL.Path)

			// Return success response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			status := gen.AlertingRuleSyncResponseStatus("synced")
			action := gen.AlertingRuleSyncResponseAction("deleted")
			syncResp := gen.AlertingRuleSyncResponse{
				Status:        &status,
				Action:        &action,
				RuleLogicalId: &ruleName,
			}
			err := json.NewEncoder(w).Encode(syncResp)
			require.NoError(t, err)
		}))
		defer server.Close()

		svc := &AlertService{
			metricsAdapterURL:    server.URL,
			metricsAdapterClient: server.Client(),
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		resp, err := svc.DeleteAlertRule(context.Background(), ruleName, sourceTypeMetric)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, adapterCalled.Load())
	})

	t.Run("routes to prometheus when metrics adapter client is nil", func(t *testing.T) {
		t.Parallel()

		svc := &AlertService{
			metricsAdapterURL:    "http://should-not-be-called",
			metricsAdapterClient: nil,
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		_, err := svc.DeleteAlertRule(context.Background(), ruleName, sourceTypeMetric)

		// Should attempt Prometheus path and fail (since not mocked)
		require.Error(t, err)
	})
}

func TestAlertService_MetricsAdapter_HTTPErrors(t *testing.T) {
	t.Parallel()

	ruleName := testMetricRuleName

	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErr    error
		expectedErrMsg string
	}{
		{
			name:           "404 not found",
			statusCode:     http.StatusNotFound,
			responseBody:   `{"error": "rule not found"}`,
			expectedErr:    ErrAlertRuleNotFound,
			expectedErrMsg: "adapter returned 404",
		},
		{
			name:           "409 conflict",
			statusCode:     http.StatusConflict,
			responseBody:   `{"error": "rule already exists"}`,
			expectedErr:    ErrAlertRuleAlreadyExists,
			expectedErrMsg: "adapter returned 409",
		},
		{
			name:         "500 internal server error",
			statusCode:   http.StatusInternalServerError,
			responseBody: `{"error": "internal error"}`,
			// No sentinel error for 500, just check the message
			expectedErrMsg: "metrics adapter returned HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.responseBody))
				require.NoError(t, err)
			}))
			defer server.Close()

			svc := &AlertService{
				metricsAdapterURL:    server.URL,
				metricsAdapterClient: server.Client(),
				logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
			}

			// Test with Get operation
			resp, err := svc.GetAlertRule(context.Background(), ruleName, sourceTypeMetric)

			assert.Nil(t, resp)
			require.Error(t, err)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			}
			assert.Contains(t, err.Error(), tt.expectedErrMsg)
		})
	}
}

func TestAlertService_MetricsAdapterRouting_Integration(t *testing.T) {
	t.Parallel()

	ruleName := "integration-test-rule"
	query := "level=error"

	logAlertReq := gen.AlertRuleRequest{
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{
			Type:  gen.AlertRuleRequestSourceTypeLog,
			Query: &query,
		},
		//nolint:revive,staticcheck // field names match generated code
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

	metricName := gen.AlertRuleRequestSourceMetric("cpu_usage")
	metricAlertReq := gen.AlertRuleRequest{
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{
			Type:   gen.AlertRuleRequestSourceTypeMetric,
			Metric: &metricName,
		},
		//nolint:revive,staticcheck // field names match generated code
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

	t.Run("log alerts do not use metrics adapter", func(t *testing.T) {
		t.Parallel()

		var metricsAdapterCalled atomic.Bool

		metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			metricsAdapterCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		}))
		defer metricsServer.Close()

		osClient := &trackingOpenSearchClient{}

		svc := &AlertService{
			osClient:             osClient,
			queryBuilder:         &opensearch.QueryBuilder{},
			config:               &config.Config{},
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
			metricsAdapterURL:    metricsServer.URL,
			metricsAdapterClient: metricsServer.Client(),
		}

		_, err := svc.CreateAlertRule(context.Background(), logAlertReq)

		require.NoError(t, err)
		assert.False(t, metricsAdapterCalled.Load(), "metrics adapter should not be called for log alerts")
		assert.Greater(t, osClient.createCalled.Load(), int32(0), "OpenSearch should be called for log alerts")
	})

	t.Run("metric alerts use metrics adapter when available", func(t *testing.T) {
		t.Parallel()

		var metricsAdapterCalled atomic.Bool

		metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			metricsAdapterCalled.Store(true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			status := gen.AlertingRuleSyncResponseStatus("synced")
			action := gen.AlertingRuleSyncResponseAction("created")
			syncResp := gen.AlertingRuleSyncResponse{
				Status:        &status,
				Action:        &action,
				RuleLogicalId: &ruleName,
			}
			_ = json.NewEncoder(w).Encode(syncResp)
		}))
		defer metricsServer.Close()

		osClient := &trackingOpenSearchClient{}

		svc := &AlertService{
			osClient:             osClient,
			queryBuilder:         &opensearch.QueryBuilder{},
			config:               &config.Config{},
			logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
			metricsAdapterURL:    metricsServer.URL,
			metricsAdapterClient: metricsServer.Client(),
		}

		_, err := svc.CreateAlertRule(context.Background(), metricAlertReq)

		require.NoError(t, err)
		assert.True(t, metricsAdapterCalled.Load(), "metrics adapter should be called for metric alerts")
		assert.Equal(t, int32(0), osClient.createCalled.Load(), "OpenSearch should not be called when using metrics adapter")
	})
}
