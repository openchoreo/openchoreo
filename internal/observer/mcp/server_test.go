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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
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
	sortOrderAsc    = "asc"
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

func (m *MockMetricsQuerier) QueryRuntimeTopology(_ context.Context, _ *types.RuntimeTopologyRequest) (*types.RuntimeTopologyResponse, error) {
	return &types.RuntimeTopologyResponse{}, nil
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

type MockAlertIncidentService struct {
	alertsRequests    []gen.AlertsQueryRequest
	incidentsRequests []gen.IncidentsQueryRequest
	alertsResponse    *gen.AlertsQueryResponse
	incidentsResponse *gen.IncidentsQueryResponse
	queryAlertsErr    error
	queryIncidentsErr error
}

func NewMockAlertIncidentService() *MockAlertIncidentService {
	return &MockAlertIncidentService{
		alertsResponse:    &gen.AlertsQueryResponse{},
		incidentsResponse: &gen.IncidentsQueryResponse{},
	}
}

func (m *MockAlertIncidentService) QueryAlerts(_ context.Context, req gen.AlertsQueryRequest) (*gen.AlertsQueryResponse, error) {
	m.alertsRequests = append(m.alertsRequests, req)
	if m.queryAlertsErr != nil {
		return nil, m.queryAlertsErr
	}
	return m.alertsResponse, nil
}

func (m *MockAlertIncidentService) QueryIncidents(_ context.Context, req gen.IncidentsQueryRequest) (*gen.IncidentsQueryResponse, error) {
	m.incidentsRequests = append(m.incidentsRequests, req)
	if m.queryIncidentsErr != nil {
		return nil, m.queryIncidentsErr
	}
	return m.incidentsResponse, nil
}

func (m *MockAlertIncidentService) UpdateIncident(_ context.Context, _ string, _ gen.IncidentPutRequest) (*gen.IncidentPutResponse, error) {
	return &gen.IncidentPutResponse{}, nil
}

func (m *MockAlertIncidentService) lastAlertsRequest() *gen.AlertsQueryRequest {
	if len(m.alertsRequests) == 0 {
		return nil
	}
	return &m.alertsRequests[len(m.alertsRequests)-1]
}

func (m *MockAlertIncidentService) lastIncidentsRequest() *gen.IncidentsQueryRequest {
	if len(m.incidentsRequests) == 0 {
		return nil
	}
	return &m.incidentsRequests[len(m.incidentsRequests)-1]
}

func (m *MockAlertIncidentService) reset() {
	m.alertsRequests = nil
	m.incidentsRequests = nil
}

// ---- Test harness ----

type testServices struct {
	logs            *MockLogsQuerier
	metrics         *MockMetricsQuerier
	traces          *MockTracesQuerier
	alertsIncidents *MockAlertIncidentService
}

func newTestServices() *testServices {
	return &testServices{
		logs:            NewMockLogsQuerier(),
		metrics:         NewMockMetricsQuerier(),
		traces:          NewMockTracesQuerier(),
		alertsIncidents: NewMockAlertIncidentService(),
	}
}

func (s *testServices) resetAll() {
	s.logs.reset()
	s.metrics.reset()
	s.traces.reset()
	s.alertsIncidents.reset()
}

func buildMCPHandler(svcs *testServices) (*MCPHandler, error) {
	logger := slog.Default()
	healthSvc, err := service.NewHealthService(logger)
	if err != nil {
		return nil, err
	}
	return NewMCPHandler(healthSvc, svcs.logs, svcs.metrics, svcs.alertsIncidents, svcs.traces, logger)
}

func setupTestServer(t *testing.T) (*mcpsdk.ClientSession, *testServices) {
	t.Helper()

	svcs := newTestServices()
	handler, err := buildMCPHandler(svcs)
	require.NoError(t, err, "Failed to build MCPHandler")

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "test-openchoreo-observer",
		Version: "1.0.0",
	}, nil)

	registerTools(server, handler)

	ctx := context.Background()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()

	_, err = server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err, "Failed to connect server")

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err, "Failed to connect client")

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
			"sort_order":    sortOrderAsc,
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.logs.lastRequest()
			require.NotNil(t, req, "Expected QueryLogs to be called")
			require.NotNil(t, req.SearchScope)
			require.NotNil(t, req.SearchScope.Component)
			scope := req.SearchScope.Component
			assert.Equal(t, testNamespace, scope.Namespace)
			assert.Equal(t, testProject, scope.Project)
			assert.Equal(t, testComponent, scope.Component)
			assert.Equal(t, testEnvironment, scope.Environment)
			assert.Equal(t, testStartTime, req.StartTime)
			assert.Equal(t, testEndTime, req.EndTime)
			assert.Equal(t, "error", req.SearchPhrase)
			if diff := cmp.Diff([]string{"ERROR", "WARN"}, req.LogLevels); diff != "" {
				t.Errorf("log_levels mismatch (-want +got):\n%s", diff)
			}
			assert.Equal(t, 50, req.Limit)
			assert.Equal(t, sortOrderAsc, req.SortOrder)
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
			require.NotNil(t, req, "Expected QueryLogs to be called")
			require.NotNil(t, req.SearchScope)
			require.NotNil(t, req.SearchScope.Workflow)
			scope := req.SearchScope.Workflow
			assert.Equal(t, testNamespace, scope.Namespace)
			assert.Equal(t, "my-workflow-run", scope.WorkflowRunName)
			assert.Equal(t, "build-step", scope.TaskName)
			assert.Equal(t, "failed", req.SearchPhrase)
			if diff := cmp.Diff([]string{"ERROR"}, req.LogLevels); diff != "" {
				t.Errorf("log_levels mismatch (-want +got):\n%s", diff)
			}
			assert.Equal(t, 75, req.Limit)
			assert.Equal(t, sortOrderDesc, req.SortOrder)
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
			require.NotNil(t, req, "Expected QueryMetrics to be called")
			assert.Equal(t, types.MetricTypeResource, req.Metric)
			assert.Equal(t, testNamespace, req.SearchScope.Namespace)
			assert.Equal(t, testProject, req.SearchScope.Project)
			assert.Equal(t, testComponent, req.SearchScope.Component)
			assert.Equal(t, testEnvironment, req.SearchScope.Environment)
			assert.Equal(t, testStartTime, req.StartTime)
			assert.Equal(t, testEndTime, req.EndTime)
			require.NotNil(t, req.Step)
			assert.Equal(t, "5m", *req.Step)
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
			require.NotNil(t, req, "Expected QueryMetrics to be called")
			assert.Equal(t, types.MetricTypeHTTP, req.Metric)
			assert.Equal(t, testNamespace, req.SearchScope.Namespace)
			require.NotNil(t, req.Step)
			assert.Equal(t, "1h", *req.Step)
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
			"sort_order":  sortOrderAsc,
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.traces.lastTracesRequest()
			require.NotNil(t, req, "Expected QueryTraces to be called")
			assert.Equal(t, testNamespace, req.SearchScope.Namespace)
			assert.Equal(t, testProject, req.SearchScope.Project)
			assert.Equal(t, testComponent, req.SearchScope.Component)
			assert.Equal(t, testEnvironment, req.SearchScope.Environment)
			expectedStart, _ := time.Parse(time.RFC3339, testStartTime)
			assert.True(t, req.StartTime.Equal(expectedStart), "Expected start_time %v, got %v", expectedStart, req.StartTime)
			expectedEnd, _ := time.Parse(time.RFC3339, testEndTime)
			assert.True(t, req.EndTime.Equal(expectedEnd), "Expected end_time %v, got %v", expectedEnd, req.EndTime)
			assert.Equal(t, 25, req.Limit)
			assert.Equal(t, sortOrderAsc, req.SortOrder)
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
			require.NotNil(t, req, "Expected QuerySpans to be called")
			require.NotEmpty(t, svcs.traces.spanDetailsTraceIDs, "Expected trace ID to be recorded")
			lastTraceID := svcs.traces.spanDetailsTraceIDs[len(svcs.traces.spanDetailsTraceIDs)-1]
			assert.Equal(t, testTraceID, lastTraceID)
			assert.Equal(t, testNamespace, req.SearchScope.Namespace)
			assert.Equal(t, 200, req.Limit)
			assert.Equal(t, sortOrderDesc, req.SortOrder)
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
			require.NotEmpty(t, svcs.traces.spanDetailsTraceIDs, "Expected GetSpanDetails to be called")
			require.NotEmpty(t, svcs.traces.spanDetailsSpanIDs, "Expected GetSpanDetails to be called")
			lastIdx := len(svcs.traces.spanDetailsSpanIDs) - 1
			assert.Equal(t, testTraceID, svcs.traces.spanDetailsTraceIDs[lastIdx])
			assert.Equal(t, testSpanID, svcs.traces.spanDetailsSpanIDs[lastIdx])
		},
	},
	{
		name:                "query_alerts",
		descriptionKeywords: []string{"alert"},
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
			"limit":       50,
			"sort_order":  sortOrderAsc,
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.alertsIncidents.lastAlertsRequest()
			require.NotNil(t, req, "Expected QueryAlerts to be called")
			assert.Equal(t, testNamespace, req.SearchScope.Namespace)
			require.NotNil(t, req.SearchScope.Project)
			assert.Equal(t, testProject, *req.SearchScope.Project)
			require.NotNil(t, req.SearchScope.Component)
			assert.Equal(t, testComponent, *req.SearchScope.Component)
			require.NotNil(t, req.SearchScope.Environment)
			assert.Equal(t, testEnvironment, *req.SearchScope.Environment)
			expectedStart, _ := time.Parse(time.RFC3339, testStartTime)
			assert.True(t, req.StartTime.Equal(expectedStart), "Expected start_time %v, got %v", expectedStart, req.StartTime)
			expectedEnd, _ := time.Parse(time.RFC3339, testEndTime)
			assert.True(t, req.EndTime.Equal(expectedEnd), "Expected end_time %v, got %v", expectedEnd, req.EndTime)
			require.NotNil(t, req.Limit)
			assert.Equal(t, 50, *req.Limit)
			require.NotNil(t, req.SortOrder)
			assert.Equal(t, sortOrderAsc, string(*req.SortOrder))
		},
	},
	{
		name:                "query_incidents",
		descriptionKeywords: []string{"incident"},
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
			"sort_order":  sortOrderDesc,
		},
		validateCall: func(t *testing.T, svcs *testServices) {
			t.Helper()
			req := svcs.alertsIncidents.lastIncidentsRequest()
			require.NotNil(t, req, "Expected QueryIncidents to be called")
			assert.Equal(t, testNamespace, req.SearchScope.Namespace)
			require.NotNil(t, req.SearchScope.Project)
			assert.Equal(t, testProject, *req.SearchScope.Project)
			require.NotNil(t, req.SearchScope.Component)
			assert.Equal(t, testComponent, *req.SearchScope.Component)
			require.NotNil(t, req.SearchScope.Environment)
			assert.Equal(t, testEnvironment, *req.SearchScope.Environment)
			expectedStart, _ := time.Parse(time.RFC3339, testStartTime)
			assert.True(t, req.StartTime.Equal(expectedStart), "Expected start_time %v, got %v", expectedStart, req.StartTime)
			expectedEnd, _ := time.Parse(time.RFC3339, testEndTime)
			assert.True(t, req.EndTime.Equal(expectedEnd), "Expected end_time %v, got %v", expectedEnd, req.EndTime)
			require.NotNil(t, req.Limit)
			assert.Equal(t, 25, *req.Limit)
			require.NotNil(t, req.SortOrder)
			assert.Equal(t, sortOrderDesc, string(*req.SortOrder))
		},
	},
}

