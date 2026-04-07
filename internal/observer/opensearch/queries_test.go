// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/labels"
)

const sortOrderAsc = "asc"

func TestQueryBuilder_BuildComponentLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	params := ComponentQueryParams{
		QueryParams: QueryParams{
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-01T23:59:59Z",
			SearchPhrase:  "error",
			ComponentID:   "component-123",
			EnvironmentID: "env-456",
			Namespace:     "default",
			Versions:      []string{"v1.0.0", "v1.0.1"},
			VersionIDs:    []string{"version-id-1", "version-id-2"},
			Limit:         100,
			SortOrder:     sortOrderDesc,
			LogType:       labels.QueryParamLogTypeRuntime,
		},
		BuildID:   "",
		BuildUUID: "",
	}

	query := qb.BuildComponentLogsQuery(params)

	// Verify query structure
	if query["size"] != 100 {
		t.Errorf("Expected size 100, got %v", query["size"])
	}

	// Verify bool query exists
	boolQuery, ok := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected bool query not found")
	}

	// Verify must conditions
	mustConditions, ok := boolQuery["must"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected must conditions not found")
	}

	// Should have component, environment, namespace, time range, and search phrase
	expectedMustCount := 5
	if len(mustConditions) != expectedMustCount {
		t.Errorf("Expected %d must conditions, got %d", expectedMustCount, len(mustConditions))
	}

	// Verify should conditions for versions
	shouldConditions, ok := boolQuery["should"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected should conditions not found")
	}

	// Should have 4 conditions: 2 versions + 2 version IDs
	expectedShouldCount := 4
	if len(shouldConditions) != expectedShouldCount {
		t.Errorf("Expected %d should conditions, got %d", expectedShouldCount, len(shouldConditions))
	}

	// Verify minimum_should_match
	if boolQuery["minimum_should_match"] != 1 {
		t.Errorf("Expected minimum_should_match 1, got %v", boolQuery["minimum_should_match"])
	}
}

func TestQueryBuilder_BuildBuildLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	params := BuildQueryParams{
		QueryParams: QueryParams{
			StartTime: "2024-01-01T00:00:00Z",
			EndTime:   "2024-01-01T23:59:59Z",
			Limit:     200,
			SortOrder: "asc",
		},
		BuildID: "build-123",
	}

	query := qb.BuildBuildLogsQuery(params)

	if query["size"] != 200 {
		t.Errorf("Expected size 200, got %v", query["size"])
	}

	boolQuery, ok := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected bool query structure")
	}

	mustConditions, ok := boolQuery["must"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected must conditions")
	}

	if len(mustConditions) != 2 {
		t.Fatalf("Expected 2 must conditions (pod match + time range), got %d", len(mustConditions))
	}

	foundWildcard := false
	for _, condition := range mustConditions {
		if wildcard, ok := condition["wildcard"].(map[string]interface{}); ok {
			field := labels.KubernetesPodName
			if value, exists := wildcard[field]; exists && value == "build-123*" {
				foundWildcard = true
			}
		}
	}

	if !foundWildcard {
		t.Fatal("Expected build pod wildcard condition not found")
	}

	sortFields, ok := query["sort"].([]map[string]interface{})
	if !ok || len(sortFields) == 0 {
		t.Fatal("Expected sort configuration")
	}

	timestampSort, ok := sortFields[0]["@timestamp"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected timestamp sort configuration")
	}

	if timestampSort["order"] != sortOrderAsc {
		t.Errorf("Expected ascending sort order, got %v", timestampSort["order"])
	}
}

func TestQueryBuilder_BuildProjectLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	params := QueryParams{
		StartTime:     "2024-01-01T00:00:00Z",
		EndTime:       "2024-01-01T23:59:59Z",
		SearchPhrase:  "info",
		ProjectID:     "project-123",
		EnvironmentID: "env-456",
		Limit:         50,
		SortOrder:     "asc",
	}

	componentIDs := []string{"8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b", "3f7b9e1a-4c6d-4e8f-a2b5-7d1c3e8f4a9b", "5e2a7c9f-8b4d-4f1e-9c3a-6f8b2d4e7a1c"}

	query := qb.BuildProjectLogsQuery(params, componentIDs)

	// Verify query structure
	if query["size"] != 50 {
		t.Errorf("Expected size 50, got %v", query["size"])
	}

	// Verify bool query exists
	boolQuery, ok := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected bool query not found")
	}

	// Verify should conditions for component IDs
	shouldConditions, ok := boolQuery["should"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected should conditions not found")
	}

	if len(shouldConditions) != len(componentIDs) {
		t.Errorf("Expected %d should conditions, got %d", len(componentIDs), len(shouldConditions))
	}
}

