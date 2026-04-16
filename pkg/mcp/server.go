// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// NewHTTPServer creates an MCP HTTP handler backed by a single shared server.
//
// When pdp is non-nil the server installs a receiving middleware that filters
// tools/list results and guards tools/call invocations based on the
// authenticated user's permissions derived from their JWT token.
// When pdp is nil (authz disabled) all registered tools are visible and
// callable — the service layer still enforces authz independently.
func NewHTTPServer(toolsets *tools.Toolsets, pdp authzcore.PDP) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "openchoreo-api",
		Version: "1.0.0",
	}, nil)
	perms := toolsets.Register(server)
	if pdp != nil {
		server.AddReceivingMiddleware(tools.NewToolFilterMiddleware(pdp, perms))
	}
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
}

// NewSTDIO creates an MCP server for STDIO transport (local CLI usage).
// Permission filtering is intentionally skipped for STDIO: there is no
// HTTP request, no JWT token, and no authenticated user identity available.
// The service layer still enforces authz for any operations that require it.
func NewSTDIO(toolsets *tools.Toolsets) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "openchoreo-cli",
		Version: "1.0.0",
	}, nil)
	toolsets.Register(server)
	return server
}
