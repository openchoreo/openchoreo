// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t *Toolsets) RegisterListNamespaces(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_namespaces",
		Description: "List all namespaces. Namespaces are top-level containers for organizing " +
			"projects, components, and other resources.",
		InputSchema: createSchema(map[string]any{}, []string{}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		result, err := t.NamespaceToolset.ListNamespaces(ctx)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetNamespace(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_namespace",
		Description: "Get detailed information about a specific namespace.",
		InputSchema: createSchema(map[string]any{
			"name": stringProperty("The name of the namespace to retrieve"),
		}, []string{"name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.NamespaceToolset.GetNamespace(ctx, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListSecretReferences(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_secret_references",
		Description: "List all secret references for an namespace. Secret references are " +
			"credentials and sensitive configuration that can be used by components.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.NamespaceToolset.ListSecretReferences(ctx, args.NamespaceName)
		return handleToolResult(result, err)
	})
}
