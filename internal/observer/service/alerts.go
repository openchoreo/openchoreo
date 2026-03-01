// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
	"github.com/openchoreo/openchoreo/internal/observer/prometheus"
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
	osClient     AlertOpenSearchClient
	queryBuilder *opensearch.QueryBuilder
	k8sClient    client.Client
	config       *config.Config
	logger       *slog.Logger
}

// NewAlertService creates a new AlertService.
func NewAlertService(
	osClient AlertOpenSearchClient,
	queryBuilder *opensearch.QueryBuilder,
	k8sClient client.Client,
	cfg *config.Config,
	logger *slog.Logger,
) *AlertService {
	return &AlertService{
		osClient:     osClient,
		queryBuilder: queryBuilder,
		k8sClient:    k8sClient,
		config:       cfg,
		logger:       logger,
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

	logicalID := ruleName
	return &gen.AlertRuleResponse{
		RuleLogicalId: &logicalID,
		RuleBackendId: &monitorID,
	}, nil
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

	logicalID := ruleName
	backendID := string(existing.UID)
	return &gen.AlertRuleResponse{
		RuleLogicalId: &logicalID,
		RuleBackendId: &backendID,
	}, nil
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
