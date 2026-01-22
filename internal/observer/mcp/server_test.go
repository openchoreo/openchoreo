// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

const (
	testComponentID     = "comp-123"
	testEnvironmentID   = "env-dev"
	testProjectID       = "proj-456"
	testOrganizationID  = "org-789"
	testNamespace       = "default"
	testServiceName     = "test-service"
	testStartTime       = "2025-01-01T00:00:00Z"
	testEndTime         = "2025-01-01T23:59:59Z"
	testLogsResponse    = `{"logs":[{"message":"test log"}],"total":1}`
	testGatewayResponse = `{"logs":[{"message":"gateway log"}],"total":1}`
	testTracesResponse  = `{"spans":[{"traceId":"trace-123","spanId":"span-456"}],"totalCount":1}`
	testMetricsResponse = `{"cpuUsage":[{"timestamp":"2025-01-01T00:00:00Z","value":0.5}],"memory":[]}`
	sortOrderDesc       = "desc"
)

// MockHandler implements Handler interface for testing
type MockHandler struct {
	// Track which methods were called and with what parameters
	calls map[string][]interface{}

	// Error injection for testing error propagation
	componentLogsError            error
	projectLogsError              error
	gatewayLogsError              error
	organizationLogsError         error
	tracesError                   error
	componentResourceMetricsError error
}

func NewMockHandler() *MockHandler {
	return &MockHandler{
		calls: make(map[string][]interface{}),
	}
}

func (m *MockHandler) recordCall(method string, args ...interface{}) {
	if m.calls == nil {
		m.calls = make(map[string][]interface{})
	}
	m.calls[method] = append(m.calls[method], args)
}

func (m *MockHandler) GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (any, error) {
	m.recordCall("GetComponentLogs", params)
	if m.componentLogsError != nil {
		return nil, m.componentLogsError
	}
	// Parse the test response JSON to match the expected structure
	var logsData map[string]interface{}
	if err := json.Unmarshal([]byte(testLogsResponse), &logsData); err != nil {
		return nil, err
	}
	return logsData, nil
}

func (m *MockHandler) GetProjectLogs(ctx context.Context, params opensearch.QueryParams) (any, error) {
	m.recordCall("GetProjectLogs", params)
	if m.projectLogsError != nil {
		return nil, m.projectLogsError
	}
	var logsData map[string]interface{}
	if err := json.Unmarshal([]byte(testLogsResponse), &logsData); err != nil {
		return nil, err
	}
	return logsData, nil
}

func (m *MockHandler) GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (any, error) {
	m.recordCall("GetGatewayLogs", params)
	if m.gatewayLogsError != nil {
		return nil, m.gatewayLogsError
	}
	var logsData map[string]interface{}
	if err := json.Unmarshal([]byte(testGatewayResponse), &logsData); err != nil {
		return nil, err
	}
	return logsData, nil
}

func (m *MockHandler) GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (any, error) {
	m.recordCall("GetOrganizationLogs", params, podLabels)
	if m.organizationLogsError != nil {
		return nil, m.organizationLogsError
	}
	var logsData map[string]interface{}
	if err := json.Unmarshal([]byte(testLogsResponse), &logsData); err != nil {
		return nil, err
	}
	return logsData, nil
}

func (m *MockHandler) GetTraces(ctx context.Context, params opensearch.TracesRequestParams) (any, error) {
	m.recordCall("GetTraces", params)
	if m.tracesError != nil {
		return nil, m.tracesError
	}
	var tracesData map[string]interface{}
	if err := json.Unmarshal([]byte(testTracesResponse), &tracesData); err != nil {
		return nil, err
	}
	return tracesData, nil
}

func (m *MockHandler) GetComponentResourceMetrics(ctx context.Context, componentID, environmentID, projectID, startTime, endTime string) (any, error) {
	m.recordCall("GetComponentResourceMetrics", componentID, environmentID, projectID, startTime, endTime)
	if m.componentResourceMetricsError != nil {
		return nil, m.componentResourceMetricsError
	}
	var metricsData map[string]interface{}
	if err := json.Unmarshal([]byte(testMetricsResponse), &metricsData); err != nil {
		return nil, err
	}
	return metricsData, nil
}

func (m *MockHandler) GetComponentHTTPMetrics(ctx context.Context, componentID, environmentID, projectID, startTime, endTime string) (any, error) {
	m.recordCall("GetComponentHTTPMetrics", componentID, environmentID, projectID, startTime, endTime)
	var metricsData map[string]interface{}
	if err := json.Unmarshal([]byte(testMetricsResponse), &metricsData); err != nil {
		return nil, err
	}
	return metricsData, nil
}

// setupTestServer creates a test MCP server with mock handler
func setupTestServer(t *testing.T) (*mcp.ClientSession, *MockHandler) {
	t.Helper()

	mockHandler := NewMockHandler()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-openchoreo-observer",
		Version: "1.0.0",
	}, nil)

	registerTools(server, mockHandler)

	// Create client connection
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect server: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}

	return clientSession, mockHandler
}

// toolTestSpec defines the complete test specification for a single MCP tool
type toolTestSpec struct {
	name string

	// Description validation
	descriptionKeywords []string
	descriptionMinLen   int

	// Schema validation
	requiredParams []string
	optionalParams []string

	// Parameter wiring test
	testArgs       map[string]any
	expectedMethod string
	validateCall   func(t *testing.T, args []interface{})
}

