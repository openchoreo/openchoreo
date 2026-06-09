// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ---------------------------------------------------------------------------
// Workflows — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_workflows", "workflow",
		"List workflows. With scope=\"namespace\" (default) lists a namespace's workflows (requires "+
			"namespace_name); with scope=\"cluster\" lists platform-wide ClusterWorkflows. Workflows are reusable "+
			"templates that define automated processes such as CI/CD pipelines executed on the workflow plane. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		scopedListHandlers{
			namespace: t.PEToolset.ListWorkflows,
			cluster:   t.PEToolset.ListClusterWorkflows,
		})
}

func (t *Toolsets) RegisterListWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_workflows", "workflow",
		"List workflows. With scope=\"namespace\" (default) lists a namespace's workflows (requires "+
			"namespace_name); with scope=\"cluster\" lists platform-wide ClusterWorkflows. Workflows are reusable "+
			"templates that define automated processes such as CI/CD pipelines executed on the workflow plane. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		scopedListHandlers{
			namespace: t.BuildToolset.ListWorkflows,
			cluster:   t.BuildToolset.ListClusterWorkflows,
		})
}

func (t *Toolsets) RegisterPEGetWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflow", "workflow",
		"Get the full definition of a workflow including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterWorkflow. Call this before update_workflow to retrieve the current spec.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		"name", "Name of the workflow. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetWorkflow,
			cluster:   t.PEToolset.GetClusterWorkflow,
		})
}

func (t *Toolsets) RegisterGetWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflow", "workflow",
		"Get the full definition of a workflow including its complete spec. Use scope=\"cluster\" for a "+
			"platform-wide ClusterWorkflow. Call this before update_workflow to retrieve the current spec.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		"name", "Name of the workflow. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.BuildToolset.GetWorkflow,
			cluster:   t.BuildToolset.GetClusterWorkflow,
		})
}

func (t *Toolsets) RegisterPEGetWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflow_schema", "workflow",
		"Get the parameter schema for a workflow. Use this to inspect what parameters a workflow accepts "+
			"before configuring a component's workflow field or triggering a workflow run. Use scope=\"cluster\" "+
			"for a platform-wide ClusterWorkflow.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		"name", "Name of the workflow. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetWorkflowSchema,
			cluster:   t.PEToolset.GetClusterWorkflowSchema,
		})
}

func (t *Toolsets) RegisterGetWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflow_schema", "workflow",
		"Get the parameter schema for a workflow. Use this to inspect what parameters a workflow accepts "+
			"before configuring a component's workflow field or triggering a workflow run. Use scope=\"cluster\" "+
			"for a platform-wide ClusterWorkflow.",
		authzcore.ActionViewWorkflow, authzcore.ActionViewClusterWorkflow,
		"name", "Name of the workflow. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.BuildToolset.GetWorkflowSchema,
			cluster:   t.BuildToolset.GetClusterWorkflowSchema,
		})
}

func (t *Toolsets) RegisterGetWorkflowCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_workflow_creation_schema", "workflow",
		"Get the spec schema for creating a workflow (runTemplate, parameters definition, repository defaults, "+
			"etc.). Use scope=\"namespace\" (default) for a namespace-scoped Workflow or scope=\"cluster\" for a "+
			"platform-wide ClusterWorkflow. Call this before create_workflow to understand the spec structure.",
		authzcore.ActionCreateWorkflow, authzcore.ActionCreateClusterWorkflow,
		scopedSchemaProviders{
			namespace: func() (any, error) { return WorkflowCreationSchema() },
			cluster:   func() (any, error) { return ClusterWorkflowCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterPECreateWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_workflow", "workflow",
		"Create a workflow. With scope=\"namespace\" (default) it is created in namespace_name; with "+
			"scope=\"cluster\" it is a platform-wide ClusterWorkflow available to all namespaces. Workflows are "+
			"reusable CI/CD pipeline templates that execute on the workflow plane.",
		authzcore.ActionCreateWorkflow, authzcore.ActionCreateClusterWorkflow,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Workflow specification. Required field: runTemplate (Argo Workflow template definition). "+
			"Use get_workflow_creation_schema (with the matching scope) to check the schema.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.WorkflowSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateWorkflow(ctx, ns, &gen.CreateWorkflowJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterWorkflowSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterWorkflow(ctx, &gen.CreateClusterWorkflowJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterPEUpdateWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_workflow", "workflow",
		"Update an existing workflow (full replacement). Use scope=\"cluster\" for a platform-wide "+
			"ClusterWorkflow. Use get_workflow to retrieve the current spec first.",
		authzcore.ActionUpdateWorkflow, authzcore.ActionUpdateClusterWorkflow,
		"Name of the workflow to update. Use list_workflows to discover valid names",
		"Full workflow spec to replace the existing one. Use get_workflow to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.WorkflowSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateWorkflow(ctx, ns, &gen.UpdateWorkflowJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterWorkflowSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterWorkflow(ctx, &gen.UpdateClusterWorkflowJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterPEDeleteWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_workflow", "workflow",
		"Delete a workflow. Use scope=\"cluster\" for a platform-wide ClusterWorkflow.",
		authzcore.ActionDeleteWorkflow, authzcore.ActionDeleteClusterWorkflow,
		"name", "Name of the workflow to delete. Use list_workflows to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteWorkflow,
			cluster:   t.PEToolset.DeleteClusterWorkflow,
		})
}

