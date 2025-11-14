// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/labels"
)

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

	componentIDs := []string{"comp-1", "comp-2", "comp-3"}

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
		OrganizationID: "org-123",
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

	// Should have must conditions for time range, org filter, search phrase, and nested bool for APIs/vhosts
	mustConditions, ok := boolQuery["must"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected must conditions not found")
	}

	// Verify minimum must conditions exist (time, org, search, nested bool)
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
			expected:  []string{"container-logs-2024.01.01"},
			shouldErr: false,
		},
		{
			name:      "multiple days",
			startTime: "2024-01-01T00:00:00Z",
			endTime:   "2024-01-03T23:59:59Z",
			expected:  []string{"container-logs-2024.01.01", "container-logs-2024.01.02", "container-logs-2024.01.03"},
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

func TestQueryBuilder_BuildComponentTracesQuery(t *testing.T) {
	qb := NewQueryBuilder("otel-v1-apm-span-")

	tests := []struct {
		name   string
		params ComponentTracesRequestParams
		want   map[string]interface{}
	}{
		{
			name: "Basic component traces query",
			params: ComponentTracesRequestParams{
				ServiceName: "test-service",
				StartTime:   "2024-01-01T00:00:00Z",
				EndTime:     "2024-01-01T23:59:59Z",
				Limit:       50,
				SortOrder:   "desc",
			},
			want: map[string]interface{}{
				"size": 50,
				"query": map[string]interface{}{
					"bool": map[string]interface{}{
						"filter": []map[string]interface{}{
							{
								"term": map[string]interface{}{
									"serviceName": "test-service",
								},
							},
							{
								"range": map[string]interface{}{
									"startTime": map[string]interface{}{
										"gte": "2024-01-01T00:00:00Z",
									},
								},
							},
							{
								"range": map[string]interface{}{
									"endTime": map[string]interface{}{
										"lte": "2024-01-01T23:59:59Z",
									},
								},
							},
						},
					},
				},
				"sort": []map[string]interface{}{
					{
						"startTime": map[string]interface{}{
							"order": "desc",
						},
					},
				},
			},
		},
		{
			name: "Component traces query with default limit",
			params: ComponentTracesRequestParams{
				ServiceName: "another-service",
				StartTime:   "2024-02-01T10:00:00Z",
				EndTime:     "2024-02-01T20:00:00Z",
				Limit:       0, // Should use this value
				SortOrder:   "asc",
			},
			want: map[string]interface{}{
				"size": 0,
				"query": map[string]interface{}{
					"bool": map[string]interface{}{
						"filter": []map[string]interface{}{
							{
								"term": map[string]interface{}{
									"serviceName": "another-service",
								},
							},
							{
								"range": map[string]interface{}{
									"startTime": map[string]interface{}{
										"gte": "2024-02-01T10:00:00Z",
									},
								},
							},
							{
								"range": map[string]interface{}{
									"endTime": map[string]interface{}{
										"lte": "2024-02-01T20:00:00Z",
									},
								},
							},
						},
					},
				},
				"sort": []map[string]interface{}{
					{
						"startTime": map[string]interface{}{
							"order": "asc",
						},
					},
				},
			},
		},
		{
			name: "Component traces query with special characters in service name",
			params: ComponentTracesRequestParams{
				ServiceName: "my-service-123_test",
				StartTime:   "2024-03-15T08:30:00Z",
				EndTime:     "2024-03-15T18:30:00Z",
				Limit:       25,
			},
			want: map[string]interface{}{
				"size": 25,
				"query": map[string]interface{}{
					"bool": map[string]interface{}{
						"filter": []map[string]interface{}{
							{
								"term": map[string]interface{}{
									"serviceName": "my-service-123_test",
								},
							},
							{
								"range": map[string]interface{}{
									"startTime": map[string]interface{}{
										"gte": "2024-03-15T08:30:00Z",
									},
								},
							},
							{
								"range": map[string]interface{}{
									"endTime": map[string]interface{}{
										"lte": "2024-03-15T18:30:00Z",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := qb.BuildComponentTracesQuery(tt.params)

			// Check size
			if got["size"] != tt.want["size"] {
				t.Errorf("BuildComponentTracesQuery() size = %v, want %v", got["size"], tt.want["size"])
			}

			// Check query structure exists
			gotQuery, ok := got["query"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected query field not found")
			}

			wantQuery := tt.want["query"].(map[string]interface{})
			gotBool, ok := gotQuery["bool"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected bool query not found")
			}

			wantBool := wantQuery["bool"].(map[string]interface{})

			// Check filter conditions
			gotFilters, ok := gotBool["filter"].([]map[string]interface{})
			if !ok {
				t.Fatal("Expected filter conditions not found")
			}

			wantFilters := wantBool["filter"].([]map[string]interface{})
			if len(gotFilters) != len(wantFilters) {
				t.Errorf("BuildComponentTracesQuery() filter count = %v, want %v", len(gotFilters), len(wantFilters))
			}

			// Verify serviceName filter
			serviceNameFound := false
			for _, filter := range gotFilters {
				if term, ok := filter["term"].(map[string]interface{}); ok {
					if serviceName, exists := term["serviceName"]; exists {
						if serviceName != tt.params.ServiceName {
							t.Errorf("BuildComponentTracesQuery() serviceName = %v, want %v", serviceName, tt.params.ServiceName)
						}
						serviceNameFound = true
						break
					}
				}
			}
			if !serviceNameFound {
				t.Error("BuildComponentTracesQuery() serviceName filter not found")
			}

			// Verify startTime range filter
			startTimeFound := false
			for _, filter := range gotFilters {
				if rangeFilter, ok := filter["range"].(map[string]interface{}); ok {
					if startTimeRange, exists := rangeFilter["startTime"].(map[string]interface{}); exists {
						if gte, ok := startTimeRange["gte"]; ok && gte == tt.params.StartTime {
							startTimeFound = true
							break
						}
					}
				}
			}
			if !startTimeFound {
				t.Error("BuildComponentTracesQuery() startTime range filter not found")
			}

			// Verify endTime range filter
			endTimeFound := false
			for _, filter := range gotFilters {
				if rangeFilter, ok := filter["range"].(map[string]interface{}); ok {
					if endTimeRange, exists := rangeFilter["endTime"].(map[string]interface{}); exists {
						if lte, ok := endTimeRange["lte"]; ok && lte == tt.params.EndTime {
							endTimeFound = true
							break
						}
					}
				}
			}
			if !endTimeFound {
				t.Error("BuildComponentTracesQuery() endTime range filter not found")
			}
		})
	}
}