func TestQueryBuilder_BuildGatewayLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	params := GatewayQueryParams{
		QueryParams: QueryParams{
			StartTime:    "2024-01-01T00:00:00Z",
			EndTime:      "2024-01-01T23:59:59Z",
			SearchPhrase: "gateway",
			Limit:        200,
			SortOrder:    sortOrderDesc,
		},
		NamespaceName: "namespace-123",
		APIIDToVersionMap: map[string]string{
			"api-1": "v1",
			"api-2": "v2",
		},
		GatewayVHosts: []string{"host1.example.com", "host2.example.com"},
	}

	query := qb.BuildGatewayLogsQuery(params)

	// Verify query structure
	if query["size"] != 200 {
		t.Errorf("Expected size 200, got %v", query["size"])
	}

	// Verify bool query exists
	boolQuery, ok := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected bool query not found")
	}

	// Should have must conditions for time range, namespace filter, search phrase, and nested bool for APIs/vhosts
	mustConditions, ok := boolQuery["must"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected must conditions not found")
	}

	// Verify minimum must conditions exist (time, namespace, search, nested bool)
	if len(mustConditions) < 3 {
		t.Errorf("Expected at least 3 must conditions, got %d", len(mustConditions))
	}
}

func TestQueryBuilder_GenerateIndices(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	tests := []struct {
		name      string
		startTime string
		endTime   string
		expected  []string
		shouldErr bool
	}{
		{
			name:      "empty times",
			startTime: "",
			endTime:   "",
			expected:  []string{"container-logs-*"},
			shouldErr: false,
		},
		{
			name:      "same day",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "2024-01-01T23:59:59Z",
			expected:  []string{"container-logs-2024-01-01"},
			shouldErr: false,
		},
		{
			name:      "multiple days",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "2024-01-03T23:59:59Z",
			expected:  []string{"container-logs-2024-01-01", "container-logs-2024-01-02", "container-logs-2024-01-03"},
			shouldErr: false,
		},
		{
			name:      "invalid start time",
			startTime: "invalid",
			endTime:   "2024-01-01T23:59:59Z",
			expected:  nil,
			shouldErr: true,
		},
		{
			name:      "invalid end time",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "invalid",
			expected:  nil,
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indices, err := qb.GenerateIndices(tt.startTime, tt.endTime)

			if tt.shouldErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(indices) != len(tt.expected) {
				t.Errorf("Expected %d indices, got %d", len(tt.expected), len(indices))
				return
			}

			for i, expected := range tt.expected {
				if indices[i] != expected {
					t.Errorf("Expected index %s, got %s", expected, indices[i])
				}
			}
		})
	}
}

func TestQueryBuilder_CheckQueryVersion(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	// Mock mapping response with wildcard type
	mappingV2 := &MappingResponse{
		Mappings: map[string]IndexMapping{
			"container-logs-2024-01-01": {
				Mappings: struct {
					Properties map[string]FieldMapping `json:"properties"`
				}{
					Properties: map[string]FieldMapping{
						"log": {
							Type: "wildcard",
						},
					},
				},
			},
		},
	}

	// Mock mapping response with text type
	mappingV1 := &MappingResponse{
		Mappings: map[string]IndexMapping{
			"container-logs-2024-01-01": {
				Mappings: struct {
					Properties map[string]FieldMapping `json:"properties"`
				}{
					Properties: map[string]FieldMapping{
						"log": {
							Type: "text",
						},
					},
				},
			},
		},
	}

	// Test V2 detection
	version := qb.CheckQueryVersion(mappingV2, "container-logs-2024-01-01")
	if version != "v2" {
		t.Errorf("Expected v2, got %s", version)
	}

	// Test V1 detection
	version = qb.CheckQueryVersion(mappingV1, "container-logs-2024-01-01")
	if version != "v1" {
		t.Errorf("Expected v1, got %s", version)
	}
}