// ---- Tests ----

// TestNewMCPHandlerValidation verifies that nil services are rejected.
func TestNewMCPHandlerValidation(t *testing.T) {
	logger := slog.Default()
	healthSvc, _ := service.NewHealthService(logger)
	alertIncidentSvc := NewMockAlertIncidentService()
	logs := NewMockLogsQuerier()
	metrics := NewMockMetricsQuerier()
	traces := NewMockTracesQuerier()

	tests := []struct {
		name                 string
		health               *service.HealthService
		logs                 service.LogsQuerier
		metrics              service.MetricsQuerier
		alertIncidentService service.AlertIncidentService
		traces               service.TracesQuerier
		log                  *slog.Logger
	}{
		{"nil healthService", nil, logs, metrics, alertIncidentSvc, traces, logger},
		{"nil logsService", healthSvc, nil, metrics, alertIncidentSvc, traces, logger},
		{"nil metricsService", healthSvc, logs, nil, alertIncidentSvc, traces, logger},
		{"nil alertIncidentService", healthSvc, logs, metrics, nil, traces, logger},
		{"nil tracesService", healthSvc, logs, metrics, alertIncidentSvc, nil, logger},
		{"nil logger", healthSvc, logs, metrics, alertIncidentSvc, traces, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMCPHandler(tt.health, tt.logs, tt.metrics, tt.alertIncidentService, tt.traces, tt.log)
			require.Error(t, err, "Expected error for %s", tt.name)
		})
	}
}

