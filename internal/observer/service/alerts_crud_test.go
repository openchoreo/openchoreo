// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
)

const (
	testNamespace = "ns-1"
	testRuleName  = "rule-1"
)

func newTestAlertService() *AlertService {
	return &AlertService{
		config: &config.Config{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestCreateAlertRule_UnsupportedSourceType(t *testing.T) {
	t.Parallel()

	svc := newTestAlertService()

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

func TestGetAlertRule_UnsupportedSourceType(t *testing.T) {
	t.Parallel()

	svc := newTestAlertService()
	_, err := svc.GetAlertRule(context.Background(), testRuleName, "unsupported")
	require.Error(t, err)
}

func TestDeleteAlertRule_UnsupportedSourceType(t *testing.T) {
	t.Parallel()

	svc := newTestAlertService()
	_, err := svc.DeleteAlertRule(context.Background(), testRuleName, "unsupported")
	require.Error(t, err)
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

	ns := testNamespace
	emptyName := ""
	svc := &AlertService{
		k8sClient: fakeK8sClient(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := svc.HandleAlertWebhook(context.Background(), gen.AlertWebhookRequest{
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

// fakeK8sClient returns a minimal fake controller-runtime client for webhook validation tests.
func fakeK8sClient() client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}
