// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/labels"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// QueryBuilder provides methods to build OpenSearch queries
type QueryBuilder struct {
	indexPrefix string
}

// NewQueryBuilder creates a new query builder with the given index prefix
func NewQueryBuilder(indexPrefix string) *QueryBuilder {
	return &QueryBuilder{
		indexPrefix: indexPrefix,
	}
}

// formatDurationForOpenSearch normalizes durations so OpenSearch monitors accept them.
// Handles hours/minutes/seconds cleanly (e.g., "1h0m0s" -> "1h", "5m0s" -> "5m").
func formatDurationForOpenSearch(d string) (string, error) {
	parsed, err := time.ParseDuration(d)
	if err != nil {
		return "", err
	}

	switch {
	case parsed%time.Hour == 0:
		return fmt.Sprintf("%dh", parsed/time.Hour), nil
	case parsed%time.Minute == 0:
		return fmt.Sprintf("%dm", parsed/time.Minute), nil
	case parsed%time.Second == 0:
		return fmt.Sprintf("%ds", parsed/time.Second), nil
	}
	return parsed.String(), nil
}

// addTimeRangeFilter adds time range filter to must conditions
func addTimeRangeFilter(mustConditions []map[string]interface{}, startTime, endTime string) []map[string]interface{} {
	if startTime != "" && endTime != "" {
		timeFilter := map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gt": startTime,
					"lt": endTime,
				},
			},
		}
		mustConditions = append(mustConditions, timeFilter)
	}
	return mustConditions
}

// addSearchPhraseFilter adds wildcard search phrase filter to must conditions
func addSearchPhraseFilter(mustConditions []map[string]interface{}, searchPhrase string) []map[string]interface{} {
	if searchPhrase != "" {
		searchFilter := map[string]interface{}{
			"wildcard": map[string]interface{}{
				"log": fmt.Sprintf("*%s*", searchPhrase),
			},
		}
		mustConditions = append(mustConditions, searchFilter)
	}
	return mustConditions
}

// addLogLevelFilter adds log level filter to must conditions
func addLogLevelFilter(mustConditions []map[string]interface{}, logLevels []string) []map[string]interface{} {
	if len(logLevels) > 0 {
		shouldConditions := []map[string]interface{}{}

		for _, logLevel := range logLevels {
			// Use match query to find log level in the log content
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"match": map[string]interface{}{
					"log": strings.ToUpper(logLevel),
				},
			})
		}

		if len(shouldConditions) > 0 {
			logLevelFilter := map[string]interface{}{
				"bool": map[string]interface{}{
					"should":               shouldConditions,
					"minimum_should_match": 1,
				},
			}
			mustConditions = append(mustConditions, logLevelFilter)
		}
	}
	return mustConditions
}

// BuildBuildLogsQuery builds a query for build logs with wildcard search
func (qb *QueryBuilder) BuildBuildLogsQuery(params BuildQueryParams) map[string]interface{} {
	mustConditions := []map[string]interface{}{
		{
			"wildcard": map[string]interface{}{
				labels.KubernetesPodName + ".keyword": params.BuildID + "*",
			},
		},
	}
	mustConditions = addTimeRangeFilter(mustConditions, params.QueryParams.StartTime, params.QueryParams.EndTime)

	// Logs from init and wait containers are not relevant to the build logs. Hence, excluded.
	mustNotConditions := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				labels.KubernetesContainerName + ".keyword": "init",
			},
		},
		{
			"term": map[string]interface{}{
				labels.KubernetesContainerName + ".keyword": "wait",
			},
		},
	}

	query := map[string]interface{}{
		"size": params.QueryParams.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":     mustConditions,
				"must_not": mustNotConditions,
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.QueryParams.SortOrder,
				},
			},
		},
	}
	return query
}

