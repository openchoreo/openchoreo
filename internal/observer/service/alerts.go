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
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreoapis "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/prometheus"
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
)

// AlertOpenSearchClient defines the OpenSearch operations used by AlertService.
type AlertOpenSearchClient interface {
	SearchMonitorByName(ctx context.Context, name string) (id string, exists bool, err error)
	GetMonitorByID(ctx context.Context, monitorID string) (monitor map[string]interface{}, err error)
	CreateMonitor(ctx context.Context, monitor map[string]interface{}) (id string, lastUpdateTime int64, err error)
	UpdateMonitor(ctx context.Context, monitorID string, monitor map[string]interface{}) (lastUpdateTime int64, err error)
	DeleteMonitor(ctx context.Context, monitorID string) error
}

// AlertService provides CRUD operations for alert rules, backing the v1alpha1 API.
type AlertService struct {
	osClient           AlertOpenSearchClient
	queryBuilder       *opensearch.QueryBuilder
	alertEntryStore    alertentry.AlertEntryStore
	incidentEntryStore incidententry.IncidentEntryStore
	k8sClient          client.Client
	config             *config.Config
	logger             *slog.Logger
	rcaServiceURL      string
	aiRCAEnabled       bool
	resolver           *ResourceUIDResolver
}

// NewAlertService creates a new AlertService.
func NewAlertService(
	osClient AlertOpenSearchClient,
	queryBuilder *opensearch.QueryBuilder,
	alertEntryStore alertentry.AlertEntryStore,
	incidentEntryStore incidententry.IncidentEntryStore,
	k8sClient client.Client,
	cfg *config.Config,
	logger *slog.Logger,
	rcaServiceURL string,
	aiRCAEnabled bool,
	resolver *ResourceUIDResolver,
) *AlertService {
	return &AlertService{
		osClient:           osClient,
		queryBuilder:       queryBuilder,
		alertEntryStore:    alertEntryStore,
		incidentEntryStore: incidentEntryStore,
		k8sClient:          k8sClient,
		config:             cfg,
		logger:             logger,
		rcaServiceURL:      rcaServiceURL,
		aiRCAEnabled:       aiRCAEnabled,
		resolver:           resolver,
	}
}