func TestQueryBuilder_BuildTracesQuery(t *testing.T) {
	qb := NewQueryBuilder("otel-v1-apm-span-")

	t.Run("Basic query with time range only", func(t *testing.T) {
		params := TracesRequestParams{
			StartTime: "2024-01-01T00:00:00Z",
			EndTime:   "2024-01-01T23:59:59Z",
			Limit:     50,
			SortOrder: sortOrderDesc,
		}

		got := qb.BuildTracesQuery(params)

		if got["size"] != 50 {
			t.Errorf("Expected size 50, got %v", got["size"])
		}

		query := got["query"].(map[string]interface{})
		boolQuery := query["bool"].(map[string]interface{})
		filters := boolQuery["filter"].([]map[string]interface{})

		if len(filters) != 2 {
			t.Errorf("Expected 2 filters (time ranges only), got %d", len(filters))
		}
	})

	t.Run("Query with TraceID", func(t *testing.T) {
		params := TracesRequestParams{
			TraceID:   "trace-123",
			StartTime: "2024-01-01T00:00:00Z",
			EndTime:   "2024-01-01T23:59:59Z",
			Limit:     50,
			SortOrder: sortOrderDesc,
		}

		got := qb.BuildTracesQuery(params)

		query := got["query"].(map[string]interface{})
		boolQuery := query["bool"].(map[string]interface{})
		filters := boolQuery["filter"].([]map[string]interface{})

		// Should have 3 filters: 2 time ranges + 1 traceId
		if len(filters) != 3 {
			t.Errorf("Expected 3 filters, got %d", len(filters))
		}

		// Verify traceId wildcard filter exists
		traceIDFound := false
		for _, filter := range filters {
			if wildcard, ok := filter["wildcard"].(map[string]interface{}); ok {
				if traceID, exists := wildcard["traceId"]; exists && traceID == "trace-123" {
					traceIDFound = true
					break
				}
			}
		}
		if !traceIDFound {
			t.Error("TraceID wildcard filter not found")
		}
	})

	t.Run("Query with ComponentUIDs array", func(t *testing.T) {
		params := TracesRequestParams{
			ComponentUIDs: []string{"8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b", "3f7b9e1a-4c6d-4e8f-a2b5-7d1c3e8f4a9b"},
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-01T23:59:59Z",
			Limit:         50,
			SortOrder:     sortOrderDesc,
		}

		got := qb.BuildTracesQuery(params)

		query := got["query"].(map[string]interface{})
		boolQuery := query["bool"].(map[string]interface{})
		filters := boolQuery["filter"].([]map[string]interface{})

		// Should have 3 filters: 2 time ranges + 1 bool with should conditions
		if len(filters) != 3 {
			t.Errorf("Expected 3 filters, got %d", len(filters))
		}

		// Verify componentUID bool filter exists with should conditions
		componentUIDFound := false
		for _, filter := range filters {
			if boolFilter, ok := filter["bool"].(map[string]interface{}); ok {
				if should, exists := boolFilter["should"].([]map[string]interface{}); exists {
					if len(should) == 2 {
						componentUIDFound = true
						// Verify the UIDs are present
						found1, found2 := false, false
						for _, shouldTerm := range should {
							if term, ok := shouldTerm["term"].(map[string]interface{}); ok {
								if uid, exists := term["resource.openchoreo.dev/component-uid"]; exists {
									if uid == "8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b" {
										found1 = true
									}
									if uid == "3f7b9e1a-4c6d-4e8f-a2b5-7d1c3e8f4a9b" {
										found2 = true
									}
								}
							}
						}
						if !found1 || !found2 {
							t.Error("ComponentUID values not found in should conditions")
						}
					}
					break
				}
			}
		}
		if !componentUIDFound {
			t.Error("ComponentUID bool filter with should conditions not found")
		}
	})

	t.Run("Query with EnvironmentUID", func(t *testing.T) {
		params := TracesRequestParams{
			EnvironmentUID: "2f5a8c1e-7d9b-4e3f-6a4c-8e1f2d7a9b5c",
			StartTime:      "2024-01-01T00:00:00Z",
			EndTime:        "2024-01-01T23:59:59Z",
			Limit:          50,
			SortOrder:      sortOrderDesc,
		}

		got := qb.BuildTracesQuery(params)

		query := got["query"].(map[string]interface{})
		boolQuery := query["bool"].(map[string]interface{})
		filters := boolQuery["filter"].([]map[string]interface{})

		// Should have 3 filters: 2 time ranges + 1 environmentUID
		if len(filters) != 3 {
			t.Errorf("Expected 3 filters, got %d", len(filters))
		}

		// Verify environmentUID filter exists
		envFound := false
		for _, filter := range filters {
			if term, ok := filter["term"].(map[string]interface{}); ok {
				if env, exists := term["resource.openchoreo.dev/environment-uid"]; exists && env == "2f5a8c1e-7d9b-4e3f-6a4c-8e1f2d7a9b5c" {
					envFound = true
					break
				}
			}
		}
		if !envFound {
			t.Error("EnvironmentUID filter not found")
		}
	})
}