// BuildWorkflowRunLogsQuery builds a query for workflow run logs with wildcard search
func (qb *QueryBuilder) BuildWorkflowRunLogsQuery(params WorkflowRunQueryParams) map[string]interface{} {
	mustConditions := []map[string]interface{}{
		{
			"wildcard": map[string]interface{}{
				labels.KubernetesPodName + ".keyword": params.WorkflowRunID + "*",
			},
		},
	}
	mustConditions = addTimeRangeFilter(mustConditions, params.QueryParams.StartTime, params.QueryParams.EndTime)

	// Add namespace filter if specified
	// Use kubernetes.namespace_name (actual K8s namespace) instead of the pod label
	// since generic workflows don't have the namespace-name label on their pods
	// The actual K8s namespace for workflows is "openchoreo-ci-<namespaceName>"
	if params.QueryParams.NamespaceName != "" {
		k8sNamespace := fmt.Sprintf("openchoreo-ci-%s", params.QueryParams.NamespaceName)
		namespaceFilter := map[string]interface{}{
			"term": map[string]interface{}{
				"kubernetes.namespace_name.keyword": k8sNamespace,
			},
		}
		mustConditions = append(mustConditions, namespaceFilter)
	}

	// Logs from init and wait containers are not relevant to workflow run logs. Hence, excluded.
	mustNotConditions := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				labels.KubernetesContainerName + ".keyword": "init",
			},
		},
		{
			"term": map[string]interface{}{
				labels.KubernetesContainerName + ".keyword": "wait",
			},
		},
	}

	query := map[string]interface{}{
		"size": params.QueryParams.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":     mustConditions,
				"must_not": mustNotConditions,
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.QueryParams.SortOrder,
				},
			},
		},
	}
	return query
}

// BuildComponentWorkflowRunLogsQuery builds a query for component workflow run logs
func (qb *QueryBuilder) BuildComponentWorkflowRunLogsQuery(params ComponentWorkflowRunQueryParams) map[string]interface{} {
	// Construct pod name pattern for Argo Workflow: runName-stepName-* or runName-* if stepName is empty
	var podNamePattern string
	if params.StepName != "" {
		podNamePattern = fmt.Sprintf("%s-%s-*", params.RunName, params.StepName)
	} else {
		podNamePattern = params.RunName + "-*"
	}

	// Build query with wildcard search on pod name
	mustConditions := []map[string]interface{}{
		{
			"wildcard": map[string]interface{}{
				labels.KubernetesPodName + ".keyword": podNamePattern,
			},
		},
	}

	// Logs from init and wait containers are not relevant. Hence, excluded.
	mustNotConditions := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				labels.KubernetesContainerName + ".keyword": "init",
			},
		},
		{
			"term": map[string]interface{}{
				labels.KubernetesContainerName + ".keyword": "wait",
			},
		},
	}

	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":     mustConditions,
				"must_not": mustNotConditions,
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": "asc",
				},
			},
		},
	}
	return query
}

