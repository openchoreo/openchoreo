// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	testNamespace   = "test-org"
	testProject     = "test-project"
	testComponent   = "test-component"
	testEnvironment = "development"
	testStartTime   = "2025-01-01T00:00:00Z"
	testEndTime     = "2025-01-01T23:59:59Z"
	testTraceID     = "trace-abc123"
	testSpanID      = "span-def456"
	sortOrderDesc   = "desc"
)

// ---- Mock service implementations ----

type MockLogsQuerier struct {
	requests []*types.LogsQueryRequest
	response *types.LogsQueryResponse
	err      error
}

func NewMockLogsQuerier() *MockLogsQuerier {
	return &MockLogsQuerier{
		response: &types.LogsQueryResponse{
			Logs:   []types.LogEntry{{Timestamp: "2025-01-01T00:00:00Z", Log: "test log", Level: "INFO"}},
			Total:  1,
			TookMs: 10,
		},
	}
}

func (m *MockLogsQuerier) QueryLogs(_ context.Context, req *types.LogsQueryRequest) (*types.LogsQueryResponse, error) {
	m.requests = append(m.requests, req)
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *MockLogsQuerier) lastRequest() *types.LogsQueryRequest {
	if len(m.requests) == 0 {
		return nil
	}
	return m.requests[len(m.requests)-1]
}

func (m *MockLogsQuerier) reset() { m.requests = nil }

type MockMetricsQuerier struct {
	requests []*types.MetricsQueryRequest
	response any
	err      error
}

func NewMockMetricsQuerier() *MockMetricsQuerier {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return &MockMetricsQuerier{
		response: &types.ResourceMetricsQueryResponse{
			CPUUsage: []types.MetricsTimeSeriesItem{{Timestamp: ts, Value: 0.5}},
		},
	}
}

func (m *MockMetricsQuerier) QueryMetrics(_ context.Context, req *types.MetricsQueryRequest) (any, error) {
	m.requests = append(m.requests, req)
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *MockMetricsQuerier) lastRequest() *types.MetricsQueryRequest {
	if len(m.requests) == 0 {
		return nil
	}
	return m.requests[len(m.requests)-1]
}

func (m *MockMetricsQuerier) reset() { m.requests = nil }

type MockTracesQuerier struct {
	tracesRequests      []*types.TracesQueryRequest
	spansRequests       []*types.TracesQueryRequest
	spanDetailsTraceIDs []string
	spanDetailsSpanIDs  []string
	tracesResponse      *types.TracesQueryResponse
	spansResponse       *types.SpansQueryResponse
	spanInfo            *types.SpanInfo
	queryTracesErr      error
	querySpansErr       error
	spanDetailsErr      error
}

func NewMockTracesQuerier() *MockTracesQuerier {
	now := time.Now()
	return &MockTracesQuerier{
		tracesResponse: &types.TracesQueryResponse{
			Traces: []types.TraceInfo{
				{TraceID: testTraceID, TraceName: "test-trace", SpanCount: 3},
			},
			Total:  1,
			TookMs: 15,
		},
		spansResponse: &types.SpansQueryResponse{
			Spans: []types.SpanInfo{
				{SpanID: testSpanID, SpanName: "test-span", StartTime: &now},
			},
			Total:  1,
			TookMs: 5,
		},
		spanInfo: &types.SpanInfo{
			SpanID:     testSpanID,
			SpanName:   "test-span",
			Attributes: map[string]any{"http.method": "GET"},
		},
	}
}

func (m *MockTracesQuerier) QueryTraces(_ context.Context, req *types.TracesQueryRequest) (*types.TracesQueryResponse, error) {
	m.tracesRequests = append(m.tracesRequests, req)
	if m.queryTracesErr != nil {
		return nil, m.queryTracesErr
	}
	return m.tracesResponse, nil
}

func (m *MockTracesQuerier) QuerySpans(_ context.Context, traceID string, req *types.TracesQueryRequest) (*types.SpansQueryResponse, error) {
	m.spansRequests = append(m.spansRequests, req)
	m.spanDetailsTraceIDs = append(m.spanDetailsTraceIDs, traceID)
	if m.querySpansErr != nil {
		return nil, m.querySpansErr
	}
	return m.spansResponse, nil
}

func (m *MockTracesQuerier) GetSpanDetails(_ context.Context, traceID string, spanID string) (*types.SpanInfo, error) {
	m.spanDetailsTraceIDs = append(m.spanDetailsTraceIDs, traceID)
	m.spanDetailsSpanIDs = append(m.spanDetailsSpanIDs, spanID)
	if m.spanDetailsErr != nil {
		return nil, m.spanDetailsErr
	}
	return m.spanInfo, nil
}

