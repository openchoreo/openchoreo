// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestToolDescriptions verifies that tool descriptions are meaningful and distinguishable
func TestToolDescriptions(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	toolsByName := make(map[string]*mcp.Tool)
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	// Test each tool's description using specs from allToolSpecs
	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			if !exists {
				t.Fatalf("Tool %q not found", spec.name)
			}

			desc := strings.ToLower(tool.Description)

			// Check minimum length
			if len(desc) < spec.descriptionMinLen {
				t.Errorf("Description too short: got %d chars, want at least %d", len(desc), spec.descriptionMinLen)
			}

			// Check for required keywords
			for _, word := range spec.descriptionKeywords {
				if !strings.Contains(desc, strings.ToLower(word)) {
					t.Errorf("Description missing required keyword %q: %s", word, tool.Description)
				}
			}
		})
	}

	// Ensure descriptions are unique across all tools
	descriptions := make(map[string]string)
	for _, tool := range toolsResult.Tools {
		if existingTool, exists := descriptions[tool.Description]; exists {
			t.Errorf("Duplicate description found: %q used by both %q and %q",
				tool.Description, tool.Name, existingTool)
		}
		descriptions[tool.Description] = tool.Name
	}
}

// TestToolSchemas verifies that tool input schemas have required properties defined
func TestToolSchemas(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	toolsByName := make(map[string]*mcp.Tool)
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	// Test each tool's schema using specs from allToolSpecs
	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			if !exists {
				t.Fatalf("Tool %q not found", spec.name)
			}

			if tool.InputSchema == nil {
				t.Fatal("InputSchema is nil")
			}

			// Convert InputSchema to map for inspection
			schemaMap, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("Expected InputSchema to be map[string]any, got %T", tool.InputSchema)
			}

			// Verify schema type is object
			schemaType, ok := schemaMap["type"].(string)
			if !ok || schemaType != "object" {
				t.Errorf("Expected schema type 'object', got %v", schemaMap["type"])
			}

			// Check required parameters
			if len(spec.requiredParams) > 0 {
				requiredInSchema := make(map[string]bool)
				if requiredList, ok := schemaMap["required"].([]interface{}); ok {
					for _, req := range requiredList {
						if reqStr, ok := req.(string); ok {
							requiredInSchema[reqStr] = true
						}
					}
				}

				for _, param := range spec.requiredParams {
					if !requiredInSchema[param] {
						t.Errorf("Required parameter %q not found in schema.required", param)
					}
				}
			}

			// Check that all parameters (required and optional) are in properties
			allParams := make([]string, len(spec.requiredParams))
			copy(allParams, spec.requiredParams)
			allParams = append(allParams, spec.optionalParams...)
			if len(allParams) > 0 {
				properties, ok := schemaMap["properties"].(map[string]any)
				if !ok {
					t.Fatal("Properties is not a map")
				}
				for _, param := range allParams {
					if _, exists := properties[param]; !exists {
						t.Errorf("Parameter %q not found in schema.properties", param)
					}
				}
			}
		})
	}
}