func TestQueryBuilder_BuildTracesAggregationQuery(t *testing.T) {
	qb := NewQueryBuilder("otel-v1-apm-span-")

	t.Run("Basic aggregation query", func(t *testing.T) {
		params := TracesRequestParams{
			StartTime:  "2024-01-01T00:00:00Z",
			EndTime:    "2024-01-01T23:59:59Z",
			ProjectUID: "project-123",
			Limit:      10,
			SortOrder:  sortOrderDesc,
		}

		got := qb.BuildTracesAggregationQuery(params)

		// Top-level size must be 0 (no hits, only aggregations)
		if got["size"] != 0 {
			t.Errorf("Expected size 0, got %v", got["size"])
		}

		// Verify query filters are preserved
		query := got["query"].(map[string]interface{})
		boolQuery := query["bool"].(map[string]interface{})
		filters := boolQuery["filter"].([]map[string]interface{})

		// Should have 3 filters: 2 time ranges + 1 projectUID
		if len(filters) != 3 {
			t.Errorf("Expected 3 filters, got %d", len(filters))
		}

		// Verify aggregation structure
		aggs, ok := got["aggs"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected aggs not found")
		}

		// Verify trace_count cardinality aggregation
		traceCount, ok := aggs["trace_count"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected trace_count aggregation not found")
		}
		cardinality, ok := traceCount["cardinality"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected cardinality in trace_count")
		}
		if cardinality["field"] != "traceId" {
			t.Errorf("Expected cardinality field 'traceId', got %v", cardinality["field"])
		}

		// Verify traces terms aggregation
		traces, ok := aggs["traces"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected traces aggregation not found")
		}
		terms, ok := traces["terms"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected terms in traces aggregation")
		}
		if terms["field"] != "traceId" {
			t.Errorf("Expected terms field 'traceId', got %v", terms["field"])
		}
		if terms["size"] != 10 {
			t.Errorf("Expected terms size 10 (matching limit), got %v", terms["size"])
		}

		// Verify sort order in terms
		order, ok := terms["order"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected order in terms")
		}
		if order["min_start_time"] != sortOrderDesc {
			t.Errorf("Expected order min_start_time 'desc', got %v", order["min_start_time"])
		}

		// Verify sub-aggregations
		subAggs, ok := traces["aggs"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected sub-aggs in traces aggregation")
		}

		if _, ok := subAggs["earliest_span"]; !ok {
			t.Error("Expected earliest_span sub-aggregation")
		}
		if _, ok := subAggs["root_span"]; !ok {
			t.Error("Expected root_span sub-aggregation")
		}
		if _, ok := subAggs["latest_span"]; !ok {
			t.Error("Expected latest_span sub-aggregation")
		}
		if _, ok := subAggs["min_start_time"]; !ok {
			t.Error("Expected min_start_time sub-aggregation (used for terms ordering)")
		}
	})

	t.Run("Aggregation query with ComponentUIDs", func(t *testing.T) {
		params := TracesRequestParams{
			ComponentUIDs: []string{"comp-1", "comp-2"},
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-01T23:59:59Z",
			Limit:         5,
			SortOrder:     "asc",
		}

		got := qb.BuildTracesAggregationQuery(params)

		query := got["query"].(map[string]interface{})
		boolQuery := query["bool"].(map[string]interface{})
		filters := boolQuery["filter"].([]map[string]interface{})

		// Should have 3 filters: 2 time ranges + 1 bool with component should
		if len(filters) != 3 {
			t.Errorf("Expected 3 filters, got %d", len(filters))
		}

		// Verify sort order is "asc"
		aggs := got["aggs"].(map[string]interface{})
		traces := aggs["traces"].(map[string]interface{})
		terms := traces["terms"].(map[string]interface{})
		order := terms["order"].(map[string]interface{})
		if order["min_start_time"] != "asc" {
			t.Errorf("Expected order 'asc', got %v", order["min_start_time"])
		}
	})
}

