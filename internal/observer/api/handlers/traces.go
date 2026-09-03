// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// QueryTraces handles POST /api/v1alpha1/traces/query
func (h *Handler) QueryTraces(
	ctx context.Context,
	request gen.QueryTracesRequestObject,
) (gen.QueryTracesResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}
	genReq := *request.Body

	// Convert from generated type to internal type
	sort := defaultSortOrder
	if genReq.SortOrder != nil {
		sort = string(*genReq.SortOrder)
	}

	req := &types.TracesQueryRequest{
		StartTime: genReq.StartTime,
		EndTime:   genReq.EndTime,
		Limit:     derefInt(genReq.Limit, 100),
		SortOrder: sort,
		SearchScope: types.ComponentSearchScope{
			Namespace:   genReq.SearchScope.Namespace,
			Project:     derefString(genReq.SearchScope.Project),
			Component:   derefString(genReq.SearchScope.Component),
			Environment: derefString(genReq.SearchScope.Environment),
		},
	}

	if err := ValidateTracesQueryRequest(&genReq); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest,
			types.ErrorCodeV1TracesInvalidRequest, err.Error()), nil
	}

	if h.tracesService == nil {
		h.logger.Error("Traces service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1TracesServiceNotReady,
			"Traces service is not initialized",
		), nil
	}

	// Authorization is enforced by the service layer.
	result, err := h.tracesService.QueryTraces(ctx, req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		}
		h.logger.Error("Failed to query traces", "error", err)
		errorCode := types.ErrorCodeV1TracesInternalGeneric
		switch {
		case errors.Is(err, service.ErrScopeAuthFailed):
			return errorResponse(
				http.StatusInternalServerError,
				gen.InternalServerError,
				types.ErrorCodeV1ScopeAuthFailed,
				"",
			), nil
		case errors.Is(err, service.ErrTracesResolveSearchScope):
			errorCode = types.ErrorCodeV1TracesResolverFailed
		case errors.Is(err, service.ErrTracesRetrieval):
			errorCode = types.ErrorCodeV1TracesRetrievalFailed
		case errors.Is(err, service.ErrTracesInvalidRequest):
			errorCode = types.ErrorCodeV1TracesInvalidRequest
			return errorResponse(http.StatusBadRequest, gen.BadRequest, errorCode, "Invalid request"), nil
		}
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve traces",
		), nil
	}

	return jsonResponse(http.StatusOK, convertTracesResponseToGen(result)), nil
}

// QuerySpansForTrace handles POST /api/v1alpha1/traces/{traceId}/spans/query
func (h *Handler) QuerySpansForTrace(
	ctx context.Context,
	request gen.QuerySpansForTraceRequestObject,
) (gen.QuerySpansForTraceResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}
	traceID := request.TraceId
	genReq := *request.Body

	// Convert from generated type to internal type
	sort := defaultSortOrder
	if genReq.SortOrder != nil {
		sort = string(*genReq.SortOrder)
	}

	req := &types.TracesQueryRequest{
		StartTime:         genReq.StartTime,
		EndTime:           genReq.EndTime,
		Limit:             derefInt(genReq.Limit, 100),
		SortOrder:         sort,
		IncludeAttributes: derefBool(genReq.IncludeAttributes),
		SearchScope: types.ComponentSearchScope{
			Namespace:   genReq.SearchScope.Namespace,
			Project:     derefString(genReq.SearchScope.Project),
			Component:   derefString(genReq.SearchScope.Component),
			Environment: derefString(genReq.SearchScope.Environment),
		},
	}

	if err := ValidateTracesQueryRequest(&genReq); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest,
			types.ErrorCodeV1TracesInvalidRequest, err.Error()), nil
	}

	if h.tracesService == nil {
		h.logger.Error("Traces service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1TracesServiceNotReady,
			"Traces service is not initialized",
		), nil
	}

	// Authorization is enforced by the service layer.
	result, err := h.tracesService.QuerySpans(ctx, traceID, req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		}
		h.logger.Error("Failed to query spans", "error", err)
		errorCode := types.ErrorCodeV1TracesInternalGeneric
		switch {
		case errors.Is(err, service.ErrScopeAuthFailed):
			return errorResponse(
				http.StatusInternalServerError,
				gen.InternalServerError,
				types.ErrorCodeV1ScopeAuthFailed,
				"",
			), nil
		case errors.Is(err, service.ErrTracesRetrieval):
			errorCode = types.ErrorCodeV1TracesRetrievalFailed
		case errors.Is(err, service.ErrTracesInvalidRequest):
			errorCode = types.ErrorCodeV1TracesInvalidRequest
			return errorResponse(http.StatusBadRequest, gen.BadRequest, errorCode, "Invalid request"), nil
		}
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve spans",
		), nil
	}

	return jsonResponse(http.StatusOK, convertSpansResponseToGen(result, req.IncludeAttributes)), nil
}