// CreateAlertRule creates a new alert rule in the observability backend.
// Returns an error wrapping ErrAlertRuleAlreadyExists if the rule already exists.
func (s *AlertService) CreateAlertRule(ctx context.Context, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	sourceType, err := sourceTypeFromRequest(req)
	if err != nil {
		return nil, err
	}

	switch sourceType {
	case sourceTypeLog:
		return s.createOpenSearchAlertRule(ctx, req)
	case sourceTypeMetric:
		return s.createPrometheusAlertRule(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// GetAlertRule fetches an alert rule from the observability backend.
// sourceType must be "log" or "metric".
// Returns an error wrapping ErrAlertRuleNotFound if the rule does not exist.
func (s *AlertService) GetAlertRule(ctx context.Context, ruleName, sourceType string) (*gen.AlertRuleResponse, error) {
	switch sourceType {
	case sourceTypeLog:
		return s.getOpenSearchAlertRule(ctx, ruleName)
	case sourceTypeMetric:
		return s.getPrometheusAlertRule(ctx, ruleName)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// UpdateAlertRule updates an existing alert rule in the observability backend.
// Returns an error wrapping ErrAlertRuleNotFound if the rule does not exist.
func (s *AlertService) UpdateAlertRule(ctx context.Context, ruleName string, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	sourceType, err := sourceTypeFromRequest(req)
	if err != nil {
		return nil, err
	}

	switch sourceType {
	case sourceTypeLog:
		return s.updateOpenSearchAlertRule(ctx, ruleName, req)
	case sourceTypeMetric:
		return s.updatePrometheusAlertRule(ctx, ruleName, req)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// DeleteAlertRule deletes an alert rule from the observability backend.
// sourceType must be "log" or "metric".
// Returns ErrAlertRuleNotFound if the rule does not exist.
func (s *AlertService) DeleteAlertRule(ctx context.Context, ruleName, sourceType string) (*gen.AlertingRuleSyncResponse, error) {
	switch sourceType {
	case sourceTypeLog:
		return s.deleteOpenSearchAlertRule(ctx, ruleName)
	case sourceTypeMetric:
		return s.deletePrometheusAlertRule(ctx, ruleName)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// HandleAlertWebhook processes an incoming alert webhook in the normalized v1alpha1 format.
// It fetches the ObservabilityAlertRule CR, enriches alert details, stores the alert entry,
// sends a notification, and optionally triggers AI RCA analysis.
func (s *AlertService) HandleAlertWebhook(ctx context.Context, req gen.AlertWebhookRequest) (*gen.AlertWebhookResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	// Validate required fields
	ruleName := stringPtrVal(req.RuleName)
	ruleNamespace := stringPtrVal(req.RuleNamespace)
	if ruleName == "" {
		return nil, fmt.Errorf("ruleName is required")
	}
	if ruleNamespace == "" {
		return nil, fmt.Errorf("ruleNamespace is required")
	}

	// Derive alertValue and timestamp from the request
	var alertValue string
	if req.AlertValue != nil {
		alertValue = strconv.FormatFloat(float64(*req.AlertValue), 'f', -1, 64)
	}

	var alertTimestamp string
	if req.AlertTimestamp != nil {
		alertTimestamp = req.AlertTimestamp.Format(time.RFC3339)
	}

	// Fetch the ObservabilityAlertRule CR
	alertRule := &choreoapis.ObservabilityAlertRule{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      ruleName,
		Namespace: ruleNamespace,
	}, alertRule); err != nil {
		return nil, fmt.Errorf("failed to get ObservabilityAlertRule %s/%s: %w", ruleNamespace, ruleName, err)
	}

	// Enrich alert details from the CR
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

	// Populate notification channels from the Actions structure.
	alertDetails.NotificationChannels = make([]string, 0, len(alertRule.Spec.Actions.Notifications.Channels))
	for _, ch := range alertRule.Spec.Actions.Notifications.Channels {
		alertDetails.NotificationChannels = append(alertDetails.NotificationChannels, string(ch))
	}

	// Populate incident actions from the Actions structure
	if alertRule.Spec.Actions.Incident != nil {
		if alertRule.Spec.Actions.Incident.Enabled != nil {
			alertDetails.IncidentEnabled = *alertRule.Spec.Actions.Incident.Enabled
		}
		if alertRule.Spec.Actions.Incident.TriggerAiRca != nil {
			alertDetails.TriggerAiRca = *alertRule.Spec.Actions.Incident.TriggerAiRca
		}
	}

	if s.alertEntryStore == nil {
		return nil, fmt.Errorf("alert entry store is not initialized")
	}

	if alertDetails.AlertTimestamp == "" {
		alertDetails.AlertTimestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	// Marshal notification channels to JSON for storage
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

	alertID, err := s.alertEntryStore.WriteAlertEntry(ctx, &alertentry.AlertEntry{
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
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store alert entry: %w", err)
	}

	s.logger.Debug("Alert entry stored", "alertID", alertID, "ruleName", ruleName)

	// Store incident entry in background to avoid retry-induced duplicate alerts.
	if alertDetails.IncidentEnabled {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if s.incidentEntryStore == nil {
				s.logger.Warn("Incident entry store is not initialized", "alertID", alertID)
				return
			}

			if _, err := s.incidentEntryStore.WriteIncidentEntry(bgCtx, &incidententry.IncidentEntry{
				AlertID:         alertID,
				Timestamp:       alertDetails.AlertTimestamp,
				Status:          incidententry.StatusTriggered,
				TriggerAiRca:    alertDetails.TriggerAiRca,
				TriggeredAt:     alertDetails.AlertTimestamp,
				Description:     alertDetails.AlertDescription,
				NamespaceName:   alertDetails.Namespace,
				ComponentName:   alertDetails.Component,
				EnvironmentName: alertDetails.Environment,
				ProjectName:     alertDetails.Project,
				ComponentID:     alertDetails.ComponentID,
				EnvironmentID:   alertDetails.EnvironmentID,
				ProjectID:       alertDetails.ProjectID,
			}); err != nil {
				s.logger.Warn("Failed to store incident entry", "error", err, "alertID", alertID)
			}
		}()
	}

	// Send notification in background
	go func() {
		notifCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.sendAlertNotification(notifCtx, alertDetails); err != nil {
			s.logger.Warn("Failed to send alert notification", "error", err, "alertID", alertID)
		}
	}()

	// Trigger AI RCA analysis in background if enabled
	if alertDetails.TriggerAiRca && s.aiRCAEnabled {
		go s.triggerRCAAnalysis(alertID, alertDetails, alertRule)
	}

	successStatus := gen.Success
	msg := fmt.Sprintf("alert acknowledged, alertID: %s", alertID)
	return &gen.AlertWebhookResponse{
		Status:  &successStatus,
		Message: &msg,
	}, nil
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

// ---- Sentinel errors ----

// ErrAlertRuleNotFound is returned when the requested alert rule does not exist in the backend.
var ErrAlertRuleNotFound = errors.New("alert rule not found")

// ErrAlertRuleAlreadyExists is returned when trying to create a rule that already exists.
var ErrAlertRuleAlreadyExists = errors.New("alert rule already exists")

// ---- OpenSearch implementations ----

func (s *AlertService) createOpenSearchAlertRule(ctx context.Context, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	ruleName := stringPtrVal(req.Metadata.Name)

	monitorBody, err := s.buildOpenSearchMonitorBody(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build monitor body: %w", err)
	}

	// Fail fast if rule already exists
	_, exists, err := s.osClient.SearchMonitorByName(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to search for existing alert rule: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("%w: %s", ErrAlertRuleAlreadyExists, ruleName)
	}

	backendID, lastUpdateTime, err := s.osClient.CreateMonitor(ctx, monitorBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create alert rule: %w", err)
	}

	s.logger.Debug("OpenSearch alert rule created", "rule_name", ruleName, "backend_id", backendID)
	return buildSyncResponse(alertActionCreated, ruleName, backendID, time.UnixMilli(lastUpdateTime).UTC()), nil
}

func (s *AlertService) getOpenSearchAlertRule(ctx context.Context, ruleName string) (*gen.AlertRuleResponse, error) {
	monitorID, exists, err := s.osClient.SearchMonitorByName(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to search for alert rule: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrAlertRuleNotFound, ruleName)
	}

	rawMonitor, err := s.osClient.GetMonitorByID(ctx, monitorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get alert rule details: %w", err)
	}

	return mapMonitorToAlertRuleResponse(rawMonitor, ruleName)
}

func (s *AlertService) updateOpenSearchAlertRule(ctx context.Context, ruleName string, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	monitorBody, err := s.buildOpenSearchMonitorBody(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build monitor body: %w", err)
	}

	monitorID, exists, err := s.osClient.SearchMonitorByName(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to search for alert rule: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrAlertRuleNotFound, ruleName)
	}

	// Compare to avoid unnecessary updates
	existingMonitor, err := s.osClient.GetMonitorByID(ctx, monitorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing alert rule: %w", err)
	}

	if monitorsAreEqual(s.logger, existingMonitor, monitorBody) {
		s.logger.Debug("OpenSearch alert rule unchanged, skipping update", "rule_name", ruleName)
		return buildSyncResponse(alertActionUnchanged, ruleName, monitorID, time.Now().UTC()), nil
	}

	lastUpdateTime, err := s.osClient.UpdateMonitor(ctx, monitorID, monitorBody)
	if err != nil {
		return nil, fmt.Errorf("failed to update alert rule: %w", err)
	}

	s.logger.Debug("OpenSearch alert rule updated", "rule_name", ruleName, "backend_id", monitorID)
	return buildSyncResponse(alertActionUpdated, ruleName, monitorID, time.UnixMilli(lastUpdateTime).UTC()), nil
}

func (s *AlertService) deleteOpenSearchAlertRule(ctx context.Context, ruleName string) (*gen.AlertingRuleSyncResponse, error) {
	monitorID, exists, err := s.osClient.SearchMonitorByName(ctx, ruleName)
	if err != nil {
		return nil, fmt.Errorf("failed to search for alert rule: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrAlertRuleNotFound, ruleName)
	}

	if err := s.osClient.DeleteMonitor(ctx, monitorID); err != nil {
		return nil, fmt.Errorf("failed to delete alert rule: %w", err)
	}

	s.logger.Debug("OpenSearch alert rule deleted", "rule_name", ruleName, "backend_id", monitorID)
	return buildSyncResponse(alertActionDeleted, ruleName, monitorID, time.Now().UTC()), nil
}

// ---- Prometheus implementations ----

func (s *AlertService) createPrometheusAlertRule(ctx context.Context, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	ruleName := stringPtrVal(req.Metadata.Name)
	prometheusRule, err := s.buildPrometheusRule(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build PrometheusRule: %w", err)
	}

	// Fail fast if rule already exists
	existing := &monitoringv1.PrometheusRule{}
	err = s.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: s.config.Alerting.ObservabilityNamespace,
		Name:      ruleName,
	}, existing)
	if err == nil {
		return nil, fmt.Errorf("%w: %s", ErrAlertRuleAlreadyExists, ruleName)
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check for existing PrometheusRule: %w", err)
	}

	if err := s.k8sClient.Create(ctx, prometheusRule); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// A concurrent request created the rule between our Get check and this Create.
			return nil, fmt.Errorf("%w: %s", ErrAlertRuleAlreadyExists, ruleName)
		}
		return nil, fmt.Errorf("failed to create PrometheusRule: %w", err)
	}

	// Re-fetch to get the assigned UID
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: s.config.Alerting.ObservabilityNamespace,
		Name:      ruleName,
	}, prometheusRule); err != nil {
		s.logger.Warn("Failed to re-fetch PrometheusRule for UID after create", "error", err)
	}

	backendID := string(prometheusRule.UID)
	s.logger.Debug("PrometheusRule created", "rule_name", ruleName, "backend_id", backendID)
	return buildSyncResponse(alertActionCreated, ruleName, backendID, time.Now().UTC()), nil
}

func (s *AlertService) getPrometheusAlertRule(ctx context.Context, ruleName string) (*gen.AlertRuleResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	existing := &monitoringv1.PrometheusRule{}
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: s.config.Alerting.ObservabilityNamespace,
		Name:      ruleName,
	}, existing)
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", ErrAlertRuleNotFound, ruleName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get PrometheusRule: %w", err)
	}

	return mapPrometheusRuleToAlertRuleResponse(existing, ruleName)
}

