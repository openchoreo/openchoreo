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
	ToolsetNamespace  ToolsetType = "namespace"
	ToolsetProject    ToolsetType = "project"
	ToolsetComponent  ToolsetType = "component"
	ToolsetDeployment ToolsetType = "deployment"
	ToolsetBuild      ToolsetType = "build"
	ToolsetPE         ToolsetType = "pe"
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
	NamespaceToolset  NamespaceToolsetHandler
	ProjectToolset    ProjectToolsetHandler
	ComponentToolset  ComponentToolsetHandler
	DeploymentToolset DeploymentToolsetHandler
	BuildToolset      BuildToolsetHandler
	PEToolset         PEToolsetHandler
}

// PEToolsetHandler handles platform engineering operations on openchoreo
type PEToolsetHandler interface {
	// Environment operations
	ListEnvironments(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	CreateEnvironment(ctx context.Context, namespaceName string, req *gen.CreateEnvironmentJSONRequestBody) (any, error)
	UpdateEnvironment(ctx context.Context, namespaceName string, req *gen.UpdateEnvironmentJSONRequestBody) (any, error)
	DeleteEnvironment(ctx context.Context, namespaceName, envName string) (any, error)

	// Component release operations
	ListComponentReleases(ctx context.Context, namespaceName, componentName string, opts ListOpts) (any, error)
	CreateComponentRelease(ctx context.Context, namespaceName, componentName, releaseName string) (any, error)
	GetComponentRelease(ctx context.Context, namespaceName, releaseName string) (any, error)
	GetComponentReleaseSchema(
		ctx context.Context, namespaceName, componentName, releaseName string,
	) (any, error)

	// DeploymentPipeline operations
	CreateDeploymentPipeline(ctx context.Context, namespaceName string,
		req *gen.CreateDeploymentPipelineJSONRequestBody) (any, error)
	UpdateDeploymentPipeline(ctx context.Context, namespaceName string,
		req *gen.UpdateDeploymentPipelineJSONRequestBody) (any, error)
	DeleteDeploymentPipeline(ctx context.Context, namespaceName, dpName string) (any, error)

	// DataPlane operations
	ListDataPlanes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetDataPlane(ctx context.Context, namespaceName, dpName string) (any, error)

	// WorkflowPlane operations
	ListWorkflowPlanes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) (any, error)

	// ObservabilityPlane operations
	ListObservabilityPlanes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) (any, error)

	// ClusterDataPlane operations
	ListClusterDataPlanes(ctx context.Context, opts ListOpts) (any, error)
	GetClusterDataPlane(ctx context.Context, cdpName string) (any, error)

	// ClusterWorkflowPlane operations
	ListClusterWorkflowPlanes(ctx context.Context, opts ListOpts) (any, error)
	GetClusterWorkflowPlane(ctx context.Context, cbpName string) (any, error)

	// ClusterObservabilityPlane operations
	ListClusterObservabilityPlanes(ctx context.Context, opts ListOpts) (any, error)
	GetClusterObservabilityPlane(ctx context.Context, copName string) (any, error)

	// Platform standards (namespace-scoped) — read
	ListComponentTypes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetComponentType(ctx context.Context, namespaceName, ctName string) (any, error)
	GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error)
	ListTraits(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetTrait(ctx context.Context, namespaceName, traitName string) (any, error)
	GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error)
	ListWorkflows(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetWorkflow(ctx context.Context, namespaceName, workflowName string) (any, error)
	GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (any, error)

	// Platform standards (namespace-scoped) — write
	CreateComponentType(
		ctx context.Context, namespaceName string, req *gen.CreateComponentTypeJSONRequestBody,
	) (any, error)
	UpdateComponentType(
		ctx context.Context, namespaceName string, req *gen.UpdateComponentTypeJSONRequestBody,
	) (any, error)
	DeleteComponentType(ctx context.Context, namespaceName, ctName string) (any, error)
	CreateTrait(ctx context.Context, namespaceName string, req *gen.CreateTraitJSONRequestBody) (any, error)
	UpdateTrait(ctx context.Context, namespaceName string, req *gen.UpdateTraitJSONRequestBody) (any, error)
	DeleteTrait(ctx context.Context, namespaceName, traitName string) (any, error)
	CreateWorkflow(ctx context.Context, namespaceName string, req *gen.CreateWorkflowJSONRequestBody) (any, error)
	UpdateWorkflow(ctx context.Context, namespaceName string, req *gen.UpdateWorkflowJSONRequestBody) (any, error)
	DeleteWorkflow(ctx context.Context, namespaceName, workflowName string) (any, error)

	// Platform standards (cluster-scoped) — read
	ListClusterComponentTypes(ctx context.Context, opts ListOpts) (any, error)
	GetClusterComponentType(ctx context.Context, cctName string) (any, error)
	GetClusterComponentTypeSchema(ctx context.Context, cctName string) (any, error)
	ListClusterTraits(ctx context.Context, opts ListOpts) (any, error)
	GetClusterTrait(ctx context.Context, ctName string) (any, error)
	GetClusterTraitSchema(ctx context.Context, ctName string) (any, error)

	// Platform standards (cluster-scoped) — write
	CreateClusterComponentType(ctx context.Context, req *gen.CreateClusterComponentTypeJSONRequestBody) (any, error)
	UpdateClusterComponentType(ctx context.Context, req *gen.UpdateClusterComponentTypeJSONRequestBody) (any, error)
	DeleteClusterComponentType(ctx context.Context, cctName string) (any, error)
	CreateClusterTrait(ctx context.Context, req *gen.CreateClusterTraitJSONRequestBody) (any, error)
	UpdateClusterTrait(ctx context.Context, req *gen.UpdateClusterTraitJSONRequestBody) (any, error)
	DeleteClusterTrait(ctx context.Context, clusterTraitName string) (any, error)
	CreateClusterWorkflow(ctx context.Context, req *gen.CreateClusterWorkflowJSONRequestBody) (any, error)
	UpdateClusterWorkflow(ctx context.Context, req *gen.UpdateClusterWorkflowJSONRequestBody) (any, error)
	DeleteClusterWorkflow(ctx context.Context, clusterWorkflowName string) (any, error)

	// Diagnostics
	GetResourceEvents(ctx context.Context, namespaceName, releaseBindingName,
		group, version, kind, name string) (any, error)
	GetResourceLogs(ctx context.Context, namespaceName, releaseBindingName,
		podName string, sinceSeconds *int64) (any, error)
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

// ComponentToolsetHandler handles component definition and configuration operations
type ComponentToolsetHandler interface {
	CreateComponent(
		ctx context.Context, namespaceName, projectName string, req *gen.CreateComponentRequest,
	) (any, error)
	ListComponents(ctx context.Context, namespaceName, projectName string, opts ListOpts) (any, error)
	GetComponent(ctx context.Context, namespaceName, componentName string) (any, error)
	PatchComponent(
		ctx context.Context, namespaceName, componentName string, req *gen.PatchComponentRequest,
	) (any, error)
	ListWorkloads(ctx context.Context, namespaceName, componentName string) (any, error)
	GetWorkload(ctx context.Context, namespaceName, workloadName string) (any, error)
	CreateWorkload(
		ctx context.Context, namespaceName, componentName string, workloadSpec interface{},
	) (any, error)
	UpdateWorkload(
		ctx context.Context, namespaceName, workloadName string, workloadSpec interface{},
	) (any, error)
	GetWorkloadSchema(ctx context.Context) (any, error)
	GetComponentSchema(ctx context.Context, namespaceName, componentName string) (any, error)

	// Platform standards (read-only, namespace-scoped)
	ListComponentTypes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error)
	ListTraits(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error)

	// Platform standards (read-only, cluster-scoped)
	ListClusterComponentTypes(ctx context.Context, opts ListOpts) (any, error)
	GetClusterComponentType(ctx context.Context, cctName string) (any, error)
	GetClusterComponentTypeSchema(ctx context.Context, cctName string) (any, error)
	ListClusterTraits(ctx context.Context, opts ListOpts) (any, error)
	GetClusterTrait(ctx context.Context, ctName string) (any, error)
	GetClusterTraitSchema(ctx context.Context, ctName string) (any, error)
}

