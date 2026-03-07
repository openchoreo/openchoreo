// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ToolsetType represents a type of toolset that can be enabled
type ToolsetType string

const (
	ToolsetNamespace      ToolsetType = "namespace"
	ToolsetProject        ToolsetType = "project"
	ToolsetComponent      ToolsetType = "component"
	ToolsetInfrastructure ToolsetType = "infrastructure"
	ToolsetPE             ToolsetType = "pe"
)

// DefaultPageSize is the default number of items per page for MCP list operations.
const DefaultPageSize = 100

// ListOpts holds optional pagination parameters for list operations.
type ListOpts struct {
	// Limit is the maximum number of items to return per page.
	// When 0 or unset, DefaultPageSize is used.
	Limit int
	// Cursor is an opaque pagination cursor from a previous response.
	Cursor string
}

// EffectiveLimit returns the limit to use, applying DefaultPageSize when unset.
func (o ListOpts) EffectiveLimit() int {
	if o.Limit <= 0 {
		return DefaultPageSize
	}
	return o.Limit
}

type Toolsets struct {
	NamespaceToolset      NamespaceToolsetHandler
	ProjectToolset        ProjectToolsetHandler
	ComponentToolset      ComponentToolsetHandler
	InfrastructureToolset InfrastructureToolsetHandler
	PEToolset             PEToolsetHandler
}

// PEToolsetHandler handles platform engineering operations on openchoreo
type PEToolsetHandler interface {
	CreateEnvironment(ctx context.Context, namespaceName string, req *gen.CreateEnvironmentJSONRequestBody) (any, error)

	// DataPlane operations
	ListDataPlanes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetDataPlane(ctx context.Context, namespaceName, dpName string) (any, error)
	CreateDataPlane(ctx context.Context, namespaceName string, req *gen.CreateDataPlaneJSONRequestBody) (any, error)

	// ObservabilityPlane operations
	ListObservabilityPlanes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)

	// BuildPlane operations
	ListBuildPlanes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)

	// ClusterDataPlane operations
	ListClusterDataPlanes(ctx context.Context, opts ListOpts) (any, error)
	GetClusterDataPlane(ctx context.Context, cdpName string) (any, error)
	CreateClusterDataPlane(ctx context.Context, req *gen.CreateClusterDataPlaneJSONRequestBody) (any, error)

	// ClusterBuildPlane operations
	ListClusterBuildPlanes(ctx context.Context, opts ListOpts) (any, error)

	// ClusterObservabilityPlane operations
	ListClusterObservabilityPlanes(ctx context.Context, opts ListOpts) (any, error)
}

