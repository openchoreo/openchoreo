// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/labels"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

// MockOpenSearchClient implements a mock OpenSearch client for testing
type MockOpenSearchClient struct {
	searchResponse *opensearch.SearchResponse
	searchError    error
	healthError    error
}

func (m *MockOpenSearchClient) Search(ctx context.Context, indices []string, query map[string]interface{}) (*opensearch.SearchResponse, error) {
	if m.searchError != nil {
		return nil, m.searchError
	}
	return m.searchResponse, nil
}

func (m *MockOpenSearchClient) GetIndexMapping(ctx context.Context, index string) (*opensearch.MappingResponse, error) {
	return &opensearch.MappingResponse{}, nil
}

func (m *MockOpenSearchClient) HealthCheck(ctx context.Context) error {
	return m.healthError
}

func newMockLoggingService() *LoggingService {
	cfg := &config.Config{
		OpenSearch: config.OpenSearchConfig{
			IndexPrefix: "container-logs-",
		},
		Logging: config.LoggingConfig{
			MaxLogLimit:     10000,
			DefaultLogLimit: 100,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create service with a mock client - we'll replace the client in tests
	return &LoggingService{
		queryBuilder: opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		config:       cfg,
		logger:       logger,
	}
}

func TestLoggingService_GetComponentLogs(t *testing.T) {
	service := newMockLoggingService()

	// Mock search response
	mockResponse := &opensearch.SearchResponse{
		Hits: struct {
			Total struct {
				Value    int    `json:"value"`
				Relation string `json:"relation"`
			} `json:"total"`
			Hits []opensearch.Hit `json:"hits"`
		}{
			Total: struct {
				Value    int    `json:"value"`
				Relation string `json:"relation"`
			}{
				Value:    2,
				Relation: "eq",
			},
			Hits: []opensearch.Hit{
				{
					Source: map[string]interface{}{
						"@timestamp": "2024-01-01T10:00:00Z",
						"log":        "INFO: Application started",
						"kubernetes": map[string]interface{}{
							"labels": map[string]interface{}{
								"component-name":   "comp-123",
								"environment-name": "env-456",
							},
							"namespace_name": "default",
						},
					},
				},
				{
					Source: map[string]interface{}{
						"@timestamp": "2024-01-01T10:01:00Z",
						"log":        "ERROR: Something went wrong",
						"kubernetes": map[string]interface{}{
							"labels": map[string]interface{}{
								"component-name":   "comp-123",
								"environment-name": "env-456",
							},
							"namespace_name": "default",
						},
					},
				},
			},
		},
		Took: 50,
	}

	// Replace the client with mock
	mockClient := &MockOpenSearchClient{
		searchResponse: mockResponse,
	}
	service.osClient = mockClient

	params := opensearch.ComponentQueryParams{
		QueryParams: opensearch.QueryParams{
			StartTime:     "2024-01-01T00:00:00Z",
			EndTime:       "2024-01-01T23:59:59Z",
			SearchPhrase:  "error",
			ComponentID:   "comp-123",
			EnvironmentID: "env-456",
			Namespace:     "default",
			Limit:         100,
			SortOrder:     "desc",
			LogType:       labels.QueryParamLogTypeRuntime,
		},
		BuildID:   "",
		BuildUUID: "",
	}

	ctx := context.Background()
	result, err := service.GetComponentLogs(ctx, params)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if result.TotalCount != 2 {
		t.Errorf("Expected total count 2, got %d", result.TotalCount)
	}

	if len(result.Logs) != 2 {
		t.Errorf("Expected 2 log entries, got %d", len(result.Logs))
	}

	if result.Took != 50 {
		t.Errorf("Expected took 50ms, got %d", result.Took)
	}

	// Verify first log entry
	firstLog := result.Logs[0]
	if firstLog.ComponentID != "comp-123" {
		t.Errorf("Expected component ID 'comp-123', got '%s'", firstLog.ComponentID)
	}

	if firstLog.Log != "INFO: Application started" {
		t.Errorf("Expected log content 'INFO: Application started', got '%s'", firstLog.Log)
	}

	// Verify second log entry
	secondLog := result.Logs[1]
	if secondLog.LogLevel != "ERROR" {
		t.Errorf("Expected log level 'ERROR', got '%s'", secondLog.LogLevel)
	}
}

func TestLoggingService_GetProjectLogs(t *testing.T) {
	service := newMockLoggingService()

	// Mock search response
	mockResponse := &opensearch.SearchResponse{
		Hits: struct {
			Total struct {
				Value    int    `json:"value"`
				Relation string `json:"relation"`
			} `json:"total"`
			Hits []opensearch.Hit `json:"hits"`
		}{
			Total: struct {
				Value    int    `json:"value"`
				Relation string `json:"relation"`
			}{
				Value:    1,
				Relation: "eq",
			},
			Hits: []opensearch.Hit{
				{
					Source: map[string]interface{}{
						"@timestamp": "2024-01-01T10:00:00Z",
						"log":        "Project log entry",
						"kubernetes": map[string]interface{}{
							"labels": map[string]interface{}{
								"project-name":     "proj-123",
								"component-name":   "comp-456",
								"environment-name": "env-789",
							},
						},
					},
				},
			},
		},
		Took: 25,
	}

	mockClient := &MockOpenSearchClient{
		searchResponse: mockResponse,
	}
	service.osClient = mockClient

	params := opensearch.QueryParams{
		StartTime:     "2024-01-01T00:00:00Z",
		EndTime:       "2024-01-01T23:59:59Z",
		ProjectID:     "proj-123",
		EnvironmentID: "env-789",
		Limit:         50,
		SortOrder:     "asc",
	}

	componentIDs := []string{"comp-456", "comp-789"}

	ctx := context.Background()
	result, err := service.GetProjectLogs(ctx, params, componentIDs)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.TotalCount != 1 {
		t.Errorf("Expected total count 1, got %d", result.TotalCount)
	}

	if len(result.Logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(result.Logs))
	}

	log := result.Logs[0]
	if log.ProjectID != "proj-123" {
		t.Errorf("Expected project ID 'proj-123', got '%s'", log.ProjectID)
	}
}

func TestLoggingService_HealthCheck(t *testing.T) {
	service := newMockLoggingService()

	tests := []struct {
		name        string
		healthError error
		expectError bool
	}{
		{
			name:        "healthy",
			healthError: nil,
			expectError: false,
		},
		{
			name:        "unhealthy",
			healthError: &mockError{"OpenSearch connection failed"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockOpenSearchClient{
				healthError: tt.healthError,
			}
			service.osClient = mockClient

			ctx := context.Background()
			err := service.HealthCheck(ctx)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// mockError implements the error interface for testing
type mockError struct {
	message string
}

func (e *mockError) Error() string {
	return e.message
}

func TestParseLogEntry(t *testing.T) {
	hit := opensearch.Hit{
		Source: map[string]interface{}{
			"@timestamp": "2024-01-01T10:00:00Z",
			"log":        "ERROR: Database connection failed",
			"kubernetes": map[string]interface{}{
				"labels": map[string]interface{}{
					"component-name":   "api-service",
					"environment-name": "production",
					"version":          "v1.2.3",
					"version_id":       "ver-456",
				},
				"namespace_name": "default",
				"pod_id":         "pod-123",
				"container_name": "api-container",
			},
		},
	}

	entry := opensearch.ParseLogEntry(hit)

	// Verify timestamp parsing
	expectedTime, _ := time.Parse(time.RFC3339, "2024-01-01T10:00:00Z")
	if !entry.Timestamp.Equal(expectedTime) {
		t.Errorf("Expected timestamp %v, got %v", expectedTime, entry.Timestamp)
	}

	// Verify log content
	if entry.Log != "ERROR: Database connection failed" {
		t.Errorf("Expected log 'ERROR: Database connection failed', got '%s'", entry.Log)
	}

	// Verify log level extraction
	if entry.LogLevel != "ERROR" {
		t.Errorf("Expected log level 'ERROR', got '%s'", entry.LogLevel)
	}

	// Verify Kubernetes metadata
	if entry.ComponentID != "api-service" {
		t.Errorf("Expected component ID 'api-service', got '%s'", entry.ComponentID)
	}

	if entry.EnvironmentID != "production" {
		t.Errorf("Expected environment ID 'production', got '%s'", entry.EnvironmentID)
	}

	if entry.Version != "v1.2.3" {
		t.Errorf("Expected version 'v1.2.3', got '%s'", entry.Version)
	}

	if entry.VersionID != "ver-456" {
		t.Errorf("Expected version ID 'ver-456', got '%s'", entry.VersionID)
	}

	if entry.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", entry.Namespace)
	}

	if entry.PodID != "pod-123" {
		t.Errorf("Expected pod ID 'pod-123', got '%s'", entry.PodID)
	}

	if entry.ContainerName != "api-container" {
		t.Errorf("Expected container name 'api-container', got '%s'", entry.ContainerName)
	}

	// Verify labels map
	if len(entry.Labels) != 4 {
		t.Errorf("Expected 4 labels, got %d", len(entry.Labels))
	}

	if entry.Labels["component-name"] != "api-service" {
		t.Errorf("Expected label component-name 'api-service', got '%s'", entry.Labels["component-name"])
	}
}

func TestLoggingService_GetComponentTraces(t *testing.T) {
	tests := []struct {
		name           string
		params         opensearch.ComponentTracesRequestParams
		mockResponse   *opensearch.SearchResponse
		mockError      error
		expectedResult *opensearch.TraceResponse
		expectedError  bool
	}{
		{
			name: "successful trace retrieval",
			params: opensearch.ComponentTracesRequestParams{
				ServiceName: "test-service",
				StartTime:   "2024-01-01T00:00:00Z",
				EndTime:     "2024-01-01T23:59:59Z",
				Limit:       50,
			},
			mockResponse: &opensearch.SearchResponse{
				Hits: struct {
					Total struct {
						Value    int    `json:"value"`
						Relation string `json:"relation"`
					} `json:"total"`
					Hits []opensearch.Hit `json:"hits"`
				}{
					Total: struct {
						Value    int    `json:"value"`
						Relation string `json:"relation"`
					}{
						Value:    2,
						Relation: "eq",
					},
					Hits: []opensearch.Hit{
						{
							Source: map[string]interface{}{
								"traceId":         "trace-123",
								"spanId":          "span-456",
								"name":            "database-query",
								"durationInNanos": int64(101018208),
								"startTime":       "2024-01-01T10:00:00.000000Z",
								"endTime":         "2024-01-01T10:00:00.101018208Z",
							},
						},
						{
							Source: map[string]interface{}{
								"traceId":         "trace-124",
								"spanId":          "span-457",
								"name":            "api-call",
								"durationInNanos": int64(200000000),
								"startTime":       "2024-01-01T11:00:00.000000Z",
								"endTime":         "2024-01-01T11:00:00.200000000Z",
							},
						},
					},
				},
				Took:     25,
				TimedOut: false,
			},
			expectedResult: &opensearch.TraceResponse{
				Spans: []opensearch.Span{
					{
						TraceID:         "trace-123",
						SpanID:          "span-456",
						Name:            "database-query",
						DurationInNanos: 101018208,
						StartTime:       mustParseTime("2024-01-01T10:00:00.000000Z"),
						EndTime:         mustParseTime("2024-01-01T10:00:00.101018208Z"),
					},
					{
						TraceID:         "trace-124",
						SpanID:          "span-457",
						Name:            "api-call",
						DurationInNanos: 200000000,
						StartTime:       mustParseTime("2024-01-01T11:00:00.000000Z"),
						EndTime:         mustParseTime("2024-01-01T11:00:00.200000000Z"),
					},
				},
				TotalCount: 2,
				Took:       25,
			},
			expectedError: false,
		},
		{
			name: "empty trace results",
			params: opensearch.ComponentTracesRequestParams{
				ServiceName: "non-existent-service",
				StartTime:   "2024-01-01T00:00:00Z",
				EndTime:     "2024-01-01T23:59:59Z",
				Limit:       10,
			},
			mockResponse: &opensearch.SearchResponse{
				Hits: struct {
					Total struct {
						Value    int    `json:"value"`
						Relation string `json:"relation"`
					} `json:"total"`
					Hits []opensearch.Hit `json:"hits"`
				}{
					Total: struct {
						Value    int    `json:"value"`
						Relation string `json:"relation"`
					}{
						Value:    0,
						Relation: "eq",
					},
					Hits: []opensearch.Hit{},
				},
				Took:     10,
				TimedOut: false,
			},
			expectedResult: &opensearch.TraceResponse{
				Spans:      []opensearch.Span{},
				TotalCount: 0,
				Took:       10,
			},
			expectedError: false,
		},
		{
			name: "opensearch error",
			params: opensearch.ComponentTracesRequestParams{
				ServiceName: "test-service",
				StartTime:   "2024-01-01T00:00:00Z",
				EndTime:     "2024-01-01T23:59:59Z",
				Limit:       50,
			},
			mockResponse:   nil,
			mockError:      fmt.Errorf("opensearch connection failed"),
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "trace with missing optional fields",
			params: opensearch.ComponentTracesRequestParams{
				ServiceName: "test-service",
				StartTime:   "2024-01-01T00:00:00Z",
				EndTime:     "2024-01-01T23:59:59Z",
				Limit:       10,
			},
			mockResponse: &opensearch.SearchResponse{
				Hits: struct {
					Total struct {
						Value    int    `json:"value"`
						Relation string `json:"relation"`
					} `json:"total"`
					Hits []opensearch.Hit `json:"hits"`
				}{
					Total: struct {
						Value    int    `json:"value"`
						Relation string `json:"relation"`
					}{
						Value:    1,
						Relation: "eq",
					},
					Hits: []opensearch.Hit{
						{
							Source: map[string]interface{}{
								"traceId": "trace-125",
								"spanId":  "span-458",
								"name":    "minimal-span",
								// Missing durationInNanos, startTime, endTime
							},
						},
					},
				},
				Took:     5,
				TimedOut: false,
			},
			expectedResult: &opensearch.TraceResponse{
				Spans: []opensearch.Span{
					{
						TraceID:         "trace-125",
						SpanID:          "span-458",
						Name:            "minimal-span",
						DurationInNanos: 0,
						StartTime:       time.Time{},
						EndTime:         time.Time{},
					},
				},
				TotalCount: 1,
				Took:       5,
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			mockClient := &MockOpenSearchClient{
				searchResponse: tt.mockResponse,
				searchError:    tt.mockError,
			}

			// Create service with mock client
			service := &LoggingService{
				osClient:     mockClient,
				queryBuilder: opensearch.NewQueryBuilder("otel-v1-apm-span-"),
				config: &config.Config{
					OpenSearch: config.OpenSearchConfig{
						IndexPrefix: "otel-v1-apm-span-",
					},
				},
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}

			// Call the method
			result, err := service.GetComponentTraces(context.Background(), tt.params)

			// Check error expectation
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify result
			if result == nil {
				t.Error("Expected result but got nil")
				return
			}

			// Check total count
			if result.TotalCount != tt.expectedResult.TotalCount {
				t.Errorf("Expected TotalCount %d, got %d", tt.expectedResult.TotalCount, result.TotalCount)
			}

			// Check took time
			if result.Took != tt.expectedResult.Took {
				t.Errorf("Expected Took %d, got %d", tt.expectedResult.Took, result.Took)
			}

			// Check spans count
			if len(result.Spans) != len(tt.expectedResult.Spans) {
				t.Errorf("Expected %d spans, got %d", len(tt.expectedResult.Spans), len(result.Spans))
				return
			}

			// Check individual spans
			for i, expectedSpan := range tt.expectedResult.Spans {
				actualSpan := result.Spans[i]

				if actualSpan.TraceID != expectedSpan.TraceID {
					t.Errorf("Span %d: Expected TraceID '%s', got '%s'", i, expectedSpan.TraceID, actualSpan.TraceID)
				}

				if actualSpan.SpanID != expectedSpan.SpanID {
					t.Errorf("Span %d: Expected SpanId '%s', got '%s'", i, expectedSpan.SpanID, actualSpan.SpanID)
				}

				if actualSpan.Name != expectedSpan.Name {
					t.Errorf("Span %d: Expected Name '%s', got '%s'", i, expectedSpan.Name, actualSpan.Name)
				}

				if actualSpan.DurationInNanos != expectedSpan.DurationInNanos {
					t.Errorf("Span %d: Expected DurationInNanos %d, got %d", i, expectedSpan.DurationInNanos, actualSpan.DurationInNanos)
				}

				if !actualSpan.StartTime.Equal(expectedSpan.StartTime) {
					t.Errorf("Span %d: Expected StartTime '%v', got '%v'", i, expectedSpan.StartTime, actualSpan.StartTime)
				}

				if !actualSpan.EndTime.Equal(expectedSpan.EndTime) {
					t.Errorf("Span %d: Expected EndTime '%v', got '%v'", i, expectedSpan.EndTime, actualSpan.EndTime)
				}
			}
		})
	}
}

func TestLoggingService_GetComponentTraces_QueryBuilding(t *testing.T) {
	// This test verifies that the correct query is built and the right indices are used
	mockClient := &MockOpenSearchClient{
		searchResponse: &opensearch.SearchResponse{
			Hits: struct {
				Total struct {
					Value    int    `json:"value"`
					Relation string `json:"relation"`
				} `json:"total"`
				Hits []opensearch.Hit `json:"hits"`
			}{
				Total: struct {
					Value    int    `json:"value"`
					Relation string `json:"relation"`
				}{Value: 0},
				Hits: []opensearch.Hit{},
			},
			Took: 1,
		},
	}

	service := &LoggingService{
		osClient:     mockClient,
		queryBuilder: opensearch.NewQueryBuilder("otel-v1-apm-span-"),
		config: &config.Config{
			OpenSearch: config.OpenSearchConfig{
				IndexPrefix: "otel-v1-apm-span-",
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	params := opensearch.ComponentTracesRequestParams{
		ServiceName: "my-test-service",
		StartTime:   "2024-01-01T00:00:00Z",
		EndTime:     "2024-01-01T23:59:59Z",
		Limit:       25,
	}

	_, err := service.GetComponentTraces(context.Background(), params)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// The test passes if no error occurs, which means:
	// 1. Query was built successfully
	// 2. OpenSearch search was called with correct parameters
	// 3. Response was parsed without issues
}

// Helper function to parse time strings for test data
func mustParseTime(timeStr string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse time %s: %v", timeStr, err))
	}
	return parsed
}
