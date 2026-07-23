// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreoapis "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
	legacytypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	alertActionCreated   = "created"
	alertActionUpdated   = "updated"
	alertActionUnchanged = "unchanged"
	alertActionDeleted   = "deleted"
	alertStatusSynced    = "synced"

	sourceTypeLog    = "log"
	sourceTypeMetric = "metric"
	sourceTypeBudget = "budget"
)

// AlertService provides CRUD operations for alert rules, backing the v1alpha1 API.
type AlertService struct {
	alertEntryStore    alertentry.AlertEntryStore
	incidentEntryStore incidententry.IncidentEntryStore
	k8sClient          client.Client
	config             *config.Config
	logger             *slog.Logger
	rcaServiceURL      string
	aiRCAEnabled       bool
	resolver           *ResourceUIDResolver
	logsAdapter        *LogsAdapter

	metricsAdapterURL    string
	metricsAdapterClient *http.Client

	finOpsAgentURL     string
	finOpsAgentEnabled bool
}

// NewAlertService creates a new AlertService.
func NewAlertService(
	alertEntryStore alertentry.AlertEntryStore,
	incidentEntryStore incidententry.IncidentEntryStore,
	k8sClient client.Client,
	cfg *config.Config,
	logger *slog.Logger,
	rcaServiceURL string,
	aiRCAEnabled bool,
	resolver *ResourceUIDResolver,
	logsAdapter *LogsAdapter,
	metricsAdapterURL string,
	metricsAdapterClient *http.Client,
	finOpsAgentURL string,
	finOpsAgentEnabled bool,
) *AlertService {
	return &AlertService{
		alertEntryStore:      alertEntryStore,
		incidentEntryStore:   incidentEntryStore,
		k8sClient:            k8sClient,
		config:               cfg,
		logger:               logger,
		rcaServiceURL:        rcaServiceURL,
		aiRCAEnabled:         aiRCAEnabled,
		resolver:             resolver,
		logsAdapter:          logsAdapter,
		metricsAdapterURL:    metricsAdapterURL,
		metricsAdapterClient: metricsAdapterClient,
		finOpsAgentURL:       finOpsAgentURL,
		finOpsAgentEnabled:   finOpsAgentEnabled,
	}
}

