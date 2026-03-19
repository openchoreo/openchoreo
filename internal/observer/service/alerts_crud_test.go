// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

const (
	testNamespace = "ns-1"
	testRuleName  = "rule-1"
)

// alreadyExistsOpenSearchClient returns exists=true from SearchMonitorByName,
// triggering the "already exists" error path on Create.
type alreadyExistsOpenSearchClient struct {
	trackingOpenSearchClient
}

func (c *alreadyExistsOpenSearchClient) SearchMonitorByName(_ context.Context, _ string) (string, bool, error) {
	c.searchCalled.Add(1)
	return "existing-id", true, nil
}

// Helper to build a valid log alert request for CRUD tests.
func newLogAlertRequest(ruleName string) gen.AlertRuleRequest {
	query := "level=error"

	return gen.AlertRuleRequest{
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
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{
			Type:  gen.AlertRuleRequestSourceTypeLog,
			Query: &query,
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

func newTestAlertService(osClient AlertOpenSearchClient) *AlertService {
	cfg := &config.Config{}
	cfg.Adapters.LogsAdapterEnabled = false
	return &AlertService{
		osClient:     osClient,
		queryBuilder: opensearch.NewQueryBuilder("logs-"),
		config:       cfg,
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestCreateAlertRule_AlreadyExists(t *testing.T) {
	t.Parallel()

	svc := newTestAlertService(&alreadyExistsOpenSearchClient{})
	_, err := svc.CreateAlertRule(context.Background(), newLogAlertRequest(testRuleName))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlertRuleAlreadyExists)
}

func TestCreateAlertRule_UnsupportedSourceType(t *testing.T) {
	t.Parallel()

	svc := newTestAlertService(&trackingOpenSearchClient{})

	req := gen.AlertRuleRequest{
		Source: struct {
			Metric *gen.AlertRuleRequestSourceMetric `json:"metric,omitempty"`
			Query  *string                           `json:"query,omitempty"`
			Type   gen.AlertRuleRequestSourceType    `json:"type"`
		}{Type: gen.AlertRuleRequestSourceType("unsupported")},
		Condition: struct {
			Enabled   bool                                  `json:"enabled"`
			Interval  string                                `json:"interval"`
			Operator  gen.AlertRuleRequestConditionOperator `json:"operator"`
			Threshold float32                               `json:"threshold"`
			Window    string                                `json:"window"`
		}{Interval: "1m", Window: "5m"},
	}

	_, err := svc.CreateAlertRule(context.Background(), req)
	require.Error(t, err)
}

func TestGetAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	// trackingOpenSearchClient returns exists=false
	svc := newTestAlertService(&trackingOpenSearchClient{})
	_, err := svc.GetAlertRule(context.Background(), "nonexistent", sourceTypeLog)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlertRuleNotFound)
}

func TestDeleteAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	svc := newTestAlertService(&trackingOpenSearchClient{})
	_, err := svc.DeleteAlertRule(context.Background(), "nonexistent", sourceTypeLog)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlertRuleNotFound)
}

func TestUpdateAlertRule_NotFound(t *testing.T) {
	t.Parallel()

	// trackingOpenSearchClient returns exists=false
	svc := newTestAlertService(&trackingOpenSearchClient{})
	_, err := svc.UpdateAlertRule(context.Background(), "nonexistent", newLogAlertRequest("nonexistent"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlertRuleNotFound)
}

// returnsMonitorOpenSearchClient is like searchExistsOpenSearchClient but
// GetMonitorByID returns a pre-built monitor body so the "unchanged" comparison succeeds.
type returnsMonitorOpenSearchClient struct {
	searchExistsOpenSearchClient
	monitor map[string]any
}

func (c *returnsMonitorOpenSearchClient) GetMonitorByID(_ context.Context, _ string) (map[string]any, error) {
	c.getCalled.Add(1)
	return c.monitor, nil
}

func TestUpdateAlertRule_Unchanged(t *testing.T) {
	t.Parallel()

	req := newLogAlertRequest(testRuleName)
	// Build the expected monitor body the same way the service does.
	svcTmp := newTestAlertService(nil)
	expectedBody, err := svcTmp.buildOpenSearchMonitorBody(req)
	require.NoError(t, err)

	osClient := &returnsMonitorOpenSearchClient{monitor: expectedBody}
	svc := newTestAlertService(osClient)
	resp, err := svc.UpdateAlertRule(context.Background(), testRuleName, req)
	require.NoError(t, err)
	require.NotNil(t, resp.Action)
	assert.Equal(t, alertActionUnchanged, string(*resp.Action))
}

func TestHandleAlertWebhook_NilK8sClient(t *testing.T) {
	t.Parallel()

	svc := &AlertService{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := svc.HandleAlertWebhook(context.Background(), gen.AlertWebhookRequest{})
	require.Error(t, err)
}

func TestHandleAlertWebhook_MissingRuleName(t *testing.T) {
	t.Parallel()

	// Test missing ruleName by providing empty string.
	ns := testNamespace
	emptyName := ""
	svc2 := &AlertService{
		k8sClient: fakeK8sClient(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := svc2.HandleAlertWebhook(context.Background(), gen.AlertWebhookRequest{
		RuleName:      &emptyName,
		RuleNamespace: &ns,
	})
	require.Error(t, err)
}

func TestHandleAlertWebhook_MissingRuleNamespace(t *testing.T) {
	t.Parallel()

	name := testRuleName
	emptyNS := ""
	svc := &AlertService{
		k8sClient: fakeK8sClient(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := svc.HandleAlertWebhook(context.Background(), gen.AlertWebhookRequest{
		RuleName:      &name,
		RuleNamespace: &emptyNS,
	})
	require.Error(t, err)
}

func TestGetAlertRule_UnsupportedSourceType(t *testing.T) {
	t.Parallel()

	svc := newTestAlertService(&trackingOpenSearchClient{})
	_, err := svc.GetAlertRule(context.Background(), testRuleName, "unsupported")
	require.Error(t, err)
}

func TestDeleteAlertRule_UnsupportedSourceType(t *testing.T) {
	t.Parallel()

	svc := newTestAlertService(&trackingOpenSearchClient{})
	_, err := svc.DeleteAlertRule(context.Background(), testRuleName, "unsupported")
	require.Error(t, err)
}

// fakeK8sClient returns a minimal fake controller-runtime client for webhook validation tests.
func fakeK8sClient() client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}