// DeploymentToolsetHandler handles release, deployment, and promotion operations
type DeploymentToolsetHandler interface {
	ListComponentReleases(ctx context.Context, namespaceName, componentName string, opts ListOpts) (any, error)
	CreateComponentRelease(ctx context.Context, namespaceName, componentName, releaseName string) (any, error)
	GetComponentRelease(ctx context.Context, namespaceName, releaseName string) (any, error)
	GetComponentReleaseSchema(
		ctx context.Context, namespaceName, componentName, releaseName string,
	) (any, error)
	ListReleaseBindings(ctx context.Context, namespaceName, componentName string, opts ListOpts) (any, error)
	GetReleaseBinding(ctx context.Context, namespaceName, bindingName string) (any, error)
	PatchReleaseBinding(
		ctx context.Context, namespaceName, bindingName string,
		req *gen.ReleaseBindingSpec,
	) (any, error)
	UpdateReleaseBindingState(
		ctx context.Context, namespaceName, bindingName string,
		state *gen.ReleaseBindingSpecState,
	) (any, error)
	ListDeploymentPipelines(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetDeploymentPipeline(ctx context.Context, namespaceName, pipelineName string) (any, error)
	ListEnvironments(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
}

// BuildToolsetHandler handles workflow and CI/CD operations
type BuildToolsetHandler interface {
	TriggerWorkflowRun(
		ctx context.Context, namespaceName, projectName, componentName, commit string,
	) (any, error)
	CreateWorkflowRun(
		ctx context.Context, namespaceName, workflowName string,
		parameters map[string]interface{},
	) (any, error)
	ListWorkflowRuns(
		ctx context.Context, namespaceName, projectName, componentName string,
		opts ListOpts,
	) (any, error)
	GetWorkflowRun(ctx context.Context, namespaceName, runName string) (any, error)
	ListWorkflows(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (any, error)
	ListClusterWorkflows(ctx context.Context, opts ListOpts) (any, error)
	GetClusterWorkflow(ctx context.Context, cwfName string) (any, error)
	GetClusterWorkflowSchema(ctx context.Context, cwfName string) (any, error)
}

// RegisterFunc is a function type for registering MCP tools
type RegisterFunc func(s *mcp.Server)