func (m *MockTracesQuerier) lastTracesRequest() *types.TracesQueryRequest {
	if len(m.tracesRequests) == 0 {
		return nil
	}
	return m.tracesRequests[len(m.tracesRequests)-1]
}

func (m *MockTracesQuerier) lastSpansRequest() *types.TracesQueryRequest {
	if len(m.spansRequests) == 0 {
		return nil
	}
	return m.spansRequests[len(m.spansRequests)-1]
}

func (m *MockTracesQuerier) reset() {
	m.tracesRequests = nil
	m.spansRequests = nil
	m.spanDetailsTraceIDs = nil
	m.spanDetailsSpanIDs = nil
}

// ---- Test harness ----

type testServices struct {
	logs    *MockLogsQuerier
	metrics *MockMetricsQuerier
	traces  *MockTracesQuerier
}

func newTestServices() *testServices {
	return &testServices{
		logs:    NewMockLogsQuerier(),
		metrics: NewMockMetricsQuerier(),
		traces:  NewMockTracesQuerier(),
	}
}

func (s *testServices) resetAll() {
	s.logs.reset()
	s.metrics.reset()
	s.traces.reset()
}

func buildMCPHandler(svcs *testServices) (*MCPHandler, error) {
	logger := slog.Default()
	healthSvc, err := service.NewHealthService(logger)
	if err != nil {
		return nil, err
	}
	alertSvc := service.NewAlertService(nil, nil, nil, nil, nil, nil, logger, "", false, nil)
	return NewMCPHandler(healthSvc, svcs.logs, svcs.metrics, alertSvc, alertSvc, svcs.traces, logger)
}

func setupTestServer(t *testing.T) (*mcpsdk.ClientSession, *testServices) {
	t.Helper()

	svcs := newTestServices()
	handler, err := buildMCPHandler(svcs)
	if err != nil {
		t.Fatalf("Failed to build MCPHandler: %v", err)
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "test-openchoreo-observer",
		Version: "1.0.0",
	}, nil)

	registerTools(server, handler)

	ctx := context.Background()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()

	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("Failed to connect server: %v", err)
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}

	return clientSession, svcs
}

// ---- Tool test specifications ----

type toolTestSpec struct {
	name string

	// Description validation
	descriptionKeywords []string
	descriptionMinLen   int

	// Schema validation
	requiredParams []string
	optionalParams []string

	// Parameter wiring test
	testArgs     map[string]any
	validateCall func(t *testing.T, svcs *testServices)
}

