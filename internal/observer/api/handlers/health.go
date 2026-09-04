// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

// Health handles GET /health
func (h *Handler) Health(
	ctx context.Context,
	_ gen.HealthRequestObject,
) (gen.HealthResponseObject, error) {
	if err := h.healthService.Check(ctx); err != nil {
		h.logger.Error("Health check failed", "error", err)
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{
			"status": "unhealthy",
			"error":  "service unavailable",
		}), nil
	}

	return jsonResponse(http.StatusOK, map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
	}), nil
}
