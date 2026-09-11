// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	defaultLimit      = 100
	defaultSortOrder  = "desc"
	sortOrderAsc      = "asc"
	maxQueryTimeRange = 30 * 24 * time.Hour // 30 days

	sourceTypeLog    = "log"
	sourceTypeMetric = "metric"
	sourceTypeBudget = "budget"
)

// ValidateLogsQueryRequest validates the LogsQueryRequest
func ValidateLogsQueryRequest(req *types.LogsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}

	// Validate search scope
	if req.SearchScope == nil {
		return fmt.Errorf("searchScope is required")
	}

	// Exactly one of component or workflow must be set
	if req.SearchScope.Component == nil && req.SearchScope.Workflow == nil {
		return fmt.Errorf("searchScope must be either a ComponentSearchScope (with namespace, and optionally project/component/environment) or WorkflowSearchScope (with namespace, and optionally workflowRunName)")
	}
	if req.SearchScope.Component != nil && req.SearchScope.Workflow != nil {
		return fmt.Errorf("searchScope cannot be both ComponentSearchScope and WorkflowSearchScope")
	}

	// Validate component scope if present
	if req.SearchScope.Component != nil {
		if err := validateComponentScope(req.SearchScope.Component); err != nil {
			return err
		}
	}

	// Validate workflow scope if present
	if req.SearchScope.Workflow != nil {
		if err := validateWorkflowScope(req.SearchScope.Workflow); err != nil {
			return err
		}
	}

	// Validate time range
	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}

	// Validate and set defaults for limit
	if err := ValidateAndSetLimit(&req.Limit); err != nil {
		return err
	}

	// Validate and set defaults for sort order
	if err := ValidateAndSetSortOrder(&req.SortOrder); err != nil {
		return err
	}

	// Validate log levels if provided
	if err := ValidateLogLevels(req.LogLevels); err != nil {
		return err
	}

	return nil
}

// ValidateEventsQueryRequest validates the EventsQueryRequest.
func ValidateEventsQueryRequest(req *types.EventsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}

	// Validate search scope
	if req.SearchScope == nil {
		return fmt.Errorf("searchScope is required")
	}

	// Exactly one of component or workflow must be set
	if req.SearchScope.Component == nil && req.SearchScope.Workflow == nil {
		return fmt.Errorf("searchScope must be either a ComponentSearchScope (with namespace, and optionally project/component/environment) or WorkflowSearchScope (with namespace, and optionally workflowRunName)")
	}
	if req.SearchScope.Component != nil && req.SearchScope.Workflow != nil {
		return fmt.Errorf("searchScope cannot be both ComponentSearchScope and WorkflowSearchScope")
	}

	// Validate component scope if present
	if req.SearchScope.Component != nil {
		if err := validateComponentScope(req.SearchScope.Component); err != nil {
			return err
		}
	}

	// Validate workflow scope if present
	if req.SearchScope.Workflow != nil {
		if err := validateWorkflowScope(req.SearchScope.Workflow); err != nil {
			return err
		}
	}

	// Validate time range
	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}

	// Validate and set defaults for limit
	if err := ValidateAndSetLimit(&req.Limit); err != nil {
		return err
	}

	// Validate and set defaults for sort order
	if err := ValidateAndSetSortOrder(&req.SortOrder); err != nil {
		return err
	}

	return nil
}

// validateComponentScope validates the ComponentSearchScope
// Per OpenAPI spec, only namespace is required
func validateComponentScope(scope *types.ComponentSearchScope) error {
	// Trim whitespace so whitespace-only identifiers are rejected early
	scope.Namespace = strings.TrimSpace(scope.Namespace)
	scope.Project = strings.TrimSpace(scope.Project)
	scope.Component = strings.TrimSpace(scope.Component)
	scope.Environment = strings.TrimSpace(scope.Environment)
	if scope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	// project, component, and environment are optional per OpenAPI spec
	// but if component is provided, project is required
	if scope.Project == "" && scope.Component != "" {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}
	return nil
}

