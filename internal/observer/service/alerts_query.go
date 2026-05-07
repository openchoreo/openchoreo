// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

const (
	defaultQueryLimit = 100
)

var ErrAlertsResolveSearchScope = errors.New("alerts search scope resolution failed")
var ErrScopeNotFound = errors.New("search scope resource not found")
var ErrScopeResolutionFailed = errors.New("search scope resolution infrastructure error")

func (s *AlertService) QueryAlerts(ctx context.Context, req gen.AlertsQueryRequest) (*gen.AlertsQueryResponse, error) {
	if s.alertEntryStore == nil {
		return nil, fmt.Errorf("alert entry store is not initialized")
	}

	scope := &req.SearchScope

	var projectUID, componentUID, environmentUID string
	if s.resolver != nil {
		projectName := stringPtrValue(scope.Project)
		componentName := stringPtrValue(scope.Component)
		environmentName := stringPtrValue(scope.Environment)

		if projectName != "" {
			var err error
			projectUID, err = s.resolver.GetProjectUID(ctx, scope.Namespace, projectName)
			if err != nil {
				return nil, wrapScopeError(err, "project", projectName)
			}
		}
		if componentName != "" {
			var err error
			componentUID, err = s.resolver.GetComponentUID(ctx, scope.Namespace, projectName, componentName)
			if err != nil {
				return nil, wrapScopeError(err, "component", componentName)
			}
		}
		if environmentName != "" {
			var err error
			environmentUID, err = s.resolver.GetEnvironmentUID(ctx, scope.Namespace, environmentName)
			if err != nil {
				return nil, wrapScopeError(err, "environment", environmentName)
			}
		}
	}

	start := time.Now()
	queryParams := alertentry.QueryParams{
		StartTime:     req.StartTime.Format(time.RFC3339Nano),
		EndTime:       req.EndTime.Format(time.RFC3339Nano),
		NamespaceName: scope.Namespace,
		ProjectID:     projectUID,
		ComponentID:   componentUID,
		EnvironmentID: environmentUID,
		Limit:         intPtrValue(req.Limit, defaultQueryLimit),
		SortOrder:     string(alertSortOrderOrDefault(req.SortOrder)),
	}

	entries, total, err := s.alertEntryStore.QueryAlertEntries(ctx, queryParams)
	if err != nil {
		return nil, fmt.Errorf("query alert entries: %w", err)
	}

	items := make([]alertQueryItemPayload, 0, len(entries))
	for _, entry := range entries {
		items = append(items, s.buildAlertQueryItem(entry))
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

	scope := &req.SearchScope

	var projectUID, componentUID, environmentUID string
	if s.resolver != nil {
		projectName := stringPtrValue(scope.Project)
		componentName := stringPtrValue(scope.Component)
		environmentName := stringPtrValue(scope.Environment)

		if projectName != "" {
			var err error
			projectUID, err = s.resolver.GetProjectUID(ctx, scope.Namespace, projectName)
			if err != nil {
				return nil, wrapScopeError(err, "project", projectName)
			}
		}
		if componentName != "" {
			var err error
			componentUID, err = s.resolver.GetComponentUID(ctx, scope.Namespace, projectName, componentName)
			if err != nil {
				return nil, wrapScopeError(err, "component", componentName)
			}
		}
		if environmentName != "" {
			var err error
			environmentUID, err = s.resolver.GetEnvironmentUID(ctx, scope.Namespace, environmentName)
			if err != nil {
				return nil, wrapScopeError(err, "environment", environmentName)
			}
		}
	}

	start := time.Now()
	queryParams := incidententry.QueryParams{
		StartTime:     req.StartTime.Format(time.RFC3339Nano),
		EndTime:       req.EndTime.Format(time.RFC3339Nano),
		NamespaceName: scope.Namespace,
		ProjectID:     projectUID,
		ComponentID:   componentUID,
		EnvironmentID: environmentUID,
		Limit:         intPtrValue(req.Limit, defaultQueryLimit),
		SortOrder:     string(incidentSortOrderOrDefault(req.SortOrder)),
	}

	entries, total, err := s.incidentEntryStore.QueryIncidentEntries(ctx, queryParams)
	if err != nil {
		return nil, fmt.Errorf("query incident entries: %w", err)
	}

	items := make([]incidentQueryItemPayload, 0, len(entries))
	for _, entry := range entries {
		items = append(items, incidentQueryItemPayload{
			Timestamp:                     parseTimePtr(entry.Timestamp),
			AlertID:                       stringPtr(strings.TrimSpace(entry.AlertID)),
			IncidentID:                    stringPtr(strings.TrimSpace(entry.ID)),
			IncidentTriggerAiRca:          boolPtr(entry.TriggerAiRca),
			IncidentTriggerAiCostAnalysis: boolPtr(entry.TriggerAiCostAnalysis),
			Status:                        stringPtr(strings.TrimSpace(entry.Status)),
			TriggeredAt:                   parseTimePtr(entry.TriggeredAt),
			AcknowledgedAt:                parseTimePtr(entry.AcknowledgedAt),
			ResolvedAt:                    parseTimePtr(entry.ResolvedAt),
			Notes:                         stringPtr(strings.TrimSpace(entry.Notes)),
			Description:                   stringPtr(strings.TrimSpace(entry.Description)),
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

func (s *AlertService) UpdateIncident(ctx context.Context, id string, req gen.IncidentPutRequest) (*gen.IncidentPutResponse, error) {
	if s.incidentEntryStore == nil {
		return nil, fmt.Errorf("incident entry store is not initialized")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("incident id is required")
	}

	status := strings.TrimSpace(string(req.Status))
	if status == "" {
		return nil, fmt.Errorf("incident status is required")
	}
	if status != incidententry.StatusActive && status != incidententry.StatusAcknowledged && status != incidententry.StatusResolved {
		return nil, fmt.Errorf("unsupported incident status %q", status)
	}

	entry, err := s.incidentEntryStore.UpdateIncidentEntry(ctx, id, status, req.Notes, req.Description, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("update incident entry: %w", err)
	}

	payload := incidentPutResponsePayload{
		IncidentID:                    stringPtr(strings.TrimSpace(entry.ID)),
		AlertID:                       stringPtr(strings.TrimSpace(entry.AlertID)),
		Status:                        stringPtr(strings.TrimSpace(entry.Status)),
		TriggeredAt:                   parseTimePtr(entry.TriggeredAt),
		AcknowledgedAt:                parseTimePtr(entry.AcknowledgedAt),
		ResolvedAt:                    parseTimePtr(entry.ResolvedAt),
		Notes:                         stringPtr(strings.TrimSpace(entry.Notes)),
		Description:                   stringPtr(strings.TrimSpace(entry.Description)),
		IncidentTriggerAiRca:          boolPtr(entry.TriggerAiRca),
		IncidentTriggerAiCostAnalysis: boolPtr(entry.TriggerAiCostAnalysis),
		Labels: buildLabelsPayload(
			entry.NamespaceName,
			entry.ProjectName,
			entry.ComponentName,
			entry.EnvironmentName,
			entry.ProjectID,
			entry.ComponentID,
			entry.EnvironmentID,
		),
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal incident put response payload: %w", err)
	}
	var response gen.IncidentPutResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal incident put response payload: %w", err)
	}
	return &response, nil
}

func (s *AlertService) buildAlertQueryItem(entry alertentry.AlertEntry) alertQueryItemPayload {
	item := alertQueryItemPayload{
		Timestamp:       parseTimePtr(entry.Timestamp),
		AlertID:         stringPtr(strings.TrimSpace(entry.ID)),
		AlertValue:      stringPtr(strings.TrimSpace(entry.AlertValue)),
		IncidentEnabled: boolPtr(entry.IncidentEnabled),
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
		NotificationChannels: parseNotificationChannelsJSON(entry.NotificationChannels),
	}

	if strings.TrimSpace(entry.Severity) != "" || strings.TrimSpace(entry.Description) != "" ||
		strings.TrimSpace(entry.SourceType) != "" || strings.TrimSpace(entry.SourceQuery) != "" ||
		strings.TrimSpace(entry.SourceMetric) != "" || strings.TrimSpace(entry.ConditionOperator) != "" ||
		entry.ConditionThreshold != 0 || strings.TrimSpace(entry.ConditionWindow) != "" ||
		strings.TrimSpace(entry.ConditionInterval) != "" {
		item.Metadata.AlertRule = &alertRulePayload{
			Name:        stringPtr(strings.TrimSpace(entry.AlertRuleName)),
			Description: stringPtr(strings.TrimSpace(entry.Description)),
			Severity:    stringPtr(strings.TrimSpace(entry.Severity)),
			Source: &alertRuleSourcePayload{
				Type:   stringPtr(strings.TrimSpace(entry.SourceType)),
				Query:  stringPtr(strings.TrimSpace(entry.SourceQuery)),
				Metric: stringPtr(strings.TrimSpace(entry.SourceMetric)),
			},
			Condition: &alertRuleConditionPayload{
				Operator:  stringPtr(strings.TrimSpace(entry.ConditionOperator)),
				Threshold: float32Ptr(float32(entry.ConditionThreshold)),
				Window:    stringPtr(strings.TrimSpace(entry.ConditionWindow)),
				Interval:  stringPtr(strings.TrimSpace(entry.ConditionInterval)),
			},
		}
	}
	return item
}

func parseNotificationChannelsJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var channels []string
	if err := json.Unmarshal([]byte(raw), &channels); err != nil {
		return []string{}
	}
	result := make([]string, 0, len(channels))
	for _, ch := range channels {
		s := strings.TrimSpace(ch)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
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
			slog.Default().Warn("failed to parse timestamp for alerts/incidents response", "value", value, "error", err)
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
	Alerts []alertQueryItemPayload `json:"alerts"`
	Total  *int                    `json:"total,omitempty"`
	TookMs *int                    `json:"tookMs,omitempty"`
}

type alertQueryItemPayload struct {
	Timestamp            *time.Time           `json:"timestamp,omitempty"`
	AlertID              *string              `json:"alertId,omitempty"`
	AlertValue           *string              `json:"alertValue,omitempty"`
	NotificationChannels []string             `json:"notificationChannels,omitempty"`
	IncidentEnabled      *bool                `json:"incidentEnabled,omitempty"`
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
	Incidents []incidentQueryItemPayload `json:"incidents"`
	Total     *int                       `json:"total,omitempty"`
	TookMs    *int                       `json:"tookMs,omitempty"`
}

type incidentQueryItemPayload struct {
	Timestamp                     *time.Time     `json:"timestamp,omitempty"`
	AlertID                       *string        `json:"alertId,omitempty"`
	IncidentID                    *string        `json:"incidentId,omitempty"`
	IncidentTriggerAiRca          *bool          `json:"incidentTriggerAiRca,omitempty"`
	IncidentTriggerAiCostAnalysis *bool          `json:"incidentTriggerAiCostAnalysis,omitempty"`
	Status                        *string        `json:"status,omitempty"`
	TriggeredAt                   *time.Time     `json:"triggeredAt,omitempty"`
	AcknowledgedAt                *time.Time     `json:"acknowledgedAt,omitempty"`
	ResolvedAt                    *time.Time     `json:"resolvedAt,omitempty"`
	Notes                         *string        `json:"notes,omitempty"`
	Description                   *string        `json:"description,omitempty"`
	Labels                        *labelsPayload `json:"labels,omitempty"`
}

type incidentPutResponsePayload struct {
	IncidentID                    *string        `json:"incidentId,omitempty"`
	AlertID                       *string        `json:"alertId,omitempty"`
	Status                        *string        `json:"status,omitempty"`
	TriggeredAt                   *time.Time     `json:"triggeredAt,omitempty"`
	AcknowledgedAt                *time.Time     `json:"acknowledgedAt,omitempty"`
	ResolvedAt                    *time.Time     `json:"resolvedAt,omitempty"`
	Notes                         *string        `json:"notes,omitempty"`
	Description                   *string        `json:"description,omitempty"`
	IncidentTriggerAiRca          *bool          `json:"incidentTriggerAiRca,omitempty"`
	IncidentTriggerAiCostAnalysis *bool          `json:"incidentTriggerAiCostAnalysis,omitempty"`
	Labels                        *labelsPayload `json:"labels,omitempty"`
}

func wrapScopeError(err error, resourceType, resourceName string) error {
	if errors.Is(err, ErrResourceNotFound) {
		return fmt.Errorf("%w: %s %q not found: %w", ErrAlertsResolveSearchScope, resourceType, resourceName, ErrScopeNotFound)
	}
	return fmt.Errorf("%w: failed to resolve %s %q: %w", ErrAlertsResolveSearchScope, resourceType, resourceName, ErrScopeResolutionFailed)
}