var allToolSpecs = []toolTestSpec{
	{
		name:                "query_component_logs",
		descriptionKeywords: []string{"component", "logs"},
		descriptionMinLen:   20,
		requiredParams:      []string{"namespace", "start_time", "end_time"},
		optionalParams:      []string{"project", "component", "environment", "search_phrase", "log_levels", "limit", "sort_order"},
		testArgs: map[string]any{
			"namespace":     testNamespace,
			"project":       testProject,
			"component":     testComponent,
			"environment":   testEnvironment,
			"start_time":    testStartTime,
			"end_time":      testEndTime,
			"search_phrase": "error",
			"log_levels":    []any{"ERROR", "WARN"},
			"limit":         50,
			"sort_order":    "asc",
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.logs.lastRequest()
			if req == nil {
				t.Fatal("Expected QueryLogs to be called")
			}
			if req.SearchScope == nil || req.SearchScope.Component == nil {
				t.Fatal("Expected ComponentSearchScope")
			}
			scope := req.SearchScope.Component
			if scope.Namespace != testNamespace {
				t.Errorf("Expected namespace %q, got %q", testNamespace, scope.Namespace)
			}
			if scope.Project != testProject {
				t.Errorf("Expected project %q, got %q", testProject, scope.Project)
			}
			if scope.Component != testComponent {
				t.Errorf("Expected component %q, got %q", testComponent, scope.Component)
			}
			if scope.Environment != testEnvironment {
				t.Errorf("Expected environment %q, got %q", testEnvironment, scope.Environment)
			}
			if req.StartTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, req.StartTime)
			}
			if req.EndTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, req.EndTime)
			}
			if req.SearchPhrase != "error" {
				t.Errorf("Expected search_phrase 'error', got %q", req.SearchPhrase)
			}
			if diff := cmp.Diff([]string{"ERROR", "WARN"}, req.LogLevels); diff != "" {
				t.Errorf("log_levels mismatch (-want +got):\n%s", diff)
			}
			if req.Limit != 50 {
				t.Errorf("Expected limit 50, got %d", req.Limit)
			}
			if req.SortOrder != "asc" {
				t.Errorf("Expected sort_order 'asc', got %q", req.SortOrder)
			}
		},
	},
	{
		name:                "query_workflow_logs",
		descriptionKeywords: []string{"workflow", "logs"},
		descriptionMinLen:   20,
		requiredParams:      []string{"namespace", "start_time", "end_time"},
		optionalParams:      []string{"workflow_run_name", "task_name", "search_phrase", "log_levels", "limit", "sort_order"},
		testArgs: map[string]any{
			"namespace":         testNamespace,
			"workflow_run_name": "my-workflow-run",
			"task_name":         "build-step",
			"start_time":        testStartTime,
			"end_time":          testEndTime,
			"search_phrase":     "failed",
			"log_levels":        []any{"ERROR"},
			"limit":             75,
			"sort_order":        sortOrderDesc,
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.logs.lastRequest()
			if req == nil {
				t.Fatal("Expected QueryLogs to be called")
			}
			if req.SearchScope == nil || req.SearchScope.Workflow == nil {
				t.Fatal("Expected WorkflowSearchScope")
			}
			scope := req.SearchScope.Workflow
			if scope.Namespace != testNamespace {
				t.Errorf("Expected namespace %q, got %q", testNamespace, scope.Namespace)
			}
			if scope.WorkflowRunName != "my-workflow-run" {
				t.Errorf("Expected workflow_run_name 'my-workflow-run', got %q", scope.WorkflowRunName)
			}
			if scope.TaskName != "build-step" {
				t.Errorf("Expected task_name 'build-step', got %q", scope.TaskName)
			}
			if req.SearchPhrase != "failed" {
				t.Errorf("Expected search_phrase 'failed', got %q", req.SearchPhrase)
			}
			if diff := cmp.Diff([]string{"ERROR"}, req.LogLevels); diff != "" {
				t.Errorf("log_levels mismatch (-want +got):\n%s", diff)
			}
			if req.Limit != 75 {
				t.Errorf("Expected limit 75, got %d", req.Limit)
			}
			if req.SortOrder != sortOrderDesc {
				t.Errorf("Expected sort_order %q, got %q", sortOrderDesc, req.SortOrder)
			}
		},
	},
	{
		name:                "query_resource_metrics",
		descriptionKeywords: []string{"resource", "metrics"},
		descriptionMinLen:   20,
		requiredParams:      []string{"namespace", "start_time", "end_time"},
		optionalParams:      []string{"project", "component", "environment", "step"},
		testArgs: map[string]any{
			"namespace":   testNamespace,
			"project":     testProject,
			"component":   testComponent,
			"environment": testEnvironment,
			"start_time":  testStartTime,
			"end_time":    testEndTime,
			"step":        "5m",
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.metrics.lastRequest()
			if req == nil {
				t.Fatal("Expected QueryMetrics to be called")
			}
			if req.Metric != types.MetricTypeResource {
				t.Errorf("Expected metric type %q, got %q", types.MetricTypeResource, req.Metric)
			}
			if req.SearchScope.Namespace != testNamespace {
				t.Errorf("Expected namespace %q, got %q", testNamespace, req.SearchScope.Namespace)
			}
			if req.SearchScope.Project != testProject {
				t.Errorf("Expected project %q, got %q", testProject, req.SearchScope.Project)
			}
			if req.SearchScope.Component != testComponent {
				t.Errorf("Expected component %q, got %q", testComponent, req.SearchScope.Component)
			}
			if req.SearchScope.Environment != testEnvironment {
				t.Errorf("Expected environment %q, got %q", testEnvironment, req.SearchScope.Environment)
			}
			if req.StartTime != testStartTime {
				t.Errorf("Expected start_time %q, got %q", testStartTime, req.StartTime)
			}
			if req.EndTime != testEndTime {
				t.Errorf("Expected end_time %q, got %q", testEndTime, req.EndTime)
			}
			if req.Step == nil || *req.Step != "5m" {
				t.Errorf("Expected step '5m', got %v", req.Step)
			}
		},
	},
	{
		name:                "query_http_metrics",
		descriptionKeywords: []string{"http", "metrics"},
		descriptionMinLen:   20,
		requiredParams:      []string{"namespace", "start_time", "end_time"},
		optionalParams:      []string{"project", "component", "environment", "step"},
		testArgs: map[string]any{
			"namespace":   testNamespace,
			"project":     testProject,
			"component":   testComponent,
			"environment": testEnvironment,
			"start_time":  testStartTime,
			"end_time":    testEndTime,
			"step":        "1h",
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.metrics.lastRequest()
			if req == nil {
				t.Fatal("Expected QueryMetrics to be called")
			}
			if req.Metric != types.MetricTypeHTTP {
				t.Errorf("Expected metric type %q, got %q", types.MetricTypeHTTP, req.Metric)
			}
			if req.SearchScope.Namespace != testNamespace {
				t.Errorf("Expected namespace %q, got %q", testNamespace, req.SearchScope.Namespace)
			}
			if req.Step == nil || *req.Step != "1h" {
				t.Errorf("Expected step '1h', got %v", req.Step)
			}
		},
	},
	{
		name:                "query_traces",
		descriptionKeywords: []string{"traces"},
		descriptionMinLen:   20,
		requiredParams:      []string{"namespace", "start_time", "end_time"},
		optionalParams:      []string{"project", "component", "environment", "limit", "sort_order"},
		testArgs: map[string]any{
			"namespace":   testNamespace,
			"project":     testProject,
			"component":   testComponent,
			"environment": testEnvironment,
			"start_time":  testStartTime,
			"end_time":    testEndTime,
			"limit":       25,
			"sort_order":  "asc",
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.traces.lastTracesRequest()
			if req == nil {
				t.Fatal("Expected QueryTraces to be called")
			}
			if req.SearchScope.Namespace != testNamespace {
				t.Errorf("Expected namespace %q, got %q", testNamespace, req.SearchScope.Namespace)
			}
			if req.SearchScope.Project != testProject {
				t.Errorf("Expected project %q, got %q", testProject, req.SearchScope.Project)
			}
			if req.SearchScope.Component != testComponent {
				t.Errorf("Expected component %q, got %q", testComponent, req.SearchScope.Component)
			}
			if req.SearchScope.Environment != testEnvironment {
				t.Errorf("Expected environment %q, got %q", testEnvironment, req.SearchScope.Environment)
			}
			expectedStart, _ := time.Parse(time.RFC3339, testStartTime)
			if !req.StartTime.Equal(expectedStart) {
				t.Errorf("Expected start_time %v, got %v", expectedStart, req.StartTime)
			}
			expectedEnd, _ := time.Parse(time.RFC3339, testEndTime)
			if !req.EndTime.Equal(expectedEnd) {
				t.Errorf("Expected end_time %v, got %v", expectedEnd, req.EndTime)
			}
			if req.Limit != 25 {
				t.Errorf("Expected limit 25, got %d", req.Limit)
			}
			if req.SortOrder != "asc" {
				t.Errorf("Expected sort 'asc', got %q", req.SortOrder)
			}
		},
	},
	{
		name:                "query_trace_spans",
		descriptionKeywords: []string{"span", "trace"},
		descriptionMinLen:   20,
		requiredParams:      []string{"trace_id", "namespace", "start_time", "end_time"},
		optionalParams:      []string{"project", "component", "environment", "limit", "sort_order"},
		testArgs: map[string]any{
			"trace_id":    testTraceID,
			"namespace":   testNamespace,
			"project":     testProject,
			"component":   testComponent,
			"environment": testEnvironment,
			"start_time":  testStartTime,
			"end_time":    testEndTime,
			"limit":       200,
			"sort_order":  sortOrderDesc,
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.traces.lastSpansRequest()
			if req == nil {
				t.Fatal("Expected QuerySpans to be called")
			}
			if len(svcs.traces.spanDetailsTraceIDs) == 0 {
				t.Fatal("Expected trace ID to be recorded")
			}
			lastTraceID := svcs.traces.spanDetailsTraceIDs[len(svcs.traces.spanDetailsTraceIDs)-1]
			if lastTraceID != testTraceID {
				t.Errorf("Expected trace_id %q, got %q", testTraceID, lastTraceID)
			}
			if req.SearchScope.Namespace != testNamespace {
				t.Errorf("Expected namespace %q, got %q", testNamespace, req.SearchScope.Namespace)
			}
			if req.Limit != 200 {
				t.Errorf("Expected limit 200, got %d", req.Limit)
			}
			if req.SortOrder != sortOrderDesc {
				t.Errorf("Expected sort %q, got %q", sortOrderDesc, req.SortOrder)
			}
		},
	},
	{
		name:                "get_span_details",
		descriptionKeywords: []string{"span"},
		descriptionMinLen:   20,
		requiredParams:      []string{"trace_id", "span_id"},
		optionalParams:      []string{},
		testArgs: map[string]any{
			"trace_id": testTraceID,
			"span_id":  testSpanID,
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			if len(svcs.traces.spanDetailsTraceIDs) == 0 || len(svcs.traces.spanDetailsSpanIDs) == 0 {
				t.Fatal("Expected GetSpanDetails to be called")
			}
			lastIdx := len(svcs.traces.spanDetailsSpanIDs) - 1
			if svcs.traces.spanDetailsTraceIDs[lastIdx] != testTraceID {
				t.Errorf("Expected trace_id %q, got %q", testTraceID, svcs.traces.spanDetailsTraceIDs[lastIdx])
			}
			if svcs.traces.spanDetailsSpanIDs[lastIdx] != testSpanID {
				t.Errorf("Expected span_id %q, got %q", testSpanID, svcs.traces.spanDetailsSpanIDs[lastIdx])
			}
		},
	},
}

