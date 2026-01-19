// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestToolRegistration verifies that all expected tools are registered
func TestToolRegistration(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	// Build expected tool names from allToolSpecs
	expectedTools := make(map[string]bool)
	for _, spec := range allToolSpecs {
		expectedTools[spec.name] = true
	}

	// Check all expected tools are present
	registeredTools := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		registeredTools[tool.Name] = true
		if !expectedTools[tool.Name] {
			t.Errorf("Unexpected tool %q found in registered tools", tool.Name)
		}
	}

	// Check no tools are missing
	for expected := range expectedTools {
		if !registeredTools[expected] {
			t.Errorf("Expected tool %q not found in registered tools", expected)
		}
	}

	if len(toolsResult.Tools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(toolsResult.Tools))
	}
}

// TestPartialToolsetRegistration verifies that only the tools from registered toolsets are available
func TestPartialToolsetRegistration(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()

	// Define which toolsets to register
	registeredToolsets := map[string]bool{
		"namespace": true,
		"project":   true,
	}

	// Register only a subset of toolsets
	toolsets := &Toolsets{
		NamespaceToolset: mockHandler,
		ProjectToolset:   mockHandler,
		// Intentionally omitting ComponentToolset, BuildToolset, DeploymentToolset, InfrastructureToolset
	}

	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list tools: %v", err)
	}

	// Build expected and unexpected tools from allToolSpecs based on registered toolsets
	expectedTools := make(map[string]bool)
	unexpectedTools := make(map[string]bool)
	for _, spec := range allToolSpecs {
		if registeredToolsets[spec.toolset] {
			expectedTools[spec.name] = true
		} else {
			unexpectedTools[spec.name] = true
		}
	}

	// Verify only expected tools are registered
	registeredTools := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		registeredTools[tool.Name] = true

		if unexpectedTools[tool.Name] {
			t.Errorf("Tool %q should not be registered (its toolset %q was not included)",
				tool.Name, getToolsetForTool(tool.Name))
		}
	}

	// Verify all expected tools are present
	for expected := range expectedTools {
		if !registeredTools[expected] {
			t.Errorf("Expected tool %q not found in registered tools", expected)
		}
	}

	if len(registeredTools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(registeredTools))
	}

	// Test that registered tools work correctly
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_projects",
		Arguments: map[string]any{"namespace_name": testNamespaceName},
	})
	if err != nil {
		t.Fatalf("Failed to call registered tool: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}

	// Test that unregistered tools are not callable
	_, err = clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_components",
		Arguments: map[string]any{"namespace_name": testNamespaceName, "project_name": testProjectName},
	})
	if err == nil {
		t.Error("Expected error when calling unregistered tool 'list_components', got nil")
	}
}

// getToolsetForTool returns the toolset name for a given tool name
func getToolsetForTool(toolName string) string {
	for _, spec := range allToolSpecs {
		if spec.name == toolName {
			return spec.toolset
		}
	}
	return "unknown"
}
