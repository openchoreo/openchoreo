// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreoapis "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

const (
	defaultQueryLimit = 100
)

func (s *AlertService) QueryAlerts(ctx context.Context, req gen.AlertsQueryRequest) (*gen.AlertsQueryResponse, error) {
	if s.alertEntryStore == nil {
		return nil, fmt.Errorf("alert entry store is not initialized")
	}

	start := time.Now()
	queryParams := alertentry.QueryParams{
		StartTime:       req.StartTime.Format(time.RFC3339Nano),
		EndTime:         req.EndTime.Format(time.RFC3339Nano),
		NamespaceName:   req.SearchScope.Namespace,
		ProjectName:     stringPtrValue(req.SearchScope.Project),
		ComponentName:   stringPtrValue(req.SearchScope.Component),
		EnvironmentName: stringPtrValue(req.SearchScope.Environment),
		Limit:           intPtrValue(req.Limit, defaultQueryLimit),
		SortOrder:       string(alertSortOrderOrDefault(req.SortOrder)),
	}

	entries, total, err := s.alertEntryStore.QueryAlertEntries(ctx, queryParams)
	if err != nil {
		return nil, err
	}

	crCache := make(map[string]*choreoapis.ObservabilityAlertRule, len(entries))
	items := make([]alertQueryItemPayload, 0, len(entries))
	for _, entry := range entries {
		crKey := strings.TrimSpace(entry.AlertRuleCRNamespace) + "/" + strings.TrimSpace(entry.AlertRuleCRName)
		if _, exists := crCache[crKey]; !exists {
			cr, getErr := s.getAlertRuleCR(ctx, entry.AlertRuleCRNamespace, entry.AlertRuleCRName)
			if getErr != nil {
				if !apierrors.IsNotFound(getErr) {
					return nil, fmt.Errorf("failed to get alert rule custom resource %s: %w", crKey, getErr)
				}
				s.logger.Debug("Alert rule CR not found for alert entry", "rule", crKey, "error", getErr)
			}
			crCache[crKey] = cr
		}
		items = append(items, s.buildAlertQueryItem(entry, crCache[crKey]))
	}

	responsePayload := alertQueryResponsePayload{
		Alerts: items,
		Total:  intPtr(total),
		TookMs: intPtr(int(time.Since(start).Milliseconds())),
	}

	raw, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal alerts query response payload: %w", err)
	}
	var response gen.AlertsQueryResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal alerts query response payload: %w", err)
	}
	return &response, nil
}

func (s *AlertService) QueryIncidents(ctx context.Context, req gen.IncidentsQueryRequest) (*gen.IncidentsQueryResponse, error) {
	if s.incidentEntryStore == nil {
		return nil, fmt.Errorf("incident entry store is not initialized")
	}

	start := time.Now()
	queryParams := incidententry.QueryParams{
		StartTime:       req.StartTime.Format(time.RFC3339Nano),
		EndTime:         req.EndTime.Format(time.RFC3339Nano),
		NamespaceName:   req.SearchScope.Namespace,
		ProjectName:     stringPtrValue(req.SearchScope.Project),
		ComponentName:   stringPtrValue(req.SearchScope.Component),
		EnvironmentName: stringPtrValue(req.SearchScope.Environment),
		Limit:           intPtrValue(req.Limit, defaultQueryLimit),
		SortOrder:       string(incidentSortOrderOrDefault(req.SortOrder)),
	}

	entries, total, err := s.incidentEntryStore.QueryIncidentEntries(ctx, queryParams)
	if err != nil {
		return nil, err
	}

	items := make([]incidentQueryItemPayload, 0, len(entries))
	for _, entry := range entries {
		items = append(items, incidentQueryItemPayload{
			Timestamp:            parseTimePtr(entry.Timestamp),
			AlertID:              stringPtr(strings.TrimSpace(entry.AlertID)),
			IncidentID:           stringPtr(strings.TrimSpace(entry.ID)),
			IncidentTriggerAiRca: boolPtr(entry.TriggerAiRca),
			Status:               stringPtr(strings.TrimSpace(entry.Status)),
			TriggeredAt:          parseTimePtr(entry.TriggeredAt),
			AcknowledgedAt:       parseTimePtr(entry.AcknowledgedAt),
			ResolvedAt:           parseTimePtr(entry.ResolvedAt),
			Notes:                stringPtr(strings.TrimSpace(entry.Notes)),
			Description:          stringPtr(strings.TrimSpace(entry.Description)),
			Labels: buildLabelsPayload(
				entry.NamespaceName,
				entry.ProjectName,
				entry.ComponentName,
				entry.EnvironmentName,
				entry.ProjectID,
				entry.ComponentID,
				entry.EnvironmentID,
			),
		})
	}

	responsePayload := incidentQueryResponsePayload{
		Incidents: items,
		Total:     intPtr(total),
		TookMs:    intPtr(int(time.Since(start).Milliseconds())),
	}

	raw, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal incidents query response payload: %w", err)
	}
	var response gen.IncidentsQueryResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal incidents query response payload: %w", err)
	}
	return &response, nil
}

