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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// defaultObserverInternalBaseURL is the internal observer service URL for v1alpha1 alert CRUD.
	// This service is only reachable within the cluster (not exposed via Gateway).
	defaultObserverInternalBaseURL = "http://observer-internal.openchoreo-observability-plane:8081"
	alertsV1alpha1BasePath         = "/api/v1alpha1/alerts/sources"
	conditionTypeSynced            = "Synced"
	// observerAPITimeout is the default timeout for HTTP calls to the observer internal API.
	observerAPITimeout = 10 * time.Second
	// AlertRuleCleanupFinalizer is used to ensure alert rules are deleted from the backend before the CR is removed
	AlertRuleCleanupFinalizer = "openchoreo.dev/alertrule-cleanup"
)

// alertRuleRequest is the payload sent to the observer v1alpha1 alerting API.
type alertRuleRequest struct {
	Metadata  alertRuleMetadata  `json:"metadata"`
	Source    alertRuleSource    `json:"source"`
	Condition alertRuleCondition `json:"condition"`
}

type alertRuleMetadata struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace,omitempty"`
	ComponentUID   string `json:"componentUid,omitempty"`
	ProjectUID     string `json:"projectUid,omitempty"`
	EnvironmentUID string `json:"environmentUid,omitempty"`
}

type alertRuleSource struct {
	Type   string `json:"type"`
	Query  string `json:"query,omitempty"`
	Metric string `json:"metric,omitempty"`
}

