// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testNamespaceName   = "my-namespace"
	testProjectName     = "my-project"
	testComponentName   = "my-component"
	testEnvName         = "dev"
	testKindProject     = "Project"
	testWorkflowRunName = "workflow-run-1"
)

func setupTestServer(t *testing.T) (*mcp.ClientSession, *MockCoreToolsetHandler) {
	t.Helper()
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{
		NamespaceToolset:      mockHandler,
		ProjectToolset:        mockHandler,
		ComponentToolset:      mockHandler,
		BuildToolset:          mockHandler,
		DeploymentToolset:     mockHandler,
		InfrastructureToolset: mockHandler,
		SchemaToolset:         mockHandler,
		ResourceToolset:       mockHandler,
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
	toolset string // "namespace", "project", "component", "build", "deployment", "infrastructure", "schema", "resource"

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

// allToolSpecs aggregates all tool specs from all toolsets
var allToolSpecs = func() []toolTestSpec {
	specs := []toolTestSpec{}
	specs = append(specs, namespaceToolSpecs()...)
	specs = append(specs, projectToolSpecs()...)
	specs = append(specs, componentToolSpecs()...)
	specs = append(specs, buildToolSpecs()...)
	specs = append(specs, deploymentToolSpecs()...)
	specs = append(specs, infrastructureToolSpecs()...)
	specs = append(specs, schemaToolSpecs()...)
	specs = append(specs, resourceToolSpecs()...)
	return specs
}()