// validateWorkflowScope validates the WorkflowSearchScope
// Per OpenAPI spec, only namespace is required
func validateWorkflowScope(scope *types.WorkflowSearchScope) error {
	// Trim whitespace so whitespace-only identifiers are rejected early
	scope.Namespace = strings.TrimSpace(scope.Namespace)
	scope.WorkflowRunName = strings.TrimSpace(scope.WorkflowRunName)
	if scope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	// workflowRunName is optional per OpenAPI spec
	return nil
}

// ValidateTimeRange validates start and end time strings against the window cap
// the log-backed endpoints share.
func ValidateTimeRange(startTime, endTime string) error {
	return ValidateTimeRangeWithMax(startTime, endTime, maxQueryTimeRange)
}

// ValidateTimeRangeWithMax validates start and end time strings against a
// caller-supplied window cap.
func ValidateTimeRangeWithMax(startTime, endTime string, maxRange time.Duration) error {
	if startTime == "" {
		return fmt.Errorf("startTime is required")
	}
	if endTime == "" {
		return fmt.Errorf("endTime is required")
	}

	parsedStart, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return fmt.Errorf("startTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): %w", err)
	}

	parsedEnd, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		return fmt.Errorf("endTime must be in RFC3339 format (e.g., 2024-01-01T00:00:00Z): %w", err)
	}

	if parsedEnd.Before(parsedStart) {
		return fmt.Errorf("endTime must be after startTime")
	}

	if parsedEnd.Sub(parsedStart) > maxRange {
		return fmt.Errorf("query time range cannot exceed %d days", maxRange/24/time.Hour)
	}

	return nil
}

// ValidateAndSetLimit validates and sets default for limit
func ValidateAndSetLimit(limit *int) error {
	if *limit == 0 {
		*limit = defaultLimit
		return nil
	}
	if *limit < 0 {
		return fmt.Errorf("limit must be a positive integer")
	}
	if *limit > config.MaxLimit {
		return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
	}
	return nil
}

// Caps on the platform logs query string. A query string has length limits a request
// body would not, so an over-long value is rejected rather than truncated. These mirror
// the maxItems/maxLength declared on the PlatformLogs* parameters in the OpenAPI spec.
const (
	maxPlatformLogsFilterItems  = 20
	maxPlatformLogsValueLength  = 253
	maxPlatformLogsSelectorLen  = 256
	maxPlatformLogsSearchLength = 256
)

// ParseLabelSelector parses an equality-based Kubernetes label selector - the syntax
// `kubectl -l` accepts, where a comma means AND - into the pairs the adapter contract
// takes. Set-based operators are not supported; rejecting them is better than silently
// treating `key!=value` as a key named "key!".
func ParseLabelSelector(selector string) (map[string]string, error) {
	if selector == "" {
		return nil, nil
	}
	if len(selector) > maxPlatformLogsSelectorLen {
		return nil, fmt.Errorf("labels selector cannot exceed %d characters", maxPlatformLogsSelectorLen)
	}

	labels := make(map[string]string)
	for _, term := range strings.Split(selector, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key, value, found := strings.Cut(term, "=")
		if !found {
			return nil, fmt.Errorf("invalid label selector %q; expected key=value", term)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("invalid label selector %q; key must not be empty", term)
		}
		// "!=" and "==" both survive the Cut above with a stray character on one side.
		if strings.HasSuffix(key, "!") || strings.HasPrefix(value, "=") {
			return nil, fmt.Errorf(
				"invalid label selector %q; only equality selectors (key=value) are supported", term)
		}
		if existing, dup := labels[key]; dup && existing != value {
			return nil, fmt.Errorf("label %q is given conflicting values; a record cannot match both", key)
		}
		labels[key] = value
	}
	return labels, nil
}