func (s *AlertService) getAlertRuleCR(ctx context.Context, ruleNamespace, ruleName string) (*choreoapis.ObservabilityAlertRule, error) {
	if s.k8sClient == nil {
		return nil, nil
	}
	if strings.TrimSpace(ruleNamespace) == "" || strings.TrimSpace(ruleName) == "" {
		return nil, nil
	}
	alertRule := &choreoapis.ObservabilityAlertRule{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: ruleName, Namespace: ruleNamespace}, alertRule); err != nil {
		return nil, err
	}
	return alertRule, nil
}

func (s *AlertService) buildAlertQueryItem(entry alertentry.AlertEntry, alertRule *choreoapis.ObservabilityAlertRule) alertQueryItemPayload {
	item := alertQueryItemPayload{
		Timestamp:  parseTimePtr(entry.Timestamp),
		AlertID:    stringPtr(strings.TrimSpace(entry.ID)),
		AlertValue: stringPtr(strings.TrimSpace(entry.AlertValue)),
		Metadata: alertMetadataPayload{
			Labels: buildLabelsPayload(
				entry.NamespaceName,
				entry.ProjectName,
				entry.ComponentName,
				entry.EnvironmentName,
				entry.ProjectID,
				entry.ComponentID,
				entry.EnvironmentID,
			),
			AlertRule: &alertRulePayload{Name: stringPtr(strings.TrimSpace(entry.AlertRuleName))},
		},
		NotificationChannels: []string{},
	}

	if alertRule == nil {
		return item
	}

	item.NotificationChannels = make([]string, 0, len(alertRule.Spec.Actions.Notifications.Channels))
	for _, channel := range alertRule.Spec.Actions.Notifications.Channels {
		chName := strings.TrimSpace(string(channel))
		if chName != "" {
			item.NotificationChannels = append(item.NotificationChannels, chName)
		}
	}
	item.Metadata.AlertRule = &alertRulePayload{
		Name:        stringPtr(strings.TrimSpace(alertRule.Spec.Name)),
		Description: stringPtr(strings.TrimSpace(alertRule.Spec.Description)),
		Severity:    stringPtr(strings.TrimSpace(string(alertRule.Spec.Severity))),
		Source: &alertRuleSourcePayload{
			Type:   stringPtr(strings.TrimSpace(string(alertRule.Spec.Source.Type))),
			Query:  stringPtr(strings.TrimSpace(alertRule.Spec.Source.Query)),
			Metric: stringPtr(strings.TrimSpace(alertRule.Spec.Source.Metric)),
		},
		Condition: &alertRuleConditionPayload{
			Operator:  stringPtr(strings.TrimSpace(string(alertRule.Spec.Condition.Operator))),
			Threshold: float32Ptr(float32(alertRule.Spec.Condition.Threshold)),
			Window:    stringPtr(strings.TrimSpace(alertRule.Spec.Condition.Window.Duration.String())),
			Interval:  stringPtr(strings.TrimSpace(alertRule.Spec.Condition.Interval.Duration.String())),
		},
	}
	item.Metadata.Labels = buildLabelsPayload(
		alertRule.Labels[labels.LabelKeyNamespaceName],
		alertRule.Labels[labels.LabelKeyProjectName],
		alertRule.Labels[labels.LabelKeyComponentName],
		alertRule.Labels[labels.LabelKeyEnvironmentName],
		alertRule.Labels[labels.LabelKeyProjectUID],
		alertRule.Labels[labels.LabelKeyComponentUID],
		alertRule.Labels[labels.LabelKeyEnvironmentUID],
	)
	return item
}

