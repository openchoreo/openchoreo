// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestToolParameterWiring verifies that parameters are correctly passed to handlers
func TestToolParameterWiring(t *testing.T) {
	clientSession, mockHandler := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	// Test each tool's parameter wiring using specs from allToolSpecs
	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			// Clear previous calls
			mockHandler.calls = make(map[string][]interface{})

			result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      spec.name,
				Arguments: spec.testArgs,
			})
			if err != nil {
				t.Fatalf("Failed to call tool: %v", err)
			}

			// Verify result is not empty
			if len(result.Content) == 0 {
				t.Fatal("Expected non-empty result content")
			}

			// Verify the correct handler method was called
			calls, ok := mockHandler.calls[spec.expectedMethod]
			if !ok {
				t.Fatalf("Expected method %q was not called. Available calls: %v",
					spec.expectedMethod, mockHandler.calls)
			}

			if len(calls) != 1 {
				t.Fatalf("Expected 1 call to %q, got %d", spec.expectedMethod, len(calls))
			}

			// Validate the call parameters using the spec's custom validator
			args := calls[0].([]interface{})
			spec.validateCall(t, args)
		})
	}
}

// TestToolResponseFormat verifies that tool responses are valid JSON
// This tests the response structure which is consistent across all tools
func TestToolResponseFormat(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	// Test with a single tool - response format is consistent across all tools
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_namespace",
		Arguments: map[string]any{"name": "test-org"},
	})
	if err != nil {
		t.Fatalf("Failed to call tool: %v", err)
	}

	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}

	// Get the text content
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("Expected TextContent")
	}

	// Verify the response is valid JSON
	var data interface{}
	if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
		t.Errorf("Response is not valid JSON: %v\nResponse: %s", err, textContent.Text)
	}
}

// TestToolErrorHandling verifies that the MCP SDK validates required parameters
// This tests that parameter validation happens before reaching handler code
func TestToolErrorHandling(t *testing.T) {
	clientSession, mockHandler := setupTestServer(t)
	defer clientSession.Close()

	ctx := context.Background()

	// Find a tool with required parameters from allToolSpecs
	var testSpec toolTestSpec
	for _, spec := range allToolSpecs {
		if len(spec.requiredParams) > 0 {
			testSpec = spec
			break
		}
	}

	if testSpec.name == "" {
		t.Fatal("No tool with required parameters found in allToolSpecs")
	}

	// Clear mock handler calls
	mockHandler.calls = make(map[string][]interface{})

	// Try calling the tool with missing required parameter
	_, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      testSpec.name,
		Arguments: map[string]any{}, // Empty arguments - missing required params
	})

	// We expect an error for missing required parameters
	if err == nil {
		t.Errorf("Expected error for tool %q with missing required parameters, got nil", testSpec.name)
	}

	// Verify the handler was NOT called (validation should fail before reaching handler)
	if len(mockHandler.calls) > 0 {
		t.Errorf("Handler should not be called when parameters are invalid, but got calls: %v", mockHandler.calls)
	}
}
