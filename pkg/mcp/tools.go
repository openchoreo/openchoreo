// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ToolsetType represents a type of toolset that can be enabled
type ToolsetType string

const (
	ToolsetOrganization   ToolsetType = "organization"
	ToolsetProject        ToolsetType = "project"
	ToolsetComponent      ToolsetType = "component"
	ToolsetBuild          ToolsetType = "build"
	ToolsetDeployment     ToolsetType = "deployment"
	ToolsetInfrastructure ToolsetType = "infrastructure"
)

type Toolsets struct {
	OrganizationToolset   OrganizationToolsetHandler
	ProjectToolset        ProjectToolsetHandler
	ComponentToolset      ComponentToolsetHandler
	BuildToolset          BuildToolsetHandler
	DeploymentToolset     DeploymentToolsetHandler
	InfrastructureToolset InfrastructureToolsetHandler
}

// OrganizationToolsetHandler handles organization operations
type OrganizationToolsetHandler interface {
	GetOrganization(ctx context.Context, name string) (string, error)
}

// ProjectToolsetHandler handles organization and project operations
type ProjectToolsetHandler interface {
	// Project operations
	ListProjects(ctx context.Context, orgName string) (string, error)
	GetProject(ctx context.Context, orgName, projectName string) (string, error)
	CreateProject(ctx context.Context, orgName string, req *models.CreateProjectRequest) (string, error)
}

// ComponentToolsetHandler handles component operations
type ComponentToolsetHandler interface {
	CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (string, error)
	ListComponents(ctx context.Context, orgName, projectName string) (string, error)
	GetComponent(
		ctx context.Context, orgName, projectName, componentName string, additionalResources []string,
	) (string, error)
	GetComponentBinding(ctx context.Context, orgName, projectName, componentName, environment string) (string, error)
	UpdateComponentBinding(
		ctx context.Context, orgName, projectName, componentName, bindingName string,
		req *models.UpdateBindingRequest,
	) (string, error)
	GetComponentWorkloads(ctx context.Context, orgName, projectName, componentName string) (string, error)
}

// BuildToolsetHandler handles build operations
type BuildToolsetHandler interface {
	ListBuildTemplates(ctx context.Context, orgName string) (string, error)
	TriggerBuild(ctx context.Context, orgName, projectName, componentName, commit string) (string, error)
	ListBuilds(ctx context.Context, orgName, projectName, componentName string) (string, error)
	GetBuildObserverURL(ctx context.Context, orgName, projectName, componentName string) (string, error)
	ListBuildPlanes(ctx context.Context, orgName string) (string, error)
}

// DeploymentToolsetHandler handles deployment operations
type DeploymentToolsetHandler interface {
	GetProjectDeploymentPipeline(ctx context.Context, orgName, projectName string) (string, error)
	GetComponentObserverURL(
		ctx context.Context, orgName, projectName, componentName, environmentName string,
	) (string, error)
}

// InfrastructureToolsetHandler handles infrastructure operations
type InfrastructureToolsetHandler interface {
	// Environment operations
	ListEnvironments(ctx context.Context, orgName string) (string, error)
	GetEnvironment(ctx context.Context, orgName, envName string) (string, error)
	CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (string, error)

	// DataPlane operations
	ListDataPlanes(ctx context.Context, orgName string) (string, error)
	GetDataPlane(ctx context.Context, orgName, dpName string) (string, error)
	CreateDataPlane(ctx context.Context, orgName string, req *models.CreateDataPlaneRequest) (string, error)
}

// RegisterFunc is a function type for registering MCP tools
type RegisterFunc func(s *mcp.Server)