// CreateAlertRule creates a new alert rule via the configured adapter.
// Returns an error wrapping ErrAlertRuleAlreadyExists if the rule already exists.
func (s *AlertService) CreateAlertRule(ctx context.Context, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	if err := validateAlertDurations(req.Condition.Interval, req.Condition.Window); err != nil {
		return nil, err
	}

	sourceType, err := sourceTypeFromRequest(req)
	if err != nil {
		return nil, err
	}

	switch sourceType {
	case sourceTypeLog:
		if s.logsAdapter == nil {
			return nil, fmt.Errorf("logs adapter is required for log alert rules")
		}
		return s.createLogAlertRuleViaAdapter(ctx, req)
	case sourceTypeMetric:
		if s.metricsAdapterClient == nil {
			return nil, fmt.Errorf("metrics adapter is required for metric alert rules")
		}
		return s.createMetricAlertRuleViaAdapter(ctx, req)
	case sourceTypeBudget:
		if s.metricsAdapterClient == nil {
			return nil, fmt.Errorf("metrics adapter is required for budget alert rules")
		}
		budgetMetric := gen.AlertRuleRequestSourceMetricBudget
		req.Source.Metric = &budgetMetric
		return s.createMetricAlertRuleViaAdapter(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// GetAlertRule fetches an alert rule via the configured adapter.
// sourceType must be "log", "metric", or "budget".
// Returns an error wrapping ErrAlertRuleNotFound if the rule does not exist.
func (s *AlertService) GetAlertRule(ctx context.Context, ruleName, sourceType string) (*gen.AlertRuleResponse, error) {
	switch sourceType {
	case sourceTypeLog:
		if s.logsAdapter == nil {
			return nil, fmt.Errorf("logs adapter is required for log alert rules")
		}
		return s.getLogAlertRuleViaAdapter(ctx, ruleName)
	case sourceTypeMetric:
		if s.metricsAdapterClient == nil {
			return nil, fmt.Errorf("metrics adapter is required for metric alert rules")
		}
		return s.getMetricAlertRuleViaAdapter(ctx, ruleName)
	case sourceTypeBudget:
		if s.metricsAdapterClient == nil {
			return nil, fmt.Errorf("metrics adapter is required for budget alert rules")
		}
		return s.getMetricAlertRuleViaAdapter(ctx, ruleName)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// UpdateAlertRule updates an existing alert rule via the configured adapter.
// Returns an error wrapping ErrAlertRuleNotFound if the rule does not exist.
func (s *AlertService) UpdateAlertRule(ctx context.Context, ruleName string, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	if err := validateAlertDurations(req.Condition.Interval, req.Condition.Window); err != nil {
		return nil, err
	}

	sourceType, err := sourceTypeFromRequest(req)
	if err != nil {
		return nil, err
	}

	switch sourceType {
	case sourceTypeLog:
		if s.logsAdapter == nil {
			return nil, fmt.Errorf("logs adapter is required for log alert rules")
		}
		return s.updateLogAlertRuleViaAdapter(ctx, ruleName, req)
	case sourceTypeMetric:
		if s.metricsAdapterClient == nil {
			return nil, fmt.Errorf("metrics adapter is required for metric alert rules")
		}
		return s.updateMetricAlertRuleViaAdapter(ctx, ruleName, req)
	case sourceTypeBudget:
		if s.metricsAdapterClient == nil {
			return nil, fmt.Errorf("metrics adapter is required for budget alert rules")
		}
		budgetMetric := gen.AlertRuleRequestSourceMetricBudget
		req.Source.Metric = &budgetMetric
		return s.updateMetricAlertRuleViaAdapter(ctx, ruleName, req)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// DeleteAlertRule deletes an alert rule via the configured adapter.
// sourceType must be "log", "metric", or "budget".
// Returns ErrAlertRuleNotFound if the rule does not exist.
func (s *AlertService) DeleteAlertRule(ctx context.Context, ruleName, sourceType string) (*gen.AlertingRuleSyncResponse, error) {
	switch sourceType {
	case sourceTypeLog:
		if s.logsAdapter == nil {
			return nil, fmt.Errorf("logs adapter is required for log alert rules")
		}
		return s.deleteLogAlertRuleViaAdapter(ctx, ruleName)
	case sourceTypeMetric:
		if s.metricsAdapterClient == nil {
			return nil, fmt.Errorf("metrics adapter is required for metric alert rules")
		}
		return s.deleteMetricAlertRuleViaAdapter(ctx, ruleName)
	case sourceTypeBudget:
		if s.metricsAdapterClient == nil {
			return nil, fmt.Errorf("metrics adapter is required for budget alert rules")
		}
		return s.deleteMetricAlertRuleViaAdapter(ctx, ruleName)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// HandleAlertWebhook processes an incoming alert webhook in the normalized v1alpha1 format.
// It fetches the ObservabilityAlertRule CR, enriches alert details, stores the alert entry,
// sends a notification, and optionally triggers AI RCA analysis.
func (s *AlertService) HandleAlertWebhook(ctx context.Context, req gen.AlertWebhookRequest) (*gen.AlertWebhookResponse, error) {
	ruleName, ruleNamespace, err := s.validateWebhookRequest(req)
	if err != nil {
		return nil, err
	}

	alertRule, err := s.fetchAlertRule(ctx, ruleName, ruleNamespace)
	if err != nil {
		return nil, err
	}

	// Check for duplicate alert within the suppression window
	if suppressed, resp := s.checkAlertSuppression(ctx, ruleName, ruleNamespace, alertRule); suppressed {
		return resp, nil
	}

	alertDetails := s.buildAlertDetails(req, alertRule)
	alertEntry := s.buildAlertEntry(alertDetails, ruleName, ruleNamespace, alertRule)

	alertID, err := s.alertEntryStore.WriteAlertEntry(ctx, alertEntry)
	if err != nil {
		return nil, fmt.Errorf("failed to store alert entry: %w", err)
	}

	s.logger.Debug("Alert entry stored", "alertID", alertID, "ruleName", ruleName)

	s.triggerBackgroundTasks(alertID, alertDetails, alertRule)

	successStatus := gen.AlertWebhookResponseStatusSuccess
	msg := fmt.Sprintf("alert acknowledged, alertID: %s", alertID)
	return &gen.AlertWebhookResponse{
		Status:  &successStatus,
		Message: &msg,
	}, nil
}

// validateWebhookRequest validates the required fields in the webhook request.
func (s *AlertService) validateWebhookRequest(req gen.AlertWebhookRequest) (string, string, error) {
	ruleName := stringPtrVal(req.RuleName)
	ruleNamespace := stringPtrVal(req.RuleNamespace)
	if ruleName == "" {
		return "", "", fmt.Errorf("ruleName is required")
	}
	if ruleNamespace == "" {
		return "", "", fmt.Errorf("ruleNamespace is required")
	}
	if s.alertEntryStore == nil {
		return "", "", fmt.Errorf("alert entry store is not initialized")
	}
	if s.k8sClient == nil {
		return "", "", fmt.Errorf("kubernetes client not configured")
	}
	return ruleName, ruleNamespace, nil
}

// fetchAlertRule fetches the ObservabilityAlertRule CR from Kubernetes.
func (s *AlertService) fetchAlertRule(ctx context.Context, ruleName, ruleNamespace string) (*choreoapis.ObservabilityAlertRule, error) {
	alertRule := &choreoapis.ObservabilityAlertRule{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      ruleName,
		Namespace: ruleNamespace,
	}, alertRule); err != nil {
		return nil, fmt.Errorf("failed to get ObservabilityAlertRule %s/%s: %w", ruleNamespace, ruleName, err)
	}
	return alertRule, nil
}

// checkAlertSuppression checks if the alert should be suppressed based on the suppression window.
func (s *AlertService) checkAlertSuppression(ctx context.Context, ruleName, ruleNamespace string, alertRule *choreoapis.ObservabilityAlertRule) (bool, *gen.AlertWebhookResponse) {
	if s.config.Alerting.AlertSuppressionWindow <= 0 {
		return false, nil
	}

	since := time.Now().UTC().Add(-s.config.Alerting.AlertSuppressionWindow)
	componentUID := alertRule.Labels[labels.LabelKeyComponentUID]
	if componentUID == "" {
		s.logger.Warn("Skipping suppression check: component UID label is missing",
			"ruleName", ruleName, "ruleNamespace", ruleNamespace)
		return false, nil
	}

	recentAlert, err := s.alertEntryStore.GetRecentAlert(ctx, ruleName, ruleNamespace, componentUID, since)
	if err != nil {
		s.logger.Warn("Failed to check alert suppression", "error", err, "ruleName", ruleName)
		return false, nil
	}

	if recentAlert == nil {
		return false, nil
	}

	// An acknowledged or resolved incident represents a completed alert occurrence.
	// A later webhook must create a new occurrence even if it arrives within the
	// suppression window. If the incident has not been persisted yet, keep
	// suppressing to avoid a race creating duplicate incidents.
	if recentAlert.IncidentEnabled && s.incidentEntryStore != nil {
		status, found, statusErr := s.incidentEntryStore.GetIncidentStatusByAlertID(ctx, recentAlert.ID)
		if statusErr != nil {
			s.logger.Warn("Failed to check incident status for alert suppression",
				"error", statusErr, "alertID", recentAlert.ID, "ruleName", ruleName)
		} else if found && status != incidententry.StatusActive {
			return false, nil
		}
	}

	s.logger.Info("Alert suppressed (duplicate within suppression window)",
		"ruleName", ruleName, "ruleNamespace", ruleNamespace,
		"suppressionWindow", s.config.Alerting.AlertSuppressionWindow)
	suppressedStatus := gen.AlertWebhookResponseStatusSuccess
	msg := "alert suppressed: duplicate within suppression window"
	return true, &gen.AlertWebhookResponse{
		Status:  &suppressedStatus,
		Message: &msg,
	}
}

// buildAlertDetails enriches alert details from the webhook request and alert rule CR.
func (s *AlertService) buildAlertDetails(req gen.AlertWebhookRequest, alertRule *choreoapis.ObservabilityAlertRule) *legacytypes.AlertDetails {
	var alertValue string
	if req.AlertValue != nil {
		alertValue = strconv.FormatFloat(float64(*req.AlertValue), 'f', -1, 64)
	}

	var alertTimestamp string
	if req.AlertTimestamp != nil {
		alertTimestamp = req.AlertTimestamp.Format(time.RFC3339)
	}
	if alertTimestamp == "" {
		alertTimestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	alertDetails := &legacytypes.AlertDetails{
		AlertName:        alertRule.Spec.Name,
		AlertTimestamp:   alertTimestamp,
		AlertSeverity:    string(alertRule.Spec.Severity),
		AlertDescription: alertRule.Spec.Description,
		AlertThreshold:   strconv.FormatInt(alertRule.Spec.Condition.Threshold, 10),
		AlertValue:       alertValue,
		AlertType:        string(alertRule.Spec.Source.Type),
		Namespace:        alertRule.Labels[labels.LabelKeyNamespaceName],
		ComponentID:      alertRule.Labels[labels.LabelKeyComponentUID],
		EnvironmentID:    alertRule.Labels[labels.LabelKeyEnvironmentUID],
		ProjectID:        alertRule.Labels[labels.LabelKeyProjectUID],
		Component:        alertRule.Labels[labels.LabelKeyComponentName],
		Project:          alertRule.Labels[labels.LabelKeyProjectName],
		Environment:      alertRule.Labels[labels.LabelKeyEnvironmentName],
	}

	alertDetails.NotificationChannels = make([]string, 0, len(alertRule.Spec.Actions.Notifications.Channels))
	for _, ch := range alertRule.Spec.Actions.Notifications.Channels {
		alertDetails.NotificationChannels = append(alertDetails.NotificationChannels, string(ch))
	}

	if alertRule.Spec.Actions.Incident != nil {
		if alertRule.Spec.Actions.Incident.Enabled != nil {
			alertDetails.IncidentEnabled = *alertRule.Spec.Actions.Incident.Enabled
		}
		if alertRule.Spec.Actions.Incident.TriggerAiRca != nil {
			alertDetails.TriggerAiRca = *alertRule.Spec.Actions.Incident.TriggerAiRca
		}
		if alertRule.Spec.Actions.Incident.TriggerAiCostAnalysis != nil {
			alertDetails.TriggerAiCostAnalysis = *alertRule.Spec.Actions.Incident.TriggerAiCostAnalysis
		}
	}

	return alertDetails
}

// buildAlertEntry constructs an AlertEntry for storage.
func (s *AlertService) buildAlertEntry(alertDetails *legacytypes.AlertDetails, ruleName, ruleNamespace string, alertRule *choreoapis.ObservabilityAlertRule) *alertentry.AlertEntry {
	var notificationChannelsJSON string
	if len(alertDetails.NotificationChannels) > 0 {
		if b, err := json.Marshal(alertDetails.NotificationChannels); err == nil {
			notificationChannelsJSON = string(b)
		}
	}

	conditionOperator, conditionThreshold, conditionWindow, conditionInterval := "", 0.0, "", ""
	if alertRule.Spec.Condition.Operator != "" {
		conditionOperator = string(alertRule.Spec.Condition.Operator)
	}
	if alertRule.Spec.Condition.Threshold != 0 {
		conditionThreshold = float64(alertRule.Spec.Condition.Threshold)
	}
	if alertRule.Spec.Condition.Window.Duration.String() != "" {
		conditionWindow = alertRule.Spec.Condition.Window.Duration.String()
	}
	if alertRule.Spec.Condition.Interval.Duration.String() != "" {
		conditionInterval = alertRule.Spec.Condition.Interval.Duration.String()
	}

	return &alertentry.AlertEntry{
		Timestamp:            alertDetails.AlertTimestamp,
		AlertRuleName:        alertDetails.AlertName,
		AlertRuleCRName:      ruleName,
		AlertRuleCRNamespace: ruleNamespace,
		AlertValue:           alertDetails.AlertValue,
		NamespaceName:        alertDetails.Namespace,
		ComponentName:        alertDetails.Component,
		EnvironmentName:      alertDetails.Environment,
		ProjectName:          alertDetails.Project,
		ComponentID:          alertDetails.ComponentID,
		EnvironmentID:        alertDetails.EnvironmentID,
		ProjectID:            alertDetails.ProjectID,
		IncidentEnabled:      alertDetails.IncidentEnabled,
		Severity:             string(alertRule.Spec.Severity),
		Description:          alertRule.Spec.Description,
		NotificationChannels: notificationChannelsJSON,
		SourceType:           string(alertRule.Spec.Source.Type),
		SourceQuery:          alertRule.Spec.Source.Query,
		SourceMetric:         alertRule.Spec.Source.Metric,
		ConditionOperator:    conditionOperator,
		ConditionThreshold:   conditionThreshold,
		ConditionWindow:      conditionWindow,
		ConditionInterval:    conditionInterval,
	}
}

// triggerBackgroundTasks spawns background goroutines for incident storage, notifications, and analysis.
func (s *AlertService) triggerBackgroundTasks(alertID string, alertDetails *legacytypes.AlertDetails, alertRule *choreoapis.ObservabilityAlertRule) {
	if alertDetails.IncidentEnabled {
		go s.storeIncidentEntry(alertID, alertDetails)
	}

	go s.sendNotificationAsync(alertID, alertDetails)

	if alertDetails.TriggerAiRca && s.aiRCAEnabled {
		go s.triggerRCAAnalysis(alertID, alertDetails, alertRule)
	}

	if alertDetails.TriggerAiCostAnalysis && s.finOpsAgentEnabled {
		go s.triggerFinOpsAnalysis(alertID, alertDetails, alertRule)
	}
}

// storeIncidentEntry stores an incident entry in the background.
func (s *AlertService) storeIncidentEntry(alertID string, alertDetails *legacytypes.AlertDetails) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if s.incidentEntryStore == nil {
		s.logger.Warn("Incident entry store is not initialized", "alertID", alertID)
		return
	}

	if _, err := s.incidentEntryStore.WriteIncidentEntry(bgCtx, &incidententry.IncidentEntry{
		AlertID:               alertID,
		Timestamp:             alertDetails.AlertTimestamp,
		Status:                incidententry.StatusActive,
		TriggerAiRca:          alertDetails.TriggerAiRca,
		TriggerAiCostAnalysis: alertDetails.TriggerAiCostAnalysis,
		TriggeredAt:           alertDetails.AlertTimestamp,
		Description:           alertDetails.AlertDescription,
		NamespaceName:         alertDetails.Namespace,
		ComponentName:         alertDetails.Component,
		EnvironmentName:       alertDetails.Environment,
		ProjectName:           alertDetails.Project,
		ComponentID:           alertDetails.ComponentID,
		EnvironmentID:         alertDetails.EnvironmentID,
		ProjectID:             alertDetails.ProjectID,
	}); err != nil {
		s.logger.Warn("Failed to store incident entry", "error", err, "alertID", alertID)
	}
}

// sendNotificationAsync sends an alert notification in the background.
func (s *AlertService) sendNotificationAsync(alertID string, alertDetails *legacytypes.AlertDetails) {
	notifCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.sendAlertNotification(notifCtx, alertDetails); err != nil {
		s.logger.Warn("Failed to send alert notification", "error", err, "alertID", alertID)
	}
}

// sendAlertNotification fetches the notification channel config from K8s and dispatches the notification.
func (s *AlertService) sendAlertNotification(ctx context.Context, alertDetails *legacytypes.AlertDetails) error {
	if len(alertDetails.NotificationChannels) == 0 {
		s.logger.Warn("No notification channels configured in alert details; this rule is invalid, skipping notification",
			"ruleName", alertDetails.AlertName)
		return nil
	}

	return DispatchAlertNotifications(ctx, alertDetails, alertDetails.NotificationChannels, s.getNotificationChannelConfig, s.logger)
}

// getNotificationChannelConfig reads the K8s ConfigMap/Secret for the notification channel.
func (s *AlertService) getNotificationChannelConfig(ctx context.Context, channelName string) (*notifications.NotificationChannelConfig, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	labelSelector := client.MatchingLabels{
		labels.LabelKeyNotificationChannelName: channelName,
	}

	configMapList := &corev1.ConfigMapList{}
	if err := s.k8sClient.List(ctx, configMapList, labelSelector); err != nil {
		return nil, fmt.Errorf("failed to list ConfigMaps: %w", err)
	}
	if len(configMapList.Items) == 0 {
		return nil, fmt.Errorf("failed to find notification channel ConfigMap with label %s=%s",
			labels.LabelKeyNotificationChannelName, channelName)
	}
	configMap := configMapList.Items[0].DeepCopy()

	secretList := &corev1.SecretList{}
	if err := s.k8sClient.List(ctx, secretList, labelSelector); err != nil {
		return nil, fmt.Errorf("failed to list Secrets: %w", err)
	}
	if len(secretList.Items) == 0 {
		return nil, fmt.Errorf("failed to find notification channel Secret with label %s=%s",
			labels.LabelKeyNotificationChannelName, channelName)
	}
	secret := secretList.Items[0].DeepCopy()

	channelType := configMap.Data["type"]
	if channelType == "" {
		return nil, fmt.Errorf("notification channel type not found in ConfigMap")
	}

	cfg := &notifications.NotificationChannelConfig{Type: channelType}
	switch channelType {
	case "email":
		emailConfig, err := notifications.PrepareEmailNotificationConfig(configMap, secret, s.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare email notification config: %w", err)
		}
		cfg.Email = emailConfig
	case "webhook":
		webhookConfig, err := notifications.PrepareWebhookNotificationConfig(configMap, secret, s.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare webhook notification config: %w", err)
		}
		cfg.Webhook = webhookConfig
	default:
		return nil, fmt.Errorf("unsupported notification channel type: %s", channelType)
	}

	return cfg, nil
}

// triggerRCAAnalysis sends an AI RCA analysis request to the configured RCA service.
func (s *AlertService) triggerRCAAnalysis(alertID string, alertDetails *legacytypes.AlertDetails, alertRule *choreoapis.ObservabilityAlertRule) {
	ruleInfo := map[string]interface{}{
		"name": alertDetails.AlertName,
	}
	if alertRule != nil {
		if alertRule.Spec.Description != "" {
			ruleInfo["description"] = alertRule.Spec.Description
		}
		if alertRule.Spec.Severity != "" {
			ruleInfo["severity"] = string(alertRule.Spec.Severity)
		}
		ruleInfo["source"] = map[string]interface{}{
			"type":   string(alertRule.Spec.Source.Type),
			"query":  alertRule.Spec.Source.Query,
			"metric": alertRule.Spec.Source.Metric,
		}
		ruleInfo["condition"] = map[string]interface{}{
			"window":    alertRule.Spec.Condition.Window.Duration.String(),
			"interval":  alertRule.Spec.Condition.Interval.Duration.String(),
			"operator":  alertRule.Spec.Condition.Operator,
			"threshold": alertRule.Spec.Condition.Threshold,
		}
	}

	rcaPayload := map[string]interface{}{
		"namespace":   alertDetails.Namespace,
		"project":     alertDetails.Project,
		"component":   alertDetails.Component,
		"environment": alertDetails.Environment,
		"alert": map[string]interface{}{
			"id":        alertID,
			"value":     alertDetails.AlertValue,
			"timestamp": alertDetails.AlertTimestamp,
			"rule":      ruleInfo,
		},
	}

	payloadBytes, err := json.Marshal(rcaPayload)
	if err != nil {
		s.logger.Error("Failed to marshal RCA request payload", "error", err)
		return
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Post(s.rcaServiceURL+"/api/v1alpha1/rca-agent/analyze", "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		s.logger.Error("Failed to send RCA analysis request", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logger.Error("RCA analysis request returned non-success status", "statusCode", resp.StatusCode, "alertID", alertID)
	} else {
		s.logger.Debug("AI RCA analysis triggered", "alertID", alertID)
	}
}

// triggerFinOpsAnalysis sends an AI cost analysis request to the configured FinOps agent.
func (s *AlertService) triggerFinOpsAnalysis(alertID string, alertDetails *legacytypes.AlertDetails, alertRule *choreoapis.ObservabilityAlertRule) {
	// Parse alertValue to float
	alertValueFloat := 0.0
	if alertDetails.AlertValue != "" {
		if val, err := strconv.ParseFloat(alertDetails.AlertValue, 64); err == nil {
			alertValueFloat = val
		}
	}

	// Build the FinOps analysis payload
	finOpsPayload := map[string]interface{}{
		"searchScope": map[string]interface{}{
			"component":   alertDetails.Component,
			"namespace":   alertDetails.Namespace,
			"project":     alertDetails.Project,
			"environment": alertDetails.Environment,
		},
		"budgetedCost": map[string]interface{}{
			"amount":   float64(alertRule.Spec.Condition.Threshold),
			"period":   alertRule.Spec.Condition.Window.Duration.String(),
			"currency": "USD",
		},
		"actualCost": map[string]interface{}{
			"amount":   alertValueFloat,
			"currency": "USD",
		},
		"budgetAlertTriggeredAt": alertDetails.AlertTimestamp,
	}

	payloadBytes, err := json.Marshal(finOpsPayload)
	if err != nil {
		s.logger.Error("Failed to marshal FinOps request payload", "error", err)
		return
	}

	if s.logger.Enabled(context.Background(), slog.LevelDebug) {
		fmt.Println(string(payloadBytes))
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Post(s.finOpsAgentURL+"/api/v1alpha1/analyses", "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		s.logger.Error("Failed to send FinOps analysis request", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logger.Error("FinOps analysis request returned non-success status", "statusCode", resp.StatusCode, "alertID", alertID)
	} else {
		// Decode response to get reportId
		var finOpsResponse struct {
			ReportID string `json:"reportId"`
			Status   string `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&finOpsResponse); err != nil {
			s.logger.Warn("Failed to decode FinOps response", "error", err, "alertID", alertID)
		} else {
			s.logger.Debug("FinOps analysis triggered", "alertID", alertID, "reportID", finOpsResponse.ReportID)
		}
	}
}

// ---- Sentinel errors ----

// ErrAlertRuleNotFound is returned when the requested alert rule does not exist in the backend.
var ErrAlertRuleNotFound = errors.New("alert rule not found")

// ErrAlertRuleAlreadyExists is returned when trying to create a rule that already exists.
var ErrAlertRuleAlreadyExists = errors.New("alert rule already exists")

// genRequestToLegacyRequest converts the generated API type to the legacy type used by the adapter clients.
func genRequestToLegacyRequest(req gen.AlertRuleRequest) legacytypes.AlertingRuleRequest {
	meta := legacytypes.AlertingRuleMetadata{
		Name:           req.Metadata.Name,
		Namespace:      req.Metadata.Namespace,
		ComponentUID:   req.Metadata.ComponentUid.String(),
		ProjectUID:     req.Metadata.ProjectUid.String(),
		EnvironmentUID: req.Metadata.EnvironmentUid.String(),
	}

	src := legacytypes.AlertingRuleSource{
		Type: string(req.Source.Type),
	}
	if req.Source.Query != nil {
		src.Query = *req.Source.Query
	}
	if req.Source.Metric != nil {
		src.Metric = string(*req.Source.Metric)
	}

	cond := legacytypes.AlertingRuleCondition{
		Enabled:   req.Condition.Enabled,
		Window:    req.Condition.Window,
		Interval:  req.Condition.Interval,
		Operator:  string(req.Condition.Operator),
		Threshold: float64(req.Condition.Threshold),
	}

	return legacytypes.AlertingRuleRequest{
		Metadata:  meta,
		Source:    src,
		Condition: cond,
	}
}

// validateAlertDurations enforces that interval and window use
// only minutes/hours and never seconds for alert rules.
func validateAlertDurations(interval, window string) error {
	if err := validateMinutesHoursDuration(window, "condition.window"); err != nil {
		return err
	}
	if err := validateMinutesHoursDuration(interval, "condition.interval"); err != nil {
		return err
	}
	return nil
}

// validateMinutesHoursDuration checks that the provided duration string:
// - parses as a Go time.Duration
// - is a whole number of minutes or hours
// - does not include any seconds component
func validateMinutesHoursDuration(value, fieldName string) error {
	if strings.Contains(value, "s") {
		return fmt.Errorf("%s must be in whole minutes or hours (e.g. 5m, 1h); seconds are not supported", fieldName)
	}

	d, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid duration (e.g. 5m, 1h): %w", fieldName, err)
	}
	if d <= 0 {
		return fmt.Errorf("%s must be greater than zero", fieldName)
	}

	// Reject anything that has a seconds component.
	if d%time.Minute != 0 {
		return fmt.Errorf("%s must be in whole minutes or hours (e.g. 5m, 1h); seconds are not supported", fieldName)
	}

	return nil
}

// ---- Utility helpers ----

func sourceTypeFromRequest(req gen.AlertRuleRequest) (string, error) {
	sourceType := string(req.Source.Type)
	if sourceType == "" {
		return "", fmt.Errorf("source.type is required")
	}
	return sourceType, nil
}

func stringPtrVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func buildSyncResponse(action, logicalID, backendID string, lastSynced time.Time) *gen.AlertingRuleSyncResponse {
	lastSyncedStr := lastSynced.Format(time.RFC3339)
	status := gen.AlertingRuleSyncResponseStatus(alertStatusSynced)
	act := gen.AlertingRuleSyncResponseAction(action)
	return &gen.AlertingRuleSyncResponse{
		Status:        &status,
		Action:        &act,
		RuleLogicalId: &logicalID,
		RuleBackendId: &backendID,
		LastSyncedAt:  &lastSyncedStr,
	}
}
