// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	choreoapis "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

const testCRNamespace = "obs-plane"

// testAlertRule returns a minimal ObservabilityAlertRule CR for webhook tests.
func testAlertRule(crName string, incidentEnabled, triggerRCA bool) *choreoapis.ObservabilityAlertRule {
	return testAlertRuleWithCostAnalysis(crName, incidentEnabled, triggerRCA, false)
}

// testAlertRuleWithCostAnalysis returns an ObservabilityAlertRule CR with optional cost analysis flag.
func testAlertRuleWithCostAnalysis(crName string, incidentEnabled, triggerRCA, triggerCostAnalysis bool) *choreoapis.ObservabilityAlertRule {
	return &choreoapis.ObservabilityAlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: testCRNamespace,
			Labels: map[string]string{
				labels.LabelKeyNamespaceName:   "ns-1",
				labels.LabelKeyComponentUID:    "comp-uid-1",
				labels.LabelKeyEnvironmentUID:  "env-uid-1",
				labels.LabelKeyProjectUID:      "proj-uid-1",
				labels.LabelKeyComponentName:   "payments",
				labels.LabelKeyProjectName:     "commerce",
				labels.LabelKeyEnvironmentName: "prod",
			},
		},
		Spec: choreoapis.ObservabilityAlertRuleSpec{
			Name:        "High Error Rate",
			Description: "Error rate exceeded threshold",
			Severity:    choreoapis.ObservabilityAlertSeverityCritical,
			Source: choreoapis.ObservabilityAlertSource{
				Type:  choreoapis.ObservabilityAlertSourceTypeLog,
				Query: "level=error",
			},
			Condition: choreoapis.ObservabilityAlertCondition{
				Window:    metav1.Duration{Duration: 5 * time.Minute},
				Interval:  metav1.Duration{Duration: 1 * time.Minute},
				Operator:  choreoapis.ObservabilityAlertConditionOperatorGt,
				Threshold: 5,
			},
			Actions: choreoapis.ObservabilityAlertActions{
				Notifications: choreoapis.ObservabilityAlertNotifications{
					Channels: []choreoapis.NotificationChannelName{"slack-main"},
				},
				Incident: &choreoapis.ObservabilityAlertIncident{
					Enabled:               &incidentEnabled,
					TriggerAiRca:          &triggerRCA,
					TriggerAiCostAnalysis: &triggerCostAnalysis,
				},
			},
		},
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, choreoapis.AddToScheme(scheme))
	return scheme
}

type webhookTestFixture struct {
	svc             *AlertService
	alertStore      alertentry.AlertEntryStore
	incidentStore   incidententry.IncidentEntryStore
	rcaCallCount    *atomic.Int32
	rcaServer       *httptest.Server
	finOpsCallCount *atomic.Int32
	finOpsServer    *httptest.Server
}

func newWebhookTestFixture(t *testing.T, suppressionWindow time.Duration, alertRule *choreoapis.ObservabilityAlertRule, aiRCAEnabled bool) *webhookTestFixture {
	return newWebhookTestFixtureWithFinOps(t, suppressionWindow, alertRule, aiRCAEnabled, false)
}

