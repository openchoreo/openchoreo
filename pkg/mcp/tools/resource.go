// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t *Toolsets) RegisterApplyResource(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "apply_resource",
		Description: "Apply a Kubernetes resource to the cluster (kubectl-like operation). Creates or updates " +
			"the resource using server-side apply. Only supports resources with 'openchoreo.dev' API group.",
		InputSchema: createSchema(map[string]any{
			"resource": map[string]any{
				"type":        "object",
				"description": "The Kubernetes resource object (must include apiVersion, kind, metadata.name)",
			},
		}, []string{"resource"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Resource map[string]interface{} `json:"resource"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ResourceToolset.ApplyResource(ctx, args.Resource)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteResource(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "delete_resource",
		Description: "Delete a Kubernetes resource from the cluster (kubectl-like operation). " +
			"Only supports resources with 'openchoreo.dev' API group.",
		InputSchema: createSchema(map[string]any{
			"resource": map[string]any{
				"type":        "object",
				"description": "The Kubernetes resource object (must include apiVersion, kind, metadata.name)",
			},
		}, []string{"resource"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Resource map[string]interface{} `json:"resource"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ResourceToolset.DeleteResource(ctx, args.Resource)
		return handleToolResult(result, err)
	})
}