// allToolSpecs defines the complete test specification for all observer MCP tools
var allToolSpecs = []toolTestSpec{
	{
		name:                "get_component_logs",
		descriptionKeywords: []string{"component", "logs"},
		descriptionMinLen:   20,
		requiredParams:      []string{"component_id", "environment_id", "start_time", "end_time"},
		optionalParams:      []string{"namespace", "search_phrase", "log_levels", "versions", "version_ids", "limit", "sort_order", "log_type", "build_id", "build_uuid"},
		testArgs: map[string]any{
			"component_id":   testComponentID,
			"environment_id": testEnvironmentID,
			"namespace":      testNamespace,
			"start_time":     testStartTime,
			"end_time":       testEndTime,
			"search_phrase":  "error",
			"log_levels":     []interface{}{"ERROR", "WARN"},
			"versions":       []interface{}{"v1.0.0"},
			"version_ids":    []interface{}{"vid-123"},
			"limit":          100,
			"sort_order":     sortOrderDesc,
			"log_type":       "application",
			"build_id":       "build-123",
			"build_uuid":     "uuid-456",
		},
		expectedMethod: "GetComponentLogs",
		validateCall: func(t *testing.T, args []interface{}) {
			if len(args) == 0 {
				t.Fatal("Expected at least one argument")
			}
			params, ok := args[0].(opensearch.ComponentQueryParams)
			if !ok {
				t.Fatalf("Expected ComponentQueryParams, got %T", args[0])
			}
			if params.ComponentID != testComponentID {
				t.Errorf("Expected component_id %q, got %q", testComponentID, params.ComponentID)
			}
			if params.EnvironmentID != testEnvironmentID {
				t.Errorf("Expected environment_id %q, got %q", testEnvironmentID, params.EnvironmentID)
			}
			if params.Namespace != testNamespace {
				t.Errorf("Expected namespace %q, got %q", testNamespace, params.Namespace)
			}
			if params.StartTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, params.StartTime)
			}
			if params.EndTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, params.EndTime)
			}
			if params.SearchPhrase != "error" {
				t.Errorf("Expected search_phrase 'error', got %q", params.SearchPhrase)
			}
			expectedLevels := []string{"ERROR", "WARN"}
			if diff := cmp.Diff(expectedLevels, params.LogLevels); diff != "" {
				t.Errorf("log_levels mismatch (-want +got):\n%s", diff)
			}
			expectedVersions := []string{"v1.0.0"}
			if diff := cmp.Diff(expectedVersions, params.Versions); diff != "" {
				t.Errorf("versions mismatch (-want +got):\n%s", diff)
			}
			expectedVersionIDs := []string{"vid-123"}
			if diff := cmp.Diff(expectedVersionIDs, params.VersionIDs); diff != "" {
				t.Errorf("version_ids mismatch (-want +got):\n%s", diff)
			}
			if params.Limit != 100 {
				t.Errorf("Expected limit 100, got %d", params.Limit)
			}
			if params.SortOrder != sortOrderDesc {
				t.Errorf("Expected sort_order %q, got %q", sortOrderDesc, params.SortOrder)
			}
			if params.LogType != "application" {
				t.Errorf("Expected log_type 'application', got %q", params.LogType)
			}
			if params.BuildID != "build-123" {
				t.Errorf("Expected build_id 'build-123', got %q", params.BuildID)
			}
			if params.BuildUUID != "uuid-456" {
				t.Errorf("Expected build_uuid 'uuid-456', got %q", params.BuildUUID)
			}
		},
	},
	{
		name:                "get_project_logs",
		descriptionKeywords: []string{"project", "logs"},
		descriptionMinLen:   20,
		requiredParams:      []string{"project_id", "environment_id", "start_time", "end_time"},
		optionalParams:      []string{"component_ids", "search_phrase", "log_levels", "limit", "sort_order"},
		testArgs: map[string]any{
			"project_id":     testProjectID,
			"environment_id": testEnvironmentID,
			"start_time":     testStartTime,
			"end_time":       testEndTime,
			"component_ids":  []interface{}{"comp-1", "comp-2"},
			"search_phrase":  "warning",
			"log_levels":     []interface{}{"WARN"},
			"limit":          50,
			"sort_order":     "asc",
		},
		expectedMethod: "GetProjectLogs",
		validateCall: func(t *testing.T, args []interface{}) {
			if len(args) < 1 {
				t.Fatal("Expected at least one argument")
			}
			params, ok := args[0].(opensearch.QueryParams)
			if !ok {
				t.Fatalf("Expected QueryParams, got %T", args[0])
			}
			if params.ProjectID != testProjectID {
				t.Errorf("Expected project_id %q, got %q", testProjectID, params.ProjectID)
			}
			if params.EnvironmentID != testEnvironmentID {
				t.Errorf("Expected environment_id %q, got %q", testEnvironmentID, params.EnvironmentID)
			}
			if params.StartTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, params.StartTime)
			}
			if params.EndTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, params.EndTime)
			}
			if params.SearchPhrase != "warning" {
				t.Errorf("Expected search_phrase 'warning', got %q", params.SearchPhrase)
			}
			expectedLevels := []string{"WARN"}
			if diff := cmp.Diff(expectedLevels, params.LogLevels); diff != "" {
				t.Errorf("log_levels mismatch (-want +got):\n%s", diff)
			}
			if params.Limit != 50 {
				t.Errorf("Expected limit 50, got %d", params.Limit)
			}
			if params.SortOrder != "asc" {
				t.Errorf("Expected sort_order 'asc', got %q", params.SortOrder)
			}
			expectedComponentIDs := []string{"comp-1", "comp-2"}
			if diff := cmp.Diff(expectedComponentIDs, params.ComponentIDs); diff != "" {
				t.Errorf("component_ids mismatch (-want +got):\n%s", diff)
			}
		},
	},
	{
		name:                "get_gateway_logs",
		descriptionKeywords: []string{"gateway", "logs"},
		descriptionMinLen:   20,
		requiredParams:      []string{"organization_id", "start_time", "end_time"},
		optionalParams:      []string{"search_phrase", "api_id_to_version_map", "gateway_vhosts", "limit", "sort_order", "log_type"},
		testArgs: map[string]any{
			"organization_id": testOrganizationID,
			"start_time":      testStartTime,
			"end_time":        testEndTime,
			"search_phrase":   "api",
			"api_id_to_version_map": map[string]interface{}{
				"api-1": "v1",
				"api-2": "v2",
			},
			"gateway_vhosts": []interface{}{"vhost1.example.com", "vhost2.example.com"},
			"limit":          200,
			"sort_order":     sortOrderDesc,
			"log_type":       "gateway",
		},
		expectedMethod: "GetGatewayLogs",
		validateCall: func(t *testing.T, args []interface{}) {
			if len(args) == 0 {
				t.Fatal("Expected at least one argument")
			}
			params, ok := args[0].(opensearch.GatewayQueryParams)
			if !ok {
				t.Fatalf("Expected GatewayQueryParams, got %T", args[0])
			}
			// The server sets OrganizationID in QueryParams.OrganizationID, not at the struct level
			if params.QueryParams.OrganizationID != testOrganizationID {
				t.Errorf("Expected organization_id %q, got %q", testOrganizationID, params.QueryParams.OrganizationID)
			}
			if params.QueryParams.StartTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, params.QueryParams.StartTime)
			}
			if params.QueryParams.EndTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, params.QueryParams.EndTime)
			}
			if params.QueryParams.SearchPhrase != "api" {
				t.Errorf("Expected search_phrase 'api', got %q", params.QueryParams.SearchPhrase)
			}
			expectedAPIMap := map[string]string{
				"api-1": "v1",
				"api-2": "v2",
			}
			if diff := cmp.Diff(expectedAPIMap, params.APIIDToVersionMap); diff != "" {
				t.Errorf("api_id_to_version_map mismatch (-want +got):\n%s", diff)
			}
			expectedVHosts := []string{"vhost1.example.com", "vhost2.example.com"}
			if diff := cmp.Diff(expectedVHosts, params.GatewayVHosts); diff != "" {
				t.Errorf("gateway_vhosts mismatch (-want +got):\n%s", diff)
			}
			if params.QueryParams.Limit != 200 {
				t.Errorf("Expected limit 200, got %d", params.QueryParams.Limit)
			}
			if params.QueryParams.SortOrder != sortOrderDesc {
				t.Errorf("Expected sort_order %q, got %q", sortOrderDesc, params.QueryParams.SortOrder)
			}
			if params.QueryParams.LogType != "gateway" {
				t.Errorf("Expected log_type 'gateway', got %q", params.QueryParams.LogType)
			}
		},
	},
	{
		name:                "get_organization_logs",
		descriptionKeywords: []string{"organization", "logs"},
		descriptionMinLen:   20,
		requiredParams:      []string{"organization_id", "environment_id", "start_time", "end_time"},
		optionalParams:      []string{"pod_labels", "search_phrase", "log_levels", "limit", "sort_order"},
		testArgs: map[string]any{
			"organization_id": testOrganizationID,
			"environment_id":  testEnvironmentID,
			"start_time":      testStartTime,
			"end_time":        testEndTime,
			"pod_labels": map[string]interface{}{
				"app":     "myapp",
				"version": "v1.0",
			},
			"search_phrase": "critical",
			"log_levels":    []interface{}{"ERROR", "FATAL"},
			"limit":         150,
			"sort_order":    sortOrderDesc,
		},
		expectedMethod: "GetOrganizationLogs",
		validateCall: func(t *testing.T, args []interface{}) {
			if len(args) < 2 {
				t.Fatal("Expected at least two arguments")
			}
			params, ok := args[0].(opensearch.QueryParams)
			if !ok {
				t.Fatalf("Expected QueryParams, got %T", args[0])
			}
			podLabels, ok := args[1].(map[string]string)
			if !ok {
				t.Fatalf("Expected map[string]string for pod_labels, got %T", args[1])
			}
			if params.OrganizationID != testOrganizationID {
				t.Errorf("Expected organization_id %q, got %q", testOrganizationID, params.OrganizationID)
			}
			if params.EnvironmentID != testEnvironmentID {
				t.Errorf("Expected environment_id %q, got %q", testEnvironmentID, params.EnvironmentID)
			}
			if params.StartTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, params.StartTime)
			}
			if params.EndTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, params.EndTime)
			}
			if params.SearchPhrase != "critical" {
				t.Errorf("Expected search_phrase 'critical', got %q", params.SearchPhrase)
			}
			expectedLevels := []string{"ERROR", "FATAL"}
			if diff := cmp.Diff(expectedLevels, params.LogLevels); diff != "" {
				t.Errorf("log_levels mismatch (-want +got):\n%s", diff)
			}
			if params.Limit != 150 {
				t.Errorf("Expected limit 150, got %d", params.Limit)
			}
			if params.SortOrder != sortOrderDesc {
				t.Errorf("Expected sort_order %q, got %q", sortOrderDesc, params.SortOrder)
			}
			expectedPodLabels := map[string]string{
				"app":     "myapp",
				"version": "v1.0",
			}
			if diff := cmp.Diff(expectedPodLabels, podLabels); diff != "" {
				t.Errorf("pod_labels mismatch (-want +got):\n%s", diff)
			}
		},
	},
	{
		name:                "get_traces",
		descriptionKeywords: []string{"traces", "component"},
		descriptionMinLen:   20,
		requiredParams:      []string{"project_uid", "start_time", "end_time"},
		optionalParams:      []string{"component_uids", "environment_uid", "trace_id", "limit", "sort_order"},
		testArgs: map[string]any{
			"project_uid":     testProjectID,
			"component_uids":  []string{testComponentID},
			"environment_uid": testEnvironmentID,
			"start_time":      testStartTime,
			"end_time":        testEndTime,
			"limit":           50,
			"sort_order":      "asc",
		},
		expectedMethod: "GetTraces",
		validateCall: func(t *testing.T, args []interface{}) {
			if len(args) == 0 {
				t.Fatal("Expected at least one argument")
			}
			params, ok := args[0].(opensearch.TracesRequestParams)
			if !ok {
				t.Fatalf("Expected TracesRequestParams, got %T", args[0])
			}
			if params.ProjectUID != testProjectID {
				t.Errorf("Expected project_uid %q, got %q", testProjectID, params.ProjectUID)
			}
			if len(params.ComponentUIDs) != 1 || params.ComponentUIDs[0] != testComponentID {
				t.Errorf("Expected component_uids [%q], got %v", testComponentID, params.ComponentUIDs)
			}
			if params.EnvironmentUID != testEnvironmentID {
				t.Errorf("Expected environment_uid %q, got %q", testEnvironmentID, params.EnvironmentUID)
			}
			if params.StartTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, params.StartTime)
			}
			if params.EndTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, params.EndTime)
			}
			if params.Limit != 50 {
				t.Errorf("Expected limit 50, got %d", params.Limit)
			}
			if params.SortOrder != "asc" {
				t.Errorf("Expected sort_order 'asc', got %q", params.SortOrder)
			}
		},
	},
	{
		name:                "get_component_resource_metrics",
		descriptionKeywords: []string{"metrics", "resource"},
		descriptionMinLen:   20,
		requiredParams:      []string{"project_id", "environment_id", "start_time", "end_time"},
		optionalParams:      []string{"component_id"},
		testArgs: map[string]any{
			"component_id":   testComponentID,
			"project_id":     testProjectID,
			"environment_id": testEnvironmentID,
			"start_time":     testStartTime,
			"end_time":       testEndTime,
		},
		expectedMethod: "GetComponentResourceMetrics",
		validateCall: func(t *testing.T, args []interface{}) {
			if len(args) < 5 {
				t.Fatalf("Expected at least 5 arguments, got %d", len(args))
			}
			componentID, ok := args[0].(string)
			if !ok {
				t.Fatalf("Expected string for component_id, got %T", args[0])
			}
			environmentID, ok := args[1].(string)
			if !ok {
				t.Fatalf("Expected string for environment_id, got %T", args[1])
			}
			projectID, ok := args[2].(string)
			if !ok {
				t.Fatalf("Expected string for project_id, got %T", args[2])
			}
			startTime, ok := args[3].(string)
			if !ok {
				t.Fatalf("Expected string for start_time, got %T", args[3])
			}
			endTime, ok := args[4].(string)
			if !ok {
				t.Fatalf("Expected string for end_time, got %T", args[4])
			}
			if componentID != testComponentID {
				t.Errorf("Expected component_id %q, got %q", testComponentID, componentID)
			}
			if environmentID != testEnvironmentID {
				t.Errorf("Expected environment_id %q, got %q", testEnvironmentID, environmentID)
			}
			if projectID != testProjectID {
				t.Errorf("Expected project_id %q, got %q", testProjectID, projectID)
			}
			if startTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, startTime)
			}
			if endTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, endTime)
			}
		},
	},
	{
		name:                "get_component_http_metrics",
		descriptionKeywords: []string{"metrics", "HTTP"},
		descriptionMinLen:   20,
		requiredParams:      []string{"project_id", "environment_id", "start_time", "end_time"},
		optionalParams:      []string{"component_id"},
		testArgs: map[string]any{
			"component_id":   testComponentID,
			"project_id":     testProjectID,
			"environment_id": testEnvironmentID,
			"start_time":     testStartTime,
			"end_time":       testEndTime,
		},
		expectedMethod: "GetComponentHTTPMetrics",
		validateCall: func(t *testing.T, args []interface{}) {
			if len(args) < 5 {
				t.Fatalf("Expected at least 5 arguments, got %d", len(args))
			}
			componentID, ok := args[0].(string)
			if !ok {
				t.Fatalf("Expected string for component_id, got %T", args[0])
			}
			environmentID, ok := args[1].(string)
			if !ok {
				t.Fatalf("Expected string for environment_id, got %T", args[1])
			}
			projectID, ok := args[2].(string)
			if !ok {
				t.Fatalf("Expected string for project_id, got %T", args[2])
			}
			startTime, ok := args[3].(string)
			if !ok {
				t.Fatalf("Expected string for start_time, got %T", args[3])
			}
			endTime, ok := args[4].(string)
			if !ok {
				t.Fatalf("Expected string for end_time, got %T", args[4])
			}
			if componentID != testComponentID {
				t.Errorf("Expected component_id %q, got %q", testComponentID, componentID)
			}
			if environmentID != testEnvironmentID {
				t.Errorf("Expected environment_id %q, got %q", testEnvironmentID, environmentID)
			}
			if projectID != testProjectID {
				t.Errorf("Expected project_id %q, got %q", testProjectID, projectID)
			}
			if startTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, startTime)
			}
			if endTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, endTime)
			}
		},
	},
}