// ---- Tests ----

// TestNewMCPHandlerValidation verifies that nil services are rejected.
func TestNewMCPHandlerValidation(t *testing.T) {
	logger := slog.Default()
	healthSvc, _ := service.NewHealthService(logger)
	alertSvc := service.NewAlertService(nil, nil, nil, nil, nil, nil, logger, "", false, nil)
	logs := NewMockLogsQuerier()
	metrics := NewMockMetricsQuerier()
	traces := NewMockTracesQuerier()

	tests := []struct {
		name             string
		health           *service.HealthService
		logs             service.LogsQuerier
		metrics          service.MetricsQuerier
		alertsQuerier    service.AlertsQuerier
		incidentsQuerier service.IncidentsQuerier
		traces           service.TracesQuerier
		log              *slog.Logger
	}{
		{"nil healthService", nil, logs, metrics, alertSvc, alertSvc, traces, logger},
		{"nil logsService", healthSvc, nil, metrics, alertSvc, alertSvc, traces, logger},
		{"nil metricsService", healthSvc, logs, nil, alertSvc, alertSvc, traces, logger},
		{"nil alertsQuerier", healthSvc, logs, metrics, nil, alertSvc, traces, logger},
		{"nil incidentsQuerier", healthSvc, logs, metrics, alertSvc, nil, traces, logger},
		{"nil tracesService", healthSvc, logs, metrics, alertSvc, alertSvc, nil, logger},
		{"nil logger", healthSvc, logs, metrics, alertSvc, alertSvc, traces, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMCPHandler(tt.health, tt.logs, tt.metrics, tt.alertsQuerier, tt.incidentsQuerier, tt.traces, tt.log)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tt.name)
			}
		})
	}
}

