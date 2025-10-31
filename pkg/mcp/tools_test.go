// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	testOrgName       = "my-org"
	testProjectName   = "my-project"
	testComponentName = "my-component"
	testEnvName       = "dev"
)

// MockCoreToolsetHandler implements CoreToolsetHandler for testing
type MockCoreToolsetHandler struct {
	// Track which methods were called and with what parameters
	calls map[string][]interface{}
}

func NewMockCoreToolsetHandler() *MockCoreToolsetHandler {
	return &MockCoreToolsetHandler{
		calls: make(map[string][]interface{}),
	}
}

func (m *MockCoreToolsetHandler) recordCall(method string, args ...interface{}) {
	m.calls[method] = append(m.calls[method], args)
}

func (m *MockCoreToolsetHandler) GetOrganization(ctx context.Context, name string) (string, error) {
	m.recordCall("GetOrganization", name)
	return `{"name":"test-org"}`, nil
}

func (m *MockCoreToolsetHandler) ListProjects(ctx context.Context, orgName string) (string, error) {
	m.recordCall("ListProjects", orgName)
	return `[{"name":"project1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetProject(ctx context.Context, orgName, projectName string) (string, error) {
	m.recordCall("GetProject", orgName, projectName)
	return `{"name":"project1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateProject(
	ctx context.Context, orgName string, req *models.CreateProjectRequest,
) (string, error) {
	m.recordCall("CreateProject", orgName, req)
	return `{"name":"new-project"}`, nil
}

func (m *MockCoreToolsetHandler) CreateComponent(
	ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest,
) (string, error) {
	m.recordCall("CreateComponent", orgName, projectName, req)
	return `{"name":"new-component"}`, nil
}

func (m *MockCoreToolsetHandler) ListComponents(ctx context.Context, orgName, projectName string) (string, error) {
	m.recordCall("ListComponents", orgName, projectName)
	return `[{"name":"component1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetComponent(
	ctx context.Context, orgName, projectName, componentName string, additionalResources []string,
) (string, error) {
	m.recordCall("GetComponent", orgName, projectName, componentName, additionalResources)
	return `{"name":"component1"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentBinding(
	ctx context.Context, orgName, projectName, componentName, environment string,
) (string, error) {
	m.recordCall("GetComponentBinding", orgName, projectName, componentName, environment)
	return `{"environment":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) UpdateComponentBinding(
	ctx context.Context, orgName, projectName, componentName, bindingName string,
	req *models.UpdateBindingRequest,
) (string, error) {
	m.recordCall("UpdateComponentBinding", orgName, projectName, componentName, bindingName, req)
	return `{"status":"updated"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentObserverURL(
	ctx context.Context, orgName, projectName, componentName, environmentName string,
) (string, error) {
	m.recordCall("GetComponentObserverURL", orgName, projectName, componentName, environmentName)
	return `{"url":"http://observer.example.com"}`, nil
}

func (m *MockCoreToolsetHandler) GetBuildObserverURL(
	ctx context.Context, orgName, projectName, componentName string,
) (string, error) {
	m.recordCall("GetBuildObserverURL", orgName, projectName, componentName)
	return `{"url":"http://build-observer.example.com"}`, nil
}

func (m *MockCoreToolsetHandler) GetComponentWorkloads(
	ctx context.Context, orgName, projectName, componentName string,
) (string, error) {
	m.recordCall("GetComponentWorkloads", orgName, projectName, componentName)
	return `[{"name":"workload1"}]`, nil
}

func (m *MockCoreToolsetHandler) ListEnvironments(ctx context.Context, orgName string) (string, error) {
	m.recordCall("ListEnvironments", orgName)
	return `[{"name":"dev"}]`, nil
}

func (m *MockCoreToolsetHandler) GetEnvironment(ctx context.Context, orgName, envName string) (string, error) {
	m.recordCall("GetEnvironment", orgName, envName)
	return `{"name":"dev"}`, nil
}

func (m *MockCoreToolsetHandler) CreateEnvironment(
	ctx context.Context, orgName string, req *models.CreateEnvironmentRequest,
) (string, error) {
	m.recordCall("CreateEnvironment", orgName, req)
	return `{"name":"new-env"}`, nil
}

func (m *MockCoreToolsetHandler) ListDataPlanes(ctx context.Context, orgName string) (string, error) {
	m.recordCall("ListDataPlanes", orgName)
	return `[{"name":"dp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetDataPlane(ctx context.Context, orgName, dpName string) (string, error) {
	m.recordCall("GetDataPlane", orgName, dpName)
	return `{"name":"dp1"}`, nil
}

func (m *MockCoreToolsetHandler) CreateDataPlane(
	ctx context.Context, orgName string, req *models.CreateDataPlaneRequest,
) (string, error) {
	m.recordCall("CreateDataPlane", orgName, req)
	return `{"name":"new-dp"}`, nil
}

func (m *MockCoreToolsetHandler) ListBuildTemplates(ctx context.Context, orgName string) (string, error) {
	m.recordCall("ListBuildTemplates", orgName)
	return `[{"name":"template1"}]`, nil
}

func (m *MockCoreToolsetHandler) TriggerBuild(
	ctx context.Context, orgName, projectName, componentName, commit string,
) (string, error) {
	m.recordCall("TriggerBuild", orgName, projectName, componentName, commit)
	return `{"buildId":"build-123"}`, nil
}

func (m *MockCoreToolsetHandler) ListBuilds(
	ctx context.Context, orgName, projectName, componentName string,
) (string, error) {
	m.recordCall("ListBuilds", orgName, projectName, componentName)
	return `[{"id":"build-123"}]`, nil
}

func (m *MockCoreToolsetHandler) ListBuildPlanes(ctx context.Context, orgName string) (string, error) {
	m.recordCall("ListBuildPlanes", orgName)
	return `[{"name":"bp1"}]`, nil
}

func (m *MockCoreToolsetHandler) GetProjectDeploymentPipeline(
	ctx context.Context, orgName, projectName string,
) (string, error) {
	m.recordCall("GetProjectDeploymentPipeline", orgName, projectName)
	return `{"stages":[]}`, nil
}

func setupTestServer(t *testing.T) (*mcp.ClientSession, *MockCoreToolsetHandler) {
	t.Helper()
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{
		OrganizationToolset:   mockHandler,
		ProjectToolset:        mockHandler,
		ComponentToolset:      mockHandler,
		BuildToolset:          mockHandler,
		DeploymentToolset:     mockHandler,
		InfrastructureToolset: mockHandler,
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
	toolset string // "organization", "project", "component", "build", "deployment", "infrastructure"

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

// allToolSpecs defines the complete test specification for all MCP tools
var allToolSpecs = []toolTestSpec{
	{
		name:                "get_organization",
		toolset:             "organization",
		descriptionKeywords: []string{"organization"},
		descriptionMinLen:   10,
		optionalParams:      []string{"name"},
		testArgs:            map[string]any{"name": "test-org"},
		expectedMethod:      "GetOrganization",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != "test-org" {
				t.Errorf("Expected org name 'test-org', got %v", args[0])
			}
		},
	},
	{
		name:                "list_projects",
		toolset:             "project",
		descriptionKeywords: []string{"list", "project"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name"},
		testArgs:            map[string]any{"org_name": testOrgName},
		expectedMethod:      "ListProjects",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
		},
	},
	{
		name:                "get_project",
		toolset:             "project",
		descriptionKeywords: []string{"project"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name"},
		testArgs: map[string]any{
			"org_name":     testOrgName,
			"project_name": testProjectName,
		},
		expectedMethod: "GetProject",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
			if args[1] != testProjectName {
				t.Errorf("Expected project name %q, got %v", testProjectName, args[1])
			}
		},
	},
	{
		name:                "create_project",
		toolset:             "project",
		descriptionKeywords: []string{"create", "project"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "name"},
		optionalParams:      []string{"description"},
		testArgs: map[string]any{
			"org_name":    testOrgName,
			"name":        "new-project",
			"description": "test project",
		},
		expectedMethod: "CreateProject",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
			// args[1] is *models.CreateProjectRequest
		},
	},
	{
		name:                "list_components",
		toolset:             "component",
		descriptionKeywords: []string{"list", "component"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name"},
		testArgs: map[string]any{
			"org_name":     testOrgName,
			"project_name": testProjectName,
		},
		expectedMethod: "ListComponents",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
			if args[1] != testProjectName {
				t.Errorf("Expected project name %q, got %v", testProjectName, args[1])
			}
		},
	},
	{
		name:                "get_component",
		toolset:             "component",
		descriptionKeywords: []string{"component"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name", "component_name"},
		optionalParams:      []string{"additional_resources"},
		testArgs: map[string]any{
			"org_name":             testOrgName,
			"project_name":         testProjectName,
			"component_name":       testComponentName,
			"additional_resources": []interface{}{"deployments", "services"},
		},
		expectedMethod: "GetComponent",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
			if args[1] != testProjectName {
				t.Errorf("Expected project name %q, got %v", testProjectName, args[1])
			}
			if args[2] != testComponentName {
				t.Errorf("Expected component name %q, got %v", testComponentName, args[2])
			}
			resources := args[3].([]string)
			expected := []string{"deployments", "services"}
			if diff := cmp.Diff(expected, resources); diff != "" {
				t.Errorf("additional_resources mismatch (-want +got):\n%s", diff)
			}
		},
	},
	{
		name:                "get_component_binding",
		toolset:             "component",
		descriptionKeywords: []string{"component", "binding"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name", "component_name", "environment"},
		testArgs: map[string]any{
			"org_name":       testOrgName,
			"project_name":   testProjectName,
			"component_name": testComponentName,
			"environment":    testEnvName,
		},
		expectedMethod: "GetComponentBinding",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != testProjectName ||
				args[2] != testComponentName || args[3] != testEnvName {
				t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
					testOrgName, testProjectName, testComponentName, testEnvName,
					args[0], args[1], args[2], args[3])
			}
		},
	},
	{
		name:                "get_component_observer_url",
		toolset:             "component",
		descriptionKeywords: []string{"observer", "component"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name", "component_name", "environment_name"},
		testArgs: map[string]any{
			"org_name":         testOrgName,
			"project_name":     testProjectName,
			"component_name":   testComponentName,
			"environment_name": testEnvName,
		},
		expectedMethod: "GetComponentObserverURL",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != testProjectName ||
				args[2] != testComponentName || args[3] != testEnvName {
				t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
					testOrgName, testProjectName, testComponentName, testEnvName,
					args[0], args[1], args[2], args[3])
			}
		},
	},
	{
		name:                "get_build_observer_url",
		toolset:             "build",
		descriptionKeywords: []string{"observer", "build"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name", "component_name"},
		testArgs: map[string]any{
			"org_name":       testOrgName,
			"project_name":   testProjectName,
			"component_name": testComponentName,
		},
		expectedMethod: "GetBuildObserverURL",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
				t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
					testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
			}
		},
	},
	{
		name:                "get_component_workloads",
		toolset:             "component",
		descriptionKeywords: []string{"workload", "component"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name", "component_name"},
		testArgs: map[string]any{
			"org_name":       testOrgName,
			"project_name":   testProjectName,
			"component_name": testComponentName,
		},
		expectedMethod: "GetComponentWorkloads",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
				t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
					testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
			}
		},
	},
	{
		name:                "list_environments",
		toolset:             "infrastructure",
		descriptionKeywords: []string{"list", "environment"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name"},
		testArgs: map[string]any{
			"org_name": testOrgName,
		},
		expectedMethod: "ListEnvironments",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
		},
	},
	{
		name:                "get_environment",
		toolset:             "infrastructure",
		descriptionKeywords: []string{"environment"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "env_name"},
		testArgs: map[string]any{
			"org_name": testOrgName,
			"env_name": testEnvName,
		},
		expectedMethod: "GetEnvironment",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != testEnvName {
				t.Errorf("Expected (%s, %s), got (%v, %v)", testOrgName, testEnvName, args[0], args[1])
			}
		},
	},
	{
		name:                "list_dataplanes",
		toolset:             "infrastructure",
		descriptionKeywords: []string{"list", "data", "plane"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name"},
		testArgs: map[string]any{
			"org_name": testOrgName,
		},
		expectedMethod: "ListDataPlanes",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
		},
	},
	{
		name:                "get_dataplane",
		toolset:             "infrastructure",
		descriptionKeywords: []string{"data", "plane"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "dp_name"},
		testArgs: map[string]any{
			"org_name": testOrgName,
			"dp_name":  "dp1",
		},
		expectedMethod: "GetDataPlane",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != "dp1" {
				t.Errorf("Expected (%s, dp1), got (%v, %v)", testOrgName, args[0], args[1])
			}
		},
	},
	{
		name:                "list_build_templates",
		toolset:             "build",
		descriptionKeywords: []string{"list", "build", "template"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name"},
		testArgs: map[string]any{
			"org_name": testOrgName,
		},
		expectedMethod: "ListBuildTemplates",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
		},
	},
	{
		name:                "trigger_build",
		toolset:             "build",
		descriptionKeywords: []string{"trigger", "build"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name", "component_name", "commit"},
		testArgs: map[string]any{
			"org_name":       testOrgName,
			"project_name":   testProjectName,
			"component_name": testComponentName,
			"commit":         "abc123",
		},
		expectedMethod: "TriggerBuild",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != testProjectName ||
				args[2] != testComponentName || args[3] != "abc123" {
				t.Errorf("Expected (%s, %s, %s, abc123), got (%v, %v, %v, %v)",
					testOrgName, testProjectName, testComponentName,
					args[0], args[1], args[2], args[3])
			}
		},
	},
	{
		name:                "list_builds",
		toolset:             "build",
		descriptionKeywords: []string{"list", "build"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name", "component_name"},
		testArgs: map[string]any{
			"org_name":       testOrgName,
			"project_name":   testProjectName,
			"component_name": testComponentName,
		},
		expectedMethod: "ListBuilds",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
				t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
					testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
			}
		},
	},
	{
		name:                "list_buildplanes",
		toolset:             "build",
		descriptionKeywords: []string{"list", "build", "plane"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name"},
		testArgs: map[string]any{
			"org_name": testOrgName,
		},
		expectedMethod: "ListBuildPlanes",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName {
				t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
			}
		},
	},
	{
		name:                "get_deployment_pipeline",
		toolset:             "deployment",
		descriptionKeywords: []string{"deployment", "pipeline"},
		descriptionMinLen:   10,
		requiredParams:      []string{"org_name", "project_name"},
		testArgs: map[string]any{
			"org_name":     testOrgName,
			"project_name": testProjectName,
		},
		expectedMethod: "GetProjectDeploymentPipeline",
		validateCall: func(t *testing.T, args []interface{}) {
			if args[0] != testOrgName || args[1] != testProjectName {
				t.Errorf("Expected (%s, %s), got (%v, %v)", testOrgName, testProjectName, args[0], args[1])
			}
		},
	},
}

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
		Name:      "get_organization",
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

// TestPartialToolsetRegistration verifies that only the tools from registered toolsets are available
func TestPartialToolsetRegistration(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()

	// Define which toolsets to register
	registeredToolsets := map[string]bool{
		"organization": true,
		"project":      true,
	}

	// Register only a subset of toolsets
	toolsets := &Toolsets{
		OrganizationToolset: mockHandler,
		ProjectToolset:      mockHandler,
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
		Arguments: map[string]any{"org_name": testOrgName},
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
		Arguments: map[string]any{"org_name": testOrgName, "project_name": testProjectName},
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
