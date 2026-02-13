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
	ToolsetNamespace      ToolsetType = "namespace"
	ToolsetProject        ToolsetType = "project"
	ToolsetComponent      ToolsetType = "component"
	ToolsetBuild          ToolsetType = "build"
	ToolsetDeployment     ToolsetType = "deployment"
	ToolsetInfrastructure ToolsetType = "infrastructure"
	ToolsetSchema         ToolsetType = "schema"
	ToolsetResource       ToolsetType = "resource"
)

type Toolsets struct {
	NamespaceToolset      NamespaceToolsetHandler
	ProjectToolset        ProjectToolsetHandler
	ComponentToolset      ComponentToolsetHandler
	BuildToolset          BuildToolsetHandler
	DeploymentToolset     DeploymentToolsetHandler
	InfrastructureToolset InfrastructureToolsetHandler
	SchemaToolset         SchemaToolsetHandler
	ResourceToolset       ResourceToolsetHandler
}

// NamespaceToolsetHandler handles namespace operations
type NamespaceToolsetHandler interface {
	GetNamespace(ctx context.Context, name string) (any, error)
	ListNamespaces(ctx context.Context) (any, error)
	CreateNamespace(ctx context.Context, req *models.CreateNamespaceRequest) (any, error)
	ListSecretReferences(ctx context.Context, namespaceName string) (any, error)
}

// ProjectToolsetHandler handles project operations
type ProjectToolsetHandler interface {
	// Project operations
	ListProjects(ctx context.Context, namespaceName string) (any, error)
	GetProject(ctx context.Context, namespaceName, projectName string) (any, error)
	CreateProject(ctx context.Context, namespaceName string, req *models.CreateProjectRequest) (any, error)
}

// ComponentToolsetHandler handles component operations
type ComponentToolsetHandler interface {
	CreateComponent(
		ctx context.Context, namespaceName, projectName string, req *models.CreateComponentRequest,
	) (any, error)
	ListComponents(ctx context.Context, namespaceName, projectName string) (any, error)
	GetComponent(
		ctx context.Context, namespaceName, projectName, componentName string, additionalResources []string,
	) (any, error)
	UpdateComponentBinding(
		ctx context.Context, namespaceName, projectName, componentName, bindingName string,
		req *models.UpdateBindingRequest,
	) (any, error)
	GetComponentWorkloads(ctx context.Context, namespaceName, projectName, componentName string) (any, error)
	// Component release operations
	ListComponentReleases(ctx context.Context, namespaceName, projectName, componentName string) (any, error)
	CreateComponentRelease(ctx context.Context, namespaceName, projectName, componentName, releaseName string) (any, error)
	GetComponentRelease(ctx context.Context, namespaceName, projectName, componentName, releaseName string) (any, error)
	// Release binding operations
	ListReleaseBindings(
		ctx context.Context, namespaceName, projectName, componentName string, environments []string,
	) (any, error)
	PatchReleaseBinding(
		ctx context.Context, namespaceName, projectName, componentName, bindingName string,
		req *models.PatchReleaseBindingRequest,
	) (any, error)
	// Deployment operations
	DeployRelease(
		ctx context.Context, namespaceName, projectName, componentName string, req *models.DeployReleaseRequest,
	) (any, error)
	PromoteComponent(
		ctx context.Context, namespaceName, projectName, componentName string, req *models.PromoteComponentRequest,
	) (any, error)
	// Workload operations
	CreateWorkload(
		ctx context.Context, namespaceName, projectName, componentName string, workloadSpec interface{},
	) (any, error)
	// Schema operations
	GetComponentSchema(ctx context.Context, namespaceName, projectName, componentName string) (any, error)
	GetComponentReleaseSchema(
		ctx context.Context, namespaceName, projectName, componentName, releaseName string,
	) (any, error)
	// Trait operations
	ListComponentTraits(ctx context.Context, namespaceName, projectName, componentName string) (any, error)
	UpdateComponentTraits(
		ctx context.Context, namespaceName, projectName, componentName string, req *models.UpdateComponentTraitsRequest,
	) (any, error)
	// Release operations
	GetEnvironmentRelease(
		ctx context.Context, namespaceName, projectName, componentName, environmentName string,
	) (any, error)
	// Component patch operations
	PatchComponent(
		ctx context.Context, namespaceName, projectName, componentName string, req *models.PatchComponentRequest,
	) (any, error)
	// Component workflow operations
	ListComponentWorkflows(ctx context.Context, namespaceName string) (any, error)
	GetComponentWorkflowSchema(ctx context.Context, namespaceName, cwName string) (any, error)
	TriggerComponentWorkflow(ctx context.Context, namespaceName, projectName, componentName, commit string) (any, error)
	ListComponentWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName string) (any, error)
	UpdateComponentWorkflowSchema(
		ctx context.Context, namespaceName, projectName, componentName string,
		req *models.UpdateComponentWorkflowRequest,
	) (any, error)
}