// ValidatePlatformLogsQueryRequest validates the PlatformLogsQueryRequest and applies
// defaults for limit and sort order.
func ValidatePlatformLogsQueryRequest(req *types.PlatformLogsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}

	filters := map[string][]string{
		"clusterInstance": req.ClusterInstances,
		"namespace":       req.Namespaces,
		"podName":         req.PodNames,
		"containerName":   req.ContainerNames,
	}
	for name, values := range filters {
		if err := validatePlatformLogsFilter(name, values); err != nil {
			return err
		}
	}

	if len(req.SearchPhrase) > maxPlatformLogsSearchLength {
		return fmt.Errorf("searchPhrase cannot exceed %d characters", maxPlatformLogsSearchLength)
	}

	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}
	if err := ValidateLogLevels(req.LogLevels); err != nil {
		return err
	}
	if err := ValidateAndSetLimit(&req.Limit); err != nil {
		return err
	}
	return ValidateAndSetSortOrder(&req.SortOrder)
}

func validatePlatformLogsFilter(name string, values []string) error {
	if len(values) > maxPlatformLogsFilterItems {
		return fmt.Errorf("%s cannot have more than %d values", name, maxPlatformLogsFilterItems)
	}
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if len(v) > maxPlatformLogsValueLength {
			return fmt.Errorf("%s values cannot exceed %d characters", name, maxPlatformLogsValueLength)
		}
		if _, dup := seen[v]; dup {
			return fmt.Errorf("duplicate %s value %q is not allowed", name, v)
		}
		seen[v] = struct{}{}
	}
	return nil
}

const (
	maxAuditLogsFilterItems  = 20
	maxAuditLogsValueLength  = 512
	maxAuditLogsSearchLength = 256
	maxAuditLogsMaxValues    = 1000
	// Deliberately not defaultLimit, which is a record page size.
	defaultAuditLogsMaxValues = 100
	// Not maxQueryTimeRange: the audit trail has its own, longer retention, and
	// an annual compliance query is ordinary.
	maxAuditLogsTimeRange            = 366 * 24 * time.Hour
	auditLogsTimelineIntervalPattern = `^[1-9][0-9]*[mhdw]$`
)

var auditLogsTimelineInterval = regexp.MustCompile(auditLogsTimelineIntervalPattern)

// auditLogsFilterPaths are the filters QueryAuditLogFilterValues can list
// values for. Mirrors the `filter` enum in openapi/observer-api.yaml.
//
// event_id and request_id are absent deliberately: both are near-unique per
// record, so a list of them is not something a client picks from.
var auditLogsFilterPaths = map[string]bool{
	"actor.id":             true,
	"actor.type":           true,
	"actor.issuer":         true,
	"actor.session_id":     true,
	"actor.entitlements":   true,
	"resource.type":        true,
	"resource.namespace":   true,
	"resource.environment": true,
	"resource.project":     true,
	"resource.component":   true,
	"resource.name":        true,
	"action":               true,
	"category":             true,
	"result":               true,
	"producer":             true,
	"surface":              true,
	"operation_id":         true,
	"source_ip":            true,
	"user_agent":           true,
}

var (
	auditLogCategories = map[string]bool{"management": true, "authorization": true, "access": true}
	auditLogResults    = map[string]bool{
		"success": true, "failure": true, "denied": true, "unauthenticated": true,
	}
	auditLogSurfaces = map[string]bool{"rest": true, "mcp": true}
)

