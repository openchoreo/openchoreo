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

// GetClusterLogs handles GET /api/v1alpha1/cluster-logs.
func (h *Handler) GetClusterLogs(
	ctx context.Context,
	request gen.GetClusterLogsRequestObject,
) (gen.GetClusterLogsResponseObject, error) {
	req, err := toTypesClusterLogsQuery(request.Params)
	if err != nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if err := ValidateClusterLogsQueryRequest(req); err != nil {
		h.logger.Debug("Cluster logs request validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.clusterLogsService == nil {
		h.logger.Error("Cluster logs service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1ClusterLogsServiceNotReady,
			"Cluster logs service is not initialized",
		), nil
	}

	result, err := h.clusterLogsService.QueryClusterLogs(ctx, req)
	if err != nil {
		return h.clusterLogsError(err), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// clusterLogsError maps cluster logs service errors onto responses.
func (h *Handler) clusterLogsError(err error) gen.GetClusterLogsResponseObject {
	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied")
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
	case errors.Is(err, service.ErrClusterLogsNotSupported):
		h.logger.Warn("Logs adapter does not support cluster logs")
		return errorResponse(
			http.StatusNotImplemented,
			gen.NotImplemented,
			types.ErrorCodeV1ClusterLogsNotSupported,
			"The configured logs adapter does not support cluster logs",
		)
	}

	errorCode := types.ErrorCodeV1ClusterLogsInternalGeneric
	if errors.Is(err, service.ErrClusterLogsRetrieval) {
		errorCode = types.ErrorCodeV1ClusterLogsRetrievalFailed
	}
	h.logger.Error("Failed to retrieve cluster logs", "error", err)
	return errorResponse(
		http.StatusInternalServerError,
		gen.InternalServerError,
		errorCode,
		"Failed to retrieve cluster logs",
	)
}