// TestToolRegistration verifies that all expected tools are registered
func TestToolRegistration(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	// Build expected tool names from allToolSpecs
	expectedTools := make(map[string]bool)
	for _, spec := range allToolSpecs {
		expectedTools[spec.name] = true
	}

	// Check all expected tools are present
	registeredTools := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		registeredTools[tool.Name] = true
		if !expectedTools[tool.Name] {
			t.Errorf("Unexpected tool %q found in registered tools", tool.Name)
		}
	}

	// Check no tools are missing
	for expected := range expectedTools {
		if !registeredTools[expected] {
			t.Errorf("Expected tool %q not found in registered tools", expected)
		}
	}

	if len(toolsResult.Tools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(toolsResult.Tools))
	}
}

// TestToolDescriptions verifies that tool descriptions are meaningful and distinguishable
func TestToolDescriptions(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	toolsByName := make(map[string]*mcp.Tool)
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	// Test each tool's description using specs from allToolSpecs
	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			if !exists {
				t.Fatalf("Tool %q not found", spec.name)
			}

			desc := strings.ToLower(tool.Description)

			// Check minimum length
			if len(desc) < spec.descriptionMinLen {
				t.Errorf("Description too short: got %d chars, want at least %d", len(desc), spec.descriptionMinLen)
			}

			// Check for required keywords
			for _, word := range spec.descriptionKeywords {
				if !strings.Contains(desc, strings.ToLower(word)) {
					t.Errorf("Description missing required keyword %q: %s", word, tool.Description)
				}
			}
		})
	}

	// Ensure descriptions are unique across all tools
	descriptions := make(map[string]string)
	for _, tool := range toolsResult.Tools {
		if existingTool, exists := descriptions[tool.Description]; exists {
			t.Errorf("Duplicate description found: %q used by both %q and %q",
				tool.Description, tool.Name, existingTool)
		}
		descriptions[tool.Description] = tool.Name
	}
}