// TestToolRegistration verifies that all expected tools are registered.
func TestToolRegistration(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	expectedTools := make(map[string]bool)
	for _, spec := range allToolSpecs {
		expectedTools[spec.name] = true
	}

	registeredTools := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		registeredTools[tool.Name] = true
		if !expectedTools[tool.Name] {
			t.Errorf("Unexpected tool %q found in registered tools", tool.Name)
		}
	}

	for expected := range expectedTools {
		if !registeredTools[expected] {
			t.Errorf("Expected tool %q not found in registered tools", expected)
		}
	}

	if len(toolsResult.Tools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(toolsResult.Tools))
	}
}

// TestToolDescriptions verifies that tool descriptions are meaningful and distinguishable.
func TestToolDescriptions(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	toolsByName := make(map[string]*mcpsdk.Tool)
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			if !exists {
				t.Fatalf("Tool %q not found", spec.name)
			}

			desc := strings.ToLower(tool.Description)

			if len(desc) < spec.descriptionMinLen {
				t.Errorf("Description too short: got %d chars, want at least %d", len(desc), spec.descriptionMinLen)
			}

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

// TestToolSchemas verifies that tool input schemas have correct parameters defined.
func TestToolSchemas(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	toolsByName := make(map[string]*mcpsdk.Tool)
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			if !exists {
				t.Fatalf("Tool %q not found", spec.name)
			}

			if tool.InputSchema == nil {
				t.Fatal("InputSchema is nil")
			}

			schemaMap, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("Expected InputSchema to be map[string]any, got %T", tool.InputSchema)
			}

			schemaType, ok := schemaMap["type"].(string)
			if !ok || schemaType != "object" {
				t.Errorf("Expected schema type 'object', got %v", schemaMap["type"])
			}

			// Check required parameters appear in schema.required
			if len(spec.requiredParams) > 0 {
				requiredInSchema := make(map[string]bool)
				if requiredList, ok := schemaMap["required"].([]any); ok {
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

			// Check all parameters exist in schema.properties
			allParams := append(append([]string{}, spec.requiredParams...), spec.optionalParams...)
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

// TestToolParameterWiring verifies that parameters are correctly passed to service methods.
func TestToolParameterWiring(t *testing.T) {
	clientSession, svcs := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			svcs.resetAll()

			result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
				Name:      spec.name,
				Arguments: spec.testArgs,
			})
			if err != nil {
				t.Fatalf("Failed to call tool: %v", err)
			}

			if len(result.Content) == 0 {
				t.Fatal("Expected non-empty result content")
			}

			spec.validateCall(t, svcs)
		})
	}
}