// NamespaceToolsetHandler handles namespace operations
type NamespaceToolsetHandler interface {
	ListNamespaces(ctx context.Context, opts ListOpts) (any, error)
	CreateNamespace(ctx context.Context, req *gen.CreateNamespaceJSONRequestBody) (any, error)
	ListSecretReferences(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
}

// ProjectToolsetHandler handles project operations
type ProjectToolsetHandler interface {
	// Project operations
	ListProjects(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	CreateProject(ctx context.Context, namespaceName string, req *gen.CreateProjectJSONRequestBody) (any, error)
}

// ComponentToolsetHandler handles component operations
type ComponentToolsetHandler interface {
	CreateComponent(
		ctx context.Context, namespaceName, projectName string, req *gen.CreateComponentRequest,
	) (any, error)
	ListComponents(ctx context.Context, namespaceName, projectName string, opts ListOpts) (any, error)
	GetComponent(ctx context.Context, namespaceName, componentName string) (any, error)
	ListWorkloads(ctx context.Context, namespaceName, componentName string) (any, error)
	GetWorkload(ctx context.Context, namespaceName, workloadName string) (any, error)
	// Component release operations
	ListComponentReleases(ctx context.Context, namespaceName, componentName string, opts ListOpts) (any, error)
	CreateComponentRelease(ctx context.Context, namespaceName, componentName, releaseName string) (any, error)
	GetComponentRelease(ctx context.Context, namespaceName, releaseName string) (any, error)
	// Release binding operations
	ListReleaseBindings(ctx context.Context, namespaceName, componentName string, opts ListOpts) (any, error)
	GetReleaseBinding(ctx context.Context, namespaceName, bindingName string) (any, error)
	PatchReleaseBinding(
		ctx context.Context, namespaceName, bindingName string,
		req *gen.ReleaseBindingSpec,
	) (any, error)
	// Deployment operations
	DeployRelease(
		ctx context.Context, namespaceName, componentName string, req *gen.DeployReleaseRequest,
	) (any, error)
	PromoteComponent(
		ctx context.Context, namespaceName, componentName string, req *gen.PromoteComponentRequest,
	) (any, error)
	// Workload operations
	CreateWorkload(
		ctx context.Context, namespaceName, componentName string, workloadSpec interface{},
	) (any, error)
	UpdateWorkload(
		ctx context.Context, namespaceName, workloadName string, workloadSpec interface{},
	) (any, error)
	GetWorkloadSchema(ctx context.Context) (any, error)
	// Schema operations
	GetComponentSchema(ctx context.Context, namespaceName, componentName string) (any, error)
	// Component patch operations
	PatchComponent(
		ctx context.Context, namespaceName, componentName string, req *gen.PatchComponentRequest,
	) (any, error)
	// Release binding state operations
	UpdateReleaseBindingState(
		ctx context.Context, namespaceName, bindingName string,
		state *gen.ReleaseBindingSpecState,
	) (any, error)
	// Component release schema
	GetComponentReleaseSchema(
		ctx context.Context, namespaceName, componentName, releaseName string,
	) (any, error)
	// Workflow run operations scoped by component
	TriggerWorkflowRun(
		ctx context.Context, namespaceName, projectName, componentName, commit string,
	) (any, error)

	// ComponentType operations
	ListComponentTypes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error)

	// Trait operations
	ListTraits(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error)

	// WorkflowRun operations
	CreateWorkflowRun(
		ctx context.Context, namespaceName, workflowName string,
		parameters map[string]interface{},
	) (any, error)
	ListWorkflowRuns(
		ctx context.Context, namespaceName, projectName, componentName string,
		opts ListOpts,
	) (any, error)
	GetWorkflowRun(ctx context.Context, namespaceName, runName string) (any, error)

	// ClusterComponentType operations
	ListClusterComponentTypes(ctx context.Context, opts ListOpts) (any, error)
	GetClusterComponentType(ctx context.Context, cctName string) (any, error)
	GetClusterComponentTypeSchema(ctx context.Context, cctName string) (any, error)

	// ClusterTrait operations
	ListClusterTraits(ctx context.Context, opts ListOpts) (any, error)
	GetClusterTrait(ctx context.Context, ctName string) (any, error)
	GetClusterTraitSchema(ctx context.Context, ctName string) (any, error)

	// ClusterWorkflow operations
	ListClusterWorkflows(ctx context.Context, opts ListOpts) (any, error)
	GetClusterWorkflow(ctx context.Context, cwfName string) (any, error)
	GetClusterWorkflowSchema(ctx context.Context, cwfName string) (any, error)

	// Workflow operations
	ListWorkflows(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (any, error)
}

// InfrastructureToolsetHandler handles infrastructure operations
type InfrastructureToolsetHandler interface {
	// Environment operations
	ListEnvironments(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetEnvironment(ctx context.Context, namespaceName, envName string) (any, error)

	// DeploymentPipeline operations
	GetDeploymentPipeline(ctx context.Context, namespaceName, pipelineName string) (any, error)
	ListDeploymentPipelines(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
}

// RegisterFunc is a function type for registering MCP tools
type RegisterFunc func(s *mcp.Server)