// TestToolRegistration verifies that all expected tools are registered.
func TestToolRegistration(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err, "Failed to list tools")

	expectedTools := make(map[string]bool)
	for _, spec := range allToolSpecs {
		expectedTools[spec.name] = true
	}

	registeredTools := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		registeredTools[tool.Name] = true
		assert.True(t, expectedTools[tool.Name], "Unexpected tool %q found in registered tools", tool.Name)
	}

	for expected := range expectedTools {
		assert.True(t, registeredTools[expected], "Expected tool %q not found in registered tools", expected)
	}

	assert.Len(t, toolsResult.Tools, len(expectedTools))
}

// TestToolDescriptions verifies that tool descriptions are meaningful and distinguishable.
func TestToolDescriptions(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err, "Failed to list tools")

	toolsByName := make(map[string]*mcpsdk.Tool)
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			require.True(t, exists, "Tool %q not found", spec.name)

			desc := strings.ToLower(tool.Description)

			assert.GreaterOrEqual(t, len(desc), spec.descriptionMinLen,
				"Description too short: got %d chars, want at least %d", len(desc), spec.descriptionMinLen)

			for _, word := range spec.descriptionKeywords {
				assert.Contains(t, desc, strings.ToLower(word),
					"Description missing required keyword %q: %s", word, tool.Description)
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
	require.NoError(t, err, "Failed to list tools")

	toolsByName := make(map[string]*mcpsdk.Tool)
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			require.True(t, exists, "Tool %q not found", spec.name)
			require.NotNil(t, tool.InputSchema, "InputSchema is nil")

			schemaMap, ok := tool.InputSchema.(map[string]any)
			require.True(t, ok, "Expected InputSchema to be map[string]any, got %T", tool.InputSchema)

			schemaType, ok := schemaMap["type"].(string)
			assert.True(t, ok && schemaType == "object", "Expected schema type 'object', got %v", schemaMap["type"])

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
					assert.True(t, requiredInSchema[param], "Required parameter %q not found in schema.required", param)
				}
			}

			// Check all parameters exist in schema.properties
			allParams := append(append([]string{}, spec.requiredParams...), spec.optionalParams...)
			if len(allParams) > 0 {
				properties, ok := schemaMap["properties"].(map[string]any)
				require.True(t, ok, "Properties is not a map")
				for _, param := range allParams {
					_, exists := properties[param]
					assert.True(t, exists, "Parameter %q not found in schema.properties", param)
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
			require.NoError(t, err, "Failed to call tool")
			require.NotEmpty(t, result.Content, "Expected non-empty result content")

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
			require.NoError(t, err, "Failed to call tool")
			require.NotEmpty(t, result.Content, "Expected non-empty result content")

			textContent, ok := result.Content[0].(*mcpsdk.TextContent)
			require.True(t, ok, "Expected TextContent")

			var data any
			err = json.Unmarshal([]byte(textContent.Text), &data)
			assert.NoError(t, err, "Response is not valid JSON: %s", textContent.Text)
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

	require.NotEmpty(t, testSpec.name, "No tool with required parameters found in allToolSpecs")

	mockHandler.resetAll()

	_, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      testSpec.name,
		Arguments: map[string]any{}, // Empty - missing required params
	})

	require.Error(t, err, "Expected error for tool %q with missing required parameters", testSpec.name)
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
			require.NoError(t, err, "Failed to call tool with minimal parameters")
			require.NotEmpty(t, result.Content, "Expected non-empty result content")

			textContent, ok := result.Content[0].(*mcpsdk.TextContent)
			require.True(t, ok, "Expected TextContent")

			var data any
			err = json.Unmarshal([]byte(textContent.Text), &data)
			assert.NoError(t, err, "Response is not valid JSON")
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
		require.NoError(t, err, "Failed to build MCPHandler")

		server := mcpsdk.NewServer(&mcpsdk.Implementation{
			Name:    "test-openchoreo-observer",
			Version: "1.0.0",
		}, nil)
		registerTools(server, handler)

		clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
		_, err = server.Connect(ctx, serverTransport, nil)
		require.NoError(t, err, "Failed to connect server")

		client := mcpsdk.NewClient(&mcpsdk.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		}, nil)

		session, err := client.Connect(ctx, clientTransport, nil)
		require.NoError(t, err, "Failed to connect client")
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
			name:     "alerts_service_error",
			toolName: "query_alerts",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) { s.alertsIncidents.queryAlertsErr = errors.New("alert store unavailable") },
		},
		{
			name:     "incidents_service_error",
			toolName: "query_incidents",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) { s.alertsIncidents.queryIncidentsErr = errors.New("incident store unavailable") },
		},
		{
			name:     "alerts_invalid_start_time",
			toolName: "query_alerts",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": "not-a-time",
				"end_time":   testEndTime,
			},
			setupErr: func(s *testServices) {},
		},
		{
			name:     "incidents_invalid_end_time",
			toolName: "query_incidents",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   "bad-time-format",
			},
			setupErr: func(s *testServices) {},
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
			assert.True(t, err != nil || (result != nil && result.IsError),
				"Expected error from handler, got err=%v, result.IsError=%v",
				err, result != nil && result.IsError)
		})
	}
}