// TestToolSchemas verifies that tool input schemas have required properties defined
func TestToolSchemas(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	toolsByName := make(map[string]*mcp.Tool)
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	// Test each tool's schema using specs from allToolSpecs
	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			if !exists {
				t.Fatalf("Tool %q not found", spec.name)
			}

			if tool.InputSchema == nil {
				t.Fatal("InputSchema is nil")
			}

			// Convert InputSchema to map for inspection
			schemaMap, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("Expected InputSchema to be map[string]any, got %T", tool.InputSchema)
			}

			// Verify schema type is object
			schemaType, ok := schemaMap["type"].(string)
			if !ok || schemaType != "object" {
				t.Errorf("Expected schema type 'object', got %v", schemaMap["type"])
			}

			// Check required parameters
			if len(spec.requiredParams) > 0 {
				requiredInSchema := make(map[string]bool)
				if requiredList, ok := schemaMap["required"].([]interface{}); ok {
					for _, req := range requiredList {
						if reqStr, ok := req.(string); ok {
							requiredInSchema[reqStr] = true
						}
					}
				}

				for _, param := range spec.requiredParams {
					if !requiredInSchema[param] {
						t.Errorf("Required parameter %q not found in schema.required", param)
					}
				}
			}

			// Check that all parameters (required and optional) are in properties
			allParams := make([]string, len(spec.requiredParams))
			copy(allParams, spec.requiredParams)
			allParams = append(allParams, spec.optionalParams...)
			if len(allParams) > 0 {
				properties, ok := schemaMap["properties"].(map[string]any)
				if !ok {
					t.Fatal("Properties is not a map")
				}
				for _, param := range allParams {
					if _, exists := properties[param]; !exists {
						t.Errorf("Parameter %q not found in schema.properties", param)
					}
				}
			}
		})
	}
}

