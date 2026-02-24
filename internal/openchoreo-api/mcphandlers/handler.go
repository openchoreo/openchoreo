// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// MCPHandler is a thin adapter between MCP tool interfaces and the new service layer.
type MCPHandler struct {
	services *handlerservices.Services
}

// NewMCPHandler creates an MCPHandler backed by the new service layer.
func NewMCPHandler(svc *handlerservices.Services) *MCPHandler {
	return &MCPHandler{services: svc}
}