// TestToolResponseFormat verifies that tool responses are valid JSON.
func TestToolResponseFormat(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	// Test response format for each tool
	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
				Name:      spec.name,
				Arguments: spec.testArgs,
			})
			if err != nil {
				t.Fatalf("Failed to call tool: %v", err)
			}

			if len(result.Content) == 0 {
				t.Fatal("Expected non-empty result content")
			}

			textContent, ok := result.Content[0].(*mcpsdk.TextContent)
			if !ok {
				t.Fatal("Expected TextContent")
			}

			var data any
			if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
				t.Errorf("Response is not valid JSON: %v\nResponse: %s", err, textContent.Text)
			}
		})
	}
}

// TestToolErrorHandling verifies that the MCP SDK validates required parameters.
func TestToolErrorHandling(t *testing.T) {
	clientSession, mockHandler := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

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

	mockHandler.resetAll()

	_, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      testSpec.name,
		Arguments: map[string]any{}, // Empty - missing required params
	})

	if err == nil {
		t.Errorf("Expected error for tool %q with missing required parameters, got nil", testSpec.name)
	}
}

// TestMinimalParameterSets verifies that tools work with only required parameters.
func TestMinimalParameterSets(t *testing.T) {
	clientSession, svcs := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	minimalTests := []struct {
		name     string
		toolName string
		args     map[string]any
	}{
		{
			name:     "query_component_logs_minimal",
			toolName: "query_component_logs",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
		},
		{
			name:     "query_workflow_logs_minimal",
			toolName: "query_workflow_logs",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
		},
		{
			name:     "query_resource_metrics_minimal",
			toolName: "query_resource_metrics",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
		},
		{
			name:     "query_http_metrics_minimal",
			toolName: "query_http_metrics",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
		},
		{
			name:     "query_traces_minimal",
			toolName: "query_traces",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
		},
		{
			name:     "query_trace_spans_minimal",
			toolName: "query_trace_spans",
			args: map[string]any{
				"trace_id":   testTraceID,
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
		},
		{
			name:     "get_span_details_minimal",
			toolName: "get_span_details",
			args: map[string]any{
				"trace_id": testTraceID,
				"span_id":  testSpanID,
			},
		},
	}

	for _, tt := range minimalTests {
		t.Run(tt.name, func(t *testing.T) {
			svcs.resetAll()

			result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
				Name:      tt.toolName,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("Failed to call tool with minimal parameters: %v", err)
			}

			if len(result.Content) == 0 {
				t.Fatal("Expected non-empty result content")
			}

			textContent, ok := result.Content[0].(*mcpsdk.TextContent)
			if !ok {
				t.Fatal("Expected TextContent")
			}

			var data any
			if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
				t.Errorf("Response is not valid JSON: %v", err)
			}
		})
	}
}

