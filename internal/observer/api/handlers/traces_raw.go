// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

// QueryRawTraces handles POST /api/v1alpha1/traces/raw-query.
// The operation is declared in the API but not served yet.
func (h *Handler) QueryRawTraces(
	_ context.Context,
	_ gen.QueryRawTracesRequestObject,
) (gen.QueryRawTracesResponseObject, error) {
	return errorResponse(http.StatusNotImplemented, gen.NotImplemented, "",
		"Raw trace queries are not implemented yet"), nil
}