func (s *AlertService) updatePrometheusAlertRule(ctx context.Context, ruleName string, req gen.AlertRuleRequest) (*gen.AlertingRuleSyncResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	prometheusRule, err := s.buildPrometheusRule(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build PrometheusRule: %w", err)
	}

	existing := &monitoringv1.PrometheusRule{}
	err = s.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: s.config.Alerting.ObservabilityNamespace,
		Name:      ruleName,
	}, existing)
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", ErrAlertRuleNotFound, ruleName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get existing PrometheusRule: %w", err)
	}

	backendID := string(existing.UID)

	// Compare specs to avoid unnecessary updates
	if prometheusSpecsAreEqual(s.logger, existing, prometheusRule) {
		s.logger.Debug("PrometheusRule unchanged, skipping update", "rule_name", ruleName)
		return buildSyncResponse(alertActionUnchanged, ruleName, backendID, time.Now().UTC()), nil
	}

	existing.Spec = prometheusRule.Spec
	existing.Labels = prometheusRule.Labels
	if err := s.k8sClient.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update PrometheusRule: %w", err)
	}

	s.logger.Debug("PrometheusRule updated", "rule_name", ruleName, "backend_id", backendID)
	return buildSyncResponse(alertActionUpdated, ruleName, backendID, time.Now().UTC()), nil
}