// verifyPodWildcardPattern checks if the query contains the expected pod name wildcard pattern
func verifyPodWildcardPattern(t *testing.T, mustConditions []map[string]interface{}, expectedPattern string) {
	t.Helper()
	foundWildcard := false
	for _, condition := range mustConditions {
		if wildcard, ok := condition["wildcard"].(map[string]interface{}); ok {
			field := labels.KubernetesPodName
			if value, exists := wildcard[field]; exists && value == expectedPattern {
				foundWildcard = true
				break
			}
		}
	}
	if !foundWildcard {
		t.Errorf("Expected pod wildcard pattern %s not found", expectedPattern)
	}
}

// verifyStepNameWildcardPattern checks if the query contains the expected Argo step name wildcard pattern
func verifyStepNameWildcardPattern(t *testing.T, mustConditions []map[string]interface{}, expectedPattern string) {
	t.Helper()
	const kubeAnnotationsPrefix = "kubernetes.annotations."
	const argoNodeNameAnnotation = "workflows_argoproj_io/node-name"
	field := kubeAnnotationsPrefix + argoNodeNameAnnotation

	foundWildcard := false
	for _, condition := range mustConditions {
		if wildcard, ok := condition["wildcard"].(map[string]interface{}); ok {
			if value, exists := wildcard[field]; exists && value == expectedPattern {
				foundWildcard = true
				break
			}
		}
	}
	if !foundWildcard {
		t.Errorf("Expected step name wildcard pattern %s not found", expectedPattern)
	}
}

// verifyContainerExclusions checks if init and wait containers are excluded
func verifyContainerExclusions(t *testing.T, mustNotConditions []map[string]interface{}) {
	t.Helper()
	if len(mustNotConditions) != 2 {
		t.Errorf("Expected 2 must_not conditions (init and wait), got %d", len(mustNotConditions))
		return
	}

	foundInit := false
	foundWait := false
	for _, condition := range mustNotConditions {
		if term, ok := condition["term"].(map[string]interface{}); ok {
			field := labels.KubernetesContainerName
			if value, exists := term[field]; exists {
				if value == "init" {
					foundInit = true
				}
				if value == "wait" {
					foundWait = true
				}
			}
		}
	}
	if !foundInit {
		t.Error("Expected init container exclusion not found")
	}
	if !foundWait {
		t.Error("Expected wait container exclusion not found")
	}
}

// verifySortOrder checks if the sort order is ascending
func verifySortOrder(t *testing.T, query map[string]interface{}) {
	t.Helper()
	sortFields, ok := query["sort"].([]map[string]interface{})
	if !ok || len(sortFields) == 0 {
		t.Fatal("Expected sort configuration")
	}

	timestampSort, ok := sortFields[0]["@timestamp"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected timestamp sort configuration")
	}

	if timestampSort["order"] != sortOrderAsc {
		t.Errorf("Expected ascending sort order, got %v", timestampSort["order"])
	}
}