// TestNewHTTPServer verifies that the HTTP server is created correctly.
func TestNewHTTPServer(t *testing.T) {
	svcs := newTestServices()
	handler, err := buildMCPHandler(svcs)
	require.NoError(t, err, "Failed to build MCPHandler")

	httpHandler := NewHTTPServer(handler)

	require.NotNil(t, httpHandler)

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
				require.NotNil(t, req, "Expected QueryLogs to be called")
				assert.Equal(t, 100, req.Limit)
				assert.Equal(t, sortOrderDesc, req.SortOrder)
				assert.Empty(t, req.LogLevels)
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
				require.NotNil(t, req, "Expected QueryLogs to be called")
				assert.Equal(t, 100, req.Limit)
				assert.Equal(t, sortOrderDesc, req.SortOrder)
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
				require.NotNil(t, req, "Expected QueryMetrics to be called")
				assert.Nil(t, req.Step)
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
				require.NotNil(t, req, "Expected QueryTraces to be called")
				assert.Equal(t, 100, req.Limit)
				assert.Equal(t, sortOrderDesc, req.SortOrder)
			},
		},
		{
			name:     "alerts_default_limit_and_sort",
			toolName: "query_alerts",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			validateCall: func(t *testing.T, svcs *testServices) {
				req := svcs.alertsIncidents.lastAlertsRequest()
				require.NotNil(t, req, "Expected QueryAlerts to be called")
				require.NotNil(t, req.Limit)
				assert.Equal(t, 100, *req.Limit)
				require.NotNil(t, req.SortOrder)
				assert.Equal(t, sortOrderDesc, string(*req.SortOrder))
			},
		},
		{
			name:     "incidents_default_limit_and_sort",
			toolName: "query_incidents",
			args: map[string]any{
				"namespace":  testNamespace,
				"start_time": testStartTime,
				"end_time":   testEndTime,
			},
			validateCall: func(t *testing.T, svcs *testServices) {
				req := svcs.alertsIncidents.lastIncidentsRequest()
				require.NotNil(t, req, "Expected QueryIncidents to be called")
				require.NotNil(t, req.Limit)
				assert.Equal(t, 100, *req.Limit)
				require.NotNil(t, req.SortOrder)
				assert.Equal(t, sortOrderDesc, string(*req.SortOrder))
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
			require.NoError(t, err, "Failed to call tool")

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
	require.NoError(t, err, "Failed to call tool")

	req := svcs.logs.lastRequest()
	require.NotNil(t, req, "Expected QueryLogs to be called")
	require.NotNil(t, req.SearchScope)
	require.NotNil(t, req.SearchScope.Component)

	scope := req.SearchScope.Component
	assert.Equal(t, "my-org", scope.Namespace)
	assert.Equal(t, "my-project", scope.Project)
	assert.Equal(t, "my-service", scope.Component)
	assert.Equal(t, "production", scope.Environment)
	assert.Equal(t, "my-search", req.SearchPhrase)
	assert.Equal(t, 42, req.Limit)
}

// TestSchemaPropertyTypes verifies that schema properties have the correct JSON types.
func TestSchemaPropertyTypes(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err, "Failed to list tools")

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
			require.True(t, ok, "Expected InputSchema to be map[string]any, got %T", tool.InputSchema)

			properties, ok := schemaMap["properties"].(map[string]any)
			require.True(t, ok, "Properties is not a map")

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

				assert.Equal(t, expectedType, actualType, "Property %q type mismatch", propName)
			}
		})
	}
}