// Helper functions to create JSON Schema definitions
func stringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func handleToolResult(result string, err error) (*mcp.CallToolResult, map[string]string, error) {
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, map[string]string{"message": result}, nil
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

func (t *Toolsets) RegisterGetOrganization(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_organization",
		Description: "Get information about organizations. If no name is provided, lists all organizations.",
		InputSchema: createSchema(map[string]any{
			"name": stringProperty("Optional: specific organization name to retrieve"),
		}, []string{}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name string `json:"name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.OrganizationToolset.GetOrganization(ctx, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListProjects(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all projects in an organization",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Organization name"),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.ProjectToolset.ListProjects(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetProject(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_project",
		Description: "Get details of a specific project",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Organization name"),
			"project_name": stringProperty("Project name"),
		}, []string{"org_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		ProjectName string `json:"project_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.ProjectToolset.GetProject(ctx, args.OrgName, args.ProjectName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateProject(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_project",
		Description: "Create a new project in an organization",
		InputSchema: createSchema(map[string]any{
			"org_name":    stringProperty("Organization name"),
			"name":        stringProperty("Project name"),
			"description": stringProperty("Project description"),
		}, []string{"org_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		projectReq := &models.CreateProjectRequest{
			Name:        args.Name,
			Description: args.Description,
		}
		result, err := t.ProjectToolset.CreateProject(ctx, args.OrgName, projectReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponents(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_components",
		Description: "List all components in a project",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Organization name"),
			"project_name": stringProperty("Project name"),
		}, []string{"org_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		ProjectName string `json:"project_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.ComponentToolset.ListComponents(ctx, args.OrgName, args.ProjectName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponent(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_component",
		Description: "Get details of a specific component",
		InputSchema: createSchema(map[string]any{
			"org_name":             stringProperty("Organization name"),
			"project_name":         stringProperty("Project name"),
			"component_name":       stringProperty("Component name"),
			"additional_resources": arrayProperty("Optional: additional resources to include", "string"),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName             string   `json:"org_name"`
		ProjectName         string   `json:"project_name"`
		ComponentName       string   `json:"component_name"`
		AdditionalResources []string `json:"additional_resources"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.ComponentToolset.GetComponent(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.AdditionalResources,
		)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterComponentBinding(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_component_binding",
		Description: "Get component binding for a specific environment",
		InputSchema: createSchema(map[string]any{
			"org_name":       stringProperty("Organization name"),
			"project_name":   stringProperty("Project name"),
			"component_name": stringProperty("Component name"),
			"environment":    stringProperty("Environment name"),
		}, []string{"org_name", "project_name", "component_name", "environment"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		Environment   string `json:"environment"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.ComponentToolset.GetComponentBinding(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.Environment,
		)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentObserverURL(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_component_observer_url",
		Description: "Get observer URL for a component in a specific environment",
		InputSchema: createSchema(map[string]any{
			"org_name":         stringProperty("Organization name"),
			"project_name":     stringProperty("Project name"),
			"component_name":   stringProperty("Component name"),
			"environment_name": stringProperty("Environment name"),
		}, []string{"org_name", "project_name", "component_name", "environment_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName         string `json:"org_name"`
		ProjectName     string `json:"project_name"`
		ComponentName   string `json:"component_name"`
		EnvironmentName string `json:"environment_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.DeploymentToolset.GetComponentObserverURL(
			ctx, args.OrgName, args.ProjectName, args.ComponentName, args.EnvironmentName,
		)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetBuildObserverURL(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_build_observer_url",
		Description: "Get observer URL for component builds",
		InputSchema: createSchema(map[string]any{
			"org_name":       stringProperty("Organization name"),
			"project_name":   stringProperty("Project name"),
			"component_name": stringProperty("Component name"),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.BuildToolset.GetBuildObserverURL(ctx, args.OrgName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentWorkloads(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_component_workloads",
		Description: "Get workloads for a component",
		InputSchema: createSchema(map[string]any{
			"org_name":       stringProperty("Organization name"),
			"project_name":   stringProperty("Project name"),
			"component_name": stringProperty("Component name"),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.ComponentToolset.GetComponentWorkloads(ctx, args.OrgName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListEnvironments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_environments",
		Description: "List all environments in an organization",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Organization name"),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.InfrastructureToolset.ListEnvironments(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetEnvironments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_environment",
		Description: "Get details of a specific environment",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Organization name"),
			"env_name": stringProperty("Environment name"),
		}, []string{"org_name", "env_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
		EnvName string `json:"env_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.InfrastructureToolset.GetEnvironment(ctx, args.OrgName, args.EnvName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_dataplanes",
		Description: "List all data planes in an organization",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Organization name"),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.InfrastructureToolset.ListDataPlanes(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetDataPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_dataplane",
		Description: "Get details of a specific data plane",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Organization name"),
			"dp_name":  stringProperty("Data plane name"),
		}, []string{"org_name", "dp_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
		DpName  string `json:"dp_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.InfrastructureToolset.GetDataPlane(ctx, args.OrgName, args.DpName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListBuildTemplates(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_build_templates",
		Description: "List all build templates in an organization",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Organization name"),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.BuildToolset.ListBuildTemplates(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterTriggerBuild(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "trigger_build",
		Description: "Trigger a new build for a component",
		InputSchema: createSchema(map[string]any{
			"org_name":       stringProperty("Organization name"),
			"project_name":   stringProperty("Project name"),
			"component_name": stringProperty("Component name"),
			"commit":         stringProperty("Git commit hash"),
		}, []string{"org_name", "project_name", "component_name", "commit"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		Commit        string `json:"commit"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.BuildToolset.TriggerBuild(ctx, args.OrgName, args.ProjectName, args.ComponentName, args.Commit)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListBuilds(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_builds",
		Description: "List all builds for a component",
		InputSchema: createSchema(map[string]any{
			"org_name":       stringProperty("Organization name"),
			"project_name":   stringProperty("Project name"),
			"component_name": stringProperty("Component name"),
		}, []string{"org_name", "project_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName       string `json:"org_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.BuildToolset.ListBuilds(ctx, args.OrgName, args.ProjectName, args.ComponentName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListBuildPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_buildplanes",
		Description: "List all build planes in an organization",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Organization name"),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.BuildToolset.ListBuildPlanes(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetDeploymentPipeline(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_deployment_pipeline",
		Description: "Get deployment pipeline for a project",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Organization name"),
			"project_name": stringProperty("Project name"),
		}, []string{"org_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName     string `json:"org_name"`
		ProjectName string `json:"project_name"`
	}) (*mcp.CallToolResult, map[string]string, error) {
		result, err := t.DeploymentToolset.GetProjectDeploymentPipeline(ctx, args.OrgName, args.ProjectName)
		return handleToolResult(result, err)
	})
}

// organizationToolRegistrations returns the list of organization toolset registration functions
func (t *Toolsets) organizationToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterGetOrganization,
	}
}

// projectToolRegistrations returns the list of org-project toolset registration functions
func (t *Toolsets) projectToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterGetOrganization,
		t.RegisterListProjects,
		t.RegisterGetProject,
		t.RegisterCreateProject,
	}
}

// componentToolRegistrations returns the list of component toolset registration functions
func (t *Toolsets) componentToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListComponents,
		t.RegisterGetComponent,
		t.RegisterComponentBinding,
		t.RegisterGetComponentWorkloads,
	}
}

// buildToolRegistrations returns the list of build toolset registration functions
func (t *Toolsets) buildToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListBuildTemplates,
		t.RegisterTriggerBuild,
		t.RegisterListBuilds,
		t.RegisterGetBuildObserverURL,
		t.RegisterListBuildPlanes,
	}
}

// deploymentToolRegistrations returns the list of deployment toolset registration functions
func (t *Toolsets) deploymentToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterGetDeploymentPipeline,
		t.RegisterGetComponentObserverURL,
	}
}

// infrastructureToolRegistrations returns the list of infrastructure toolset registration functions
func (t *Toolsets) infrastructureToolRegistrations() []RegisterFunc {
	return []RegisterFunc{
		t.RegisterListEnvironments,
		t.RegisterGetEnvironments,
		t.RegisterListDataPlanes,
		t.RegisterGetDataPlane,
	}
}

func (t *Toolsets) Register(s *mcp.Server) {
	// Register organization tools if OrganizationToolset is enabled
	if t.OrganizationToolset != nil {
		for _, registerFunc := range t.organizationToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register project tools if ProjectToolset is enabled
	if t.ProjectToolset != nil {
		for _, registerFunc := range t.projectToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register component tools if ComponentToolset is enabled
	if t.ComponentToolset != nil {
		for _, registerFunc := range t.componentToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register build tools if BuildToolset is enabled
	if t.BuildToolset != nil {
		for _, registerFunc := range t.buildToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register deployment tools if DeploymentToolset is enabled
	if t.DeploymentToolset != nil {
		for _, registerFunc := range t.deploymentToolRegistrations() {
			registerFunc(s)
		}
	}

	// Register infrastructure tools if InfrastructureToolset is enabled
	if t.InfrastructureToolset != nil {
		for _, registerFunc := range t.infrastructureToolRegistrations() {
			registerFunc(s)
		}
	}
}
