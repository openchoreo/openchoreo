// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func (t *Toolsets) RegisterListNamespaces(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_namespaces"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewNamespace}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all namespaces. Namespaces are top-level containers for organizing " +
			"projects, components, and other resources. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), []string{}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.NamespaceToolset.ListNamespaces(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateNamespace(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_namespace"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateNamespace}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
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
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}

		createReq := &gen.CreateNamespaceJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
		}
		result, err := t.NamespaceToolset.CreateNamespace(ctx, createReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListSecretReferences(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_secret_references"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewSecretReference}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all secret references for an namespace. Secret references are " +
			"credentials and sensitive configuration that can be used by components. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.NamespaceToolset.ListSecretReferences(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}