// ValidateAuditLogsQueryRequest validates the AuditLogsQueryRequest and applies
// defaults for limit and sort order.
func ValidateAuditLogsQueryRequest(req *types.AuditLogsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}

	filters := map[string][]string{
		"actor.id":             req.Actor.IDs,
		"actor.type":           req.Actor.Types,
		"actor.issuer":         req.Actor.Issuers,
		"actor.session_id":     req.Actor.SessionIDs,
		"actor.entitlements":   req.Actor.Entitlements,
		"resource.type":        req.Resource.Types,
		"resource.namespace":   req.Resource.Namespaces,
		"resource.environment": req.Resource.Environments,
		"resource.project":     req.Resource.Projects,
		"resource.component":   req.Resource.Components,
		"resource.name":        req.Resource.Names,
		"action":               req.Actions,
		"category":             req.Categories,
		"result":               req.Results,
		"producer":             req.Producers,
		"surface":              req.Surfaces,
		"operation_id":         req.OperationIDs,
		"request_id":           req.RequestIDs,
		"event_id":             req.EventIDs,
		"source_ip":            req.SourceIPs,
		"user_agent":           req.UserAgents,
	}
	for name, values := range filters {
		if err := validateAuditLogsFilter(name, values); err != nil {
			return err
		}
	}

	// Checked here because the generated types are string aliases: an unknown
	// value decodes cleanly and would match nothing.
	closed := []struct {
		name   string
		values []string
		valid  map[string]bool
	}{
		{"category", req.Categories, auditLogCategories},
		{"result", req.Results, auditLogResults},
		{"surface", req.Surfaces, auditLogSurfaces},
	}
	for _, c := range closed {
		for _, v := range c.values {
			if !c.valid[v] {
				return fmt.Errorf("invalid %s value %q", c.name, v)
			}
		}
	}

	if len(req.SearchPhrase) > maxAuditLogsSearchLength {
		return fmt.Errorf("searchPhrase cannot exceed %d characters", maxAuditLogsSearchLength)
	}
	if req.TimelineInterval != "" && !auditLogsTimelineInterval.MatchString(req.TimelineInterval) {
		return fmt.Errorf(
			"timelineInterval must be <count><unit> where unit is m, h, d or w (e.g. 15m)")
	}

	if err := ValidateTimeRangeWithMax(req.StartTime, req.EndTime, maxAuditLogsTimeRange); err != nil {
		return err
	}
	if err := ValidateAndSetLimit(&req.Limit); err != nil {
		return err
	}
	return ValidateAndSetSortOrder(&req.SortOrder)
}

// ValidateAuditLogFilterValuesRequest validates the request and applies the
// default for maxValues. The nested query is validated by the record query's
// rules; its limit and sort order are defaulted rather than rejected, since the
// contract ignores rather than forbids them.
func ValidateAuditLogFilterValuesRequest(req *types.AuditLogFilterValuesRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if !auditLogsFilterPaths[req.Filter] {
		return fmt.Errorf("invalid filter %q", req.Filter)
	}
	if len(req.ValueSearch) > maxAuditLogsSearchLength {
		return fmt.Errorf("valueSearch cannot exceed %d characters", maxAuditLogsSearchLength)
	}
	if req.MaxValues < 0 {
		return fmt.Errorf("maxValues must be a positive integer")
	}
	if req.MaxValues > maxAuditLogsMaxValues {
		return fmt.Errorf("maxValues cannot exceed %d", maxAuditLogsMaxValues)
	}
	if req.MaxValues == 0 {
		req.MaxValues = defaultAuditLogsMaxValues
	}
	return ValidateAuditLogsQueryRequest(&req.Query)
}

func validateAuditLogsFilter(name string, values []string) error {
	if len(values) > maxAuditLogsFilterItems {
		return fmt.Errorf("%s cannot have more than %d values", name, maxAuditLogsFilterItems)
	}
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if len(v) > maxAuditLogsValueLength {
			return fmt.Errorf("%s values cannot exceed %d characters", name, maxAuditLogsValueLength)
		}
		if _, dup := seen[v]; dup {
			return fmt.Errorf("duplicate %s value %q is not allowed", name, v)
		}
		seen[v] = struct{}{}
	}
	return nil
}

