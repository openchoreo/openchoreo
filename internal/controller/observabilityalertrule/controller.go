// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertrule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	defaultObserverBaseURL = "http://observer.openchoreo-observability-plane:8080"
	alertingRulePath       = "/api/alerting/rule"
	conditionTypeSynced    = "Synced"
	// AlertRuleCleanupFinalizer is used to ensure alert rules are deleted from the backend before the CR is removed
	AlertRuleCleanupFinalizer = "openchoreo.dev/alertrule-cleanup"
)

// AlertingRuleMetadata represents the metadata section of the observer alerting rule API.
type AlertingRuleMetadata struct {
	Name                      string `json:"name"`
	ComponentUID              string `json:"component-uid,omitempty"`
	ProjectUID                string `json:"project-uid,omitempty"`
	EnvironmentUID            string `json:"environment-uid,omitempty"`
	Severity                  string `json:"severity,omitempty"`
	EnableAiRootCauseAnalysis bool   `json:"enableAiRootCauseAnalysis,omitempty"`
	NotificationChannel       string `json:"notificationChannel,omitempty"`
}

// AlertingRuleSource represents the source section of the observer alerting rule API.
type AlertingRuleSource struct {
	Type  string `json:"type"`
	Query string `json:"query,omitempty"`
	// Metric is kept for future metric-based alerting support.
	Metric string `json:"metric,omitempty"`
}

// AlertingRuleCondition represents the condition section of the observer alerting rule API.
type AlertingRuleCondition struct {
	Enabled   bool   `json:"enabled"`
	Window    string `json:"window"`
	Interval  string `json:"interval"`
	Operator  string `json:"operator"`
	Threshold int64  `json:"threshold"`
}

// AlertingRuleRequest is the payload sent to the observer alerting API.
type AlertingRuleRequest struct {
	Metadata  AlertingRuleMetadata  `json:"metadata"`
	Source    AlertingRuleSource    `json:"source"`
	Condition AlertingRuleCondition `json:"condition"`
}

// AlertingRuleSyncResponse is the response from the observer alerting API.
type AlertingRuleSyncResponse struct {
	Status     string `json:"status"`
	LogicalID  string `json:"logicalId"`
	BackendID  string `json:"backendId"`
	Action     string `json:"action"`
	LastSynced string `json:"lastSynced"`
}