func (s *AlertService) deletePrometheusAlertRule(ctx context.Context, ruleName string) (*gen.AlertingRuleSyncResponse, error) {
	if s.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	existing := &monitoringv1.PrometheusRule{}
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: s.config.Alerting.ObservabilityNamespace,
		Name:      ruleName,
	}, existing)
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("%w: %s", ErrAlertRuleNotFound, ruleName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get PrometheusRule: %w", err)
	}

	backendID := string(existing.UID)
	if err := s.k8sClient.Delete(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to delete PrometheusRule: %w", err)
	}

	s.logger.Debug("PrometheusRule deleted", "rule_name", ruleName, "backend_id", backendID)
	return buildSyncResponse(alertActionDeleted, ruleName, backendID, time.Now().UTC()), nil
}

// ---- Builder helpers ----

// buildOpenSearchMonitorBody converts a gen.AlertRuleRequest to the OpenSearch monitor body map.
func (s *AlertService) buildOpenSearchMonitorBody(req gen.AlertRuleRequest) (map[string]interface{}, error) {
	legacy := genRequestToLegacyRequest(req)
	return s.queryBuilder.BuildLogAlertingRuleMonitorBody(legacy)
}

// buildPrometheusRule converts a gen.AlertRuleRequest to a PrometheusRule CR.
func (s *AlertService) buildPrometheusRule(req gen.AlertRuleRequest) (*monitoringv1.PrometheusRule, error) {
	legacy := genRequestToLegacyRequest(req)
	builder := prometheus.NewAlertRuleBuilder(s.config.Alerting.ObservabilityNamespace)
	return builder.BuildPrometheusRule(legacy)
}