// TestToolParameterWiring verifies that parameters are correctly passed to handlers
func TestToolParameterWiring(t *testing.T) {
	clientSession, mockHandler := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	// Test each tool's parameter wiring using specs from allToolSpecs
	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			// Clear previous calls
			mockHandler.calls = make(map[string][]interface{})

			result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      spec.name,
				Arguments: spec.testArgs,
			})
			if err != nil {
				t.Fatalf("Failed to call tool: %v", err)
			}

			// Verify result is not empty
			if len(result.Content) == 0 {
				t.Fatal("Expected non-empty result content")
			}

			// Verify the correct handler method was called
			calls, ok := mockHandler.calls[spec.expectedMethod]
			if !ok {
				t.Fatalf("Expected method %q was not called. Available calls: %v",
					spec.expectedMethod, mockHandler.calls)
			}

			if len(calls) != 1 {
				t.Fatalf("Expected 1 call to %q, got %d", spec.expectedMethod, len(calls))
			}

			// Validate the call parameters using the spec's custom validator
			args := calls[0].([]interface{})
			spec.validateCall(t, args)
		})
	}
}

// TestToolResponseFormat verifies that tool responses are valid JSON
func TestToolResponseFormat(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	// Test with a single tool - response format is consistent across all tools
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_component_logs",
		Arguments: map[string]any{
			"component_id":   testComponentID,
			"environment_id": testEnvironmentID,
			"namespace":      testNamespace,
			"start_time":     testStartTime,
			"end_time":       testEndTime,
		},
	})
	if err != nil {
		t.Fatalf("Failed to call tool: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}

	// Get the text content
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("Expected TextContent")
	}

	// Verify the response is valid JSON
	var data interface{}
	if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
		t.Errorf("Response is not valid JSON: %v\nResponse: %s", err, textContent.Text)
	}
}

