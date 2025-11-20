// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t *Toolsets) RegisterExplainSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "explain_schema",
		Description: "Get the schema definition of a Kubernetes resource in structured JSON format. " +
			"Returns detailed information about resource fields including types, descriptions, and required status. " +
			"Use this to understand OpenChoreo resources like Component, Project, Environment, etc. " +
			"Optionally provide a path to drill down into nested fields (e.g., 'spec', 'spec.build'). " +
			"The response includes: group, kind, version, field (if path specified), type, description, " +
			"properties array with field details, and required fields list.",
		InputSchema: createSchema(map[string]any{
			"kind": stringProperty("The Kubernetes resource kind to explain (e.g., 'Component', 'Project', 'Environment')"),
			"path": stringProperty("Optional: field path to drill down into (e.g., 'spec', 'spec.build', 'metadata')"),
		}, []string{"kind"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.SchemaToolset.ExplainSchema(ctx, args.Kind, args.Path)
		return handleToolResult(result, err)
	})
}
