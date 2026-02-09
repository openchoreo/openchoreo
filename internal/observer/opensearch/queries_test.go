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
			SortOrder:     "desc",
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
			field := labels.KubernetesPodName + ".keyword"
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
			SortOrder:    "desc",
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
			SortOrder: "desc",
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
			SortOrder: "desc",
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
			SortOrder:     "desc",
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
			SortOrder:      "desc",
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

// verifyPodWildcardPattern checks if the query contains the expected pod name wildcard pattern
func verifyPodWildcardPattern(t *testing.T, mustConditions []map[string]interface{}, expectedPattern string) {
	t.Helper()
	foundWildcard := false
	for _, condition := range mustConditions {
		if wildcard, ok := condition["wildcard"].(map[string]interface{}); ok {
			field := labels.KubernetesPodName + ".keyword"
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
			field := labels.KubernetesContainerName + ".keyword"
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

func TestQueryBuilder_BuildComponentWorkflowRunLogsQuery(t *testing.T) {
	qb := NewQueryBuilder("container-logs-")

	t.Run("Query with runName only (no stepName)", func(t *testing.T) {
		params := ComponentWorkflowRunQueryParams{
			RunName:  "workflow-run-123",
			StepName: "",
			Limit:    100,
		}

		query := qb.BuildComponentWorkflowRunLogsQuery(params)

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
		params := ComponentWorkflowRunQueryParams{
			RunName:  "workflow-run-456",
			StepName: "build-step",
			Limit:    200,
		}

		query := qb.BuildComponentWorkflowRunLogsQuery(params)

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

		verifyPodWildcardPattern(t, mustConditions, "workflow-run-456-build-step-*")
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
				params := ComponentWorkflowRunQueryParams{
					RunName:  "test-run",
					StepName: "",
					Limit:    tt.limit,
				}

				query := qb.BuildComponentWorkflowRunLogsQuery(params)

				if query["size"] != tt.limit {
					t.Errorf("Expected size %d, got %v", tt.limit, query["size"])
				}
			})
		}
	})
}
