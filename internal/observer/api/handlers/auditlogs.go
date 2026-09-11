// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// QueryAuditLogs handles POST /api/v1alpha1/audit-logs/query.
func (h *Handler) QueryAuditLogs(
	ctx context.Context,
	request gen.QueryAuditLogsRequestObject,
) (gen.QueryAuditLogsResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "request body is required"), nil
	}

	req, err := toTypesAuditLogsQuery(*request.Body)
	if err != nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}
	if err := ValidateAuditLogsQueryRequest(req); err != nil {
		h.logger.Debug("Audit logs request validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.auditLogsService == nil {
		h.logger.Error("Audit logs service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1AuditLogsServiceNotReady,
			"Audit logs service is not initialized",
		), nil
	}

	result, err := h.auditLogsService.QueryAuditLogs(ctx, req)
	if err != nil {
		return h.auditLogsError(err), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// QueryAuditLogFilterValues handles POST /api/v1alpha1/audit-logs/filter-values.
func (h *Handler) QueryAuditLogFilterValues(
	ctx context.Context,
	request gen.QueryAuditLogFilterValuesRequestObject,
) (gen.QueryAuditLogFilterValuesResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "request body is required"), nil
	}

	req, err := toTypesAuditLogFilterValues(*request.Body)
	if err != nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}
	if err := ValidateAuditLogFilterValuesRequest(req); err != nil {
		h.logger.Debug("Audit log filter values request validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.auditLogsService == nil {
		h.logger.Error("Audit logs service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1AuditLogsServiceNotReady,
			"Audit logs service is not initialized",
		), nil
	}

	result, err := h.auditLogsService.QueryAuditLogFilterValues(ctx, req)
	if err != nil {
		return h.auditLogsError(err), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// auditLogsError maps audit logs service errors onto responses. Shared by both
// operations: the sentinels are disjoint, so one table serves both without
// either being able to produce the other's.
//
// The two not-supported sentinels carry different codes on purpose. A module may
// serve records while unable to aggregate, and a client told only that "audit
// logs are not supported" would abandon records that in fact work.
func (h *Handler) auditLogsError(err error) apiResponse {
	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied")
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
	case errors.Is(err, service.ErrAuditLogsNotSupported):
		h.logger.Warn("Logs adapter does not support audit logs")
		return errorResponse(
			http.StatusNotImplemented,
			gen.NotImplemented,
			types.ErrorCodeV1AuditLogsNotSupported,
			"The configured logs adapter does not support audit logs",
		)
	case errors.Is(err, service.ErrAuditLogFilterValuesNotSupported):
		h.logger.Warn("Logs adapter does not support audit log filter values")
		return errorResponse(
			http.StatusNotImplemented,
			gen.NotImplemented,
			types.ErrorCodeV1AuditLogsFilterValuesNotSupported,
			"The configured logs adapter does not support audit log filter values",
		)
	case errors.Is(err, service.ErrAuditLogsCursorExpired):
		// A 4xx, never an empty page: an empty page reads as the end of the
		// trail, and the caller's remedy is to restart rather than stop.
		return errorResponse(
			http.StatusBadRequest,
			gen.BadRequest,
			types.ErrorCodeV1AuditLogsCursorExpired,
			"The audit logs cursor has expired; restart the query",
		)
	}

	errorCode := types.ErrorCodeV1AuditLogsInternalGeneric
	if errors.Is(err, service.ErrAuditLogsRetrieval) {
		errorCode = types.ErrorCodeV1AuditLogsRetrievalFailed
	}
	h.logger.Error("Failed to retrieve audit logs", "error", err)
	return errorResponse(
		http.StatusInternalServerError,
		gen.InternalServerError,
		errorCode,
		"Failed to retrieve audit logs",
	)
}
