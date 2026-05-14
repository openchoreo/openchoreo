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

// requestedToolsetsCtxKey is the context key used to carry the set of toolsets
// the client requested via the ?toolsets= query param.
type requestedToolsetsCtxKey struct{}

// filterByAuthzCtxKey is the context key used to carry the per-session
// filterByAuthz flag from the ?filterByAuthz= query param.
type filterByAuthzCtxKey struct{}

// includeDeprecatedToolsCtxKey is the context key used to carry the per-session
// includeDeprecatedTools flag from the ?includeDeprecatedTools= query param.
type includeDeprecatedToolsCtxKey struct{}

// WithRequestedToolsets returns a copy of ctx that carries the set of toolsets
// the client requested. Empty or nil set means "no narrowing" — the middleware
// will not apply a toolset filter.
func WithRequestedToolsets(ctx context.Context, requested map[ToolsetType]bool) context.Context {
	if len(requested) == 0 {
		return ctx
	}
	return context.WithValue(ctx, requestedToolsetsCtxKey{}, requested)
}

// RequestedToolsetsFromContext returns the set of toolsets the client requested
// for this session, if any. The second return value reports whether the client
// supplied any narrowing.
func RequestedToolsetsFromContext(ctx context.Context) (map[ToolsetType]bool, bool) {
	v, ok := ctx.Value(requestedToolsetsCtxKey{}).(map[ToolsetType]bool)
	return v, ok && len(v) > 0
}

// WithFilterByAuthz returns a copy of ctx carrying the per-session decision of
// whether to apply MCP-layer authz filtering. The default (no value in ctx) is
// true.
func WithFilterByAuthz(ctx context.Context, filter bool) context.Context {
	return context.WithValue(ctx, filterByAuthzCtxKey{}, filter)
}

// FilterByAuthzFromContext returns the per-session filterByAuthz flag if the
// client explicitly supplied one. The second return value reports whether a
// value was set; callers should default to true when not set.
func FilterByAuthzFromContext(ctx context.Context) (bool, bool) {
	v, ok := ctx.Value(filterByAuthzCtxKey{}).(bool)
	return v, ok
}

// WithIncludeDeprecatedTools returns a copy of ctx carrying the per-session
// decision of whether tools/list should include deprecated compatibility-alias
// tools. The default (no value in ctx) is true: deprecated aliases are listed
// alongside the canonical tools, each carrying a description-level deprecation
// banner and a structured _meta marker. Clients that want to preview the
// post-deprecation surface can set this to false to hide the aliases.
func WithIncludeDeprecatedTools(ctx context.Context, include bool) context.Context {
	return context.WithValue(ctx, includeDeprecatedToolsCtxKey{}, include)
}