// BuildComponentLogsQuery builds a query for component logs with wildcard search
func (qb *QueryBuilder) BuildComponentLogsQuery(params ComponentQueryParams) map[string]interface{} {
	mustConditions := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				labels.OSComponentID + ".keyword": params.ComponentID,
			},
		},
	}

	// Add environment filter only for RUNTIME logs, not for BUILD logs
	if params.LogType != labels.QueryParamLogTypeBuild {
		environmentFilter := map[string]interface{}{
			"term": map[string]interface{}{
				labels.OSEnvironmentID + ".keyword": params.EnvironmentID,
			},
		}
		mustConditions = append(mustConditions, environmentFilter)
	}

	// Add namespace filter only if specified
	if params.Namespace != "" {
		namespaceFilter := map[string]interface{}{
			"term": map[string]interface{}{
				"kubernetes.namespace_name.keyword": params.Namespace,
			},
		}
		mustConditions = append(mustConditions, namespaceFilter)
	}

	// Add type-specific filters based on LogType
	if params.LogType == labels.QueryParamLogTypeBuild {
		// For BUILD logs, add target filter to identify build logs
		targetFilter := map[string]interface{}{
			"term": map[string]interface{}{
				labels.OSTarget + ".keyword": labels.TargetBuild,
			},
		}
		mustConditions = append(mustConditions, targetFilter)

		// For BUILD logs, add BuildID and BuildUUID filters instead of date filter
		if params.BuildID != "" {
			buildIDFilter := map[string]interface{}{
				"term": map[string]interface{}{
					labels.OSBuildID + ".keyword": params.BuildID,
				},
			}
			mustConditions = append(mustConditions, buildIDFilter)
		}

		if params.BuildUUID != "" {
			buildUUIDFilter := map[string]interface{}{
				"term": map[string]interface{}{
					labels.OSBuildUUID + ".keyword": params.BuildUUID,
				},
			}
			mustConditions = append(mustConditions, buildUUIDFilter)
		}

		// Skip date filter for BUILD logs
	} else {
		// For RUNTIME logs, use the existing behavior with date filter
		mustConditions = addTimeRangeFilter(mustConditions, params.StartTime, params.EndTime)
	}

	// Add common filters for both types
	mustConditions = addSearchPhraseFilter(mustConditions, params.SearchPhrase)
	mustConditions = addLogLevelFilter(mustConditions, params.LogLevels)

	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustConditions,
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	// Add version filters as "should" conditions
	if len(params.Versions) > 0 || len(params.VersionIDs) > 0 {
		shouldConditions := []map[string]interface{}{}

		for _, version := range params.Versions {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"term": map[string]interface{}{
					labels.OSVersion + ".keyword": version,
				},
			})
		}

		for _, versionID := range params.VersionIDs {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"term": map[string]interface{}{
					labels.OSVersionID + ".keyword": versionID,
				},
			})
		}

		if len(shouldConditions) > 0 {
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"] = shouldConditions
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["minimum_should_match"] = 1
		}
	}

	return query
}

// BuildProjectLogsQuery builds a query for project logs with wildcard search
func (qb *QueryBuilder) BuildProjectLogsQuery(params QueryParams, componentIDs []string) map[string]interface{} {
	mustConditions := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				labels.OSProjectID + ".keyword": params.ProjectID,
			},
		},
		{
			"term": map[string]interface{}{
				labels.OSEnvironmentID + ".keyword": params.EnvironmentID,
			},
		},
	}

	// Add common filters
	mustConditions = addTimeRangeFilter(mustConditions, params.StartTime, params.EndTime)
	mustConditions = addSearchPhraseFilter(mustConditions, params.SearchPhrase)
	mustConditions = addLogLevelFilter(mustConditions, params.LogLevels)

	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustConditions,
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	// Add component ID filters as "should" conditions
	if len(componentIDs) > 0 {
		shouldConditions := []map[string]interface{}{}

		for _, componentID := range componentIDs {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"term": map[string]interface{}{
					labels.OSComponentID + ".keyword": componentID,
				},
			})
		}

		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"] = shouldConditions
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["minimum_should_match"] = 1
	}

	return query
}

