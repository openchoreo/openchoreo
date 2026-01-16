// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
