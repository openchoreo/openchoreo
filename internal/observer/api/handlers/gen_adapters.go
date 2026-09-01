// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// apiResponse is the single response type used by every public operation.
//
// The generated `<Operation>ResponseObject` interfaces each declare one
// Visit<Operation>Response method, so one type implementing all fourteen
// satisfies all of them. It writes the value it is given with the same
// httputil.WriteJSON the handlers used before the migration, which makes every
// response byte-identical to what the pre-migration handlers produced.
//
// Why not the generated typed responses (gen.QueryLogs200JSONResponse and
// friends):
//
//   - Three services return bare `any` (QueryMetrics, GetComponentCosts,
//     GetRecommendations) and gen.MetricsQueryResponse is a oneOf union, so
//     there is no typed value to return without re-encoding through a union
//     wrapper.
//   - The rest return types.* values, not gen.* ones. Converting them would
//     mean a schema round-trip per response, which can silently drop a field
//     that types.* has and the spec's response schema does not.
//
// This is the shape traces.go already used before the migration: bind the
// generated request type, convert to the internal type, write the service's own
// response. What the generated server is being adopted for is the half that was
// genuinely missing -- routing, path and query binding, and the security scheme
// that drives auth. Response schemas were never type-checked against the spec
// here, before or after; see §7 of the migration plan, whose criteria cover
// routing, parameters and auth.
type apiResponse struct {
	status int
	body   any
}

// jsonResponse builds a success response carrying the service's own value.
func jsonResponse(status int, body any) apiResponse {
	return apiResponse{status: status, body: body}
}

// errorResponse builds the standard observer error payload, identical to what
// baseHandler.writeErrorResponse produced.
func errorResponse(status int, title gen.ErrorResponseTitle, errorCode, message string) apiResponse {
	return apiResponse{status: status, body: errorPayload(title, errorCode, message)}
}

// errorPayload builds a gen.ErrorResponse. Kept separate from errorResponse so
// the generated-server error hooks in wiring.go can reuse the same shape.
func errorPayload(title gen.ErrorResponseTitle, errorCode, message string) gen.ErrorResponse {
	return gen.ErrorResponse{
		Title:     &title,
		ErrorCode: &errorCode,
		Message:   &message,
	}
}

func (resp apiResponse) write(w http.ResponseWriter) error {
	return httputil.WriteJSON(w, resp.status, resp.body)
}

// The fourteen public operations. Each generated ResponseObject interface needs
// its own method name; all of them do the same thing.

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

func (resp apiResponse) VisitQueryAlertsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitQueryIncidentsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitUpdateIncidentResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitGetComponentCostsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp apiResponse) VisitGetRecommendationsResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

// ---------------------------------------------------------------------------
// Request adapters: generated body types -> internal service types
// ---------------------------------------------------------------------------

// remarshalJSON converts between two types that describe the same JSON schema,
// by encoding the source and decoding into the destination.
//
// Both sides here derive from openapi/observer-api.yaml -- the gen.* types are
// generated from it and the types.* types are hand-written against it -- so
// their JSON representations agree by construction. Round-tripping is therefore
// more faithful than a field-by-field mapper, and it is the only approach that
// preserves types.SearchScope.UnmarshalJSON: the generated oneOf wrapper holds
// the scope as raw JSON, and re-decoding it runs the hand-written
// discriminator, including its rejection of a searchScope that mixes
// workflowRunName with project/component/environment.
//
// A field-by-field mapper would have to reimplement that discrimination, and
// would drift the moment either side gains a field.
func remarshalJSON(src, dst any) error {
	raw, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decoding request: %w", err)
	}
	return nil
}

// rfc3339OrEmpty renders a generated time.Time as the string the internal types
// expect, mapping the zero value to "".
//
// Both matter:
//
//   - The generated time fields have no omitempty, so an absent startTime
//     decodes to the zero time. Formatting that yields
//     "0001-01-01T00:00:00Z", which parses fine -- so ValidateTimeRange's
//     "startTime is required" would never fire and the caller would instead get
//     "query time range cannot exceed N days". Mapping zero to "" keeps the
//     original message.
//   - RFC3339Nano rather than RFC3339, because RFC3339 truncates sub-second
//     precision that the pre-migration handlers passed through verbatim.
func rfc3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

// toTypesLogsQuery converts the generated logs query body to the internal type.
func toTypesLogsQuery(src gen.LogsQueryRequest) (*types.LogsQueryRequest, error) {
	var dst types.LogsQueryRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

// toTypesEventsQuery converts the generated events query body to the internal type.
func toTypesEventsQuery(src gen.EventsQueryRequest) (*types.EventsQueryRequest, error) {
	var dst types.EventsQueryRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

// toTypesMetricsQuery converts the generated metrics query body to the internal type.
func toTypesMetricsQuery(src gen.MetricsQueryRequest) (*types.MetricsQueryRequest, error) {
	var dst types.MetricsQueryRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

// toTypesRuntimeTopology converts the generated runtime-topology body to the
// internal type.
func toTypesRuntimeTopology(src gen.RuntimeTopologyRequest) (*types.RuntimeTopologyRequest, error) {
	var dst types.RuntimeTopologyRequest
	if err := remarshalJSON(src, &dst); err != nil {
		return nil, err
	}
	dst.StartTime = rfc3339OrEmpty(src.StartTime)
	dst.EndTime = rfc3339OrEmpty(src.EndTime)
	return &dst, nil
}

// toTypesCostQuery converts the generated cost-query path and query parameters
// to the internal type.
//
// Field-mapped rather than remarshalled: types.CostQueryRequest carries no JSON
// tags, so there is no shared JSON representation to round-trip through.
//
// The tradeoff is that this mapping is not compiler-checked against the spec: a
// new query parameter added to the spec silently arrives as a zero value here
// rather than failing to build, unlike the remarshalled bodies above. Add the
// field here when you add one there.
func toTypesCostQuery(
	namespace, environment string,
	params gen.GetComponentCostsParams,
) *types.CostQueryRequest {
	return &types.CostQueryRequest{
		Namespace:   namespace,
		Environment: environment,
		Project:     derefString(params.Project),
		Component:   derefString(params.Component),
		StartTime:   rfc3339OrEmpty(params.StartTime),
		EndTime:     rfc3339OrEmpty(params.EndTime),
		Granularity: derefString(params.Granularity),
	}
}

// toTypesRecommendationQuery converts the generated recommendation-query path
// and query parameters to the internal type. Field-mapped for the same reason as
// toTypesCostQuery.
func toTypesRecommendationQuery(
	namespace, environment string,
	params gen.GetRecommendationsParams,
) *types.RecommendationQueryRequest {
	return &types.RecommendationQueryRequest{
		Namespace:   namespace,
		Environment: environment,
		Project:     derefString(params.Project),
		Component:   derefString(params.Component),
		StartTime:   rfc3339OrEmpty(params.StartTime),
		EndTime:     rfc3339OrEmpty(params.EndTime),
	}
}