// TestHandlerErrorPropagation verifies that service errors are propagated as MCP errors.
func TestHandlerErrorPropagation(t *testing.T) {
	ctx := context.Background()

	newServerWithError := func(t *testing.T, setupErr func(*testServices)) *mcpsdk.ClientSession {
		t.Helper()
		svcs := newTestServices()
		setupErr(svcs)

		handler, err := buildMCPHandler(svcs)
		if err != nil {
			t.Fatalf("Failed to build MCPHandler: %v", err)
		}

		server := mcpsdk.NewServer(&mcpsdk.Implementation{
			Name:    "test-openchoreo-observer",
			Version: "1.0.0",
		}, nil)
		registerTools(server, handler)

		clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
		if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
			t.Fatalf("Failed to connect server: %v", err)
		}

		client := mcpsdk.NewClient(&mcpsdk.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		}, nil)

		session, err := client.Connect(ctx, clientTransport, nil)
		if err != nil {
			t.Fatalf("Failed to connect client: %v", err)
		}
		return session
	}

	errorTests := []struct {
		name     string
		toolName string
		args     map[string]any
		setupErr func(*testServices)
	}{
		{
			name:     "logs_service_error",
			toolName: "query_component_logs",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) { s.logs.err = errors.New("opensearch unavailable") },
		},
		{
			name:     "workflow_logs_service_error",
			toolName: "query_workflow_logs",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) { s.logs.err = errors.New("connection refused") },
		},
		{
			name:     "resource_metrics_service_error",
			toolName: "query_resource_metrics",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) { s.metrics.err = errors.New("prometheus unavailable") },
		},
		{
			name:     "http_metrics_service_error",
			toolName: "query_http_metrics",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) { s.metrics.err = errors.New("query timeout") },
		},
		{
			name:     "traces_service_error",
			toolName: "query_traces",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) { s.traces.queryTracesErr = errors.New("trace service down") },
		},
		{
			name:     "trace_spans_service_error",
			toolName: "query_trace_spans",
			args: map[string]any{
				"trace_id":   testTraceID,
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) { s.traces.querySpansErr = errors.New("span query failed") },
		},
		{
			name:     "span_details_service_error",
			toolName: "get_span_details",
			args: map[string]any{
				"trace_id": testTraceID,
				"span_id":  testSpanID,
			},
			setupErr: func(s *testServices) { s.traces.spanDetailsErr = errors.New("span not found") },
		},
		{
			name:     "traces_invalid_start_time",
			toolName: "query_traces",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": "not-a-time",
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) {}, // error comes from time parsing, not service
		},
		{
			name:     "trace_spans_invalid_end_time",
			toolName: "query_trace_spans",
			args: map[string]any{
				"trace_id":   testTraceID,
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   "bad-time-format",
			},
			setupErr: func(s *testServices) {}, // error comes from time parsing, not service
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			session := newServerWithError(t, tt.setupErr)
			defer session.Close()

			result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
				Name:      tt.toolName,
				Arguments: tt.args,
			})

			// MCP SDK wraps errors in result.IsError rather than returning Go errors
			if err == nil && (result == nil || !result.IsError) {
				t.Errorf("Expected error from handler, got err=%v, result.IsError=%v",
					err, result != nil && result.IsError)
			}
		})
	}
}

// TestNewHTTPServer verifies that the HTTP server is created correctly.
func TestNewHTTPServer(t *testing.T) {
	svcs := newTestServices()
	handler, err := buildMCPHandler(svcs)
	if err != nil {
		t.Fatalf("Failed to build MCPHandler: %v", err)
	}

	httpHandler := NewHTTPServer(handler)

	if httpHandler == nil {
		t.Fatal("Expected non-nil HTTP handler")
	}

	var _ http.Handler = httpHandler
}

// TestOptionalParametersDefaults verifies that optional parameters have sensible defaults.
func TestOptionalParametersDefaults(t *testing.T) {
	clientSession, svcs := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	tests := []struct {
		name         string
		toolName     string
		args         map[string]any
		validateCall func(t *testing.T, svcs *testServices)
	}{
		{
			name:     "component_logs_default_limit_and_sort",
			toolName: "query_component_logs",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			validateCall: func(t *testing.T, svcs *testServices) {
				req := svcs.logs.lastRequest()
				if req == nil {
					t.Fatal("Expected QueryLogs to be called")
				}
				if req.Limit != 100 {
					t.Errorf("Expected default limit 100, got %d", req.Limit)
				}
				if req.SortOrder != sortOrderDesc {
					t.Errorf("Expected default sort_order %q, got %q", sortOrderDesc, req.SortOrder)
				}
				if len(req.LogLevels) != 0 {
					t.Errorf("Expected empty log_levels by default, got %v", req.LogLevels)
				}
			},
		},
		{
			name:     "workflow_logs_default_limit_and_sort",
			toolName: "query_workflow_logs",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			validateCall: func(t *testing.T, svcs *testServices) {
				req := svcs.logs.lastRequest()
				if req == nil {
					t.Fatal("Expected QueryLogs to be called")
				}
				if req.Limit != 100 {
					t.Errorf("Expected default limit 100, got %d", req.Limit)
				}
				if req.SortOrder != sortOrderDesc {
					t.Errorf("Expected default sort_order %q, got %q", sortOrderDesc, req.SortOrder)
				}
			},
		},
		{
			name:     "resource_metrics_no_step",
			toolName: "query_resource_metrics",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			validateCall: func(t *testing.T, svcs *testServices) {
				req := svcs.metrics.lastRequest()
				if req == nil {
					t.Fatal("Expected QueryMetrics to be called")
				}
				if req.Step != nil {
					t.Errorf("Expected nil step when not provided, got %v", req.Step)
				}
			},
		},
		{
			name:     "traces_default_limit_and_sort",
			toolName: "query_traces",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			validateCall: func(t *testing.T, svcs *testServices) {
				req := svcs.traces.lastTracesRequest()
				if req == nil {
					t.Fatal("Expected QueryTraces to be called")
				}
				if req.Limit != 100 {
					t.Errorf("Expected default limit 100, got %d", req.Limit)
				}
				if req.SortOrder != sortOrderDesc {
					t.Errorf("Expected default sort %q, got %q", sortOrderDesc, req.SortOrder)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svcs.resetAll()

			_, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
				Name:      tt.toolName,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("Failed to call tool: %v", err)
			}

			tt.validateCall(t, svcs)
		})
	}
}

