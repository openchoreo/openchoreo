// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func NewHTTPServer(tools *tools.Toolsets) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "openchoreo-api",
		Version: "1.0.0",
	}, nil)
	tools.Register(server)
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
}

func NewSTDIO(tools *tools.Toolsets) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "openchoreo-cli",
		Version: "1.0.0",
	}, nil)
	tools.Register(server)
	return server
}