// ValidateAndSetSortOrder validates and sets default for sort order
func ValidateAndSetSortOrder(sortOrder *string) error {
	if *sortOrder == "" {
		*sortOrder = defaultSortOrder
		return nil
	}
	if *sortOrder != sortOrderAsc && *sortOrder != defaultSortOrder {
		return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
	}
	return nil
}

// ValidateMetricsQueryRequest validates the MetricsQueryRequest
func ValidateMetricsQueryRequest(req *types.MetricsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request must not be nil")
	}

	// Validate metric type
	if req.Metric == "" {
		return fmt.Errorf("metric is required")
	}
	if req.Metric != types.MetricTypeResource && req.Metric != types.MetricTypeHTTP {
		return fmt.Errorf("metric must be either %s or %s", types.MetricTypeResource, types.MetricTypeHTTP)
	}

	// Validate time range
	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}

	// Validate searchScope (required for metrics)
	if req.SearchScope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	if req.SearchScope.Component != "" && req.SearchScope.Project == "" {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}

	// Validate step format if provided
	if req.Step != nil && *req.Step != "" {
		step, err := time.ParseDuration(*req.Step)
		if err != nil {
			return fmt.Errorf("step must be a valid duration (e.g. 1m, 5m, 15m, 30m, 1h): %w", err)
		}
		if step <= 0 {
			return fmt.Errorf("step must be greater than 0")
		}
	}

	return nil
}

// ValidateRuntimeTopologyRequest validates the request body for
// POST /api/v1alpha1/metrics/runtime-topology. Runtime topology requires the
// project + environment to be specified explicitly; namespace alone is not
// enough to scope a cell diagram.
func ValidateRuntimeTopologyRequest(req *types.RuntimeTopologyRequest) error {
	if req == nil {
		return fmt.Errorf("request must not be nil")
	}

	scope := req.SearchScope
	if strings.TrimSpace(scope.Namespace) == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	if strings.TrimSpace(scope.Project) == "" {
		return fmt.Errorf("searchScope.project is required")
	}
	if strings.TrimSpace(scope.Environment) == "" {
		return fmt.Errorf("searchScope.environment is required")
	}

	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}

	return nil
}

// ValidateLogLevels validates the log levels array
func ValidateLogLevels(logLevels []string) error {
	validLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	seen := make(map[string]struct{}, len(logLevels))

	for _, level := range logLevels {
		if !validLevels[level] {
			return fmt.Errorf("invalid log level '%s'; valid levels are: DEBUG, INFO, WARN, ERROR", level)
		}
		if _, exists := seen[level]; exists {
			return fmt.Errorf("duplicate log level '%s' is not allowed", level)
		}
		seen[level] = struct{}{}
	}
	return nil
}