// genRequestToLegacyRequest converts the generated API type to the legacy type used by the builders.
func genRequestToLegacyRequest(req gen.AlertRuleRequest) legacytypes.AlertingRuleRequest {
	var meta legacytypes.AlertingRuleMetadata
	if req.Metadata != nil {
		if req.Metadata.Name != nil {
			meta.Name = *req.Metadata.Name
		}
		if req.Metadata.Namespace != nil {
			meta.Namespace = *req.Metadata.Namespace
		}
		if req.Metadata.ComponentUid != nil {
			meta.ComponentUID = req.Metadata.ComponentUid.String()
		}
		if req.Metadata.ProjectUid != nil {
			meta.ProjectUID = req.Metadata.ProjectUid.String()
		}
		if req.Metadata.EnvironmentUid != nil {
			meta.EnvironmentUID = req.Metadata.EnvironmentUid.String()
		}
	}

	var src legacytypes.AlertingRuleSource
	if req.Source != nil {
		if req.Source.Type != nil {
			src.Type = string(*req.Source.Type)
		}
		if req.Source.Query != nil {
			src.Query = *req.Source.Query
		}
		if req.Source.Metric != nil {
			src.Metric = string(*req.Source.Metric)
		}
	}

	var cond legacytypes.AlertingRuleCondition
	if req.Condition != nil {
		if req.Condition.Enabled != nil {
			cond.Enabled = *req.Condition.Enabled
		}
		if req.Condition.Window != nil {
			cond.Window = *req.Condition.Window
		}
		if req.Condition.Interval != nil {
			cond.Interval = *req.Condition.Interval
		}
		if req.Condition.Operator != nil {
			cond.Operator = string(*req.Condition.Operator)
		}
		if req.Condition.Threshold != nil {
			cond.Threshold = float64(*req.Condition.Threshold)
		}
	}

	return legacytypes.AlertingRuleRequest{
		Metadata:  meta,
		Source:    src,
		Condition: cond,
	}
}

// ---- Comparison helpers ----

func monitorsAreEqual(logger *slog.Logger, existing, newMonitor map[string]interface{}) bool {
	existingJSON, err := json.Marshal(existing)
	if err != nil {
		logger.Warn("Failed to marshal existing monitor for comparison", "error", err)
		return false
	}
	var existingBody opensearch.MonitorBody
	if err := json.Unmarshal(existingJSON, &existingBody); err != nil {
		logger.Warn("Failed to unmarshal existing monitor to MonitorBody", "error", err)
		return false
	}

	newJSON, err := json.Marshal(newMonitor)
	if err != nil {
		logger.Warn("Failed to marshal new monitor for comparison", "error", err)
		return false
	}
	var newBody opensearch.MonitorBody
	if err := json.Unmarshal(newJSON, &newBody); err != nil {
		logger.Warn("Failed to unmarshal new monitor to MonitorBody", "error", err)
		return false
	}

	return reflect.DeepEqual(existingBody, newBody)
}

func prometheusSpecsAreEqual(logger *slog.Logger, existing, newRule *monitoringv1.PrometheusRule) bool {
	existingJSON, err := json.Marshal(existing.Spec)
	if err != nil {
		logger.Warn("Failed to marshal existing PrometheusRule spec", "error", err)
		return false
	}
	newJSON, err := json.Marshal(newRule.Spec)
	if err != nil {
		logger.Warn("Failed to marshal new PrometheusRule spec", "error", err)
		return false
	}
	return string(existingJSON) == string(newJSON)
}

// ---- Monitor mapping helpers ----

// webhookTemplateData represents the metadata embedded in the OpenSearch monitor webhook message template.
type webhookTemplateData struct {
	RuleName       string `json:"ruleName"`
	RuleNamespace  string `json:"ruleNamespace"`
	ComponentUID   string `json:"componentUid"`
	ProjectUID     string `json:"projectUid"`
	EnvironmentUID string `json:"environmentUid"`
}