// TestToolErrorHandling verifies that the MCP SDK validates required parameters
func TestToolErrorHandling(t *testing.T) {
	clientSession, mockHandler := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	// Find a tool with required parameters from allToolSpecs
	var testSpec toolTestSpec
	for _, spec := range allToolSpecs {
		if len(spec.requiredParams) > 0 {
			testSpec = spec
			break
		}
	}

	if testSpec.name == "" {
		t.Fatal("No tool with required parameters found in allToolSpecs")
	}

	// Clear mock handler calls
	mockHandler.calls = make(map[string][]interface{})

	// Try calling the tool with missing required parameter
	_, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      testSpec.name,
		Arguments: map[string]any{}, // Empty arguments - missing required params
	})

	// We expect an error for missing required parameters
	if err == nil {
		t.Errorf("Expected error for tool %q with missing required parameters, got nil", testSpec.name)
	}

	// Verify the handler was NOT called (validation should fail before reaching handler)
	if len(mockHandler.calls) > 0 {
		t.Errorf("Handler should not be called when parameters are invalid, but got calls: %v", mockHandler.calls)
	}
}

// TestMinimalParameterSets verifies that tools work with only required parameters
func TestMinimalParameterSets(t *testing.T) {
	clientSession, mockHandler := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	minimalTests := []struct {
		name     string
		toolName string
		args     map[string]any
	}{
		{
			name:     "get_component_logs_minimal",
			toolName: "get_component_logs",
			args: map[string]any{
				"component_id":   testComponentID,
				"environment_id": testEnvironmentID,
				"namespace":      testNamespace,
				"start_time":     testStartTime,
				"end_time":       testEndTime,
			},
		},
		{
			name:     "get_project_logs_minimal",
			toolName: "get_project_logs",
			args: map[string]any{
				"project_id":     testProjectID,
				"environment_id": testEnvironmentID,
				"start_time":     testStartTime,
				"end_time":       testEndTime,
			},
		},
		{
			name:     "get_gateway_logs_minimal",
			toolName: "get_gateway_logs",
			args: map[string]any{
				"organization_id": testOrganizationID,
				"start_time":      testStartTime,
				"end_time":        testEndTime,
			},
		},
		{
			name:     "get_organization_logs_minimal",
			toolName: "get_organization_logs",
			args: map[string]any{
				"organization_id": testOrganizationID,
				"environment_id":  testEnvironmentID,
				"start_time":      testStartTime,
				"end_time":        testEndTime,
			},
		},
		{
			name:     "get_traces_minimal",
			toolName: "get_traces",
			args: map[string]any{
				"project_uid": testProjectID,
				"start_time":  testStartTime,
				"end_time":    testEndTime,
			},
		},
		{
			name:     "get_component_resource_metrics_minimal",
			toolName: "get_component_resource_metrics",
			args: map[string]any{
				"project_id":     testProjectID,
				"environment_id": testEnvironmentID,
				"start_time":     testStartTime,
				"end_time":       testEndTime,
			},
		},
	}

	for _, tt := range minimalTests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous calls
			mockHandler.calls = make(map[string][]interface{})

			result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.toolName,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("Failed to call tool with minimal parameters: %v", err)
			}

			// Verify result is not empty
			if len(result.Content) == 0 {
				t.Fatal("Expected non-empty result content")
			}

			// Verify the response is valid JSON
			textContent, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatal("Expected TextContent")
			}

			var data interface{}
			if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
				t.Errorf("Response is not valid JSON: %v", err)
			}

			// Verify handler was called
			if len(mockHandler.calls) == 0 {
				t.Error("Expected handler to be called")
			}
		})
	}
}