// TestParameterMappingRegression demonstrates that validateCall catches real parameter mapping bugs.
func TestParameterMappingRegression(t *testing.T) {
	clientSession, svcs := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	_, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "query_component_logs",
		Arguments: map[string]any{
			"namespace":     "my-org",
			"project":       "my-project",
			"component":     "my-service",
			"environment":   "production",
			"start_time":    testStartTime,
			"end_time":      testEndTime,
			"search_phrase": "my-search",
			"limit":         42,
		},
	})
	if err != nil {
		t.Fatalf("Failed to call tool: %v", err)
	}

	req := svcs.logs.lastRequest()
	if req == nil {
		t.Fatal("Expected QueryLogs to be called")
	}
	if req.SearchScope == nil || req.SearchScope.Component == nil {
		t.Fatal("Expected ComponentSearchScope")
	}

	scope := req.SearchScope.Component
	if scope.Namespace != "my-org" {
		t.Errorf("Namespace mapping broken: expected 'my-org', got %q", scope.Namespace)
	}
	if scope.Project != "my-project" {
		t.Errorf("Project mapping broken: expected 'my-project', got %q", scope.Project)
	}
	if scope.Component != "my-service" {
		t.Errorf("Component mapping broken: expected 'my-service', got %q", scope.Component)
	}
	if scope.Environment != "production" {
		t.Errorf("Environment mapping broken: expected 'production', got %q", scope.Environment)
	}
	if req.SearchPhrase != "my-search" {
		t.Errorf("SearchPhrase mapping broken: expected 'my-search', got %q", req.SearchPhrase)
	}
	if req.Limit != 42 {
		t.Errorf("Limit mapping broken: expected 42, got %d", req.Limit)
	}
}

// TestSchemaPropertyTypes verifies that schema properties have the correct JSON types.
func TestSchemaPropertyTypes(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	expectedTypes := map[string]map[string]string{
		"query_component_logs": {
			"namespace":     "string",
			"project":       "string",
			"component":     "string",
			"environment":   "string",
			"start_time":    "string",
			"end_time":      "string",
			"search_phrase": "string",
			"log_levels":    "array",
			"limit":         "number",
			"sort_order":    "string",
		},
		"query_workflow_logs": {
			"namespace":         "string",
			"workflow_run_name": "string",
			"task_name":         "string",
			"start_time":        "string",
			"end_time":          "string",
			"search_phrase":     "string",
			"log_levels":        "array",
			"limit":             "number",
			"sort_order":        "string",
		},
		"query_resource_metrics": {
			"namespace":   "string",
			"project":     "string",
			"component":   "string",
			"environment": "string",
			"start_time":  "string",
			"end_time":    "string",
			"step":        "string",
		},
		"query_http_metrics": {
			"namespace":   "string",
			"project":     "string",
			"component":   "string",
			"environment": "string",
			"start_time":  "string",
			"end_time":    "string",
			"step":        "string",
		},
		"query_traces": {
			"namespace":   "string",
			"project":     "string",
			"component":   "string",
			"environment": "string",
			"start_time":  "string",
			"end_time":    "string",
			"limit":       "number",
			"sort_order":  "string",
		},
		"query_trace_spans": {
			"trace_id":    "string",
			"namespace":   "string",
			"project":     "string",
			"component":   "string",
			"environment": "string",
			"start_time":  "string",
			"end_time":    "string",
			"limit":       "number",
			"sort_order":  "string",
		},
		"get_span_details": {
			"trace_id": "string",
			"span_id":  "string",
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