// mapMonitorToAlertRuleResponse converts a raw OpenSearch monitor into a gen.AlertRuleResponse.
func mapMonitorToAlertRuleResponse(rawMonitor map[string]interface{}, ruleName string) (*gen.AlertRuleResponse, error) {
	monitorJSON, err := json.Marshal(rawMonitor)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal monitor: %w", err)
	}
	var monitor opensearch.MonitorBody
	if err := json.Unmarshal(monitorJSON, &monitor); err != nil {
		return nil, fmt.Errorf("failed to parse monitor body: %w", err)
	}

	resp := &gen.AlertRuleResponse{}

	// Extract metadata from the webhook message template
	templateData := extractWebhookTemplateData(monitor)
	name := ruleName
	namespace := templateData.RuleNamespace
	sourceType := gen.AlertRuleResponseSourceType(sourceTypeLog)

	//nolint:revive,staticcheck // field names match generated code (e.g. ComponentUid not ComponentUID)
	resp.Metadata = &struct {
		ComponentUid   *openapi_types.UUID `json:"componentUid,omitempty"`
		EnvironmentUid *openapi_types.UUID `json:"environmentUid,omitempty"`
		Name           *string             `json:"name,omitempty"`
		Namespace      *string             `json:"namespace,omitempty"`
		ProjectUid     *openapi_types.UUID `json:"projectUid,omitempty"`
	}{
		Name:      &name,
		Namespace: &namespace,
	}
	if uid, err := uuid.Parse(templateData.ComponentUID); err == nil {
		resp.Metadata.ComponentUid = &uid
	}
	if uid, err := uuid.Parse(templateData.ProjectUID); err == nil {
		resp.Metadata.ProjectUid = &uid
	}
	if uid, err := uuid.Parse(templateData.EnvironmentUID); err == nil {
		resp.Metadata.EnvironmentUid = &uid
	}

	// Extract source
	query := extractQueryFromMonitor(monitor)
	resp.Source = &struct {
		Metric *gen.AlertRuleResponseSourceMetric `json:"metric,omitempty"`
		Query  *string                            `json:"query,omitempty"`
		Type   *gen.AlertRuleResponseSourceType   `json:"type,omitempty"`
	}{
		Type:  &sourceType,
		Query: &query,
	}

	// Extract condition
	operator, threshold := extractTriggerCondition(monitor)
	interval := formatMinutesDuration(monitor.Schedule.Period.Interval)
	resp.Condition = &struct {
		Enabled   *bool                                   `json:"enabled,omitempty"`
		Interval  *string                                 `json:"interval,omitempty"`
		Operator  *gen.AlertRuleResponseConditionOperator `json:"operator,omitempty"`
		Threshold *float32                                `json:"threshold,omitempty"`
		Window    *string                                 `json:"window,omitempty"`
	}{
		Enabled:  &monitor.Enabled,
		Interval: &interval,
	}
	if operator != "" {
		op := gen.AlertRuleResponseConditionOperator(operator)
		resp.Condition.Operator = &op
	}
	if threshold != nil {
		resp.Condition.Threshold = threshold
	}
	if window := extractWindowFromQuery(monitor); window != "" {
		resp.Condition.Window = &window
	}

	return resp, nil
}

// extractWebhookTemplateData parses the webhook message template to recover metadata.
func extractWebhookTemplateData(monitor opensearch.MonitorBody) webhookTemplateData {
	var data webhookTemplateData
	if len(monitor.Triggers) == 0 {
		return data
	}
	trigger := monitor.Triggers[0].QueryLevelTrigger
	if trigger == nil || len(trigger.Actions) == 0 {
		return data
	}
	tmpl := trigger.Actions[0].MessageTemplate.Source
	// The template contains Mustache expressions (e.g. {{ctx.results...}}) that are not valid JSON.
	// Truncate at the first {{ to get parseable JSON, then close the object.
	if idx := strings.Index(tmpl, ",\"alertValue\":{{"); idx > 0 {
		tmpl = tmpl[:idx] + "}"
	}
	_ = json.Unmarshal([]byte(tmpl), &data)
	return data
}

// extractQueryFromMonitor extracts the log search query from the monitor's wildcard filter.
func extractQueryFromMonitor(monitor opensearch.MonitorBody) string {
	if len(monitor.Inputs) == 0 {
		return ""
	}
	query := monitor.Inputs[0].Search.Query
	filters := extractBoolFilters(query)
	for _, filter := range filters {
		if wc, ok := filter["wildcard"].(map[string]interface{}); ok {
			if logEntry, ok := wc["log"].(map[string]interface{}); ok {
				if pattern, ok := logEntry["wildcard"].(string); ok {
					return strings.TrimSuffix(strings.TrimPrefix(pattern, "*"), "*")
				}
			}
		}
	}
	return ""
}

