// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
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

func (t *Toolsets) RegisterCreateNamespace(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_namespace",
		Description: "Create a new namespace. Namespaces are top-level containers for organizing " +
			"projects, components, and other resources.",
		InputSchema: createSchema(map[string]any{
			"name":         stringProperty("The name of the namespace to create"),
			"display_name": stringProperty("Optional display name for the namespace"),
			"description":  stringProperty("Optional description of the namespace"),
		}, []string{"name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name,omitempty"`
		Description string `json:"description,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		createReq := &models.CreateNamespaceRequest{
			Name:        args.Name,
			DisplayName: args.DisplayName,
			Description: args.Description,
		}
		result, err := t.NamespaceToolset.CreateNamespace(ctx, createReq)
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