type alertRuleCondition struct {
	Enabled   bool    `json:"enabled"`
	Window    string  `json:"window"`
	Interval  string  `json:"interval"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
}

// alertRuleSyncResponse is the response from the observer v1alpha1 alerting API for write operations.
type alertRuleSyncResponse struct {
	Status        string `json:"status"`
	Action        string `json:"action"`
	RuleLogicalID string `json:"ruleLogicalId"`
	RuleBackendID string `json:"ruleBackendId"`
	LastSyncedAt  string `json:"lastSyncedAt"`
}

// alertRuleGetResponse is the response from the observer v1alpha1 GET endpoint.
type alertRuleGetResponse struct {
	RuleLogicalID string `json:"ruleLogicalId"`
	RuleBackendID string `json:"ruleBackendId"`
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
		return ctrl.Result{Requeue: true}, nil
	}

	// Build the alerting rule request from the CR spec.
	requestPayload, err := buildAlertRuleRequest(alertRule)
	if err != nil {
		logger.Error(err, "failed to build alert rule request")
		return r.updateStatusWithError(ctx, alertRule, err)
	}

	// Use GET-first logic to decide between POST (create) and PUT (update).
	syncResp, err := r.upsertAlertRule(ctx, alertRule, requestPayload)
	if err != nil {
		logger.Error(err, "failed to upsert alert rule via observer internal API")
		return r.updateStatusWithError(ctx, alertRule, err)
	}

	// Update status from sync response.
	// Re-fetch the latest resource version before updating status to avoid
	// "object has been modified" conflict errors caused by concurrent reconciles.
	latest := &openchoreov1alpha1.ObservabilityAlertRule{}
	if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
		logger.Error(err, "failed to re-fetch ObservabilityAlertRule before status update")
		return ctrl.Result{}, err
	}

	now := metav1.NewTime(time.Now())
	latest.Status.ObservedGeneration = alertRule.GetGeneration()
	latest.Status.LastReconcileTime = &now
	latest.Status.BackendMonitorID = syncResp.RuleBackendID

	if syncResp.LastSyncedAt != "" {
		if t, err := time.Parse(time.RFC3339, syncResp.LastSyncedAt); err == nil {
			ts := metav1.NewTime(t)
			latest.Status.LastSyncTime = &ts
		}
	}

	// Phase and conditions.
	if syncResp.Status == "synced" {
		latest.Status.Phase = openchoreov1alpha1.ObservabilityAlertRulePhaseReady
		setStatusCondition(latest, metav1.ConditionTrue, "SyncSucceeded",
			fmt.Sprintf("Alert rule %s %s in backend", syncResp.RuleLogicalID, syncResp.Action))
	} else {
		latest.Status.Phase = openchoreov1alpha1.ObservabilityAlertRulePhaseError
		setStatusCondition(latest, metav1.ConditionFalse, "SyncNotSynced",
			fmt.Sprintf("Alert rule status reported as %q", syncResp.Status))
	}

	if err := r.Status().Update(ctx, latest); err != nil {
		logger.Error(err, "failed to update ObservabilityAlertRule status after sync")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// upsertAlertRule performs a GET-first upsert by rule name:
//   - If GET returns 200, update the existing rule via PUT.
//   - If GET returns 404, create a new rule via POST.
//   - Any other status is treated as an error.
func (r *Reconciler) upsertAlertRule(ctx context.Context, alertRule *openchoreov1alpha1.ObservabilityAlertRule, payload *alertRuleRequest) (*alertRuleSyncResponse, error) {
	baseURL := getObserverInternalBaseURL()
	ruleName := alertRule.Name
	sourceType := string(alertRule.Spec.Source.Type)

	_, statusCode, err := r.getAlertRule(ctx, baseURL, ruleName, sourceType)
	if err != nil {
		return nil, fmt.Errorf("failed to GET alert rule: %w", err)
	}

	switch statusCode {
	case http.StatusOK:
		return r.putAlertRule(ctx, baseURL, ruleName, payload)
	case http.StatusNotFound:
		return r.postAlertRule(ctx, baseURL, payload)
	default:
		return nil, fmt.Errorf("unexpected GET status %d for alert rule %q", statusCode, ruleName)
	}
}

// getAlertRule calls GET /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}
func (r *Reconciler) getAlertRule(ctx context.Context, baseURL, ruleName, sourceType string) (*alertRuleGetResponse, int, error) {
	url := fmt.Sprintf("%s%s/%s/rules/%s", baseURL, alertsV1alpha1BasePath, sourceType, ruleName)
	reqCtx, cancel := context.WithTimeout(ctx, observerAPITimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create GET request: %w", err)
	}

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("GET request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var getResp alertRuleGetResponse
		if err := json.NewDecoder(resp.Body).Decode(&getResp); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("failed to decode GET response: %w", err)
		}
		return &getResp, resp.StatusCode, nil
	}
	return nil, resp.StatusCode, nil
}

// postAlertRule calls POST /api/v1alpha1/alerts/sources/{sourceType}/rules and expects 201.
func (r *Reconciler) postAlertRule(ctx context.Context, baseURL string, payload *alertRuleRequest) (*alertRuleSyncResponse, error) {
	syncResp, statusCode, err := r.postAlertRuleWithStatus(ctx, baseURL, payload)
	if err != nil {
		return nil, err
	}
	if statusCode != http.StatusCreated {
		return nil, fmt.Errorf("observer POST returned unexpected status %d", statusCode)
	}
	return syncResp, nil
}

// postAlertRuleWithStatus calls POST /api/v1alpha1/alerts/sources/{sourceType}/rules and returns the status code alongside the response.
func (r *Reconciler) postAlertRuleWithStatus(ctx context.Context, baseURL string, payload *alertRuleRequest) (*alertRuleSyncResponse, int, error) {
	url := fmt.Sprintf("%s%s/%s/rules", baseURL, alertsV1alpha1BasePath, payload.Source.Type)
	return r.callWriteEndpoint(ctx, http.MethodPost, url, payload)
}

// putAlertRule calls PUT /api/v1alpha1/alerts/sources/{sourceType}/rules/{ruleName}.
func (r *Reconciler) putAlertRule(ctx context.Context, baseURL, ruleName string, payload *alertRuleRequest) (*alertRuleSyncResponse, error) {
	url := fmt.Sprintf("%s%s/%s/rules/%s", baseURL, alertsV1alpha1BasePath, payload.Source.Type, ruleName)
	syncResp, statusCode, err := r.callWriteEndpoint(ctx, http.MethodPut, url, payload)
	if err != nil {
		return nil, err
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("observer PUT returned unexpected status %d", statusCode)
	}
	return syncResp, nil
}

// callWriteEndpoint performs a POST or PUT and returns the parsed response and status code.
func (r *Reconciler) callWriteEndpoint(ctx context.Context, method, url string, payload *alertRuleRequest) (*alertRuleSyncResponse, int, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, observerAPITimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, method, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create %s request: %w", method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("%s request failed: %w", method, err)
	}
	defer resp.Body.Close()

	// On non-2xx (except 409 which the caller may handle), try to surface the error message.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var syncResp alertRuleSyncResponse
		if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("failed to decode response: %w", err)
		}
		return &syncResp, resp.StatusCode, nil
	}

	// Return status code for the caller to inspect (e.g. 409 Conflict).
	var errBody struct {
		Message string `json:"message,omitempty"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&errBody)

	if resp.StatusCode == http.StatusConflict {
		return nil, resp.StatusCode, nil
	}

	return nil, resp.StatusCode, fmt.Errorf("observer API %s %s returned status %d: %s", method, url, resp.StatusCode, errBody.Message)
}

