// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
)

// apiResponse writes an arbitrary value at an arbitrary status, for the
// operations that cannot use the generated typed responses.
//
// Each generated `<Operation>ResponseObject` interface declares one
// Visit<Operation>Response method, so a single type carrying several of those
// methods satisfies all of their interfaces at once. Every method below does the
// same thing: hand the value to httputil.WriteJSON.
//
// Two groups still need it, for different reasons:
//
//   - QueryMetrics, GetComponentCosts and GetRecommendations return bare `any`.
//     Each is a json.RawMessage passed straight through from the metrics or
//     finops adapter (service/metrics_adapter.go, service/finops_adapter.go);
//     observer never inspects the payload, so typing them would mean decoding
//     bodies it currently proxies untouched.
//   - QueryLogs, QueryEvents, QueryRuntimeTopology, QueryTraces,
//     QuerySpansForTrace, GetSpanDetailsForTrace, Health and
//     GetOAuthProtectedResourceMetadata return types.* values or ad-hoc maps
//     rather than gen.* ones, so a generated type would need a conversion per
//     response. QueryLogs additionally sits behind the logs oneOf union wrapper.
//
// The remaining three operations — QueryAlerts, QueryIncidents and
// UpdateIncident — return gen.* typed responses, so their bodies and statuses
// are compiler-checked against the spec. Anything routed through here is not,
// which is what spec_conformance_test.go exists to cover.
type apiResponse struct {
	status int
	body   any
}

// jsonResponse builds a success response carrying the service's own value.
func jsonResponse(status int, body any) apiResponse {
	return apiResponse{status: status, body: body}
}

// errorResponse builds the standard observer error payload. The shape comes from
// errorPayload in errors.go, which the generated-server hooks reuse.
func errorResponse(status int, title gen.ErrorResponseTitle, errorCode, message string) apiResponse {
	return apiResponse{status: status, body: errorPayload(title, errorCode, message)}
}

func (resp apiResponse) write(w http.ResponseWriter) error {
	return httputil.WriteJSON(w, resp.status, resp.body)
}

// One method per operation still routed through apiResponse. Each generated
// ResponseObject interface needs its own method name; all of them do the same
// thing.

func (resp apiResponse) VisitHealthResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitGetOAuthProtectedResourceMetadataResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitQueryLogsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitQueryEventsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitQueryMetricsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitQueryRuntimeTopologyResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitQueryTracesResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitQuerySpansForTraceResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitGetSpanDetailsForTraceResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitGetComponentCostsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitGetRecommendationsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}
