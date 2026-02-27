// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"net/http"
	"time"
)

// Health handles GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.healthService.Check(ctx); err != nil {
		h.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