// BuildGatewayLogsQuery builds a query for gateway logs with wildcard search
func (qb *QueryBuilder) BuildGatewayLogsQuery(params GatewayQueryParams) map[string]interface{} {
	mustConditions := []map[string]interface{}{}

	// Add common filters
	mustConditions = addTimeRangeFilter(mustConditions, params.StartTime, params.EndTime)
	mustConditions = addSearchPhraseFilter(mustConditions, params.SearchPhrase)

	// Add namespace path filter
	if params.NamespaceName != "" {
		namespaceFilter := map[string]interface{}{
			"wildcard": map[string]interface{}{
				"log": fmt.Sprintf("*\"apiPath\":\"/%s*", params.NamespaceName),
			},
		}
		mustConditions = append(mustConditions, namespaceFilter)
	}

	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustConditions,
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	// Add gateway vhost filters
	if len(params.GatewayVHosts) > 0 {
		shouldConditions := []map[string]interface{}{}

		for _, vhost := range params.GatewayVHosts {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"wildcard": map[string]interface{}{
					"log": fmt.Sprintf("*\"gwHost\":%q*", vhost),
				},
			})
		}

		if len(shouldConditions) > 0 {
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"] = shouldConditions
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["minimum_should_match"] = 1
		}
	}

	// Add API ID filters
	if len(params.APIIDToVersionMap) > 0 {
		apiShouldConditions := []map[string]interface{}{}

		for apiID := range params.APIIDToVersionMap {
			apiShouldConditions = append(apiShouldConditions, map[string]interface{}{
				"wildcard": map[string]interface{}{
					"log": fmt.Sprintf("*\"apiUuid\":%q*", apiID),
				},
			})
		}

		if len(apiShouldConditions) > 0 {
			// Combine with existing should conditions using nested bool
			if existing := query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"]; existing != nil {
				// Create a nested bool query to combine both should conditions
				nestedBool := map[string]interface{}{
					"bool": map[string]interface{}{
						"should": []map[string]interface{}{
							{
								"bool": map[string]interface{}{
									"should":               existing,
									"minimum_should_match": 1,
								},
							},
							{
								"bool": map[string]interface{}{
									"should":               apiShouldConditions,
									"minimum_should_match": 1,
								},
							},
						},
						"minimum_should_match": 2, // Both conditions must match
					},
				}
				mustConditions = append(mustConditions, nestedBool)
				query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = mustConditions
				delete(query["query"].(map[string]interface{})["bool"].(map[string]interface{}), "should")
			} else {
				query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"] = apiShouldConditions
				query["query"].(map[string]interface{})["bool"].(map[string]interface{})["minimum_should_match"] = 1
			}
		}
	}

	return query
}

// GenerateIndices generates the list of indices to search based on time range
func (qb *QueryBuilder) GenerateIndices(startTime, endTime string) ([]string, error) {
	if startTime == "" || endTime == "" {
		return []string{qb.indexPrefix + "*"}, nil
	}

	start, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start time format: %w", err)
	}

	end, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end time format: %w", err)
	}

	indices := []string{}
	current := start

	for current.Before(end) || current.Equal(end) {
		indexName := qb.indexPrefix + current.Format("2006-01-02")
		indices = append(indices, indexName)
		current = current.AddDate(0, 0, 1) // Add 1 day
	}

	// Handle edge case where end date might need its own index
	endIndexName := qb.indexPrefix + end.Format("2006-01-02")
	if !contains(indices, endIndexName) {
		indices = append(indices, endIndexName)
	}

	return indices, nil
}

// BuildNamespaceLogsQuery builds a query for namespace logs with wildcard search
func (qb *QueryBuilder) BuildNamespaceLogsQuery(params QueryParams, podLabels map[string]string) map[string]interface{} {
	mustConditions := []map[string]interface{}{}

	// Add namespace filter - this is the key fix!
	if params.NamespaceName != "" {
		namespaceFilter := map[string]interface{}{
			"term": map[string]interface{}{
				labels.OSNamespaceName + ".keyword": params.NamespaceName,
			},
		}
		mustConditions = append(mustConditions, namespaceFilter)
	}

	// Add environment filter if specified
	if params.EnvironmentID != "" {
		envFilter := map[string]interface{}{
			"term": map[string]interface{}{
				labels.OSEnvironmentID + ".keyword": params.EnvironmentID,
			},
		}
		mustConditions = append(mustConditions, envFilter)
	}

	// Add namespace filter if specified
	if params.Namespace != "" {
		namespaceFilter := map[string]interface{}{
			"term": map[string]interface{}{
				"kubernetes.namespace_name.keyword": params.Namespace,
			},
		}
		mustConditions = append(mustConditions, namespaceFilter)
	}

	// Add common filters
	mustConditions = addTimeRangeFilter(mustConditions, params.StartTime, params.EndTime)
	mustConditions = addSearchPhraseFilter(mustConditions, params.SearchPhrase)
	mustConditions = addLogLevelFilter(mustConditions, params.LogLevels)

	// Add pod labels filters
	for key, value := range podLabels {
		labelFilter := map[string]interface{}{
			"term": map[string]interface{}{
				fmt.Sprintf("kubernetes.labels.%s.keyword", key): value,
			},
		}
		mustConditions = append(mustConditions, labelFilter)
	}

	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustConditions,
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	return query
}

