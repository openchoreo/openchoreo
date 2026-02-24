// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacytools

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

// RegisterGetResource registers the get_resource MCP tool.
// TODO: cleanup when the upstream endpoint is deprecated.
func (t *Toolsets) RegisterGetResource(s *mcp.Server) {
	kindDesc := "The kind of the resource " +
		"(e.g. Component, Project, Environment, DataPlane, ComponentType, Trait, Workflow)."
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_resource",
		Description: "Get a Kubernetes resource by kind and name from the cluster. " +
			"Only supports resources with 'openchoreo.dev' API group. " +
			"Use explain_schema to discover valid kinds.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": stringProperty(
				"The namespace of the resource. Required for namespaced resources, " +
					"omit for cluster-scoped resources (e.g. ClusterDataPlane)."),
			"kind":          stringProperty(kindDesc),
			"resource_name": stringProperty("The name of the resource to retrieve."),
		}, []string{"kind", "resource_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Kind          string `json:"kind"`
		ResourceName  string `json:"resource_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ResourceToolset.GetResource(ctx, args.NamespaceName, args.Kind, args.ResourceName)
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