func newWebhookTestFixtureWithFinOps(t *testing.T, suppressionWindow time.Duration, alertRule *choreoapis.ObservabilityAlertRule, aiRCAEnabled, finOpsEnabled bool) *webhookTestFixture {
	t.Helper()

	alertDSN := fmt.Sprintf("file:%s_alerts?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	alertStore, err := alertentry.New(alertentry.BackendSQLite, alertDSN, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, alertStore.Close()) })
	require.NoError(t, alertStore.Initialize(context.Background()))

	incidentDSN := fmt.Sprintf("file:%s_incidents?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	incidentStore, err := incidententry.New(incidententry.BackendSQLite, incidentDSN, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, incidentStore.Close()) })
	require.NoError(t, incidentStore.Initialize(context.Background()))

	var rcaCallCount atomic.Int32
	rcaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rcaCallCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(rcaServer.Close)

	var finOpsCallCount atomic.Int32
	finOpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finOpsCallCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"reportId": "test-report-123", "status": "processing"}`))
	}))
	t.Cleanup(finOpsServer.Close)

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(alertRule).
		Build()

	svc := &AlertService{
		alertEntryStore:    alertStore,
		incidentEntryStore: incidentStore,
		k8sClient:          k8sClient,
		config: &config.Config{
			Alerting: config.AlertingConfig{
				AlertSuppressionWindow: suppressionWindow,
			},
		},
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		rcaServiceURL:      rcaServer.URL,
		aiRCAEnabled:       aiRCAEnabled,
		finOpsAgentURL:     finOpsServer.URL,
		finOpsAgentEnabled: finOpsEnabled,
	}

	return &webhookTestFixture{
		svc:             svc,
		alertStore:      alertStore,
		incidentStore:   incidentStore,
		rcaCallCount:    &rcaCallCount,
		rcaServer:       rcaServer,
		finOpsCallCount: &finOpsCallCount,
		finOpsServer:    finOpsServer,
	}
}

// alertCount returns the total number of alert entries, or -1 on error.
// require is not used because assert.Eventually/Never run conditions in a
// goroutine, and t.FailNow from a non-test goroutine panics.
func (f *webhookTestFixture) alertCount(t *testing.T) int {
	t.Helper()
	_, total, err := f.alertStore.QueryAlertEntries(context.Background(), alertentry.QueryParams{
		StartTime: "2000-01-01T00:00:00Z",
		EndTime:   "2099-01-01T00:00:00Z",
		Limit:     100,
	})
	if err != nil {
		return -1
	}
	return total
}

// incidentCount returns the total number of incident entries, or -1 on error.
// See alertCount for why require is not used.
func (f *webhookTestFixture) incidentCount(t *testing.T) int {
	t.Helper()
	_, total, err := f.incidentStore.QueryIncidentEntries(context.Background(), incidententry.QueryParams{
		StartTime: "2000-01-01T00:00:00Z",
		EndTime:   "2099-01-01T00:00:00Z",
		Limit:     100,
	})
	if err != nil {
		return -1
	}
	return total
}

func webhookReq(crName string) gen.AlertWebhookRequest {
	val := float32(42)
	now := time.Now().UTC()
	ns := testCRNamespace
	return gen.AlertWebhookRequest{
		RuleName:       &crName,
		RuleNamespace:  &ns,
		AlertValue:     &val,
		AlertTimestamp: &now,
	}
}

func TestWebhook_FirstAlert_FullProcessing(t *testing.T) {
	rule := testAlertRule("rule-cr-1", true, true)
	f := newWebhookTestFixture(t, 1*time.Hour, rule, true)

	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, gen.AlertWebhookResponseStatusSuccess, *resp.Status)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	// Alert entry must be persisted
	assert.Equal(t, 1, f.alertCount(t))

	// Incident and RCA run in goroutines — wait briefly for them
	assert.Eventually(t, func() bool { return f.incidentCount(t) == 1 }, 2*time.Second, 50*time.Millisecond,
		"expected 1 incident entry")
	assert.Eventually(t, func() bool { return f.rcaCallCount.Load() == 1 }, 2*time.Second, 50*time.Millisecond,
		"expected 1 RCA call")
}

func TestWebhook_DuplicateWithinWindow_Suppressed(t *testing.T) {
	rule := testAlertRule("rule-cr-1", true, true)
	f := newWebhookTestFixture(t, 1*time.Hour, rule, true)

	// First alert — should be processed
	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")
	assert.Equal(t, 1, f.alertCount(t))

	// Wait for goroutines from first alert
	assert.Eventually(t, func() bool { return f.incidentCount(t) == 1 }, 2*time.Second, 50*time.Millisecond)
	assert.Eventually(t, func() bool { return f.rcaCallCount.Load() == 1 }, 2*time.Second, 50*time.Millisecond)

	// Second alert (same rule, within window) — should be suppressed
	resp, err = f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "suppressed")

	// No new alert entry, incident, or RCA call
	assert.Equal(t, 1, f.alertCount(t), "suppressed alert should not write a new entry")
	assert.Never(t, func() bool { return f.incidentCount(t) > 1 }, 300*time.Millisecond, 50*time.Millisecond,
		"suppressed alert should not create a new incident")
	assert.Equal(t, int32(1), f.rcaCallCount.Load(), "suppressed alert should not trigger RCA")
}

func TestWebhook_DifferentRule_NotSuppressed(t *testing.T) {
	rule1 := testAlertRule("rule-cr-1", false, false)
	rule2 := testAlertRule("rule-cr-2", false, false)

	alertDSN := fmt.Sprintf("file:%s_alerts?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	alertStore, err := alertentry.New(alertentry.BackendSQLite, alertDSN, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, alertStore.Close()) })
	require.NoError(t, alertStore.Initialize(context.Background()))

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(rule1, rule2).
		Build()

	svc := &AlertService{
		alertEntryStore: alertStore,
		k8sClient:       k8sClient,
		config: &config.Config{
			Alerting: config.AlertingConfig{
				AlertSuppressionWindow: 1 * time.Hour,
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	// Different rule — should NOT be suppressed
	resp, err = svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-2"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	_, total, err := alertStore.QueryAlertEntries(context.Background(), alertentry.QueryParams{
		StartTime: "2000-01-01T00:00:00Z",
		EndTime:   "2099-01-01T00:00:00Z",
		Limit:     100,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total, "different rules should both be stored")
}

func TestWebhook_SuppressionDisabled_BothProcessed(t *testing.T) {
	rule := testAlertRule("rule-cr-1", false, false)
	f := newWebhookTestFixture(t, 0, rule, false)

	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	resp, err = f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	assert.Equal(t, 2, f.alertCount(t), "suppression disabled — both alerts should be stored")
}

func TestWebhook_IncidentDisabled_NoIncidentCreated(t *testing.T) {
	rule := testAlertRule("rule-cr-1", false, false)
	f := newWebhookTestFixture(t, 1*time.Hour, rule, false)

	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	assert.Equal(t, 1, f.alertCount(t))

	assert.Never(t, func() bool { return f.incidentCount(t) > 0 }, 300*time.Millisecond, 50*time.Millisecond,
		"incident should not be created when disabled")
	assert.Equal(t, int32(0), f.rcaCallCount.Load(), "RCA should not be triggered when disabled")
}

func TestWebhook_RCADisabled_NoRCACall(t *testing.T) {
	rule := testAlertRule("rule-cr-1", true, true)
	// aiRCAEnabled=false at service level
	f := newWebhookTestFixture(t, 1*time.Hour, rule, false)

	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	assert.Eventually(t, func() bool { return f.incidentCount(t) == 1 }, 2*time.Second, 50*time.Millisecond)

	assert.Never(t, func() bool { return f.rcaCallCount.Load() > 0 }, 300*time.Millisecond, 50*time.Millisecond,
		"RCA should not be triggered when aiRCAEnabled is false")
}

func TestWebhook_RecreatedComponent_NotSuppressed(t *testing.T) {
	rule := testAlertRule("rule-cr-1", false, false)
	f := newWebhookTestFixture(t, 1*time.Hour, rule, false)

	// First alert — should be processed
	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")
	assert.Equal(t, 1, f.alertCount(t))

	// Simulate component deletion and recreation by updating the CR's component UID label
	rule.Labels[labels.LabelKeyComponentUID] = "comp-uid-new"
	require.NoError(t, f.svc.k8sClient.Update(context.Background(), rule))

	// Second alert (same CR name/namespace but different component UID) — should NOT be suppressed
	resp, err = f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged",
		"alert for recreated component should not be suppressed")
	assert.Equal(t, 2, f.alertCount(t))
}

func TestWebhook_MissingComponentUID_SuppressionSkipped(t *testing.T) {
	rule := testAlertRule("rule-cr-1", false, false)
	rule.Labels[labels.LabelKeyComponentUID] = ""
	f := newWebhookTestFixture(t, 1*time.Hour, rule, false)

	// First alert
	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	// Second alert — suppression should be skipped due to missing component UID
	resp, err = f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged",
		"alert should not be suppressed when component UID label is missing")
	assert.Equal(t, 2, f.alertCount(t))
}

func TestWebhook_SuppressionStoreError_AlertStillProcessed(t *testing.T) {
	rule := testAlertRule("rule-cr-1", false, false)
	f := newWebhookTestFixture(t, 1*time.Hour, rule, false)

	// First alert — processed normally
	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	// Close the store to force an error on the next HasRecentAlert call
	require.NoError(t, f.alertStore.Close())

	// Reopen a fresh store for WriteAlertEntry to succeed, but replace the
	// service's store with a closed one so HasRecentAlert fails.
	closedStore := f.svc.alertEntryStore

	freshDSN := fmt.Sprintf("file:%s_fresh?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	freshStore, err := alertentry.New(alertentry.BackendSQLite, freshDSN, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { _ = freshStore.Close() })
	require.NoError(t, freshStore.Initialize(context.Background()))

	// Use the closed store — HasRecentAlert will error, but the alert should still be processed
	f.svc.alertEntryStore = closedStore
	resp, err = f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))

	// HasRecentAlert error is logged as a warning; the write will also fail,
	// so we expect an error from WriteAlertEntry, not a suppressed response.
	// The key assertion: it was NOT suppressed.
	if err != nil {
		assert.NotContains(t, err.Error(), "suppressed")
	} else {
		assert.Contains(t, *resp.Message, "alert acknowledged")
	}
}

func TestWebhook_FinOpsEnabled_TriggersAnalysis(t *testing.T) {
	rule := testAlertRuleWithCostAnalysis("rule-cr-1", true, false, true)
	f := newWebhookTestFixtureWithFinOps(t, 1*time.Hour, rule, false, true)

	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, gen.AlertWebhookResponseStatusSuccess, *resp.Status)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	// Alert entry must be persisted
	assert.Equal(t, 1, f.alertCount(t))

	// FinOps analysis runs in a goroutine — wait briefly for it
	assert.Eventually(t, func() bool { return f.finOpsCallCount.Load() == 1 }, 2*time.Second, 50*time.Millisecond,
		"expected 1 FinOps call")
}

func TestWebhook_FinOpsDisabled_NoAnalysis(t *testing.T) {
	rule := testAlertRuleWithCostAnalysis("rule-cr-1", true, false, true)
	// finOpsEnabled=false at service level
	f := newWebhookTestFixtureWithFinOps(t, 1*time.Hour, rule, false, false)

	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	assert.Equal(t, 1, f.alertCount(t))

	// FinOps should NOT be triggered when disabled
	assert.Never(t, func() bool { return f.finOpsCallCount.Load() > 0 }, 300*time.Millisecond, 50*time.Millisecond,
		"FinOps should not be triggered when finOpsAgentEnabled is false")
}

func TestWebhook_CostAnalysisNotRequested_NoFinOpsCall(t *testing.T) {
	rule := testAlertRuleWithCostAnalysis("rule-cr-1", true, false, false)
	f := newWebhookTestFixtureWithFinOps(t, 1*time.Hour, rule, false, true)

	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	assert.Equal(t, 1, f.alertCount(t))

	// FinOps should NOT be triggered when TriggerAiCostAnalysis is false
	assert.Never(t, func() bool { return f.finOpsCallCount.Load() > 0 }, 300*time.Millisecond, 50*time.Millisecond,
		"FinOps should not be triggered when TriggerAiCostAnalysis is false in rule spec")
}

func TestWebhook_BothRCAAndFinOps_BothTriggered(t *testing.T) {
	rule := testAlertRuleWithCostAnalysis("rule-cr-1", true, true, true)
	f := newWebhookTestFixtureWithFinOps(t, 1*time.Hour, rule, true, true)

	resp, err := f.svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, gen.AlertWebhookResponseStatusSuccess, *resp.Status)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	// Alert entry must be persisted
	assert.Equal(t, 1, f.alertCount(t))

	// Both RCA and FinOps run in goroutines — wait briefly for them
	assert.Eventually(t, func() bool { return f.incidentCount(t) == 1 }, 2*time.Second, 50*time.Millisecond,
		"expected 1 incident entry")
	assert.Eventually(t, func() bool { return f.rcaCallCount.Load() == 1 }, 2*time.Second, 50*time.Millisecond,
		"expected 1 RCA call")
	assert.Eventually(t, func() bool { return f.finOpsCallCount.Load() == 1 }, 2*time.Second, 50*time.Millisecond,
		"expected 1 FinOps call")
}

func TestWebhook_FinOpsServerError_Logged(t *testing.T) {
	rule := testAlertRuleWithCostAnalysis("rule-cr-1", true, false, true)

	// Create a server that returns error
	var finOpsCallCount atomic.Int32
	finOpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finOpsCallCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(finOpsServer.Close)

	alertDSN := fmt.Sprintf("file:%s_alerts?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	alertStore, err := alertentry.New(alertentry.BackendSQLite, alertDSN, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, alertStore.Close()) })
	require.NoError(t, alertStore.Initialize(context.Background()))

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(rule).
		Build()

	svc := &AlertService{
		alertEntryStore: alertStore,
		k8sClient:       k8sClient,
		config: &config.Config{
			Alerting: config.AlertingConfig{
				AlertSuppressionWindow: 1 * time.Hour,
			},
		},
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		finOpsAgentURL:     finOpsServer.URL,
		finOpsAgentEnabled: true,
	}

	resp, err := svc.HandleAlertWebhook(context.Background(), webhookReq("rule-cr-1"))
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	// FinOps should be called even if it fails
	assert.Eventually(t, func() bool { return finOpsCallCount.Load() == 1 }, 2*time.Second, 50*time.Millisecond,
		"expected 1 FinOps call even with server error")
}

func TestWebhook_FinOpsWithEmptyAlertValue(t *testing.T) {
	rule := testAlertRuleWithCostAnalysis("rule-cr-1", true, false, true)
	f := newWebhookTestFixtureWithFinOps(t, 1*time.Hour, rule, false, true)

	// Create a webhook request without alert value
	ns := testCRNamespace
	crName := "rule-cr-1"
	now := time.Now().UTC()
	req := gen.AlertWebhookRequest{
		RuleName:       &crName,
		RuleNamespace:  &ns,
		AlertValue:     nil, // No alert value
		AlertTimestamp: &now,
	}

	resp, err := f.svc.HandleAlertWebhook(context.Background(), req)
	require.NoError(t, err)
	assert.Contains(t, *resp.Message, "alert acknowledged")

	// FinOps should still be called, with 0.0 for alert value
	assert.Eventually(t, func() bool { return f.finOpsCallCount.Load() == 1 }, 2*time.Second, 50*time.Millisecond,
		"expected 1 FinOps call even with empty alert value")
}
