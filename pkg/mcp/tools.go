// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ToolsetType represents a type of toolset that can be enabled
type ToolsetType string

const (
	// ToolsetCore represents the core toolset with basic operations
	ToolsetCore ToolsetType = "core"
)

type Toolsets struct {
	CoreToolset CoreToolsetHandler
}

type CoreToolsetHandler interface {
	// Organization operations
	GetOrganization(ctx context.Context, name string) (string, error)

	// Project operations
	ListProjects(ctx context.Context, orgName string) (string, error)
	GetProject(ctx context.Context, orgName, projectName string) (string, error)
	CreateProject(ctx context.Context, orgName string, req *models.CreateProjectRequest) (string, error)

	// Component operations
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
	GetComponentObserverURL(
		ctx context.Context, orgName, projectName, componentName, environmentName string,
	) (string, error)
	GetBuildObserverURL(ctx context.Context, orgName, projectName, componentName string) (string, error)
	GetComponentWorkloads(ctx context.Context, orgName, projectName, componentName string) (string, error)

	// Environment operations
	ListEnvironments(ctx context.Context, orgName string) (string, error)
	GetEnvironment(ctx context.Context, orgName, envName string) (string, error)
	CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (string, error)

	// DataPlane operations
	ListDataPlanes(ctx context.Context, orgName string) (string, error)
	GetDataPlane(ctx context.Context, orgName, dpName string) (string, error)
	CreateDataPlane(ctx context.Context, orgName string, req *models.CreateDataPlaneRequest) (string, error)

	// Build operations
	ListBuildTemplates(ctx context.Context, orgName string) (string, error)
	TriggerBuild(ctx context.Context, orgName, projectName, componentName, commit string) (string, error)
	ListBuilds(ctx context.Context, orgName, projectName, componentName string) (string, error)

	// BuildPlane operations
	ListBuildPlanes(ctx context.Context, orgName string) (string, error)

	// Deployment Pipeline operations
	GetProjectDeploymentPipeline(ctx context.Context, orgName, projectName string) (string, error)
}

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
	contentBytes, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	stringContent := string(contentBytes)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: stringContent},
		},
	}, map[string]string{"message": stringContent}, nil
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
	if t.CoreToolset != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "get_organization",
			Description: "Get information about organizations. If no name is provided, lists all organizations.",
			InputSchema: createSchema(map[string]any{
				"name": stringProperty("Optional: specific organization name to retrieve"),
			}, []string{}),
		}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			Name string `json:"name"`
		}) (*mcp.CallToolResult, map[string]string, error) {
			result, err := t.CoreToolset.GetOrganization(ctx, args.Name)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterListProjects(s *mcp.Server) {
	if t.CoreToolset != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "list_projects",
			Description: "List all projects in an organization",
			InputSchema: createSchema(map[string]any{
				"org_name": stringProperty("Organization name"),
			}, []string{"org_name"}),
		}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			OrgName string `json:"org_name"`
		}) (*mcp.CallToolResult, map[string]string, error) {
			result, err := t.CoreToolset.ListProjects(ctx, args.OrgName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterGetProject(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetProject(ctx, args.OrgName, args.ProjectName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterCreateProject(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.CreateProject(ctx, args.OrgName, projectReq)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterListComponents(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.ListComponents(ctx, args.OrgName, args.ProjectName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterGetComponent(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetComponent(
				ctx, args.OrgName, args.ProjectName, args.ComponentName, args.AdditionalResources,
			)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterComponentBinding(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetComponentBinding(
				ctx, args.OrgName, args.ProjectName, args.ComponentName, args.Environment,
			)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterGetComponentObserverURL(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetComponentObserverURL(
				ctx, args.OrgName, args.ProjectName, args.ComponentName, args.EnvironmentName,
			)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterGetBuildObserverURL(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetBuildObserverURL(ctx, args.OrgName, args.ProjectName, args.ComponentName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterGetComponentWorkloads(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetComponentWorkloads(ctx, args.OrgName, args.ProjectName, args.ComponentName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterListEnvironments(s *mcp.Server) {
	if t.CoreToolset != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "list_environments",
			Description: "List all environments in an organization",
			InputSchema: createSchema(map[string]any{
				"org_name": stringProperty("Organization name"),
			}, []string{"org_name"}),
		}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			OrgName string `json:"org_name"`
		}) (*mcp.CallToolResult, map[string]string, error) {
			result, err := t.CoreToolset.ListEnvironments(ctx, args.OrgName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterGetEnvironments(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetEnvironment(ctx, args.OrgName, args.EnvName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server) {
	if t.CoreToolset != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "list_dataplanes",
			Description: "List all data planes in an organization",
			InputSchema: createSchema(map[string]any{
				"org_name": stringProperty("Organization name"),
			}, []string{"org_name"}),
		}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			OrgName string `json:"org_name"`
		}) (*mcp.CallToolResult, map[string]string, error) {
			result, err := t.CoreToolset.ListDataPlanes(ctx, args.OrgName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterGetDataPlane(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetDataPlane(ctx, args.OrgName, args.DpName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterListBuildTemplates(s *mcp.Server) {
	if t.CoreToolset != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "list_build_templates",
			Description: "List all build templates in an organization",
			InputSchema: createSchema(map[string]any{
				"org_name": stringProperty("Organization name"),
			}, []string{"org_name"}),
		}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			OrgName string `json:"org_name"`
		}) (*mcp.CallToolResult, map[string]string, error) {
			result, err := t.CoreToolset.ListBuildTemplates(ctx, args.OrgName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterTriggerBuild(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.TriggerBuild(ctx, args.OrgName, args.ProjectName, args.ComponentName, args.Commit)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterListBuilds(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.ListBuilds(ctx, args.OrgName, args.ProjectName, args.ComponentName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterListBuildPlanes(s *mcp.Server) {
	if t.CoreToolset != nil {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "list_buildplanes",
			Description: "List all build planes in an organization",
			InputSchema: createSchema(map[string]any{
				"org_name": stringProperty("Organization name"),
			}, []string{"org_name"}),
		}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
			OrgName string `json:"org_name"`
		}) (*mcp.CallToolResult, map[string]string, error) {
			result, err := t.CoreToolset.ListBuildPlanes(ctx, args.OrgName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) RegisterGetDeploymentPipeline(s *mcp.Server) {
	if t.CoreToolset != nil {
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
			result, err := t.CoreToolset.GetProjectDeploymentPipeline(ctx, args.OrgName, args.ProjectName)
			return handleToolResult(result, err)
		})
	}
}

func (t *Toolsets) Register(s *mcp.Server) {
	t.RegisterGetOrganization(s)
	t.RegisterListProjects(s)
	t.RegisterGetProject(s)
	t.RegisterCreateProject(s)
	t.RegisterListComponents(s)
	t.RegisterGetComponent(s)
	t.RegisterComponentBinding(s)
	t.RegisterGetComponentObserverURL(s)
	t.RegisterGetBuildObserverURL(s)
	t.RegisterGetComponentWorkloads(s)
	t.RegisterListEnvironments(s)
	t.RegisterGetEnvironments(s)
	t.RegisterListDataPlanes(s)
	t.RegisterGetDataPlane(s)
	t.RegisterListBuildTemplates(s)
	t.RegisterTriggerBuild(s)
	t.RegisterListBuilds(s)
	t.RegisterListBuildPlanes(s)
	t.RegisterGetDeploymentPipeline(s)
}
