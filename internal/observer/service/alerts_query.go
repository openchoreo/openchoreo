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

	items := make([]gen.Alert, 0, len(entries))
	for _, entry := range entries {
		items = append(items, s.buildAlertQueryItem(entry))
	}

	return &gen.AlertsQueryResponse{
		Alerts: &items,
		Total:  intPtr(total),
		TookMs: intPtr(int(time.Since(start).Milliseconds())),
	}, nil
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

	items := make([]gen.Incident, 0, len(entries))
	for _, entry := range entries {
		items = append(items, gen.Incident{
			Timestamp:                     parseTimePtr(entry.Timestamp),
			AlertId:                       stringPtr(strings.TrimSpace(entry.AlertID)),
			IncidentId:                    stringPtr(strings.TrimSpace(entry.ID)),
			IncidentTriggerAiRca:          boolPtr(entry.TriggerAiRca),
			IncidentTriggerAiCostAnalysis: boolPtr(entry.TriggerAiCostAnalysis),
			Status:                        enumPtr[gen.IncidentStatus](entry.Status),
			TriggeredAt:                   parseTimePtr(entry.TriggeredAt),
			AcknowledgedAt:                parseTimePtr(entry.AcknowledgedAt),
			ResolvedAt:                    parseTimePtr(entry.ResolvedAt),
			Notes:                         stringPtr(strings.TrimSpace(entry.Notes)),
			Description:                   stringPtr(strings.TrimSpace(entry.Description)),
			Labels: buildResourceLabels(
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

	return &gen.IncidentsQueryResponse{
		Incidents: &items,
		Total:     intPtr(total),
		TookMs:    intPtr(int(time.Since(start).Milliseconds())),
	}, nil
}

// IncidentScope reads the incident's namespace/project/component so an
// authorization check can be made against its real hierarchy. See
// IncidentsUpdater.
func (s *AlertService) IncidentScope(ctx context.Context, id string) (string, string, string, error) {
	if s.incidentEntryStore == nil {
		return "", "", "", fmt.Errorf("incident entry store is not initialized")
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", "", fmt.Errorf("incident id is required")
	}

	entry, err := s.incidentEntryStore.GetIncidentEntry(ctx, id)
	if err != nil {
		return "", "", "", err
	}
	return strings.TrimSpace(entry.NamespaceName),
		strings.TrimSpace(entry.ProjectName),
		strings.TrimSpace(entry.ComponentName),
		nil
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

	return &gen.IncidentPutResponse{
		IncidentId:                    stringPtr(strings.TrimSpace(entry.ID)),
		AlertId:                       stringPtr(strings.TrimSpace(entry.AlertID)),
		Status:                        enumPtr[gen.IncidentStatus](entry.Status),
		TriggeredAt:                   parseTimePtr(entry.TriggeredAt),
		AcknowledgedAt:                parseTimePtr(entry.AcknowledgedAt),
		ResolvedAt:                    parseTimePtr(entry.ResolvedAt),
		Notes:                         stringPtr(strings.TrimSpace(entry.Notes)),
		Description:                   stringPtr(strings.TrimSpace(entry.Description)),
		IncidentTriggerAiRca:          boolPtr(entry.TriggerAiRca),
		IncidentTriggerAiCostAnalysis: boolPtr(entry.TriggerAiCostAnalysis),
		Labels: buildResourceLabels(
			entry.NamespaceName,
			entry.ProjectName,
			entry.ComponentName,
			entry.EnvironmentName,
			entry.ProjectID,
			entry.ComponentID,
			entry.EnvironmentID,
		),
	}, nil
}

func (s *AlertService) buildAlertQueryItem(entry alertentry.AlertEntry) gen.Alert {
	// Metadata is always non-nil: the field carries `omitempty`, so a nil pointer
	// would drop the `metadata` key from the response rather than emit it empty.
	item := gen.Alert{
		Timestamp:       parseTimePtr(entry.Timestamp),
		AlertId:         stringPtr(strings.TrimSpace(entry.ID)),
		AlertValue:      stringPtr(strings.TrimSpace(entry.AlertValue)),
		IncidentEnabled: boolPtr(entry.IncidentEnabled),
		Metadata: &gen.AlertMetadata{
			Labels: buildResourceLabels(
				entry.NamespaceName,
				entry.ProjectName,
				entry.ComponentName,
				entry.EnvironmentName,
				entry.ProjectID,
				entry.ComponentID,
				entry.EnvironmentID,
			),
			AlertRule: &gen.AlertRule{Name: stringPtr(strings.TrimSpace(entry.AlertRuleName))},
		},
		NotificationChannels: notificationChannelsOrNil(entry.NotificationChannels),
	}

	if strings.TrimSpace(entry.Severity) != "" || strings.TrimSpace(entry.Description) != "" ||
		strings.TrimSpace(entry.SourceType) != "" || strings.TrimSpace(entry.SourceQuery) != "" ||
		strings.TrimSpace(entry.SourceMetric) != "" || strings.TrimSpace(entry.ConditionOperator) != "" ||
		entry.ConditionThreshold != 0 || strings.TrimSpace(entry.ConditionWindow) != "" ||
		strings.TrimSpace(entry.ConditionInterval) != "" {
		item.Metadata.AlertRule = &gen.AlertRule{
			Name:        stringPtr(strings.TrimSpace(entry.AlertRuleName)),
			Description: stringPtr(strings.TrimSpace(entry.Description)),
			Severity:    enumPtr[gen.AlertRuleSeverity](entry.Severity),
			Source: &gen.AlertRuleSource{
				Type:   enumPtr[gen.AlertRuleSourceType](entry.SourceType),
				Query:  stringPtr(strings.TrimSpace(entry.SourceQuery)),
				Metric: stringPtr(strings.TrimSpace(entry.SourceMetric)),
			},
			Condition: &gen.AlertRuleCondition{
				Operator:  enumPtr[gen.AlertRuleConditionOperator](entry.ConditionOperator),
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

// notificationChannelsOrNil returns nil rather than an empty slice, so the
// field stays absent from the response.
//
// `omitempty` on a *[]string only tests the pointer, so a non-nil pointer to an
// empty slice would emit `"notificationChannels":[]` for every alert that failed
// to notify.
func notificationChannelsOrNil(raw string) *[]string {
	channels := parseNotificationChannelsJSON(raw)
	if len(channels) == 0 {
		return nil
	}
	return &channels
}

func buildResourceLabels(
	namespace, project, component, environment string,
	projectUID, componentUID, environmentUID string,
) *gen.ResourceLabels {
	return &gen.ResourceLabels{
		NamespaceName:   stringPtr(strings.TrimSpace(namespace)),
		ProjectName:     stringPtr(strings.TrimSpace(project)),
		ComponentName:   stringPtr(strings.TrimSpace(component)),
		EnvironmentName: stringPtr(strings.TrimSpace(environment)),
		ProjectUid:      uuidPtr(projectUID),
		ComponentUid:    uuidPtr(componentUID),
		EnvironmentUid:  uuidPtr(environmentUID),
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

// uuidPtr parses value into a UUID, returning nil when it is blank or not a
// valid UUID.
//
// Dropping an unparseable UID rather than erroring is deliberate: the alert and
// incident stores are the source of truth for these values, and a malformed one
// should not fail the whole query. The generated field is a *uuid.UUID, so
// without the nil the zero UUID would surface on the wire as
// "00000000-0000-0000-0000-000000000000".
func uuidPtr(value string) *uuid.UUID {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return nil
	}
	return &parsed
}

// enumPtr converts a store string into one of the generated enum types,
// mapping blank to nil the way stringPtr does.
//
// The value is not checked against the enum's permitted set. The stores hold
// values written by the alerting pipeline, and rejecting one here would turn a
// data problem into a failed query.
func enumPtr[T ~string](value string) *T {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	converted := T(trimmed)
	return &converted
}

func wrapScopeError(err error, resourceType, resourceName string) error {
	if errors.Is(err, ErrResourceNotFound) {
		return fmt.Errorf("%w: %s %q not found: %w", ErrAlertsResolveSearchScope, resourceType, resourceName, ErrScopeNotFound)
	}
	return fmt.Errorf("%w: failed to resolve %s %q: %w", ErrAlertsResolveSearchScope, resourceType, resourceName, ErrScopeResolutionFailed)
}