// TestHandlerErrorPropagation verifies that errors from handlers are properly propagated
func TestHandlerErrorPropagation(t *testing.T) {
	ctx := context.Background()

	errorTests := []struct {
		name     string
		toolName string
		args     map[string]any
		setupErr func(*MockHandler)
	}{
		{
			name:     "get_component_logs_error",
			toolName: "get_component_logs",
			args: map[string]any{
				"component_id":   testComponentID,
				"environment_id": testEnvironmentID,
				"namespace":      testNamespace,
				"start_time":     testStartTime,
				"end_time":       testEndTime,
			},
			setupErr: func(h *MockHandler) {
				h.componentLogsError = errors.New("connection failed")
			},
		},
		{
			name:     "get_project_logs_error",
			toolName: "get_project_logs",
			args: map[string]any{
				"project_id":     testProjectID,
				"environment_id": testEnvironmentID,
				"start_time":     testStartTime,
				"end_time":       testEndTime,
			},
			setupErr: func(h *MockHandler) {
				h.projectLogsError = errors.New("query failed")
			},
		},
		{
			name:     "get_gateway_logs_error",
			toolName: "get_gateway_logs",
			args: map[string]any{
				"organization_id": testOrganizationID,
				"start_time":      testStartTime,
				"end_time":        testEndTime,
			},
			setupErr: func(h *MockHandler) {
				h.gatewayLogsError = errors.New("invalid time range")
			},
		},
		{
			name:     "get_organization_logs_error",
			toolName: "get_organization_logs",
			args: map[string]any{
				"organization_id": testOrganizationID,
				"environment_id":  testEnvironmentID,
				"start_time":      testStartTime,
				"end_time":        testEndTime,
			},
			setupErr: func(h *MockHandler) {
				h.organizationLogsError = errors.New("unauthorized")
			},
		},
		{
			name:     "get_traces_error",
			toolName: "get_traces",
			args: map[string]any{
				"project_uid": testProjectID,
				"start_time":  testStartTime,
				"end_time":    testEndTime,
			},
			setupErr: func(h *MockHandler) {
				h.tracesError = errors.New("trace service unavailable")
			},
		},
		{
			name:     "get_component_resource_metrics_error",
			toolName: "get_component_resource_metrics",
			args: map[string]any{
				"project_id":     testProjectID,
				"environment_id": testEnvironmentID,
				"start_time":     testStartTime,
				"end_time":       testEndTime,
			},
			setupErr: func(h *MockHandler) {
				h.componentResourceMetricsError = errors.New("prometheus unavailable")
			},
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh mock handler for this test
			mockHandler := NewMockHandler()
			tt.setupErr(mockHandler)

			server := mcp.NewServer(&mcp.Implementation{
				Name:    "test-openchoreo-observer",
				Version: "1.0.0",
			}, nil)

			registerTools(server, mockHandler)

			// Create client connection
			clientTransport, serverTransport := mcp.NewInMemoryTransports()

			_, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatalf("Failed to connect server: %v", err)
			}

			client := mcp.NewClient(&mcp.Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			}, nil)

			clientSession, err := client.Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatalf("Failed to connect client: %v", err)
			}
			defer clientSession.Close()

			result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.toolName,
				Arguments: tt.args,
			})

			// We expect an error to be returned
			// Note: MCP SDK wraps errors in the result, not as Go errors
			if err == nil && (result == nil || !result.IsError) {
				t.Errorf("Expected error from handler, got err=%v, result.IsError=%v", err, result != nil && result.IsError)
			}

			// Verify the handler was called
			if len(mockHandler.calls) == 0 {
				t.Error("Handler was not called")
			}
		})
	}
}

// TestNewHTTPServer verifies that the HTTP server is created correctly
func TestNewHTTPServer(t *testing.T) {
	mockHandler := NewMockHandler()
	handler := NewHTTPServer(mockHandler)

	if handler == nil {
		t.Fatal("Expected non-nil HTTP handler")
	}

	// Verify it's actually an http.Handler
	var _ http.Handler = handler
}

// TestOptionalParametersDefaults verifies that optional parameters have sensible defaults
func TestOptionalParametersDefaults(t *testing.T) {
	clientSession, mockHandler := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	tests := []struct {
		name         string
		toolName     string
		args         map[string]any
		validateCall func(t *testing.T, args []interface{})
	}{
		{
			name:     "component_logs_without_optional_params",
			toolName: "get_component_logs",
			args: map[string]any{
				"component_id":   testComponentID,
				"environment_id": testEnvironmentID,
				"namespace":      testNamespace,
				"start_time":     testStartTime,
				"end_time":       testEndTime,
			},
			validateCall: func(t *testing.T, args []interface{}) {
				params := args[0].(opensearch.ComponentQueryParams)
				// Verify optional params have default values
				if params.SearchPhrase != "" {
					t.Errorf("Expected empty search_phrase, got %q", params.SearchPhrase)
				}
				if len(params.LogLevels) != 0 {
					t.Errorf("Expected empty log_levels, got %v", params.LogLevels)
				}
				if params.Limit != 100 {
					t.Errorf("Expected default limit of 100, got %d", params.Limit)
				}
			},
		},
		{
			name:     "project_logs_without_component_ids",
			toolName: "get_project_logs",
			args: map[string]any{
				"project_id":     testProjectID,
				"environment_id": testEnvironmentID,
				"start_time":     testStartTime,
				"end_time":       testEndTime,
			},
			validateCall: func(t *testing.T, args []interface{}) {
				params := args[0].(opensearch.QueryParams)
				// Verify component_ids is nil/empty when not provided
				if len(params.ComponentIDs) != 0 {
					t.Errorf("Expected nil or empty component_ids, got %v", params.ComponentIDs)
				}
			},
		},
		{
			name:     "gateway_logs_without_optional_filters",
			toolName: "get_gateway_logs",
			args: map[string]any{
				"organization_id": testOrganizationID,
				"start_time":      testStartTime,
				"end_time":        testEndTime,
			},
			validateCall: func(t *testing.T, args []interface{}) {
				params := args[0].(opensearch.GatewayQueryParams)
				// Verify optional params are nil/empty
				if len(params.APIIDToVersionMap) != 0 {
					t.Errorf("Expected nil or empty api_id_to_version_map, got %v", params.APIIDToVersionMap)
				}
				if len(params.GatewayVHosts) != 0 {
					t.Errorf("Expected empty gateway_vhosts, got %v", params.GatewayVHosts)
				}
			},
		},
		{
			name:     "organization_logs_without_pod_labels",
			toolName: "get_organization_logs",
			args: map[string]any{
				"organization_id": testOrganizationID,
				"environment_id":  testEnvironmentID,
				"start_time":      testStartTime,
				"end_time":        testEndTime,
			},
			validateCall: func(t *testing.T, args []interface{}) {
				podLabels := args[1].(map[string]string)
				// Verify pod_labels is nil/empty when not provided
				if len(podLabels) != 0 {
					t.Errorf("Expected nil or empty pod_labels, got %v", podLabels)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHandler.calls = make(map[string][]interface{})

			_, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.toolName,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("Failed to call tool: %v", err)
			}

			// Find the first (and should be only) method call
			var callArgs []interface{}
			for _, calls := range mockHandler.calls {
				if len(calls) > 0 {
					callArgs = calls[0].([]interface{})
					break
				}
			}

			if callArgs == nil {
				t.Fatal("No handler method was called")
			}

			tt.validateCall(t, callArgs)
		})
	}
}