// GetSpanDetailsForTrace handles GET /api/v1alpha1/traces/{traceId}/spans/{spanId}
func (h *Handler) GetSpanDetailsForTrace(
	ctx context.Context,
	request gen.GetSpanDetailsForTraceRequestObject,
) (gen.GetSpanDetailsForTraceResponseObject, error) {
	traceID := request.TraceId
	spanID := request.SpanId

	h.logger.Debug("GetSpanDetailsForTrace called", "traceId", traceID, "spanId", spanID)

	if traceID == "" {
		return errorResponse(http.StatusBadRequest, gen.BadRequest,
			types.ErrorCodeV1TracesInvalidRequest, "traceId is required"), nil
	}
	if spanID == "" {
		return errorResponse(http.StatusBadRequest, gen.BadRequest,
			types.ErrorCodeV1TracesInvalidRequest, "spanId is required"), nil
	}

	if h.tracesService == nil {
		h.logger.Error("Traces service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1TracesServiceNotReady,
			"Traces service is not initialized",
		), nil
	}

	spanInfo, err := h.tracesService.GetSpanDetails(ctx, traceID, spanID)
	if err != nil {
		h.logger.Error("Failed to get span details", "error", err)
		errorCode := types.ErrorCodeV1TracesInternalGeneric
		switch {
		case errors.Is(err, service.ErrScopeAuthFailed):
			return errorResponse(
				http.StatusInternalServerError,
				gen.InternalServerError,
				types.ErrorCodeV1ScopeAuthFailed,
				"",
			), nil
		case errors.Is(err, service.ErrSpanNotFound):
			return errorResponse(http.StatusNotFound, gen.NotFound,
				types.ErrorCodeV1TracesSpanNotFound, "Span not found"), nil
		case errors.Is(err, service.ErrTracesRetrieval):
			errorCode = types.ErrorCodeV1TracesRetrievalFailed
		case errors.Is(err, service.ErrTracesInvalidRequest):
			errorCode = types.ErrorCodeV1TracesInvalidRequest
			return errorResponse(http.StatusBadRequest, gen.BadRequest, errorCode, "Invalid request"), nil
		}
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve span details",
		), nil
	}

	return jsonResponse(http.StatusOK, convertSpanDetailsToGen(spanInfo)), nil
}

// Helper functions

func derefInt(ptr *int, defaultVal int) int {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

func derefString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func derefBool(ptr *bool) bool {
	if ptr == nil {
		return false
	}
	return *ptr
}

// convertTracesResponseToGen converts internal response to generated type
func convertTracesResponseToGen(resp *types.TracesQueryResponse) *gen.TracesQueryResponse {
	if resp == nil {
		return nil
	}

	// Convert traces to map and then to JSON for proper struct marshaling
	traceData := make([]map[string]interface{}, len(resp.Traces))
	for i, trace := range resp.Traces {
		traceData[i] = map[string]interface{}{
			"traceId":      trace.TraceID,
			"traceName":    trace.TraceName,
			"spanCount":    trace.SpanCount,
			"rootSpanId":   trace.RootSpanID,
			"rootSpanName": trace.RootSpanName,
			"rootSpanKind": trace.RootSpanKind,
			"startTime":    trace.StartTime,
			"endTime":      trace.EndTime,
			"durationNs":   trace.DurationNs,
			"hasErrors":    trace.HasErrors,
		}
	}

	// Use JSON round-trip to properly construct the generated type
	mapResp := map[string]interface{}{
		"traces": traceData,
		"total":  resp.Total,
		"tookMs": resp.TookMs,
	}

	jsonData, _ := json.Marshal(mapResp)
	var genResp gen.TracesQueryResponse
	if err := json.Unmarshal(jsonData, &genResp); err != nil {
		return nil
	}
	return &genResp
}

// convertSpansResponseToGen converts internal response to generated type
func convertSpansResponseToGen(resp *types.SpansQueryResponse, includeAttributes bool) *gen.TraceSpansQueryResponse {
	if resp == nil {
		return nil
	}

	// Convert spans to map and then to JSON for proper struct marshaling
	spanData := make([]map[string]interface{}, len(resp.Spans))
	for i, span := range resp.Spans {
		spanData[i] = map[string]interface{}{
			"spanId":     span.SpanID,
			"spanName":   span.SpanName,
			"startTime":  span.StartTime,
			"endTime":    span.EndTime,
			"durationNs": span.DurationNs,
		}
		if span.SpanKind != "" {
			spanData[i]["spanKind"] = span.SpanKind
		}
		if span.Status != nil {
			spanData[i]["status"] = span.Status
		}
		if span.ParentSpanID != "" {
			spanData[i]["parentSpanId"] = span.ParentSpanID
		}
		if includeAttributes {
			if span.Attributes != nil {
				spanData[i]["attributes"] = span.Attributes
			}
			if span.ResourceAttributes != nil {
				spanData[i]["resourceAttributes"] = span.ResourceAttributes
			}
		}
	}

	// Use JSON round-trip to properly construct the generated type
	mapResp := map[string]interface{}{
		"spans":  spanData,
		"total":  resp.Total,
		"tookMs": resp.TookMs,
	}

	jsonData, _ := json.Marshal(mapResp)
	var genResp gen.TraceSpansQueryResponse
	if err := json.Unmarshal(jsonData, &genResp); err != nil {
		return nil
	}
	return &genResp
}

// convertSpanDetailsToGen converts a single span to the generated type
func convertSpanDetailsToGen(span *types.SpanInfo) map[string]interface{} {
	if span == nil {
		return nil
	}

	spanData := map[string]interface{}{
		"spanId":     span.SpanID,
		"spanName":   span.SpanName,
		"startTime":  span.StartTime,
		"endTime":    span.EndTime,
		"durationNs": span.DurationNs,
	}
	if span.SpanKind != "" {
		spanData["spanKind"] = span.SpanKind
	}
	if span.Status != nil {
		spanData["status"] = span.Status
	}
	if span.ParentSpanID != "" {
		spanData["parentSpanId"] = span.ParentSpanID
	}
	if span.Attributes != nil {
		spanData["attributes"] = span.Attributes
	}
	if span.ResourceAttributes != nil {
		spanData["resourceAttributes"] = span.ResourceAttributes
	}

	return spanData
}