// ---------------------------------------------------------------------------
// Deprecated cluster-prefixed aliases — workflows
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListClusterWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_workflows", "list_workflows",
		"Lists platform-wide cluster-scoped workflows. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterWorkflow, t.BuildToolset.ListClusterWorkflows)
}

func (t *Toolsets) RegisterPEListClusterWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_workflows", "list_workflows",
		"Lists platform-wide cluster-scoped workflows. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterWorkflow, t.PEToolset.ListClusterWorkflows)
}

func (t *Toolsets) RegisterGetClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflow", "get_workflow",
		"Gets the full definition of a platform-wide cluster-scoped workflow.",
		authzcore.ActionViewClusterWorkflow,
		"name", "Cluster workflow name. Use list_cluster_workflows to discover valid names",
		t.BuildToolset.GetClusterWorkflow)
}

func (t *Toolsets) RegisterPEGetClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflow", "get_workflow",
		"Gets the full definition of a platform-wide cluster-scoped workflow.",
		authzcore.ActionViewClusterWorkflow,
		"name", "Cluster workflow name. Use list_cluster_workflows to discover valid names",
		t.PEToolset.GetClusterWorkflow)
}

func (t *Toolsets) RegisterGetClusterWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflow_schema", "get_workflow_schema",
		"Gets the schema for a platform-wide cluster-scoped workflow.",
		authzcore.ActionViewClusterWorkflow,
		"name", "Cluster workflow name. Use list_cluster_workflows to discover valid names",
		t.BuildToolset.GetClusterWorkflowSchema)
}

func (t *Toolsets) RegisterPEGetClusterWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflow_schema", "get_workflow_schema",
		"Gets the schema for a platform-wide cluster-scoped workflow.",
		authzcore.ActionViewClusterWorkflow,
		"name", "Cluster workflow name. Use list_cluster_workflows to discover valid names",
		t.PEToolset.GetClusterWorkflowSchema)
}

func (t *Toolsets) RegisterGetClusterWorkflowCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterSchemaTool(s, perms,
		"get_cluster_workflow_creation_schema", "get_workflow_creation_schema",
		"Returns the spec schema for creating a platform-wide cluster-scoped workflow.",
		authzcore.ActionCreateClusterWorkflow,
		func() (any, error) { return ClusterWorkflowCreationSchema() })
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterWriteTool(s, perms, "create_cluster_workflow", "create_workflow",
		"Creates a platform-wide cluster-scoped workflow available to all namespaces.",
		authzcore.ActionCreateClusterWorkflow,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Cluster workflow specification. Required field: runTemplate (Argo Workflow template definition). "+
			"Use get_cluster_workflow_schema on an existing workflow to see the full structure.",
		func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
			spec, err := buildSpec[gen.ClusterWorkflowSpec](specRaw)
			if err != nil {
				return nil, err
			}
			return t.PEToolset.CreateClusterWorkflow(ctx, &gen.CreateClusterWorkflowJSONRequestBody{
				Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
			})
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterWriteTool(s, perms, "update_cluster_workflow", "update_workflow",
		"Updates a platform-wide cluster-scoped workflow (full replacement). "+
			"Use get_cluster_workflow to retrieve the current definition first.",
		authzcore.ActionUpdateClusterWorkflow,
		"Name of the cluster workflow to update. Use list_cluster_workflows to discover valid names",
		"Full cluster workflow spec to replace the existing one. "+
			"Use get_cluster_workflow to retrieve the current spec first.",
		func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
			spec, err := buildSpec[gen.ClusterWorkflowSpec](specRaw)
			if err != nil {
				return nil, err
			}
			return t.PEToolset.UpdateClusterWorkflow(ctx, &gen.UpdateClusterWorkflowJSONRequestBody{
				Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
			})
		})
}

func (t *Toolsets) RegisterDeleteClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "delete_cluster_workflow", "delete_workflow",
		"Deletes a platform-wide cluster-scoped workflow.",
		authzcore.ActionDeleteClusterWorkflow,
		"name", "Name of the cluster workflow to delete. Use list_cluster_workflows to discover valid names",
		t.PEToolset.DeleteClusterWorkflow)
}
