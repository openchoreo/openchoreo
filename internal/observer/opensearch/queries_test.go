// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"testing"
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
			LogType:       "RUNTIME",
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