// validateAlertRuleRequest validates the new API AlertRuleRequest type.
func validateAlertRuleRequest(req internalgen.AlertRuleRequest) error {
	// Metadata validations
	if strings.TrimSpace(req.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if req.Metadata.ComponentUid == (openapi_types.UUID{}) {
		return fmt.Errorf("metadata.componentUid is required")
	}
	if req.Metadata.ProjectUid == (openapi_types.UUID{}) {
		return fmt.Errorf("metadata.projectUid is required")
	}
	if req.Metadata.EnvironmentUid == (openapi_types.UUID{}) {
		return fmt.Errorf("metadata.environmentUid is required")
	}

	// Source validations
	sourceType := string(req.Source.Type)
	if sourceType != sourceTypeLog && sourceType != sourceTypeMetric && sourceType != sourceTypeBudget {
		return fmt.Errorf("source.type must be 'log', 'metric', or 'budget'")
	}
	if sourceType == sourceTypeLog {
		if req.Source.Query == nil || strings.TrimSpace(*req.Source.Query) == "" {
			return fmt.Errorf("source.query is required for log-based alert rules")
		}
	}
	if sourceType == sourceTypeMetric {
		if req.Source.Metric == nil {
			return fmt.Errorf("source.metric is required for metric-based alert rules")
		}
		metric := string(*req.Source.Metric)
		if metric != "cpu_usage" && metric != "memory_usage" && metric != "budget" {
			return fmt.Errorf("source.metric must be 'cpu_usage', 'memory_usage' or 'budget' for metric-based alert rules")
		}
	}

	// Condition validations
	windowDuration, err := time.ParseDuration(req.Condition.Window)
	if err != nil {
		return fmt.Errorf("condition.window must be a valid duration (e.g., 5m): %w", err)
	}
	if windowDuration <= 0 {
		return fmt.Errorf("condition.window must be greater than zero")
	}

	intervalDuration, err := time.ParseDuration(req.Condition.Interval)
	if err != nil {
		return fmt.Errorf("condition.interval must be a valid duration (e.g., 1m): %w", err)
	}
	if intervalDuration <= 0 {
		return fmt.Errorf("condition.interval must be greater than zero")
	}
	if intervalDuration > windowDuration {
		return fmt.Errorf("condition.interval must not exceed condition.window")
	}

	allowedOps := []string{"gt", "gte", "lt", "lte", "eq", "neq"}
	if !slices.Contains(allowedOps, string(req.Condition.Operator)) {
		return fmt.Errorf("condition.operator must be one of %s", strings.Join(allowedOps, ", "))
	}

	if req.Condition.Threshold <= 0 {
		return fmt.Errorf("condition.threshold must be greater than zero")
	}

	return nil
}

// validateSourceType checks that the sourceType path parameter is a known value.
// Returns an error with a descriptive message for use in a 400 Bad Request response.
func validateSourceType(sourceType string) error {
	switch sourceType {
	case sourceTypeLog, sourceTypeMetric, sourceTypeBudget:
		return nil
	default:
		return fmt.Errorf("sourceType %q is invalid: must be 'log', 'metric', or 'budget'", sourceType)
	}
}

// ValidateTracesQueryRequest validates a traces query request
func ValidateTracesQueryRequest(req *gen.TracesQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}

	// Validate time range (generated types use time.Time directly)
	if req.StartTime.IsZero() {
		return fmt.Errorf("startTime is required")
	}
	if req.EndTime.IsZero() {
		return fmt.Errorf("endTime is required")
	}
	startStr := req.StartTime.Format(time.RFC3339)
	endStr := req.EndTime.Format(time.RFC3339)
	if err := ValidateTimeRange(startStr, endStr); err != nil {
		return err
	}

	// Validate search scope (required for traces)
	if req.SearchScope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	// Handle pointer fields in SearchScope
	componentVal := ""
	if req.SearchScope.Component != nil {
		componentVal = *req.SearchScope.Component
	}
	projectVal := ""
	if req.SearchScope.Project != nil {
		projectVal = *req.SearchScope.Project
	}
	if componentVal != "" && projectVal == "" {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}

	// Validate and set defaults for limit
	if req.Limit != nil {
		if *req.Limit <= 0 {
			return fmt.Errorf("limit must be a positive integer greater than zero")
		}
		if *req.Limit > config.MaxLimit {
			return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
		}
	}

	// Validate and set defaults for sort order
	if req.SortOrder != nil {
		validSort := string(*req.SortOrder)
		if validSort != sortOrderAsc && validSort != defaultSortOrder {
			return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
		}
	}

	return nil
}