// IncludeDeprecatedToolsFromContext reports whether tools/list should include
// the deprecated compatibility-alias tools for this session. Defaults to true
// when the client did not set the flag.
func IncludeDeprecatedToolsFromContext(ctx context.Context) bool {
	v, ok := ctx.Value(includeDeprecatedToolsCtxKey{}).(bool)
	if !ok {
		return true
	}
	return v
}

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
	ListClusterWorkflows(ctx context.Context, opts ListOpts) (any, error)
	GetClusterWorkflow(ctx context.Context, cwfName string) (any, error)
	GetClusterWorkflowSchema(ctx context.Context, cwfName string) (any, error)

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
	GetResourceTree(ctx context.Context, namespaceName, releaseBindingName string) (any, error)
	GetResourceEvents(ctx context.Context, namespaceName, releaseBindingName,
		group, version, kind, name string) (any, error)
	GetResourceLogs(ctx context.Context, namespaceName, releaseBindingName,
		podName string, sinceSeconds *int64) (any, error)

	// Authz roles (namespace-scoped)
	ListAuthzRoles(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetAuthzRole(ctx context.Context, namespaceName, roleName string) (any, error)
	CreateAuthzRole(
		ctx context.Context, namespaceName string, req *gen.CreateNamespaceRoleJSONRequestBody,
	) (any, error)
	UpdateAuthzRole(
		ctx context.Context, namespaceName string, req *gen.UpdateNamespaceRoleJSONRequestBody,
	) (any, error)
	DeleteAuthzRole(ctx context.Context, namespaceName, roleName string) (any, error)

	// Authz roles (cluster-scoped)
	ListClusterAuthzRoles(ctx context.Context, opts ListOpts) (any, error)
	GetClusterAuthzRole(ctx context.Context, roleName string) (any, error)
	CreateClusterAuthzRole(ctx context.Context, req *gen.CreateClusterRoleJSONRequestBody) (any, error)
	UpdateClusterAuthzRole(ctx context.Context, req *gen.UpdateClusterRoleJSONRequestBody) (any, error)
	DeleteClusterAuthzRole(ctx context.Context, roleName string) (any, error)

	// Authz role bindings (namespace-scoped)
	ListAuthzRoleBindings(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetAuthzRoleBinding(ctx context.Context, namespaceName, bindingName string) (any, error)
	CreateAuthzRoleBinding(
		ctx context.Context, namespaceName string, req *gen.CreateNamespaceRoleBindingJSONRequestBody,
	) (any, error)
	UpdateAuthzRoleBinding(
		ctx context.Context, namespaceName string, req *gen.UpdateNamespaceRoleBindingJSONRequestBody,
	) (any, error)
	DeleteAuthzRoleBinding(ctx context.Context, namespaceName, bindingName string) (any, error)

	// Authz role bindings (cluster-scoped)
	ListClusterAuthzRoleBindings(ctx context.Context, opts ListOpts) (any, error)
	GetClusterAuthzRoleBinding(ctx context.Context, bindingName string) (any, error)
	CreateClusterAuthzRoleBinding(
		ctx context.Context, req *gen.CreateClusterRoleBindingJSONRequestBody,
	) (any, error)
	UpdateClusterAuthzRoleBinding(
		ctx context.Context, req *gen.UpdateClusterRoleBindingJSONRequestBody,
	) (any, error)
	DeleteClusterAuthzRoleBinding(ctx context.Context, bindingName string) (any, error)

	// Authz diagnostics
	EvaluateAuthz(ctx context.Context, requests []gen.EvaluateRequest) (any, error)
	ListAuthzActions(ctx context.Context) (any, error)
}

// NamespaceToolsetHandler handles namespace operations
type NamespaceToolsetHandler interface {
	ListNamespaces(ctx context.Context, opts ListOpts) (any, error)
	CreateNamespace(ctx context.Context, req *gen.CreateNamespaceJSONRequestBody) (any, error)
	ListSecretReferences(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (any, error)
	CreateSecretReference(
		ctx context.Context, namespaceName string, req *gen.CreateSecretReferenceJSONRequestBody,
	) (any, error)
	UpdateSecretReference(
		ctx context.Context, namespaceName string, req *gen.UpdateSecretReferenceJSONRequestBody,
	) (any, error)
	DeleteSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (any, error)
}

// ProjectToolsetHandler handles project operations
type ProjectToolsetHandler interface {
	// Project operations
	ListProjects(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	CreateProject(ctx context.Context, namespaceName string, req *gen.CreateProjectJSONRequestBody) (any, error)
	UpdateProject(ctx context.Context, namespaceName, projectName string, req *gen.PatchProjectRequest) (any, error)
	DeleteProject(ctx context.Context, namespaceName, projectName string) (any, error)
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
	ListWorkloads(ctx context.Context, namespaceName, componentName string, opts ListOpts) (any, error)
	GetWorkload(ctx context.Context, namespaceName, workloadName string) (any, error)
	CreateWorkload(
		ctx context.Context, namespaceName, componentName string, workloadSpec any,
	) (any, error)
	UpdateWorkload(
		ctx context.Context, namespaceName, workloadName string, workloadSpec any,
	) (any, error)
	DeleteComponent(ctx context.Context, namespaceName, componentName string) (any, error)
	DeleteWorkload(ctx context.Context, namespaceName, workloadName string) (any, error)
	GetWorkloadSchema(ctx context.Context) (any, error)
	GetComponentSchema(ctx context.Context, namespaceName, componentName string) (any, error)

	// Platform standards (read-only, namespace-scoped)
	ListComponentTypes(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetComponentType(ctx context.Context, namespaceName, ctName string) (any, error)
	GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error)
	ListTraits(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetTrait(ctx context.Context, namespaceName, traitName string) (any, error)
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
	CreateReleaseBinding(
		ctx context.Context, namespaceName string,
		req *gen.ReleaseBindingSpec,
	) (any, error)
	UpdateReleaseBinding(
		ctx context.Context, namespaceName, bindingName string,
		req *gen.ReleaseBindingSpec,
	) (any, error)
	DeleteReleaseBinding(ctx context.Context, namespaceName, bindingName string) (any, error)
	DeleteComponentRelease(ctx context.Context, namespaceName, componentReleaseName string) (any, error)
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
		parameters map[string]any,
	) (any, error)
	ListWorkflowRuns(
		ctx context.Context, namespaceName, projectName, componentName string,
		opts ListOpts,
	) (any, error)
	GetWorkflowRun(ctx context.Context, namespaceName, runName string) (any, error)
	GetWorkflowRunStatus(ctx context.Context, namespaceName, runName string) (any, error)
	GetWorkflowRunLogs(
		ctx context.Context, namespaceName, runName, taskName string, sinceSeconds *int64,
	) (any, error)
	GetWorkflowRunEvents(ctx context.Context, namespaceName, runName, taskName string) (any, error)
	ListWorkflows(ctx context.Context, namespaceName string, opts ListOpts) (any, error)
	GetWorkflow(ctx context.Context, namespaceName, workflowName string) (any, error)
	GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (any, error)
	ListClusterWorkflows(ctx context.Context, opts ListOpts) (any, error)
	GetClusterWorkflow(ctx context.Context, cwfName string) (any, error)
	GetClusterWorkflowSchema(ctx context.Context, cwfName string) (any, error)
}

// RegisterFunc is a function type for registering MCP tools.
// Each RegisterFunc must declare its required permission by writing to the perms map.
type RegisterFunc func(s *mcp.Server, perms map[string]ToolPermission)

// ToolPermission associates an MCP tool with the authz action(s) required to use
// it. Action values must be action constants defined in
// internal/authz/core/actions.go.
type ToolPermission struct {
	// ToolName is the MCP tool name (first arg to mcp.AddTool).
	ToolName string
	// Action is the required authz action (e.g. "namespace:view", "component:create")
	// for tools whose required action does not depend on the request arguments.
	// Mutually exclusive with ScopedActions.
	Action string
	// ScopedActions maps a `scope` argument value (ScopeNamespace / ScopeCluster)
	// to the authz action required for that scope. It is populated for
	// scope-collapsed tools whose required action depends on the requested scope.
	// When set, Action is empty.
	ScopedActions map[string]string
}

// Actions returns every authz action this permission may require. For a
// scope-collapsed tool this is the union of all per-scope actions; for a plain
// tool it is the single action (or empty).
func (p ToolPermission) Actions() []string {
	if len(p.ScopedActions) > 0 {
		out := make([]string, 0, len(p.ScopedActions))
		for _, a := range p.ScopedActions {
			if a != "" {
				out = append(out, a)
			}
		}
		return out
	}
	if p.Action == "" {
		return nil
	}
	return []string{p.Action}
}

// ActionForScope returns the authz action required for the given scope value. For
// a plain tool it returns Action regardless of scope. For a scope-collapsed tool
// an empty scope is treated as ScopeNamespace.
func (p ToolPermission) ActionForScope(scope string) string {
	if len(p.ScopedActions) == 0 {
		return p.Action
	}
	if scope == "" {
		scope = ScopeNamespace
	}
	return p.ScopedActions[scope]
}
