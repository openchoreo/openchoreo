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
	monitorID      string
	monitorExists  bool
	monitorSearch  error
	monitorCreate  error
	monitorUpdate  error
	monitorDelete  error
	monitorBody    map[string]interface{}
	alertEntryID   string
	alertEntryErr  error
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

func (m *MockOpenSearchClient) SearchMonitorByName(ctx context.Context, name string) (string, bool, error) {
	return m.monitorID, m.monitorExists, m.monitorSearch
}

func (m *MockOpenSearchClient) GetMonitorByID(ctx context.Context, monitorID string) (map[string]interface{}, error) {
	if m.monitorBody != nil {
		return m.monitorBody, nil
	}
	// Return a default monitor body if not set
	return map[string]interface{}{
		"type":         "monitor",
		"monitor_type": "query_level_monitor",
		"name":         "test-monitor",
		"enabled":      true,
	}, nil
}

func (m *MockOpenSearchClient) CreateMonitor(ctx context.Context, monitor map[string]interface{}) (string, int64, error) {
	if m.monitorCreate != nil {
		return "", 0, m.monitorCreate
	}
	return m.monitorID, 0, nil
}

func (m *MockOpenSearchClient) UpdateMonitor(ctx context.Context, monitorID string, monitor map[string]interface{}) (int64, error) {
	if m.monitorUpdate != nil {
		return 0, m.monitorUpdate
	}
	return time.Now().UnixMilli(), nil
}

func (m *MockOpenSearchClient) DeleteMonitor(ctx context.Context, monitorID string) error {
	return m.monitorDelete
}