// extractTriggerCondition parses the trigger script to extract operator and threshold.
// The script format is: "ctx.results[0].hits.total.value > 100"
func extractTriggerCondition(monitor opensearch.MonitorBody) (string, *float32) {
	if len(monitor.Triggers) == 0 {
		return "", nil
	}
	trigger := monitor.Triggers[0].QueryLevelTrigger
	if trigger == nil {
		return "", nil
	}
	script := trigger.Condition.Script.Source

	operatorMap := map[string]string{
		">=": "gte",
		"<=": "lte",
		">":  "gt",
		"<":  "lt",
		"==": "eq",
		"!=": "neq",
	}
	// Try longer operators first to avoid matching ">" before ">="
	for _, sym := range []string{">=", "<=", "!=", "==", ">", "<"} {
		if parts := strings.SplitN(script, " "+sym+" ", 2); len(parts) == 2 {
			op := operatorMap[sym]
			if val, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32); err == nil {
				f := float32(val)
				return op, &f
			}
			return op, nil
		}
	}
	return "", nil
}

// extractWindowFromQuery parses the timestamp range filter to extract the window duration.
// The "from" field has format: "{{period_end}}||-5m"
func extractWindowFromQuery(monitor opensearch.MonitorBody) string {
	if len(monitor.Inputs) == 0 {
		return ""
	}
	filters := extractBoolFilters(monitor.Inputs[0].Search.Query)
	for _, filter := range filters {
		if rangeMap, ok := filter["range"].(map[string]interface{}); ok {
			if ts, ok := rangeMap["@timestamp"].(map[string]interface{}); ok {
				if from, ok := ts["from"].(string); ok {
					// Format: "{{period_end}}||-5m"
					if idx := strings.Index(from, "||-"); idx >= 0 {
						return from[idx+3:]
					}
				}
			}
		}
	}
	return ""
}

// extractBoolFilters extracts the filter array from a bool query.
func extractBoolFilters(query map[string]interface{}) []map[string]interface{} {
	q, ok := query["query"].(map[string]interface{})
	if !ok {
		return nil
	}
	b, ok := q["bool"].(map[string]interface{})
	if !ok {
		return nil
	}
	filters, ok := b["filter"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(filters))
	for _, f := range filters {
		if fm, ok := f.(map[string]interface{}); ok {
			result = append(result, fm)
		}
	}
	return result
}

// formatMinutesDuration converts a float64 minutes value to a Go duration string (e.g. "5m").
func formatMinutesDuration(minutes float64) string {
	d := time.Duration(minutes * float64(time.Minute))
	return d.String()
}

// ---- Prometheus mapping helpers ----