func TestQueryBuilder_BuildWorkflowRunLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	t.Run("Basic query without stepName or namespace", func(t *testing.T) {
		params := WorkflowRunQueryParams{
			QueryParams: QueryParams{
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-01T23:59:59Z",
				Limit:     100,
				SortOrder: sortOrderDesc,
			},
			WorkflowRunID: "wf-run-123",
		}

		query := qb.BuildWorkflowRunLogsQuery(params)

		if query["size"] != 100 {
			t.Errorf("Expected size 100, got %v", query["size"])
		}

		boolQuery := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
		mustConditions := boolQuery["must"].([]map[string]interface{})

		// Should have pod wildcard + time range = 2
		if len(mustConditions) != 2 {
			t.Errorf("Expected 2 must conditions, got %d", len(mustConditions))
		}

		verifyPodWildcardPattern(t, mustConditions, "wf-run-123*")
		verifyContainerExclusions(t, boolQuery["must_not"].([]map[string]interface{}))
	})

	t.Run("Query with stepName", func(t *testing.T) {
		params := WorkflowRunQueryParams{
			QueryParams: QueryParams{
				StartTime: "2024-01-01T00:00:00Z",
				EndTime:   "2024-01-01T23:59:59Z",
				Limit:     100,
				SortOrder: sortOrderDesc,
			},
			WorkflowRunID: "wf-run-456",
			StepName:      "build-step",
		}

		query := qb.BuildWorkflowRunLogsQuery(params)

		boolQuery := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
		mustConditions := boolQuery["must"].([]map[string]interface{})

		// Should have pod wildcard + step annotation + time range = 3
		if len(mustConditions) != 3 {
			t.Errorf("Expected 3 must conditions, got %d", len(mustConditions))
		}

		verifyStepNameWildcardPattern(t, mustConditions, "*build-step*")
	})

	t.Run("Query with namespace", func(t *testing.T) {
		params := WorkflowRunQueryParams{
			QueryParams: QueryParams{
				StartTime:     "2024-01-01T00:00:00Z",
				EndTime:       "2024-01-01T23:59:59Z",
				NamespaceName: "my-namespace",
				Limit:         100,
				SortOrder:     sortOrderDesc,
			},
			WorkflowRunID: "wf-run-789",
		}

		query := qb.BuildWorkflowRunLogsQuery(params)

		boolQuery := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
		mustConditions := boolQuery["must"].([]map[string]interface{})

		// Should have pod wildcard + time range + namespace = 3
		if len(mustConditions) != 3 {
			t.Errorf("Expected 3 must conditions, got %d", len(mustConditions))
		}

		// Verify namespace filter uses "workflows-" prefix
		nsFound := false
		for _, condition := range mustConditions {
			if term, ok := condition["term"].(map[string]interface{}); ok {
				if val, exists := term[labels.KubernetesNamespaceName]; exists && val == "workflows-my-namespace" {
					nsFound = true
					break
				}
			}
		}
		if !nsFound {
			t.Error("Expected namespace filter with 'workflows-' prefix not found")
		}
	})
}

func TestQueryBuilder_BuildSpanDetailsQuery(t *testing.T) {
	qb := NewQueryBuilder("otel-v1-apm-span-")

	traceID := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
	spanID := "1a2b3c4d5e6f7a8b"

	query := qb.BuildSpanDetailsQuery(traceID, spanID)

	if query["size"] != 1 {
		t.Errorf("Expected size 1, got %v", query["size"])
	}

	boolQuery := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
	filters := boolQuery["filter"].([]map[string]interface{})

	if len(filters) != 2 {
		t.Errorf("Expected 2 filters (traceId + spanId), got %d", len(filters))
	}

	traceFound, spanFound := false, false
	for _, filter := range filters {
		if term, ok := filter["term"].(map[string]interface{}); ok {
			if val, exists := term["traceId"]; exists && val == traceID {
				traceFound = true
			}
			if val, exists := term["spanId"]; exists && val == spanID {
				spanFound = true
			}
		}
	}
	if !traceFound {
		t.Error("Expected traceId filter not found")
	}
	if !spanFound {
		t.Error("Expected spanId filter not found")
	}
}