// TestParameterMappingRegression demonstrates that validateCall catches real implementation bugs
// This test intentionally shows what would fail if someone broke the parameter mapping
func TestParameterMappingRegression(t *testing.T) {
	clientSession, mockHandler := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	// Call the tool with specific arguments
	_, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_component_logs",
		Arguments: map[string]any{
			"component_id":   "test-comp-id",
			"environment_id": "test-env-id",
			"namespace":      "test-namespace",
			"start_time":     "2025-01-01T00:00:00Z",
			"end_time":       "2025-01-01T23:59:59Z",
			"search_phrase":  "my-search-term",
			"limit":          999,
		},
	})
	if err != nil {
		t.Fatalf("Failed to call tool: %v", err)
	}

	// Verify the handler received the EXACT values we sent
	// This proves we're testing the real parameter mapping code in server.go
	calls := mockHandler.calls["GetComponentLogs"]
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}

	args := calls[0].([]interface{})
	params := args[0].(opensearch.ComponentQueryParams)

	// If someone changed the mapping in server.go (e.g., swapped component_id with environment_id),
	// these assertions would fail, proving validateCall catches real bugs
	if params.ComponentID != "test-comp-id" {
		t.Errorf("Component mapping broken: expected 'test-comp-id', got %q", params.ComponentID)
	}
	if params.EnvironmentID != "test-env-id" {
		t.Errorf("Environment mapping broken: expected 'test-env-id', got %q", params.EnvironmentID)
	}
	if params.Namespace != "test-namespace" {
		t.Errorf("Namespace mapping broken: expected 'test-namespace', got %q", params.Namespace)
	}
	if params.SearchPhrase != "my-search-term" {
		t.Errorf("SearchPhrase mapping broken: expected 'my-search-term', got %q", params.SearchPhrase)
	}
	if params.Limit != 999 {
		t.Errorf("Limit mapping broken: expected 999, got %d", params.Limit)
	}
}

// TestSchemaPropertyTypes verifies that schema properties have correct types
func TestSchemaPropertyTypes(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	expectedTypes := map[string]map[string]string{
		"get_component_logs": {
			"component_id":   "string",
			"environment_id": "string",
			"namespace":      "string",
			"start_time":     "string",
			"end_time":       "string",
			"search_phrase":  "string",
			"log_levels":     "array",
			"versions":       "array",
			"version_ids":    "array",
			"limit":          "number",
			"sort_order":     "string",
			"log_type":       "string",
			"build_id":       "string",
			"build_uuid":     "string",
		},
		"get_project_logs": {
			"project_id":     "string",
			"environment_id": "string",
			"start_time":     "string",
			"end_time":       "string",
			"component_ids":  "array",
			"search_phrase":  "string",
			"log_levels":     "array",
			"limit":          "number",
			"sort_order":     "string",
		},
		"get_gateway_logs": {
			"organization_id":       "string",
			"start_time":            "string",
			"end_time":              "string",
			"search_phrase":         "string",
			"api_id_to_version_map": "object",
			"gateway_vhosts":        "array",
			"limit":                 "number",
			"sort_order":            "string",
			"log_type":              "string",
		},
		"get_organization_logs": {
			"organization_id": "string",
			"environment_id":  "string",
			"start_time":      "string",
			"end_time":        "string",
			"pod_labels":      "object",
			"search_phrase":   "string",
			"log_levels":      "array",
			"limit":           "number",
			"sort_order":      "string",
		},
		"get_traces": {
			"project_uid":     "string",
			"component_uids":  "array",
			"environment_uid": "string",
			"trace_id":        "string",
			"start_time":      "string",
			"end_time":        "string",
			"limit":           "number",
			"sort_order":      "string",
		},
		"get_component_resource_metrics": {
			"component_id":   "string",
			"project_id":     "string",
			"environment_id": "string",
			"start_time":     "string",
			"end_time":       "string",
		},
	}

	for _, tool := range toolsResult.Tools {
		expectedProps, ok := expectedTypes[tool.Name]
		if !ok {
			continue
		}

		t.Run(tool.Name, func(t *testing.T) {
			schemaMap, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("Expected InputSchema to be map[string]any, got %T", tool.InputSchema)
			}

			properties, ok := schemaMap["properties"].(map[string]any)
			if !ok {
				t.Fatal("Properties is not a map")
			}

			for propName, expectedType := range expectedProps {
				prop, exists := properties[propName]
				if !exists {
					t.Errorf("Property %q not found", propName)
					continue
				}

				propMap, ok := prop.(map[string]any)
				if !ok {
					t.Errorf("Property %q is not a map", propName)
					continue
				}

				actualType, ok := propMap["type"].(string)
				if !ok {
					t.Errorf("Property %q has no type field", propName)
					continue
				}

				if actualType != expectedType {
					t.Errorf("Property %q has type %q, expected %q", propName, actualType, expectedType)
				}
			}
		})
	}
}
