// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Helper functions to create JSON Schema definitions
func stringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func defaultStringProperty() map[string]any {
	return map[string]any{
		"type": "string",
	}
}

func handleToolResult(result any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, result, nil
}

func arrayProperty(description, itemType string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type": itemType,
		},
	}
}

func intProperty(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}

// addPaginationProperties adds optional "limit" and "cursor" properties to a
// property map used for list tool input schemas.
func addPaginationProperties(properties map[string]any) map[string]any {
	properties["limit"] = intProperty(
		fmt.Sprintf("Maximum number of items to return per page (default %d)", DefaultPageSize))
	properties["cursor"] = stringProperty(
		"Opaque pagination cursor from a previous response's next_cursor field")
	return properties
}

func createSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