func TestQueryBuilder_BuildComponentLogsQueryV1(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	t.Run("Error on missing required fields", func(t *testing.T) {
		_, err := qb.BuildComponentLogsQueryV1(ComponentLogsQueryParamsV1{})
		if err == nil {
			t.Error("Expected error for missing required fields")
		}
	})

	t.Run("Basic query with required fields only", func(t *testing.T) {
		params := ComponentLogsQueryParamsV1{
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-01T23:59:59Z",
			NamespaceName: "test-ns",
		}

		query, err := qb.BuildComponentLogsQueryV1(params)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Default limit should be 100
		if query["size"] != 100 {
			t.Errorf("Expected default size 100, got %v", query["size"])
		}

		// Default sort order should be "desc"
		sortFields := query["sort"].([]map[string]interface{})
		timestampSort := sortFields[0]["@timestamp"].(map[string]interface{})
		if timestampSort["order"] != "desc" {
			t.Errorf("Expected default sort order 'desc', got %v", timestampSort["order"])
		}
	})

	t.Run("Query with all optional filters", func(t *testing.T) {
		params := ComponentLogsQueryParamsV1{
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-01T23:59:59Z",
			NamespaceName: "test-ns",
			ProjectID:     "7b3e9a1f-4c6d-4e8f-a2b5-1d3c5e7f9a2b",
			ComponentID:   "9f1a3b5c-7d8e-4f2a-b6c4-8e0f2a4b6c8d",
			EnvironmentID: "c4e6a8b0-2d4f-4a6c-8e0b-2d4f6a8c0e2b",
			SearchPhrase:  "error",
			LogLevels:     []string{"ERROR"},
			Limit:         50,
			SortOrder:     "asc",
		}

		query, err := qb.BuildComponentLogsQueryV1(params)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if query["size"] != 50 {
			t.Errorf("Expected size 50, got %v", query["size"])
		}

		boolQuery := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
		mustConditions := boolQuery["must"].([]map[string]interface{})

		// time range + namespace + project + component + environment + search phrase + log level = 7
		if len(mustConditions) != 7 {
			t.Errorf("Expected 7 must conditions, got %d", len(mustConditions))
		}
	})
}

func TestQueryBuilder_BuildTracesAggregationQuery_DefaultSortOrder(t *testing.T) {
	qb := NewQueryBuilder("otel-v1-apm-span-")

	params := TracesRequestParams{
		StartTime: "2024-01-01T00:00:00Z",
		EndTime:   "2024-01-01T23:59:59Z",
		Limit:     10,
		SortOrder: "", // empty should default to "desc"
	}

	got := qb.BuildTracesAggregationQuery(params)

	aggs := got["aggs"].(map[string]interface{})
	traces := aggs["traces"].(map[string]interface{})
	terms := traces["terms"].(map[string]interface{})
	order := terms["order"].(map[string]interface{})
	if order["min_start_time"] != "desc" {
		t.Errorf("Expected default order 'desc', got %v", order["min_start_time"])
	}
}

func TestQueryBuilder_BuildTracesAggregationQuery_WithEnvironmentUID(t *testing.T) {
	qb := NewQueryBuilder("otel-v1-apm-span-")

	params := TracesRequestParams{
		StartTime:      "2024-01-01T00:00:00Z",
		EndTime:        "2024-01-01T23:59:59Z",
		EnvironmentUID: "d5f7b9c1-3e5a-4c7f-8a2d-6b8e0c2f4a6d",
		Limit:          10,
		SortOrder:      sortOrderDesc,
	}

	got := qb.BuildTracesAggregationQuery(params)

	query := got["query"].(map[string]interface{})
	boolQuery := query["bool"].(map[string]interface{})
	filters := boolQuery["filter"].([]map[string]interface{})

	// Should have 3 filters: 2 time ranges + 1 environmentUID
	if len(filters) != 3 {
		t.Errorf("Expected 3 filters, got %d", len(filters))
	}

	envFound := false
	for _, filter := range filters {
		if term, ok := filter["term"].(map[string]interface{}); ok {
			if val, exists := term["resource.openchoreo.dev/environment-uid"]; exists && val == "d5f7b9c1-3e5a-4c7f-8a2d-6b8e0c2f4a6d" {
				envFound = true
				break
			}
		}
	}
	if !envFound {
		t.Error("EnvironmentUID filter not found in aggregation query")
	}
}