// buildAlertRuleRequest converts the ObservabilityAlertRule spec into the new v1alpha1 API request payload.
func buildAlertRuleRequest(rule *openchoreov1alpha1.ObservabilityAlertRule) (*alertRuleRequest, error) {
	spec := rule.Spec

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

	req := &alertRuleRequest{
		Metadata: alertRuleMetadata{
			Name:           rule.Name,
			Namespace:      rule.Namespace,
			ComponentUID:   componentUID,
			ProjectUID:     projectUID,
			EnvironmentUID: environmentUID,
		},
		Source: alertRuleSource{
			Type:   string(spec.Source.Type),
			Query:  spec.Source.Query,
			Metric: spec.Source.Metric,
		},
		Condition: alertRuleCondition{
			Enabled:   enabled,
			Window:    formatMinutesHours(spec.Condition.Window.Duration),
			Interval:  formatMinutesHours(spec.Condition.Interval.Duration),
			Operator:  string(spec.Condition.Operator),
			Threshold: float64(spec.Condition.Threshold),
		},
	}

	return req, nil
}

// getObserverInternalBaseURL returns the observer internal base URL, allowing override via environment variable.
func getObserverInternalBaseURL() string {
	if v := os.Getenv("OBSERVER_INTERNAL_ENDPOINT"); v != "" {
		return v
	}
	// Fall back to legacy OBSERVER_ENDPOINT for backwards compatibility in tests.
	if v := os.Getenv("OBSERVER_ENDPOINT"); v != "" {
		return v
	}
	return defaultObserverInternalBaseURL
}

// formatMinutesHours converts a duration to a minutes/hours-only string (e.g. "5m", "1h").
func formatMinutesHours(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	return fmt.Sprintf("%dm", d/time.Minute)
}

// updateStatusWithError sets the rule status to Error and records a failing condition.
// nolint:unparam // Result is always empty but required by controller-runtime interface
func (r *Reconciler) updateStatusWithError(ctx context.Context, rule *openchoreov1alpha1.ObservabilityAlertRule, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Re-fetch the latest resource version to avoid conflict errors on status update.
	latest := &openchoreov1alpha1.ObservabilityAlertRule{}
	if fetchErr := r.Get(ctx, client.ObjectKeyFromObject(rule), latest); fetchErr != nil {
		logger.Info("Failed to re-fetch ObservabilityAlertRule for status update; falling back to stale object",
			"name", rule.Name, "error", fetchErr)
		// Best-effort: if we can't re-fetch, fall back to the original object.
		latest = rule
	}

	now := metav1.NewTime(time.Now())
	latest.Status.ObservedGeneration = rule.GetGeneration()
	latest.Status.LastReconcileTime = &now
	latest.Status.Phase = openchoreov1alpha1.ObservabilityAlertRulePhaseError
	setStatusCondition(latest, metav1.ConditionFalse, "SyncFailed", err.Error())

	if updateErr := r.Status().Update(ctx, latest); updateErr != nil {
		logger.Info("Failed to update status after error", "updateError", updateErr)
	}

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

// finalize handles the cleanup of the alert rule from the observer backend.
func (r *Reconciler) finalize(ctx context.Context, alertRule *openchoreov1alpha1.ObservabilityAlertRule) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(alertRule, AlertRuleCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	logger.Info("Deleting alert rule from observer backend", "name", alertRule.Name, "sourceType", alertRule.Spec.Source.Type)

	baseURL := getObserverInternalBaseURL()
	url := fmt.Sprintf("%s%s/%s/rules/%s", baseURL, alertsV1alpha1BasePath, alertRule.Spec.Source.Type, alertRule.Name)

	reqCtx, cancel := context.WithTimeout(ctx, observerAPITimeout)
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

	// Accept 200, 204, or 404 (already deleted) as success.
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

	// Remove finalizer after successful cleanup.
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
	r.httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ObservabilityAlertRule{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("observabilityalertrule").
		Complete(r)
}