// ValidateAlertsQueryRequest validates an alerts query request.
func ValidateAlertsQueryRequest(req *gen.AlertsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if req.StartTime.IsZero() {
		return fmt.Errorf("startTime is required")
	}
	if req.EndTime.IsZero() {
		return fmt.Errorf("endTime is required")
	}
	if err := ValidateTimeRange(req.StartTime.Format(time.RFC3339), req.EndTime.Format(time.RFC3339)); err != nil {
		return err
	}
	trimmedNamespace := strings.TrimSpace(req.SearchScope.Namespace)
	if trimmedNamespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	req.SearchScope.Namespace = trimmedNamespace
	if req.SearchScope.Component != nil && strings.TrimSpace(*req.SearchScope.Component) != "" &&
		(req.SearchScope.Project == nil || strings.TrimSpace(*req.SearchScope.Project) == "") {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}
	if req.Limit != nil {
		if *req.Limit <= 0 {
			return fmt.Errorf("limit must be a positive integer greater than zero")
		}
		if *req.Limit > config.MaxLimit {
			return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
		}
	}
	if req.SortOrder != nil {
		order := string(*req.SortOrder)
		if order != sortOrderAsc && order != defaultSortOrder {
			return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
		}
	}
	return nil
}

// ValidateIncidentsQueryRequest validates an incidents query request.
func ValidateIncidentsQueryRequest(req *gen.IncidentsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if req.StartTime.IsZero() {
		return fmt.Errorf("startTime is required")
	}
	if req.EndTime.IsZero() {
		return fmt.Errorf("endTime is required")
	}
	if err := ValidateTimeRange(req.StartTime.Format(time.RFC3339), req.EndTime.Format(time.RFC3339)); err != nil {
		return err
	}
	trimmedNamespace := strings.TrimSpace(req.SearchScope.Namespace)
	if trimmedNamespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	req.SearchScope.Namespace = trimmedNamespace
	if req.SearchScope.Component != nil && strings.TrimSpace(*req.SearchScope.Component) != "" &&
		(req.SearchScope.Project == nil || strings.TrimSpace(*req.SearchScope.Project) == "") {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}
	if req.Limit != nil {
		if *req.Limit <= 0 {
			return fmt.Errorf("limit must be a positive integer greater than zero")
		}
		if *req.Limit > config.MaxLimit {
			return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
		}
	}
	if req.SortOrder != nil {
		order := string(*req.SortOrder)
		if order != sortOrderAsc && order != defaultSortOrder {
			return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
		}
	}
	return nil
}

// ValidateIncidentPutRequest validates an incident update (PUT) request.
func ValidateIncidentPutRequest(req *gen.IncidentPutRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	status := strings.TrimSpace(string(req.Status))
	if status == "" {
		return fmt.Errorf("status is required")
	}
	switch status {
	case string(gen.Active),
		string(gen.Acknowledged),
		string(gen.Resolved):
		// valid
	default:
		return fmt.Errorf("status must be one of 'active', 'acknowledged', or 'resolved'")
	}
	return nil
}

// timeGranularityPattern matches the <count><unit> time granularity notation
// (e.g. 1h, 2d, 3w).
var timeGranularityPattern = regexp.MustCompile(`^[1-9][0-9]*[hdw]$`)

// ValidateCostQueryRequest validates the CostQueryRequest.
func ValidateCostQueryRequest(req *types.CostQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if strings.TrimSpace(req.Namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if strings.TrimSpace(req.Environment) == "" {
		return fmt.Errorf("environment is required")
	}
	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}
	if strings.TrimSpace(req.Component) != "" && strings.TrimSpace(req.Project) == "" {
		return fmt.Errorf("component requires project to also be set")
	}
	if req.Granularity != "" && !timeGranularityPattern.MatchString(req.Granularity) {
		return fmt.Errorf("granularity must match <count><unit> notation (e.g. 1h, 2d, 3w)")
	}
	return nil
}

// ValidateRecommendationQueryRequest validates the RecommendationQueryRequest.
func ValidateRecommendationQueryRequest(req *types.RecommendationQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if strings.TrimSpace(req.Namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if strings.TrimSpace(req.Environment) == "" {
		return fmt.Errorf("environment is required")
	}
	if err := ValidateTimeRange(req.StartTime, req.EndTime); err != nil {
		return err
	}
	if strings.TrimSpace(req.Component) != "" && strings.TrimSpace(req.Project) == "" {
		return fmt.Errorf("component requires project to also be set")
	}
	return nil
}