func (qb *QueryBuilder) BuildTracesQuery(params TracesRequestParams) map[string]interface{} {
	filterConditions := []map[string]interface{}{
		{
			"range": map[string]interface{}{
				"startTime": map[string]interface{}{
					"gte": params.StartTime,
				},
			},
		},
		{
			"range": map[string]interface{}{
				"endTime": map[string]interface{}{
					"lte": params.EndTime,
				},
			},
		},
	}

	// Add TraceID filter if present
	if params.TraceID != "" {
		filterConditions = append(filterConditions, map[string]interface{}{
			"wildcard": map[string]interface{}{
				"traceId": params.TraceID,
			},
		})
	}

	// Add ComponentUIDs filter if present
	if len(params.ComponentUIDs) > 0 {
		shouldConditions := []map[string]interface{}{}
		for _, componentUID := range params.ComponentUIDs {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"term": map[string]interface{}{
					"resource.openchoreo.dev/component-uid": componentUID,
				},
			})
		}
		filterConditions = append(filterConditions, map[string]interface{}{
			"bool": map[string]interface{}{
				"should": shouldConditions,
			},
		})
	}

	// Add EnvironmentUID filter if present
	if params.EnvironmentUID != "" {
		filterConditions = append(filterConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/environment-uid": params.EnvironmentUID,
			},
		})
	}

	if params.ProjectUID != "" {
		filterConditions = append(filterConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/project-uid": params.ProjectUID,
			},
		})
	}

	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": filterConditions,
			},
		},
		"sort": []map[string]interface{}{
			{
				"startTime": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	return query
}

// CheckQueryVersion determines if the index supports V2 wildcard queries
func (qb *QueryBuilder) CheckQueryVersion(mapping *MappingResponse, indexName string) string {
	for name, indexMapping := range mapping.Mappings {
		if strings.Contains(name, indexName) || strings.Contains(indexName, name) {
			if logField, exists := indexMapping.Mappings.Properties["log"]; exists {
				if logField.Type == "wildcard" {
					return "v2"
				}
			}
		}
	}
	return "v1"
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (qb *QueryBuilder) BuildLogAlertingRuleQuery(params types.AlertingRuleRequest) (map[string]interface{}, error) {
	window, err := formatDurationForOpenSearch(params.Condition.Window)
	if err != nil {
		return nil, fmt.Errorf("failed to format window duration: %w", err)
	}
	filterConditions := []map[string]interface{}{
		{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"from":          "{{period_end}}||-" + window,
					"to":            "{{period_end}}",
					"format":        "epoch_millis",
					"include_lower": true,
					"include_upper": true,
					"boost":         1,
				},
			},
		},
		{
			"term": map[string]interface{}{
				labels.OSComponentID + ".keyword": map[string]interface{}{
					"value": params.Metadata.ComponentUID,
					"boost": 1,
				},
			},
		},
		{
			"term": map[string]interface{}{
				labels.OSEnvironmentID + ".keyword": map[string]interface{}{
					"value": params.Metadata.EnvironmentUID,
					"boost": 1,
				},
			},
		},
		{
			"term": map[string]interface{}{
				labels.OSProjectID + ".keyword": map[string]interface{}{
					"value": params.Metadata.ProjectUID,
					"boost": 1,
				},
			},
		},
		{
			"wildcard": map[string]interface{}{
				"log": map[string]interface{}{
					"wildcard": fmt.Sprintf("*%s*", params.Source.Query),
					"boost":    1,
				},
			},
		},
	}

	query := map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter":               filterConditions,
				"adjust_pure_negative": true,
				"boost":                1,
			},
		},
	}
	return query, nil
}

