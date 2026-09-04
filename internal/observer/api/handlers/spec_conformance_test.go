// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

// spec_conformance_test.go validates that the bodies observer actually emits match
// the response schemas declared in openapi/observer-api.yaml.
//
// The generated server checks routing, parameter binding and auth against the spec,
// but not response shape: handlers return types.* and gen.* values through the
// untyped apiResponse passthrough, so nothing else ties a response body to the
// contract.
//
// Modeled on internal/openchoreo-api/api/handlers/http_test_helpers_test.go, which
// does the same for the other generated server in this repo.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// assertConformsToSpec validates one response body against the OpenAPI contract.
//
// Unlike its openchoreo-api counterpart this fails rather than skips when the route
// is absent: every path driven here is a spec operation, so a lookup miss is a real
// defect and not a test-only path.
func assertConformsToSpec(t *testing.T, req *http.Request, rr *httptest.ResponseRecorder) {
	t.Helper()

	swagger, err := gen.GetSwagger()
	require.NoError(t, err, "failed to load OpenAPI spec")
	swagger.Servers = nil // disable server URL matching so local test paths are accepted

	router, err := legacyrouter.NewRouter(swagger)
	require.NoError(t, err, "failed to build OpenAPI router for %s %s", req.Method, req.URL.Path)

	route, pathParams, err := router.FindRoute(req)
	require.NoError(t, err, "route %s %s must be registered in the OpenAPI spec",
		req.Method, req.URL.Path)

	reqIn := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options:    &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	}
	respIn := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: reqIn,
		Status:                 rr.Code,
		Header:                 rr.Header(),
	}
	respIn.SetBodyBytes(rr.Body.Bytes())

	err = openapi3filter.ValidateResponse(context.Background(), respIn)
	require.NoError(t, err, "response does not conform to OpenAPI contract for %s %s -> %d\nbody: %s",
		req.Method, req.URL.Path, rr.Code, rr.Body.String())
}

// unmarshalInto builds a gen response value from JSON.
//
// The alerts and incidents response types cannot be written as struct literals: their
// item schemas are anonymous inline objects in the spec, so oapi-codegen emits deeply
// nested unnameable anonymous structs (gen.AlertsQueryResponse.Alerts is
// *[]struct{ ... Metadata *struct{ AlertRule *struct{ ... } } }). Decoding from JSON
// is the only way to populate them from outside the package; service/alerts_query.go
// works around the same constraint with private mirror structs.
func unmarshalInto(t *testing.T, raw string, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(raw), dst), "test fixture must decode")
}

func mustTime(t *testing.T, s string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err)
	return &parsed
}

