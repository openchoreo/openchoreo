// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

type MCPHandler struct {
	Services *services.Services
	Logger   *slog.Logger
}

// warnIfTruncated logs a warning if the result count is large,
// indicating potential context window usage for MCP clients.
func (h *MCPHandler) warnIfTruncated(resourceType string, count int) {
	const LargeResultThreshold = 1000
	if count >= LargeResultThreshold {
		h.Logger.Warn("Large result set returned to MCP",
			"resource_type", resourceType,
			"count", count,
			"threshold", LargeResultThreshold,
			"hint", "Large datasets may consume significant context window")
	}
}
