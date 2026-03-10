// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testNamespaceName  = "my-namespace"
	testProjectName    = "my-project"
	testComponentName  = "my-component"
	testEnvName        = "dev"
	testKindProject    = "Project"
	testGoServiceName  = "go-service"
	testAutoscalerName = "autoscaler"
	testBuildWorkflow  = "build-workflow"
)

func setupTestServer(t *testing.T) (*mcp.ClientSession, *MockCoreToolsetHandler) {
	t.Helper()
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{
		NamespaceToolset:  mockHandler,
		ProjectToolset:    mockHandler,
		ComponentToolset:  mockHandler,
		DeploymentToolset: mockHandler,
		BuildToolset:      mockHandler,
		PEToolset:         mockHandler,
	}
	clientSession := setupTestServerWithToolset(t, toolsets)
	return clientSession, mockHandler
}

// setupTestServerWithToolset creates a test MCP server with the provided toolsets
func setupTestServerWithToolset(t *testing.T, toolsets *Toolsets) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-openchoreo-api",
		Version: "1.0.0",
	}, nil)

	toolsets.Register(server)

	// Create client connection
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect server: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}

	return clientSession
}

// toolTestSpec defines the complete test specification for a single MCP tool
type toolTestSpec struct {
	name string

	// Toolset association
	toolset string // "namespace", "project", "component", "infrastructure"

	// Description validation
	descriptionKeywords []string
	descriptionMinLen   int

	// Schema validation
	requiredParams []string
	optionalParams []string

	// Parameter wiring test
	testArgs       map[string]any
	expectedMethod string
	validateCall   func(t *testing.T, args []interface{})
}

// makeNamespacedListGetSpecs creates a list+get pair of tool test specs for namespace-scoped resources.
func makeNamespacedListGetSpecs(
	toolset, listToolName, getToolName string,
	listKeywords, getKeywords []string,
	getParamName, getParamValue, listMethod, getMethod string,
) []toolTestSpec {
	getVal := getParamValue
	return []toolTestSpec{
		{
			name:                listToolName,
			toolset:             toolset,
			descriptionKeywords: listKeywords,
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName},
			expectedMethod:      listMethod,
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                getToolName,
			toolset:             toolset,
			descriptionKeywords: getKeywords,
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", getParamName},
			testArgs:            map[string]any{"namespace_name": testNamespaceName, getParamName: getVal},
			expectedMethod:      getMethod,
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != getVal {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, getVal, args[0], args[1])
				}
			},
		},
	}
}

// allToolSpecs aggregates all tool specs from all toolsets
var allToolSpecs = func() []toolTestSpec {
	specs := make([]toolTestSpec, 0, 8)
	specs = append(specs, namespaceToolSpecs()...)
	specs = append(specs, projectToolSpecs()...)
	specs = append(specs, componentToolSpecs()...)
	specs = append(specs, deploymentToolSpecs()...)
	specs = append(specs, buildToolSpecs()...)
	specs = append(specs, peToolSpecs()...)
	return specs
}()
