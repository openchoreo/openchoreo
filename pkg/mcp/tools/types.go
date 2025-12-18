// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

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
	ToolsetSchema         ToolsetType = "schema"
	ToolsetResource       ToolsetType = "resource"
)

type Toolsets struct {
	OrganizationToolset   OrganizationToolsetHandler
	ProjectToolset        ProjectToolsetHandler
	ComponentToolset      ComponentToolsetHandler
	BuildToolset          BuildToolsetHandler
	DeploymentToolset     DeploymentToolsetHandler
	InfrastructureToolset InfrastructureToolsetHandler
	SchemaToolset         SchemaToolsetHandler
	ResourceToolset       ResourceToolsetHandler
}

// OrganizationToolsetHandler handles organization operations
type OrganizationToolsetHandler interface {
	GetOrganization(ctx context.Context, name string) (any, error)
	ListOrganizations(ctx context.Context) (any, error)
	ListSecretReferences(ctx context.Context, orgName string) (any, error)
}

// ProjectToolsetHandler handles organization and project operations
type ProjectToolsetHandler interface {
	// Project operations
	ListProjects(ctx context.Context, orgName string) (any, error)
	GetProject(ctx context.Context, orgName, projectName string) (any, error)
	CreateProject(ctx context.Context, orgName string, req *models.CreateProjectRequest) (any, error)
}

// ComponentToolsetHandler handles component operations
type ComponentToolsetHandler interface {
	CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (any, error)
	ListComponents(ctx context.Context, orgName, projectName string) (any, error)
	GetComponent(
		ctx context.Context, orgName, projectName, componentName string, additionalResources []string,
	) (any, error)
	UpdateComponentBinding(
		ctx context.Context, orgName, projectName, componentName, bindingName string,
		req *models.UpdateBindingRequest,
	) (any, error)
	GetComponentWorkloads(ctx context.Context, orgName, projectName, componentName string) (any, error)
	// Component release operations
	ListComponentReleases(ctx context.Context, orgName, projectName, componentName string) (any, error)
	CreateComponentRelease(ctx context.Context, orgName, projectName, componentName, releaseName string) (any, error)
	GetComponentRelease(ctx context.Context, orgName, projectName, componentName, releaseName string) (any, error)
	// Release binding operations
	ListReleaseBindings(
		ctx context.Context, orgName, projectName, componentName string, environments []string,
	) (any, error)
	PatchReleaseBinding(
		ctx context.Context, orgName, projectName, componentName, bindingName string,
		req *models.PatchReleaseBindingRequest,
	) (any, error)
	// Deployment operations
	DeployRelease(
		ctx context.Context, orgName, projectName, componentName string, req *models.DeployReleaseRequest,
	) (any, error)
	PromoteComponent(
		ctx context.Context, orgName, projectName, componentName string, req *models.PromoteComponentRequest,
	) (any, error)
	// Workload operations
	CreateWorkload(ctx context.Context, orgName, projectName, componentName string, workloadSpec interface{}) (any, error)
	// Schema operations
	GetComponentSchema(ctx context.Context, orgName, projectName, componentName string) (any, error)
	GetComponentReleaseSchema(ctx context.Context, orgName, projectName, componentName, releaseName string) (any, error)
	// Trait operations
	ListComponentTraits(ctx context.Context, orgName, projectName, componentName string) (any, error)
	UpdateComponentTraits(
		ctx context.Context, orgName, projectName, componentName string, req *models.UpdateComponentTraitsRequest,
	) (any, error)
	// Release operations
	GetEnvironmentRelease(ctx context.Context, orgName, projectName, componentName, environmentName string) (any, error)
	// Component patch operations
	PatchComponent(
		ctx context.Context, orgName, projectName, componentName string, req *models.PatchComponentRequest,
	) (any, error)
	// Component workflow operations
	ListComponentWorkflows(ctx context.Context, orgName string) (any, error)
	GetComponentWorkflowSchema(ctx context.Context, orgName, cwName string) (any, error)
	TriggerComponentWorkflow(ctx context.Context, orgName, projectName, componentName, commit string) (any, error)
	ListComponentWorkflowRuns(ctx context.Context, orgName, projectName, componentName string) (any, error)
	UpdateComponentWorkflowSchema(
		ctx context.Context, orgName, projectName, componentName string,
		req *models.UpdateComponentWorkflowRequest,
	) (any, error)
}

// BuildToolsetHandler handles build operations
type BuildToolsetHandler interface {
	ListBuildTemplates(ctx context.Context, orgName string) (any, error)
	TriggerBuild(ctx context.Context, orgName, projectName, componentName, commit string) (any, error)
	ListBuilds(ctx context.Context, orgName, projectName, componentName string) (any, error)
	GetBuildObserverURL(ctx context.Context, orgName, projectName, componentName string) (any, error)
	ListBuildPlanes(ctx context.Context, orgName string) (any, error)
}

// DeploymentToolsetHandler handles deployment operations
type DeploymentToolsetHandler interface {
	GetProjectDeploymentPipeline(ctx context.Context, orgName, projectName string) (any, error)
	GetComponentObserverURL(
		ctx context.Context, orgName, projectName, componentName, environmentName string,
	) (any, error)
}

// InfrastructureToolsetHandler handles infrastructure operations
type InfrastructureToolsetHandler interface {
	// Environment operations
	ListEnvironments(ctx context.Context, orgName string) (any, error)
	GetEnvironment(ctx context.Context, orgName, envName string) (any, error)
	CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (any, error)

	// DataPlane operations
	ListDataPlanes(ctx context.Context, orgName string) (any, error)
	GetDataPlane(ctx context.Context, orgName, dpName string) (any, error)
	CreateDataPlane(ctx context.Context, orgName string, req *models.CreateDataPlaneRequest) (any, error)

	// ComponentType operations
	ListComponentTypes(ctx context.Context, orgName string) (any, error)
	GetComponentTypeSchema(ctx context.Context, orgName, ctName string) (any, error)

	// Workflow operations
	ListWorkflows(ctx context.Context, orgName string) (any, error)
	GetWorkflowSchema(ctx context.Context, orgName, workflowName string) (any, error)

	// Trait operations
	ListTraits(ctx context.Context, orgName string) (any, error)
	GetTraitSchema(ctx context.Context, orgName, traitName string) (any, error)

	// ObservabilityPlane operations
	ListObservabilityPlanes(ctx context.Context, orgName string) (any, error)

	// ComponentWorkflow operations (org-level)
	ListComponentWorkflows(ctx context.Context, orgName string) (any, error)
	GetComponentWorkflowSchema(ctx context.Context, orgName, cwName string) (any, error)
}

// SchemaToolsetHandler handles schema and resource explanation operations
type SchemaToolsetHandler interface {
	ExplainSchema(ctx context.Context, kind, path string) (any, error)
}

// ResourceToolsetHandler handles kubectl-like resource operations (apply/delete)
type ResourceToolsetHandler interface {
	ApplyResource(ctx context.Context, resource map[string]interface{}) (any, error)
	DeleteResource(ctx context.Context, resource map[string]interface{}) (any, error)
}

// RegisterFunc is a function type for registering MCP tools
type RegisterFunc func(s *mcp.Server)