// TestResponsesConformToSpec drives one populated success response per operation and
// validates it against the spec.
//
// Payloads are populated rather than zero-valued on purpose: an empty response
// satisfies almost any schema, so a zero value would report conformance that the real
// service does not have.
//
// Three operations are absent by design — queryMetrics, getComponentCosts and
// getRecommendations. Those services return json.RawMessage straight from the metrics
// and finops adapters (service/metrics_adapter.go, service/finops_adapter.go);
// observer never inspects the payload, so any body asserted here would be an invention
// of this test rather than something observer produces. Their conformance can only be
// established against a live adapter.
func TestResponsesConformToSpec(t *testing.T) {
	t.Parallel()

	t.Run("health", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockHealthChecker(t)
		svc.On("Check", mock.Anything).Return(nil)

		h := &Handler{
			baseHandler:   baseHandler{logger: noopLogger()},
			healthService: svc,
		}

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("getOAuthProtectedResourceMetadata", func(t *testing.T) {
		t.Parallel()

		h := &Handler{
			baseHandler: baseHandler{logger: noopLogger()},
			oauthMetadata: OAuthMetadataConfig{
				ResourceName:         "OpenChoreo Observer MCP Server",
				ResourceURL:          "http://localhost:9097/mcp",
				AuthorizationServers: []string{"https://auth.example.com"},
				ScopesSupported:      []string{"openid", "profile", "email"},
				SecurityEnabled:      false,
			},
		}

		req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	// Both scopes plus an empty result, because LogEntry is one schema covering
	// two field populations: component queries fill level and metadata, workflow
	// queries fill neither. A single fixture would only ever exercise one.
	t.Run("queryLogs component-scoped", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockLogsQuerier(t)
		svc.On("QueryLogs", mock.Anything, mock.Anything).Return(&types.LogsQueryResponse{
			Logs: []types.LogEntry{{
				Timestamp: "2026-06-05T03:07:12Z",
				Log:       "hello",
				Level:     "INFO",
				Metadata: &types.LogMetadata{
					ComponentName:   "checkout",
					ProjectName:     "storefront",
					EnvironmentName: "production",
					NamespaceName:   "default",
					ComponentUID:    "5b7f9c2e-1d4a-4c8b-9f3e-2a6d8b0c4e11",
					ProjectUID:      "9c2e5b7f-4a1d-8b4c-3e9f-8b0c2a6d4e22",
					EnvironmentUID:  "2e5b7f9c-1d4a-4c8b-9f3e-0c4e2a6d8b33",
					ContainerName:   "main",
					PodName:         "checkout-7d9f-abcde",
					PodNamespace:    "dp-default",
				},
			}},
			Total:  1,
			TookMs: 5,
		}, nil)

		h := &Handler{
			baseHandler: baseHandler{logger: noopLogger()},
			logsService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	// Mirrors convertWorkflowLogsToResponse (service/logs.go), which sets only
	// Timestamp and Log — Level stays empty because the adapter never assigns it
	// (logs_adapter.go:195-211), and Metadata is never set at all.
	t.Run("queryLogs workflow-scoped", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockLogsQuerier(t)
		svc.On("QueryLogs", mock.Anything, mock.Anything).Return(&types.LogsQueryResponse{
			Logs: []types.LogEntry{{
				Timestamp: "2026-06-05T03:07:12Z",
				Log:       "step completed",
			}},
			Total:  1,
			TookMs: 5,
		}, nil)

		h := &Handler{
			baseHandler: baseHandler{logger: noopLogger()},
			logsService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	// A zero-result query. service/logs.go builds a non-nil empty slice and
	// types.LogsQueryResponse.Logs has no omitempty, so this emits `"logs":[]`.
	t.Run("queryLogs empty result", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockLogsQuerier(t)
		svc.On("QueryLogs", mock.Anything, mock.Anything).Return(&types.LogsQueryResponse{
			Logs:   []types.LogEntry{},
			Total:  0,
			TookMs: 2,
		}, nil)

		h := &Handler{
			baseHandler: baseHandler{logger: noopLogger()},
			logsService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("queryEvents", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockEventsQuerier(t)
		svc.On("QueryEvents", mock.Anything, mock.Anything).Return(&types.EventsQueryResponse{
			Events: []types.EventEntry{{
				Timestamp: "2026-06-05T03:07:12Z",
				Message:   "Created pod",
				Type:      "Normal",
				Reason:    "SuccessfulCreate",
				Metadata: &types.EventMetadata{
					ComponentName:   "checkout",
					ProjectName:     "storefront",
					EnvironmentName: "production",
					NamespaceName:   "default",
					ComponentUID:    "5b7f9c2e-1d4a-4c8b-9f3e-2a6d8b0c4e11",
					ProjectUID:      "9c2e5b7f-4a1d-8b4c-3e9f-8b0c2a6d4e22",
					EnvironmentUID:  "2e5b7f9c-1d4a-4c8b-9f3e-0c4e2a6d8b33",
					ObjectKind:      "Pod",
					ObjectName:      "checkout-7d9f-abcde",
					ObjectNamespace: "dp-default",
				},
			}},
			Total:  1,
			TookMs: 5,
		}, nil)

		h := &Handler{
			baseHandler:   baseHandler{logger: noopLogger()},
			eventsService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/query", validEventsRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("queryRuntimeTopology", func(t *testing.T) {
		t.Parallel()

		start, end := mustTime(t, "2026-06-05T02:07:12Z"), mustTime(t, "2026-06-05T03:07:12Z")

		svc := servicemocks.NewMockMetricsQuerier(t)
		svc.On("QueryRuntimeTopology", mock.Anything, mock.Anything).
			Return(&types.RuntimeTopologyResponse{
				Summary: types.RuntimeTopologySummary{
					StartTime:   *start,
					EndTime:     *end,
					GeneratedAt: *end,
				},
			}, nil)

		h := &Handler{
			baseHandler:    baseHandler{logger: noopLogger()},
			metricsService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/metrics/runtime-topology",
			validRuntimeTopologyRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("queryTraces", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockTracesQuerier(t)
		svc.On("QueryTraces", mock.Anything, mock.Anything).Return(&types.TracesQueryResponse{
			Traces: []types.TraceInfo{{
				TraceID:      "0af7651916cd43dd8448eb211c80319c",
				TraceName:    "GET /checkout",
				SpanCount:    3,
				RootSpanID:   "b7ad6b7169203331",
				RootSpanName: "GET /checkout",
				RootSpanKind: "SPAN_KIND_SERVER",
				StartTime:    mustTime(t, "2026-06-05T03:07:12Z"),
				EndTime:      mustTime(t, "2026-06-05T03:07:13Z"),
				DurationNs:   1000000000,
				HasErrors:    false,
			}},
			Total:  1,
			TookMs: 5,
		}, nil)

		h := &Handler{
			baseHandler:   baseHandler{logger: noopLogger()},
			tracesService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("querySpansForTrace", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockTracesQuerier(t)
		svc.On("QuerySpans", mock.Anything, "trace-1", mock.Anything).
			Return(&types.SpansQueryResponse{
				Spans: []types.SpanInfo{{
					SpanID:       "b7ad6b7169203331",
					SpanName:     "GET /checkout",
					SpanKind:     "SPAN_KIND_SERVER",
					ParentSpanID: "",
					StartTime:    mustTime(t, "2026-06-05T03:07:12Z"),
					EndTime:      mustTime(t, "2026-06-05T03:07:13Z"),
					DurationNs:   1000000000,
					Attributes:   map[string]any{"http.method": "GET"},
				}},
				Total:  1,
				TookMs: 5,
			}, nil)

		h := &Handler{
			baseHandler:   baseHandler{logger: noopLogger()},
			tracesService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query",
			validTracesRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("getSpanDetailsForTrace", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockTracesQuerier(t)
		svc.On("GetSpanDetails", mock.Anything, "trace-1", "span-1").
			Return(&types.SpanInfo{
				SpanID:     "span-1",
				SpanName:   "GET /checkout",
				SpanKind:   "SPAN_KIND_SERVER",
				StartTime:  mustTime(t, "2026-06-05T03:07:12Z"),
				EndTime:    mustTime(t, "2026-06-05T03:07:13Z"),
				DurationNs: 1000000000,
				Attributes: map[string]any{"http.method": "GET"},
			}, nil)

		h := &Handler{
			baseHandler:   baseHandler{logger: noopLogger()},
			tracesService: svc,
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces/trace-1/spans/span-1", nil)
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("queryAlerts", func(t *testing.T) {
		t.Parallel()

		var resp gen.AlertsQueryResponse
		unmarshalInto(t, `{
			"alerts": [{
				"alertId": "alert-1",
				"alertValue": "0.93",
				"incidentEnabled": true,
				"metadata": {
					"alertRule": {
						"name": "high-cpu",
						"description": "CPU above threshold",
						"condition": {
							"interval": "1m",
							"operator": "gt",
							"threshold": 0.9,
							"window": "5m"
						}
					}
				}
			}],
			"total": 1,
			"tookMs": 5
		}`, &resp)

		svc := servicemocks.NewMockAlertIncidentService(t)
		svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(&resp, nil)

		h := &Handler{
			baseHandler:          baseHandler{logger: noopLogger()},
			alertIncidentService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("queryIncidents", func(t *testing.T) {
		t.Parallel()

		var resp gen.IncidentsQueryResponse
		unmarshalInto(t, `{
			"incidents": [{
				"incidentId": "inc-1",
				"alertId": "alert-1",
				"description": "CPU above threshold",
				"acknowledgedAt": "2026-06-05T03:07:12Z",
				"incidentTriggerAiRca": false,
				"incidentTriggerAiCostAnalysis": false,
				"labels": {
					"componentName": "checkout",
					"projectName": "storefront",
					"environmentName": "production",
					"namespaceName": "default"
				}
			}],
			"total": 1,
			"tookMs": 5
		}`, &resp)

		svc := servicemocks.NewMockAlertIncidentService(t)
		svc.On("QueryIncidents", mock.Anything, mock.Anything).Return(&resp, nil)

		h := &Handler{
			baseHandler:          baseHandler{logger: noopLogger()},
			alertIncidentService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("updateIncident", func(t *testing.T) {
		t.Parallel()

		var resp gen.IncidentPutResponse
		unmarshalInto(t, `{
			"incidentId": "inc-1",
			"alertId": "alert-1",
			"status": "acknowledged",
			"description": "CPU above threshold",
			"acknowledgedAt": "2026-06-05T03:07:12Z",
			"incidentTriggerAiRca": false,
			"incidentTriggerAiCostAnalysis": false,
			"labels": {
				"componentName": "checkout",
				"projectName": "storefront",
				"environmentName": "production",
				"namespaceName": "default"
			}
		}`, &resp)

		svc := servicemocks.NewMockAlertIncidentService(t)
		svc.On("UpdateIncident", mock.Anything, "inc-1", mock.Anything).Return(&resp, nil)

		h := &Handler{
			baseHandler:          baseHandler{logger: noopLogger()},
			alertIncidentService: svc,
		}

		raw, err := json.Marshal(gen.IncidentPutRequest{
			Status: gen.Acknowledged,
		})
		require.NoError(t, err, "failed to marshal incident put request")

		req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		assertConformsToSpec(t, req, serve(t, h, req))
	})
	t.Run("queryDoraMetrics", func(t *testing.T) {
		t.Parallel()

		var resp gen.DoraMetricsQueryResponse
		unmarshalInto(t, `{
			"scope": {"namespace": "default", "project": "storefront"},
			"granularity": "daily",
			"window": {
				"startTime": "2026-08-01T00:00:00Z",
				"endTime": "2026-08-31T00:00:00Z",
				"generatedAt": "2026-08-31T00:05:00Z"
			},
			"summary": {
				"deploymentFrequency": {
					"total": 42,
					"perDay": 1.4,
					"classification": "High",
					"deltaPct": 12.5
				},
				"leadTime": {
					"p50Ms": 3600000,
					"p95Ms": 86400000,
					"coverage": 0.8,
					"classification": "Elite",
					"deltaPct": null
				}
			},
			"series": {
				"deploymentFrequency": [
					{"bucketStart": "2026-08-29T00:00:00Z", "count": 2},
					{"bucketStart": "2026-08-30T00:00:00Z", "count": 0}
				],
				"leadTime": [
					{"bucketStart": "2026-08-29T00:00:00Z", "p50Ms": 3600000, "p75Ms": 7200000, "p95Ms": 86400000}
				],
				"changeFailureRate": [
					{"bucketStart": "2026-08-29T00:00:00Z", "rate": 0.5, "failed": 1, "total": 2}
				]
			}
		}`, &resp)

		svc := servicemocks.NewMockInsightsService(t)
		svc.On("QueryDoraMetrics", mock.Anything, mock.Anything).Return(&resp, nil)

		h := &Handler{
			baseHandler:     baseHandler{logger: noopLogger()},
			insightsService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, doraMetricsPath, validDoraRequestBody(t))
		req.Header.Set("Content-Type", "application/json")
		assertConformsToSpec(t, req, serve(t, h, req))
	})

	t.Run("queryDoraDeployments", func(t *testing.T) {
		t.Parallel()

		var resp gen.DoraDeploymentsQueryResponse
		unmarshalInto(t, `{
			"deployments": [{
				"deployedAt": "2026-08-30T10:00:00Z",
				"projectName": "storefront",
				"componentName": "checkout",
				"environmentName": "production",
				"componentRelease": "checkout-v3",
				"commit": "9f2c1ab4d5e6f70819a2b3c4d5e6f70819a2b3c4",
				"outcome": "success",
				"leadTimeMs": 3600000
			}, {
				"deployedAt": "2026-08-30T12:00:00Z",
				"projectName": "storefront",
				"componentName": "checkout",
				"environmentName": "production",
				"componentRelease": "checkout-v4",
				"outcome": "failed",
				"failedBy": "incident",
				"failureReason": "CrashLoopBackOff",
				"incidentId": "inc-7",
				"leadTimeMs": null
			}],
			"totalCount": 1,
			"tookMs": 4
		}`, &resp)

		svc := servicemocks.NewMockInsightsService(t)
		svc.On("QueryDoraDeployments", mock.Anything, mock.Anything).Return(&resp, nil)

		h := &Handler{
			baseHandler:     baseHandler{logger: noopLogger()},
			insightsService: svc,
		}

		req := httptest.NewRequest(http.MethodPost, doraDeploymentsPath, validDoraRequestBody(t))
		req.Header.Set("Content-Type", "application/json")
		assertConformsToSpec(t, req, serve(t, h, req))
	})
}