func buildLabelsPayload(
	namespace, project, component, environment string,
	projectUID, componentUID, environmentUID string,
) *labelsPayload {
	return &labelsPayload{
		NamespaceName:   stringPtr(strings.TrimSpace(namespace)),
		ProjectName:     stringPtr(strings.TrimSpace(project)),
		ComponentName:   stringPtr(strings.TrimSpace(component)),
		EnvironmentName: stringPtr(strings.TrimSpace(environment)),
		ProjectUID:      uuidStringPtr(strings.TrimSpace(projectUID)),
		ComponentUID:    uuidStringPtr(strings.TrimSpace(componentUID)),
		EnvironmentUID:  uuidStringPtr(strings.TrimSpace(environmentUID)),
	}
}

func intPtrValue(v *int, defaultValue int) int {
	if v == nil || *v <= 0 {
		return defaultValue
	}
	return *v
}

func alertSortOrderOrDefault(order *gen.AlertsQueryRequestSortOrder) gen.AlertsQueryRequestSortOrder {
	if order == nil || strings.TrimSpace(string(*order)) == "" {
		return gen.AlertsQueryRequestSortOrderDesc
	}
	return *order
}

func incidentSortOrderOrDefault(order *gen.IncidentsQueryRequestSortOrder) gen.IncidentsQueryRequestSortOrder {
	if order == nil || strings.TrimSpace(string(*order)) == "" {
		return gen.IncidentsQueryRequestSortOrderDesc
	}
	return *order
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func parseTimePtr(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return nil
		}
	}
	parsed = parsed.UTC()
	return &parsed
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func float32Ptr(value float32) *float32 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func uuidStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return nil
	}
	return &value
}

type alertQueryResponsePayload struct {
	Alerts []alertQueryItemPayload `json:"alerts,omitempty"`
	Total  *int                    `json:"total,omitempty"`
	TookMs *int                    `json:"tookMs,omitempty"`
}

type alertQueryItemPayload struct {
	Timestamp            *time.Time           `json:"timestamp,omitempty"`
	AlertID              *string              `json:"alertId,omitempty"`
	AlertValue           *string              `json:"alertValue,omitempty"`
	NotificationChannels []string             `json:"notificationChannels,omitempty"`
	Metadata             alertMetadataPayload `json:"metadata,omitempty"`
}

type alertMetadataPayload struct {
	AlertRule *alertRulePayload `json:"alertRule,omitempty"`
	Labels    *labelsPayload    `json:"labels,omitempty"`
}

type alertRulePayload struct {
	Name        *string                    `json:"name,omitempty"`
	Description *string                    `json:"description,omitempty"`
	Severity    *string                    `json:"severity,omitempty"`
	Source      *alertRuleSourcePayload    `json:"source,omitempty"`
	Condition   *alertRuleConditionPayload `json:"condition,omitempty"`
}

type alertRuleSourcePayload struct {
	Type   *string `json:"type,omitempty"`
	Query  *string `json:"query,omitempty"`
	Metric *string `json:"metric,omitempty"`
}

type alertRuleConditionPayload struct {
	Operator  *string  `json:"operator,omitempty"`
	Threshold *float32 `json:"threshold,omitempty"`
	Window    *string  `json:"window,omitempty"`
	Interval  *string  `json:"interval,omitempty"`
}

type labelsPayload struct {
	ComponentName   *string `json:"componentName,omitempty"`
	ComponentUID    *string `json:"componentUid,omitempty"`
	EnvironmentName *string `json:"environmentName,omitempty"`
	EnvironmentUID  *string `json:"environmentUid,omitempty"`
	NamespaceName   *string `json:"namespaceName,omitempty"`
	ProjectName     *string `json:"projectName,omitempty"`
	ProjectUID      *string `json:"projectUid,omitempty"`
}

type incidentQueryResponsePayload struct {
	Incidents []incidentQueryItemPayload `json:"incidents,omitempty"`
	Total     *int                       `json:"total,omitempty"`
	TookMs    *int                       `json:"tookMs,omitempty"`
}

type incidentQueryItemPayload struct {
	Timestamp            *time.Time     `json:"timestamp,omitempty"`
	AlertID              *string        `json:"alertId,omitempty"`
	IncidentID           *string        `json:"incidentId,omitempty"`
	IncidentTriggerAiRca *bool          `json:"incidentTriggerAiRca,omitempty"`
	Status               *string        `json:"status,omitempty"`
	TriggeredAt          *time.Time     `json:"triggeredAt,omitempty"`
	AcknowledgedAt       *time.Time     `json:"acknowledgedAt,omitempty"`
	ResolvedAt           *time.Time     `json:"resolvedAt,omitempty"`
	Notes                *string        `json:"notes,omitempty"`
	Description          *string        `json:"description,omitempty"`
	Labels               *labelsPayload `json:"labels,omitempty"`
}