// mapPrometheusRuleToAlertRuleResponse converts a PrometheusRule CR into a gen.AlertRuleResponse.
func mapPrometheusRuleToAlertRuleResponse(pr *monitoringv1.PrometheusRule, ruleName string) (*gen.AlertRuleResponse, error) {
	resp := &gen.AlertRuleResponse{}

	// Extract the first rule from the first group
	if len(pr.Spec.Groups) == 0 || len(pr.Spec.Groups[0].Rules) == 0 {
		return nil, fmt.Errorf("PrometheusRule has no rule groups or rules")
	}
	group := pr.Spec.Groups[0]
	rule := group.Rules[0]

	// Metadata: name from ruleName, namespace from annotations
	name := ruleName
	namespace := rule.Annotations["rule_namespace"]
	sourceType := gen.AlertRuleResponseSourceType(sourceTypeMetric)

	//nolint:revive,staticcheck // field names match generated code (e.g. ComponentUid not ComponentUID)
	resp.Metadata = &struct {
		ComponentUid   *openapi_types.UUID `json:"componentUid,omitempty"`
		EnvironmentUid *openapi_types.UUID `json:"environmentUid,omitempty"`
		Name           *string             `json:"name,omitempty"`
		Namespace      *string             `json:"namespace,omitempty"`
		ProjectUid     *openapi_types.UUID `json:"projectUid,omitempty"`
	}{
		Name:      &name,
		Namespace: &namespace,
	}

	// Extract UIDs from the PromQL expression label filters
	expr := rule.Expr.String()
	// These are the Prometheus label names used in PromQL expressions, derived from
	// the Kubernetes labels by replacing dots/dashes/slashes with underscores and prefixing with "label_".
	const (
		promComponentUIDLabel   = "label_openchoreo_dev_component_uid"
		promProjectUIDLabel     = "label_openchoreo_dev_project_uid"
		promEnvironmentUIDLabel = "label_openchoreo_dev_environment_uid"
	)
	if uid, err := uuid.Parse(extractPromLabelValue(expr, promComponentUIDLabel)); err == nil {
		resp.Metadata.ComponentUid = &uid
	}
	if uid, err := uuid.Parse(extractPromLabelValue(expr, promProjectUIDLabel)); err == nil {
		resp.Metadata.ProjectUid = &uid
	}
	if uid, err := uuid.Parse(extractPromLabelValue(expr, promEnvironmentUIDLabel)); err == nil {
		resp.Metadata.EnvironmentUid = &uid
	}

	// Source: type is always "metric", detect metric from the expression
	metric := detectMetricType(expr)
	resp.Source = &struct {
		Metric *gen.AlertRuleResponseSourceMetric `json:"metric,omitempty"`
		Query  *string                            `json:"query,omitempty"`
		Type   *gen.AlertRuleResponseSourceType   `json:"type,omitempty"`
	}{
		Type:   &sourceType,
		Metric: &metric,
	}

	// Condition
	operator, threshold := extractPromOperatorAndThreshold(expr)
	enabled := true // PrometheusRule CRs are always enabled when they exist
	resp.Condition = &struct {
		Enabled   *bool                                   `json:"enabled,omitempty"`
		Interval  *string                                 `json:"interval,omitempty"`
		Operator  *gen.AlertRuleResponseConditionOperator `json:"operator,omitempty"`
		Threshold *float32                                `json:"threshold,omitempty"`
		Window    *string                                 `json:"window,omitempty"`
	}{
		Enabled: &enabled,
	}
	if group.Interval != nil {
		interval := string(*group.Interval)
		resp.Condition.Interval = &interval
	}
	if rule.For != nil {
		window := string(*rule.For)
		resp.Condition.Window = &window
	}
	if operator != "" {
		op := gen.AlertRuleResponseConditionOperator(operator)
		resp.Condition.Operator = &op
	}
	if threshold != nil {
		resp.Condition.Threshold = threshold
	}

	return resp, nil
}

// extractPromLabelValue extracts the value of a label from a PromQL expression.
// Looks for patterns like: label_name="value"
func extractPromLabelValue(expr, labelName string) string {
	prefix := labelName + `="`
	idx := strings.Index(expr, prefix)
	if idx < 0 {
		return ""
	}
	start := idx + len(prefix)
	end := strings.Index(expr[start:], `"`)
	if end < 0 {
		return ""
	}
	return expr[start : start+end]
}

// detectMetricType determines the metric type from the PromQL expression.
func detectMetricType(expr string) gen.AlertRuleResponseSourceMetric {
	if strings.Contains(expr, "container_cpu_usage_seconds_total") {
		return gen.AlertRuleResponseSourceMetricCpuUsage
	}
	return gen.AlertRuleResponseSourceMetricMemoryUsage
}

// extractPromOperatorAndThreshold extracts the comparison operator and threshold
// from the end of a PromQL expression like: "...) * 100 >= 80"
func extractPromOperatorAndThreshold(expr string) (string, *float32) {
	operatorMap := map[string]string{
		">=": "gte",
		"<=": "lte",
		">":  "gt",
		"<":  "lt",
		"==": "eq",
		"!=": "neq",
	}
	for _, sym := range []string{">=", "<=", "!=", "==", ">", "<"} {
		if parts := strings.SplitN(expr, " "+sym+" ", 2); len(parts) == 2 {
			op := operatorMap[sym]
			if val, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32); err == nil {
				f := float32(val)
				return op, &f
			}
			return op, nil
		}
	}
	return "", nil
}

// ---- Utility helpers ----

func sourceTypeFromRequest(req gen.AlertRuleRequest) (string, error) {
	if req.Source == nil || req.Source.Type == nil {
		return "", fmt.Errorf("source.type is required")
	}
	return string(*req.Source.Type), nil
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