func TestQueryBuilder_BuildTracesQuery_WithProjectUID(t *testing.T) {
	qb := NewQueryBuilder("otel-v1-apm-span-")

	params := TracesRequestParams{
		StartTime:  "2024-01-01T00:00:00Z",
		EndTime:    "2024-01-01T23:59:59Z",
		ProjectUID: "e6a8c0d2-4f6b-4e8a-9c1d-3f5a7b9e1c3d",
		Limit:      50,
		SortOrder:  sortOrderDesc,
	}

	got := qb.BuildTracesQuery(params)

	query := got["query"].(map[string]interface{})
	boolQuery := query["bool"].(map[string]interface{})
	filters := boolQuery["filter"].([]map[string]interface{})

	// Should have 3 filters: 2 time ranges + 1 projectUID
	if len(filters) != 3 {
		t.Errorf("Expected 3 filters, got %d", len(filters))
	}

	projFound := false
	for _, filter := range filters {
		if term, ok := filter["term"].(map[string]interface{}); ok {
			if val, exists := term["resource.openchoreo.dev/project-uid"]; exists && val == "e6a8c0d2-4f6b-4e8a-9c1d-3f5a7b9e1c3d" {
				projFound = true
				break
			}
		}
	}
	if !projFound {
		t.Error("ProjectUID filter not found")
	}
}

func TestQueryBuilder_BuildWorkflowRunPodLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	t.Run("Query with runName only (no stepName)", func(t *testing.T) {
		params := WorkflowRunLogsQueryParams{
			RunName:  "workflow-run-123",
			StepName: "",
			Limit:    100,
		}

		query := qb.BuildWorkflowRunPodLogsQuery(params)

		if query["size"] != 100 {
			t.Errorf("Expected size 100, got %v", query["size"])
		}

		boolQuery, ok := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected bool query not found")
		}

		mustConditions, ok := boolQuery["must"].([]map[string]interface{})
		if !ok {
			t.Fatal("Expected must conditions not found")
		}

		if len(mustConditions) != 1 {
			t.Errorf("Expected 1 must condition (pod wildcard), got %d", len(mustConditions))
		}

		verifyPodWildcardPattern(t, mustConditions, "workflow-run-123-*")

		mustNotConditions, ok := boolQuery["must_not"].([]map[string]interface{})
		if !ok {
			t.Fatal("Expected must_not conditions not found")
		}

		verifyContainerExclusions(t, mustNotConditions)
		verifySortOrder(t, query)
	})

	t.Run("Query with both runName and stepName", func(t *testing.T) {
		params := WorkflowRunLogsQueryParams{
			RunName:  "workflow-run-456",
			StepName: "build-step",
			Limit:    200,
		}

		query := qb.BuildWorkflowRunPodLogsQuery(params)

		if query["size"] != 200 {
			t.Errorf("Expected size 200, got %v", query["size"])
		}

		boolQuery, ok := query["query"].(map[string]interface{})["bool"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected bool query not found")
		}

		mustConditions, ok := boolQuery["must"].([]map[string]interface{})
		if !ok {
			t.Fatal("Expected must conditions not found")
		}
		if len(mustConditions) != 2 {
			t.Errorf("Expected 2 must conditions (pod wildcard and step annotation wildcard), got %d", len(mustConditions))
		}

		verifyStepNameWildcardPattern(t, mustConditions, "*build-step*")
		verifySortOrder(t, query)
	})

	t.Run("Query with different limit values", func(t *testing.T) {
		tests := []struct {
			name  string
			limit int
		}{
			{"limit 50", 50},
			{"limit 500", 500},
			{"limit 0", 0},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				params := WorkflowRunLogsQueryParams{
					RunName:  "test-run",
					StepName: "",
					Limit:    tt.limit,
				}

				query := qb.BuildWorkflowRunPodLogsQuery(params)

				if query["size"] != tt.limit {
					t.Errorf("Expected size %d, got %v", tt.limit, query["size"])
				}
			})
		}
	})
}