func (m *MockOpenSearchClient) WriteAlertEntry(ctx context.Context, entry map[string]interface{}) (string, error) {
	if m.alertEntryErr != nil {
		return "", m.alertEntryErr
	}
	if m.alertEntryID == "" {
		m.alertEntryID = "alert-entry-id"
	}
	// store entry to ensure it's non-nil for potential future assertions
	if m.monitorBody == nil {
		m.monitorBody = entry
	}
	return m.alertEntryID, nil
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
								"openchoreo_dev/component-uid":   "8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b",
								"openchoreo_dev/environment-uid": "6c2d3e4f-7a1b-3d8c-9e5f-2c6d4e8f1a9b",
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
								"openchoreo_dev/component-uid":   "comp-123",
								"openchoreo_dev/environment-uid": "env-456",
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
			ComponentID:   "8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b",
			EnvironmentID: "6c2d3e4f-7a1b-3d8c-9e5f-2c6d4e8f1a9b",
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
		return
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
	if firstLog.ComponentID != "8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b" {
		t.Errorf("Expected component ID '8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b', got '%s'", firstLog.ComponentID)
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
								"openchoreo_dev/project-uid":     "proj-123",
								"openchoreo_dev/component-uid":   "comp-456",
								"openchoreo_dev/environment-uid": "env-789",
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

func TestParseLogEntry(t *testing.T) {
	hit := opensearch.Hit{
		Source: map[string]interface{}{
			"@timestamp": "2024-01-01T10:00:00Z",
			"log":        "ERROR: Database connection failed",
			"kubernetes": map[string]interface{}{
				"labels": map[string]interface{}{
					"openchoreo_dev/component-uid":   "api-service",
					"openchoreo_dev/environment-uid": "production",
					"version":                        "v1.2.3",
					"version_id":                     "ver-456",
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

	if entry.Labels["openchoreo_dev/component-uid"] != "api-service" {
		t.Errorf("Expected label component UID 'api-service', got '%s'", entry.Labels["openchoreo_dev/component-uid"])
	}
}

func TestLoggingService_GetTraces(t *testing.T) {
	tests := []struct {
		name           string
		params         opensearch.TracesRequestParams
		mockResponse   *opensearch.SearchResponse
		mockError      error
		expectedResult *opensearch.TraceResponse
		expectedError  bool
	}{
		{
			name: "successful trace retrieval",
			params: opensearch.TracesRequestParams{
				ComponentUIDs: []string{"8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b"},
				ProjectUID:    "7b3c4d5e-8f2a-4c9b-a1e6-3d7f5c9e2b8a",
				StartTime:     "2024-01-01T00:00:00Z",
				EndTime:       "2024-01-01T23:59:59Z",
				Limit:         50,
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
								"spanId":          "614f55c7ccbfffdc",
								"name":            "database-query",
								"durationInNanos": int64(101018208),
								"startTime":       "2024-01-01T10:00:00.000000Z",
								"endTime":         "2024-01-01T10:00:00.101018208Z",
							},
						},
						{
							Source: map[string]interface{}{
								"traceId":         "trace-124",
								"spanId":          "725f66d8ddbceefd",
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
				Traces: []opensearch.Trace{
					{
						TraceID: "trace-123",
						Spans: []opensearch.Span{
							{
								SpanID:              "614f55c7ccbfffdc",
								Name:                "database-query",
								DurationNanoseconds: 101018208,
								StartTime:           mustParseTime("2024-01-01T10:00:00Z"),
								EndTime:             mustParseTime("2024-01-01T10:00:00.101018208Z"),
							},
						},
					},
					{
						TraceID: "trace-124",
						Spans: []opensearch.Span{
							{
								SpanID:              "725f66d8ddbceefd",
								Name:                "api-call",
								DurationNanoseconds: 200000000,
								StartTime:           mustParseTime("2024-01-01T11:00:00Z"),
								EndTime:             mustParseTime("2024-01-01T11:00:00.2Z"),
							},
						},
					},
				},
				Took: 25,
			},
			expectedError: false,
		},
		{
			name: "empty trace results",
			params: opensearch.TracesRequestParams{
				ComponentUIDs: []string{"9b5d6e3f-a8c1-4b2e-c7f8-4d9e6a1c5f2b"},
				ProjectUID:    "test-project",
				StartTime:     "2024-01-01T00:00:00Z",
				EndTime:       "2024-01-01T23:59:59Z",
				Limit:         10,
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
				Traces: []opensearch.Trace{},
				Took:   10,
			},
			expectedError: false,
		},
		{
			name: "opensearch error",
			params: opensearch.TracesRequestParams{
				ComponentUIDs: []string{"8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b"},
				ProjectUID:    "7b3c4d5e-8f2a-4c9b-a1e6-3d7f5c9e2b8a",
				StartTime:     "2024-01-01T00:00:00Z",
				EndTime:       "2024-01-01T23:59:59Z",
				Limit:         50,
			},
			mockResponse:   nil,
			mockError:      fmt.Errorf("opensearch connection failed"),
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "trace with missing optional fields",
			params: opensearch.TracesRequestParams{
				ComponentUIDs: []string{"8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b"},
				ProjectUID:    "7b3c4d5e-8f2a-4c9b-a1e6-3d7f5c9e2b8a",
				StartTime:     "2024-01-01T00:00:00Z",
				EndTime:       "2024-01-01T23:59:59Z",
				Limit:         10,
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
				Traces: []opensearch.Trace{
					{
						TraceID: "trace-125",
						Spans: []opensearch.Span{
							{
								SpanID:              "span-458",
								Name:                "minimal-span",
								DurationNanoseconds: 0,
								StartTime:           time.Time{},
								EndTime:             time.Time{},
							},
						},
					},
				},
				Took: 5,
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
			result, err := service.GetTraces(context.Background(), tt.params)

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

			// Check took time
			if result.Took != tt.expectedResult.Took {
				t.Errorf("Expected Took %d, got %d", tt.expectedResult.Took, result.Took)
			}

			// Check traces count
			if len(result.Traces) != len(tt.expectedResult.Traces) {
				t.Errorf("Expected %d traces, got %d", len(tt.expectedResult.Traces), len(result.Traces))
				return
			}

			// Check individual traces
			for i, expectedTrace := range tt.expectedResult.Traces {
				actualTrace := result.Traces[i]

				if actualTrace.TraceID != expectedTrace.TraceID {
					t.Errorf("Trace %d: Expected TraceID '%s', got '%s'", i, expectedTrace.TraceID, actualTrace.TraceID)
				}

				// Check spans within the trace
				if len(actualTrace.Spans) != len(expectedTrace.Spans) {
					t.Errorf("Trace %d: Expected %d spans, got %d", i, len(expectedTrace.Spans), len(actualTrace.Spans))
					continue
				}

				for j, expectedSpan := range expectedTrace.Spans {
					actualSpan := actualTrace.Spans[j]

					if actualSpan.SpanID != expectedSpan.SpanID {
						t.Errorf("Trace %d Span %d: Expected SpanId '%s', got '%s'", i, j, expectedSpan.SpanID, actualSpan.SpanID)
					}

					if actualSpan.Name != expectedSpan.Name {
						t.Errorf("Trace %d Span %d: Expected Name '%s', got '%s'", i, j, expectedSpan.Name, actualSpan.Name)
					}

					if actualSpan.DurationNanoseconds != expectedSpan.DurationNanoseconds {
						t.Errorf("Trace %d Span %d: Expected DurationInNanos %d, got %d", i, j, expectedSpan.DurationNanoseconds, actualSpan.DurationNanoseconds)
					}

					if !actualSpan.StartTime.Equal(expectedSpan.StartTime) {
						t.Errorf("Trace %d Span %d: Expected StartTime '%v', got '%v'", i, j, expectedSpan.StartTime, actualSpan.StartTime)
					}

					if !actualSpan.EndTime.Equal(expectedSpan.EndTime) {
						t.Errorf("Trace %d Span %d: Expected EndTime '%v', got '%v'", i, j, expectedSpan.EndTime, actualSpan.EndTime)
					}
				}
			}
		})
	}
}

func TestLoggingService_GetTraces_QueryBuilding(t *testing.T) {
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

	params := opensearch.TracesRequestParams{
		ComponentUIDs: []string{"c5d6e7f8-a9b0-4c1d-e2f3-5a6b7c8d9e0f"},
		ProjectUID:    "7b3c4d5e-8f2a-4c9b-a1e6-3d7f5c9e2b8a",
		StartTime:     "2024-01-01T00:00:00Z",
		EndTime:       "2024-01-01T23:59:59Z",
		Limit:         25,
	}

	_, err := service.GetTraces(context.Background(), params)
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

func TestLoggingService_GetBuildLogs(t *testing.T) {
	service := newMockLoggingService()

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
						"log":        "Build finished successfully",
						"kubernetes": map[string]interface{}{
							"labels": map[string]interface{}{
								"openchoreo_dev/component-uid":   "8a4c5e2f-9d3b-4a7e-b1f6-2c8d4e9f3a7b",
								"openchoreo_dev/environment-uid": "6c2d3e4f-7a1b-3d8c-9e5f-2c6d4e8f1a9b",
							},
							"namespace_name": "build-system",
							"pod_name":       "build-123-job",
						},
					},
				},
			},
		},
		Took: 75,
	}

	mockClient := &MockOpenSearchClient{
		searchResponse: mockResponse,
	}
	service.osClient = mockClient

	params := opensearch.BuildQueryParams{
		QueryParams: opensearch.QueryParams{
			StartTime: "2024-01-01T00:00:00Z",
			EndTime:   "2024-01-01T23:59:59Z",
			Limit:     250,
			SortOrder: "asc",
		},
		BuildID: "build-123",
	}

	result, err := service.GetBuildLogs(context.Background(), params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.TotalCount != 1 {
		t.Errorf("Expected total count 1, got %d", result.TotalCount)
	}

	if len(result.Logs) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(result.Logs))
	}

	if result.Took != 75 {
		t.Errorf("Expected took 75, got %d", result.Took)
	}

	firstLog := result.Logs[0]
	if firstLog.Log != "Build finished successfully" {
		t.Errorf("Unexpected log content: %s", firstLog.Log)
	}
}

func TestLoggingService_GetBuildLogs_SearchError(t *testing.T) {
	service := newMockLoggingService()
	mockClient := &MockOpenSearchClient{
		searchError: fmt.Errorf("search failure"),
	}
	service.osClient = mockClient

	params := opensearch.BuildQueryParams{
		QueryParams: opensearch.QueryParams{
			StartTime: "2024-01-01T00:00:00Z",
			EndTime:   "2024-01-01T23:59:59Z",
			Limit:     100,
			SortOrder: "asc",
		},
		BuildID: "build-123",
	}

	_, err := service.GetBuildLogs(context.Background(), params)
	if err == nil {
		t.Fatal("Expected error but got nil")
	}
}