// BuildToolsetHandler handles build operations
type BuildToolsetHandler interface {
	ListBuildTemplates(ctx context.Context, namespaceName string) (any, error)
	TriggerBuild(ctx context.Context, namespaceName, projectName, componentName, commit string) (any, error)
	ListBuilds(ctx context.Context, namespaceName, projectName, componentName string) (any, error)
	GetBuildObserverURL(ctx context.Context, namespaceName, projectName, componentName string) (any, error)
	ListBuildPlanes(ctx context.Context, namespaceName string) (any, error)
}

// DeploymentToolsetHandler handles deployment operations
type DeploymentToolsetHandler interface {
	GetProjectDeploymentPipeline(ctx context.Context, namespaceName, projectName string) (any, error)
	GetComponentObserverURL(
		ctx context.Context, namespaceName, projectName, componentName, environmentName string,
	) (any, error)
}

// InfrastructureToolsetHandler handles infrastructure operations
type InfrastructureToolsetHandler interface {
	// Environment operations
	ListEnvironments(ctx context.Context, namespaceName string) (any, error)
	GetEnvironment(ctx context.Context, namespaceName, envName string) (any, error)
	CreateEnvironment(ctx context.Context, namespaceName string, req *models.CreateEnvironmentRequest) (any, error)

	// DataPlane operations
	ListDataPlanes(ctx context.Context, namespaceName string) (any, error)
	GetDataPlane(ctx context.Context, namespaceName, dpName string) (any, error)
	CreateDataPlane(ctx context.Context, namespaceName string, req *models.CreateDataPlaneRequest) (any, error)

	// ComponentType operations
	ListComponentTypes(ctx context.Context, namespaceName string) (any, error)
	GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error)

	// Workflow operations
	ListWorkflows(ctx context.Context, namespaceName string) (any, error)
	GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (any, error)

	// Trait operations
	ListTraits(ctx context.Context, namespaceName string) (any, error)
	GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error)

	// ObservabilityPlane operations
	ListObservabilityPlanes(ctx context.Context, namespaceName string) (any, error)

	// ComponentWorkflow operations (namespace-level)
	ListComponentWorkflows(ctx context.Context, namespaceName string) (any, error)
	GetComponentWorkflowSchema(ctx context.Context, namespaceName, cwName string) (any, error)
}

// ClusterPlaneHandler is an optional extension of InfrastructureToolsetHandler
// for cluster-scoped plane operations. Handlers that implement this interface
// alongside InfrastructureToolsetHandler will have cluster-plane MCP tools
// registered automatically. If the InfrastructureToolset does not implement
// ClusterPlaneHandler, the cluster-plane tools are silently skipped.
type ClusterPlaneHandler interface {
	// ClusterDataPlane operations
	ListClusterDataPlanes(ctx context.Context) (any, error)
	GetClusterDataPlane(ctx context.Context, cdpName string) (any, error)
	CreateClusterDataPlane(ctx context.Context, req *models.CreateClusterDataPlaneRequest) (any, error)

	// ClusterBuildPlane operations
	ListClusterBuildPlanes(ctx context.Context) (any, error)

	// ClusterObservabilityPlane operations
	ListClusterObservabilityPlanes(ctx context.Context) (any, error)
}

// SchemaToolsetHandler handles schema and resource explanation operations
type SchemaToolsetHandler interface {
	ExplainSchema(ctx context.Context, kind, path string) (any, error)
}

// ResourceToolsetHandler handles kubectl-like resource operations (apply/delete/get)
type ResourceToolsetHandler interface {
	ApplyResource(ctx context.Context, resource map[string]interface{}) (any, error)
	DeleteResource(ctx context.Context, resource map[string]interface{}) (any, error)
	GetResource(ctx context.Context, namespaceName, kind, resourceName string) (any, error)
}

// RegisterFunc is a function type for registering MCP tools
type RegisterFunc func(s *mcp.Server)