// Reconciler reconciles a ObservabilityAlertRule object
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	httpClient *http.Client
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertrules/finalizers,verbs=update

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ObservabilityAlertRule instance.
	alertRule := &openchoreov1alpha1.ObservabilityAlertRule{}
	if err := r.Get(ctx, req.NamespacedName, alertRule); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted – nothing to do.
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get ObservabilityAlertRule")
		return ctrl.Result{}, err
	}

	// Handle deletion - delete alert rule from observer backend
	if !alertRule.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, alertRule)
	}

	// Ensure finalizer is added for cleanup
	if !controllerutil.ContainsFinalizer(alertRule, AlertRuleCleanupFinalizer) {
		controllerutil.AddFinalizer(alertRule, AlertRuleCleanupFinalizer)
		if err := r.Update(ctx, alertRule); err != nil {
			logger.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Re-fetch after update to avoid conflicts
		return ctrl.Result{Requeue: true}, nil
	}

	// Build the alerting rule request from the CR spec.
	requestPayload, err := buildAlertingRuleRequest(alertRule)
	if err != nil {
		logger.Error(err, "failed to build alerting rule request")
		return r.updateStatusWithError(ctx, alertRule, err)
	}

	observerURL := getObserverBaseURL()
	url := fmt.Sprintf("%s%s/%s/%s", observerURL, alertingRulePath, alertRule.Spec.Source.Type, alertRule.Name)

	bodyBytes, err := json.Marshal(requestPayload)
	if err != nil {
		logger.Error(err, "failed to marshal alerting rule request")
		return r.updateStatusWithError(ctx, alertRule, err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPut, url, bytes.NewReader(bodyBytes))
	if err != nil {
		logger.Error(err, "failed to create HTTP request")
		return r.updateStatusWithError(ctx, alertRule, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		logger.Error(err, "failed to call observer alerting API")
		return r.updateStatusWithError(ctx, alertRule, err)
	}
	defer resp.Body.Close()

	var syncResp AlertingRuleSyncResponse
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
			logger.Error(err, "failed to decode observer alerting API response")
			return r.updateStatusWithError(ctx, alertRule, err)
		}
	} else {
		// Best-effort read of error body for diagnostics.
		var errBody struct {
			Message string `json:"message,omitempty"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		err = fmt.Errorf("observer alerting API returned status %d: %s", resp.StatusCode, errBody.Message)
		logger.Error(err, "observer alerting API call failed")
		return r.updateStatusWithError(ctx, alertRule, err)
	}

	// Update status from sync response.
	now := metav1.NewTime(time.Now())
	alertRule.Status.ObservedGeneration = alertRule.GetGeneration()
	alertRule.Status.LastReconcileTime = &now
	alertRule.Status.BackendMonitorID = syncResp.BackendID

	if syncResp.LastSynced != "" {
		if t, err := time.Parse(time.RFC3339, syncResp.LastSynced); err == nil {
			ts := metav1.NewTime(t)
			alertRule.Status.LastSyncTime = &ts
		}
	}

	// Phase and conditions.
	if syncResp.Status == "synced" {
		alertRule.Status.Phase = openchoreov1alpha1.ObservabilityAlertRulePhaseReady
		setStatusCondition(alertRule, metav1.ConditionTrue, "SyncSucceeded", fmt.Sprintf("Alert rule %s %s in backend", syncResp.LogicalID, syncResp.Action))
	} else {
		alertRule.Status.Phase = openchoreov1alpha1.ObservabilityAlertRulePhaseError
		setStatusCondition(alertRule, metav1.ConditionFalse, "SyncNotSynced", fmt.Sprintf("Alert rule status reported as %q", syncResp.Status))
	}

	if err := r.Status().Update(ctx, alertRule); err != nil {
		logger.Error(err, "failed to update ObservabilityAlertRule status after sync")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildAlertingRuleRequest converts the ObservabilityAlertRule spec into the observer API request payload.
func buildAlertingRuleRequest(rule *openchoreov1alpha1.ObservabilityAlertRule) (*AlertingRuleRequest, error) {
	spec := rule.Spec

	// Derive metadata UIDs from labels if present.
	componentUID := rule.Labels["openchoreo.dev/component-uid"]
	projectUID := rule.Labels["openchoreo.dev/project-uid"]
	environmentUID := rule.Labels["openchoreo.dev/environment-uid"]

	if componentUID == "" {
		return nil, fmt.Errorf("component UID is required")
	}
	if projectUID == "" {
		return nil, fmt.Errorf("project UID is required")
	}
	if environmentUID == "" {
		return nil, fmt.Errorf("environment UID is required")
	}

	enabled := true
	if spec.Enabled != nil {
		enabled = *spec.Enabled
	}

	req := &AlertingRuleRequest{
		Metadata: AlertingRuleMetadata{
			Name:                      rule.Name,
			ComponentUID:              componentUID,
			ProjectUID:                projectUID,
			EnvironmentUID:            environmentUID,
			Severity:                  string(spec.Severity),
			EnableAiRootCauseAnalysis: spec.EnableAiRootCauseAnalysis,
			NotificationChannel:       spec.NotificationChannel,
		},
		Source: AlertingRuleSource{
			Type:   string(spec.Source.Type),
			Query:  spec.Source.Query,
			Metric: spec.Source.Metric,
		},
		Condition: AlertingRuleCondition{
			Enabled:   enabled,
			Window:    spec.Condition.Window.Duration.String(),
			Interval:  spec.Condition.Interval.Duration.String(),
			Operator:  string(spec.Condition.Operator),
			Threshold: spec.Condition.Threshold,
		},
	}

	return req, nil
}

// getObserverBaseURL returns the observer base URL, allowing override via environment variable.
func getObserverBaseURL() string {
	if v := os.Getenv("OBSERVER_ENDPOINT"); v != "" {
		return v
	}
	return defaultObserverBaseURL
}

// updateStatusWithError sets the rule status to Error and records a failing condition.
// nolint:unparam // Result is always empty but required by controller-runtime interface
func (r *Reconciler) updateStatusWithError(ctx context.Context, rule *openchoreov1alpha1.ObservabilityAlertRule, err error) (ctrl.Result, error) {
	now := metav1.NewTime(time.Now())
	rule.Status.ObservedGeneration = rule.GetGeneration()
	rule.Status.LastReconcileTime = &now
	rule.Status.Phase = openchoreov1alpha1.ObservabilityAlertRulePhaseError
	setStatusCondition(rule, metav1.ConditionFalse, "SyncFailed", err.Error())

	// Best-effort status update; on conflict, let the reconcile be retried.
	_ = r.Status().Update(ctx, rule)

	return ctrl.Result{}, err
}

// setStatusCondition upserts a condition on the rule status.
func setStatusCondition(rule *openchoreov1alpha1.ObservabilityAlertRule, status metav1.ConditionStatus, reason, message string) {
	now := metav1.NewTime(time.Now())
	newCond := metav1.Condition{
		Type:               conditionTypeSynced,
		Status:             status,
		ObservedGeneration: rule.GetGeneration(),
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	conds := rule.Status.Conditions
	for i := range conds {
		if conds[i].Type == conditionTypeSynced {
			conds[i] = newCond
			rule.Status.Conditions = conds
			return
		}
	}
	rule.Status.Conditions = append(rule.Status.Conditions, newCond)
}

// finalize handles the cleanup of the alert rule from the observer backend
func (r *Reconciler) finalize(ctx context.Context, alertRule *openchoreov1alpha1.ObservabilityAlertRule) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(alertRule, AlertRuleCleanupFinalizer) {
		// Finalizer not present, nothing to clean up
		return ctrl.Result{}, nil
	}

	logger.Info("Deleting alert rule from observer backend", "name", alertRule.Name, "sourceType", alertRule.Spec.Source.Type)

	// Call DELETE endpoint on observer
	observerURL := getObserverBaseURL()
	url := fmt.Sprintf("%s%s/%s/%s", observerURL, alertingRulePath, alertRule.Spec.Source.Type, alertRule.Name)

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodDelete, url, nil)
	if err != nil {
		logger.Error(err, "failed to create DELETE HTTP request")
		return ctrl.Result{}, err
	}

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		logger.Error(err, "failed to call observer alerting DELETE API")
		return ctrl.Result{}, err
	}
	defer resp.Body.Close()

	// Accept 200, 204, or 404 (already deleted) as success
	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == http.StatusNotFound {
		logger.Info("Alert rule deleted from observer backend", "name", alertRule.Name, "statusCode", resp.StatusCode)
	} else {
		var errBody struct {
			Message string `json:"message,omitempty"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		err = fmt.Errorf("observer alerting DELETE API returned status %d: %s", resp.StatusCode, errBody.Message)
		logger.Error(err, "observer alerting DELETE API call failed")
		return ctrl.Result{}, err
	}

	// Remove finalizer after successful cleanup
	controllerutil.RemoveFinalizer(alertRule, AlertRuleCleanupFinalizer)
	if err := r.Update(ctx, alertRule); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully finalized alert rule", "name", alertRule.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize HTTP client once during setup
	r.httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ObservabilityAlertRule{}).
		Named("observabilityalertrule").
		Complete(r)
}
