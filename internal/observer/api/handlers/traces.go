// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// QueryTraces handles POST /api/v1alpha1/traces/query
func (h *Handler) QueryTraces(w http.ResponseWriter, r *http.Request) {
	// 1. BIND REQUEST (from generated type)
	var genReq gen.TracesQueryRequest
	if err := httputil.BindJSON(r, &genReq); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "Invalid request format")
		return
	}

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

	// 2. VALIDATE REQUEST
	if err := ValidateTracesQueryRequest(&genReq); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, types.ErrorCodeV1TracesInvalidRequest, err.Error())
		return
	}

	// 3. CHECK SERVICE INITIALIZATION
	ctx := r.Context()
	if h.tracesService == nil {
		h.logger.Error("Traces service is not initialized")
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1TracesServiceNotReady,
			"Traces service is not initialized",
		)
		return
	}

	// 4. CALL SERVICE (authorization is enforced by the service layer)
	result, err := h.tracesService.QueryTraces(ctx, req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
			return
		}
		h.logger.Error("Failed to query traces", "error", err)
		errorCode := types.ErrorCodeV1TracesInternalGeneric
		switch {
		case errors.Is(err, service.ErrTracesResolveSearchScope):
			errorCode = types.ErrorCodeV1TracesResolverFailed
		case errors.Is(err, service.ErrTracesRetrieval):
			errorCode = types.ErrorCodeV1TracesRetrievalFailed
		case errors.Is(err, service.ErrTracesInvalidRequest):
			errorCode = types.ErrorCodeV1TracesInvalidRequest
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, errorCode, "Invalid request")
			return
		}
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve traces",
		)
		return
	}

	// 5. CONVERT TO GENERATED TYPE AND RETURN
	genResp := convertTracesResponseToGen(result)
	h.writeJSON(w, http.StatusOK, genResp)
}

// QuerySpansForTrace handles POST /api/v1alpha1/traces/{traceId}/spans/query
func (h *Handler) QuerySpansForTrace(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceId")

	// 1. BIND REQUEST (from generated type)
	var genReq gen.TracesQueryRequest
	if err := httputil.BindJSON(r, &genReq); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "Invalid request format")
		return
	}

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

	// 2. VALIDATE REQUEST
	if err := ValidateTracesQueryRequest(&genReq); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, types.ErrorCodeV1TracesInvalidRequest, err.Error())
		return
	}

	// 3. CHECK SERVICE INITIALIZATION
	ctx := r.Context()
	if h.tracesService == nil {
		h.logger.Error("Traces service is not initialized")
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1TracesServiceNotReady,
			"Traces service is not initialized",
		)
		return
	}

	// 4. CALL SERVICE (authorization is enforced by the service layer)
	result, err := h.tracesService.QuerySpans(ctx, traceID, req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
			return
		}
		h.logger.Error("Failed to query spans", "error", err)
		errorCode := types.ErrorCodeV1TracesInternalGeneric
		switch {
		case errors.Is(err, service.ErrTracesRetrieval):
			errorCode = types.ErrorCodeV1TracesRetrievalFailed
		case errors.Is(err, service.ErrTracesInvalidRequest):
			errorCode = types.ErrorCodeV1TracesInvalidRequest
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, errorCode, "Invalid request")
			return
		}
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve spans",
		)
		return
	}

	// 5. CONVERT TO GENERATED TYPE AND RETURN
	genResp := convertSpansResponseToGen(result)
	h.writeJSON(w, http.StatusOK, genResp)
}

// GetSpanDetailsForTrace handles GET /api/v1alpha1/traces/{traceId}/spans/{spanId}
func (h *Handler) GetSpanDetailsForTrace(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceId")
	spanID := r.PathValue("spanId")

	h.logger.Debug("GetSpanDetailsForTrace called", "traceId", traceID, "spanId", spanID)

	// 1. VALIDATE PATH PARAMETERS
	if traceID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, types.ErrorCodeV1TracesInvalidRequest, "traceId is required")
		return
	}
	if spanID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, types.ErrorCodeV1TracesInvalidRequest, "spanId is required")
		return
	}

	// 2. CHECK SERVICE INITIALIZATION
	if h.tracesService == nil {
		h.logger.Error("Traces service is not initialized")
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1TracesServiceNotReady,
			"Traces service is not initialized",
		)
		return
	}

	// 3. CALL SERVICE
	ctx := r.Context()
	spanInfo, err := h.tracesService.GetSpanDetails(ctx, traceID, spanID)
	if err != nil {
		h.logger.Error("Failed to get span details", "error", err)
		errorCode := types.ErrorCodeV1TracesInternalGeneric
		switch {
		case errors.Is(err, service.ErrSpanNotFound):
			h.writeErrorResponse(w, http.StatusNotFound, gen.NotFound, types.ErrorCodeV1TracesSpanNotFound, "Span not found")
			return
		case errors.Is(err, service.ErrTracesRetrieval):
			errorCode = types.ErrorCodeV1TracesRetrievalFailed
		case errors.Is(err, service.ErrTracesInvalidRequest):
			errorCode = types.ErrorCodeV1TracesInvalidRequest
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, errorCode, "Invalid request")
			return
		}
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve span details",
		)
		return
	}

	// 4. CONVERT TO GENERATED TYPE AND RETURN
	genResp := convertSpanDetailsToGen(spanInfo)
	h.writeJSON(w, http.StatusOK, genResp)
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
func convertSpansResponseToGen(resp *types.SpansQueryResponse) *gen.TraceSpansQueryResponse {
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
		if span.ParentSpanID != "" {
			spanData[i]["parentSpanId"] = span.ParentSpanID
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