func (qb *QueryBuilder) BuildLogAlertingRuleMonitorBody(params types.AlertingRuleRequest) (map[string]interface{}, error) {
	intervalDuration, err := time.ParseDuration(params.Condition.Interval)
	if err != nil {
		return nil, fmt.Errorf("invalid interval format: %w", err)
	}

	query, err := qb.BuildLogAlertingRuleQuery(params)
	if err != nil {
		return nil, fmt.Errorf("failed to build log alerting rule query: %w", err)
	}

	monitorBody := MonitorBody{
		Type:        "monitor",
		MonitorType: "query_level_monitor",
		Name:        params.Metadata.Name,
		Enabled:     params.Condition.Enabled,
		Schedule: MonitorSchedule{
			Period: MonitorSchedulePeriod{
				Interval: intervalDuration.Minutes(),
				Unit:     "MINUTES",
			},
		},
		Inputs: []MonitorInput{
			{
				Search: MonitorInputSearch{
					Indices: []string{qb.indexPrefix + "*"},
					Query:   query,
				},
			},
		},
		Triggers: []MonitorTrigger{
			{
				QueryLevelTrigger: &MonitorTriggerQueryLevelTrigger{
					Name:     "trigger-" + params.Metadata.Name,
					Severity: "1",
					Condition: MonitorTriggerCondition{
						Script: MonitorTriggerConditionScript{
							Source: fmt.Sprintf("ctx.results[0].hits.total.value %s %s", GetOperatorSymbol(params.Condition.Operator), strconv.FormatFloat(params.Condition.Threshold, 'f', -1, 64)),
							Lang:   "painless",
						},
					},
					Actions: []MonitorTriggerAction{
						{
							Name:          "action-" + params.Metadata.Name,
							DestinationID: "openchoreo-observer-alerting-webhook",
							MessageTemplate: MonitorMessageTemplate{
								Source: buildWebhookMessageTemplate(params),
								Lang:   "mustache",
							},
							ThrottleEnabled: true,
							Throttle: MonitorTriggerActionThrottle{
								Value: 60, // TODO: Make throttle value configurable in future
								Unit:  "MINUTES",
							},
							SubjectTemplate: MonitorMessageTemplate{
								Source: "TheSubject", // TODO: Add appropriate subject template
								Lang:   "mustache",
							},
							ActionExecutionPolicy: MonitorTriggerActionExecutionPolicy{
								ActionExecutionScope: MonitorTriggerActionExecutionScope{
									PerAlert: MonitorActionExecutionScopePerAlert{
										ActionableAlerts: []string{"DEDUPED", "NEW"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Convert to map[string]interface{} for compatibility with existing code
	bodyBytes, err := json.Marshal(monitorBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal monitor body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal monitor body: %w", err)
	}

	return result, nil
}

func GetOperatorSymbol(operator string) string {
	switch operator {
	case "gt":
		return ">"
	case "gte":
		return ">="
	case "lt":
		return "<"
	case "lte":
		return "<="
	}
	return ""
}

// buildWebhookMessageTemplate builds a JSON message template for webhook notifications
// It includes all metadata and alert context that will be available when the alert fires
func buildWebhookMessageTemplate(params types.AlertingRuleRequest) string {
	// Escape JSON strings properly
	ruleName, _ := json.Marshal(params.Metadata.Name)
	ruleNamespace, _ := json.Marshal(params.Metadata.Namespace)
	componentUID, _ := json.Marshal(params.Metadata.ComponentUID)
	projectUID, _ := json.Marshal(params.Metadata.ProjectUID)
	environmentUID, _ := json.Marshal(params.Metadata.EnvironmentUID)
	notificationChannel, _ := json.Marshal(params.Metadata.NotificationChannel)
	enableAiRootCauseAnalysis, _ := json.Marshal(params.Metadata.EnableAiRootCauseAnalysis)

	// Build the JSON template with Mustache variables
	return fmt.Sprintf(
		`{"ruleName":%s,"ruleNamespace":%s,"componentUid":%s,"projectUid":%s,"environmentUid":%s,"notificationChannel":%s,"enableAiRootCauseAnalysis":%s,"alertValue":{{ctx.results.0.hits.total.value}},"timestamp":"{{ctx.periodStart}}"}`,
		string(ruleName),
		string(ruleNamespace),
		string(componentUID),
		string(projectUID),
		string(environmentUID),
		string(notificationChannel),
		string(enableAiRootCauseAnalysis),
	)
}
