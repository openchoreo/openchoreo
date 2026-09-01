// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

// GetClusterLogs handles GET /api/v1alpha1/cluster-logs.
//
// The contract is defined; the query path is not implemented yet, so this
// answers 501 rather than leaving the generated strict interface unsatisfied.
func (h *Handler) GetClusterLogs(
	_ context.Context,
	_ gen.GetClusterLogsRequestObject,
) (gen.GetClusterLogsResponseObject, error) {
	return errorResponse(
		http.StatusNotImplemented,
		gen.NotImplemented,
		"",
		"Cluster logs query is not implemented yet",
	), nil
}
