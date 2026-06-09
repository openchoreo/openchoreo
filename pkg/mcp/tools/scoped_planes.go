// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// ---------------------------------------------------------------------------
// Plane resources (data plane / workflow plane / observability plane)
// — scope-collapsed canonical tools
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_dataplanes", "data plane",
		"List data planes. With scope=\"namespace\" (default) lists a namespace's data planes (requires "+
			"namespace_name); with scope=\"cluster\" lists cluster-scoped data planes shared by platform admins. "+
			"Data planes are the Kubernetes clusters or cluster regions where component workloads execute. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewDataPlane, authzcore.ActionViewClusterDataPlane,
		scopedListHandlers{
			namespace: t.PEToolset.ListDataPlanes,
			cluster:   t.PEToolset.ListClusterDataPlanes,
		})
}

func (t *Toolsets) RegisterGetDataPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_dataplane", "data plane",
		"Get detailed information about a data plane including cluster details, capacity, health status, "+
			"associated environments, and network configuration. Use scope=\"cluster\" for a cluster-scoped data plane.",
		authzcore.ActionViewDataPlane, authzcore.ActionViewClusterDataPlane,
		"name", "Data plane name. Use list_dataplanes to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetDataPlane,
			cluster:   t.PEToolset.GetClusterDataPlane,
		})
}

func (t *Toolsets) RegisterListWorkflowPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_workflowplanes", "workflow plane",
		"List workflow planes. With scope=\"namespace\" (default) lists a namespace's workflow planes (requires "+
			"namespace_name); with scope=\"cluster\" lists cluster-scoped workflow planes shared by platform admins. "+
			"Workflow planes handle continuous integration and container image building. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewWorkflowPlane, authzcore.ActionViewClusterWorkflowPlane,
		scopedListHandlers{
			namespace: t.PEToolset.ListWorkflowPlanes,
			cluster:   t.PEToolset.ListClusterWorkflowPlanes,
		})
}

func (t *Toolsets) RegisterGetWorkflowPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_workflowplane", "workflow plane",
		"Get detailed information about a workflow plane including cluster details, health status, and agent "+
			"connection state. Use scope=\"cluster\" for a cluster-scoped workflow plane.",
		authzcore.ActionViewWorkflowPlane, authzcore.ActionViewClusterWorkflowPlane,
		"name", "Workflow plane name. Use list_workflowplanes to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetWorkflowPlane,
			cluster:   t.PEToolset.GetClusterWorkflowPlane,
		})
}

func (t *Toolsets) RegisterListObservabilityPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_observability_planes", "observability plane",
		"List observability planes. With scope=\"namespace\" (default) lists a namespace's observability planes "+
			"(requires namespace_name); with scope=\"cluster\" lists cluster-scoped observability planes shared by "+
			"platform admins. Observability planes provide monitoring, logging, tracing, and metrics collection for "+
			"deployed components. Supports pagination via limit and cursor.",
		authzcore.ActionViewObservabilityPlane, authzcore.ActionViewClusterObservabilityPlane,
		scopedListHandlers{
			namespace: t.PEToolset.ListObservabilityPlanes,
			cluster:   t.PEToolset.ListClusterObservabilityPlanes,
		})
}

func (t *Toolsets) RegisterGetObservabilityPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_observability_plane", "observability plane",
		"Get detailed information about an observability plane including observer URL, health status, and agent "+
			"connection state. Use scope=\"cluster\" for a cluster-scoped observability plane.",
		authzcore.ActionViewObservabilityPlane, authzcore.ActionViewClusterObservabilityPlane,
		"name", "Observability plane name. Use list_observability_planes to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetObservabilityPlane,
			cluster:   t.PEToolset.GetClusterObservabilityPlane,
		})
}

// ---------------------------------------------------------------------------
// Deprecated cluster-prefixed aliases — plane resources
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListClusterDataPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_dataplanes", "list_dataplanes",
		"Lists cluster-scoped data planes shared by platform admins. Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterDataPlane, t.PEToolset.ListClusterDataPlanes)
}

func (t *Toolsets) RegisterGetClusterDataPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_dataplane", "get_dataplane",
		"Gets detailed information about a cluster-scoped data plane including cluster details, "+
			"capacity, health status, and network configuration.",
		authzcore.ActionViewClusterDataPlane,
		"name", "Cluster data plane name. Use list_cluster_dataplanes to discover valid names",
		t.PEToolset.GetClusterDataPlane)
}

func (t *Toolsets) RegisterListClusterWorkflowPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_workflowplanes", "list_workflowplanes",
		"Lists cluster-scoped workflow planes shared by platform admins. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterWorkflowPlane, t.PEToolset.ListClusterWorkflowPlanes)
}

func (t *Toolsets) RegisterGetClusterWorkflowPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_workflowplane", "get_workflowplane",
		"Gets detailed information about a cluster-scoped workflow plane including cluster details, "+
			"health status, and agent connection state.",
		authzcore.ActionViewClusterWorkflowPlane,
		"name", "Cluster workflow plane name. Use list_cluster_workflowplanes to discover valid names",
		t.PEToolset.GetClusterWorkflowPlane)
}

func (t *Toolsets) RegisterListClusterObservabilityPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterListTool(s, perms, "list_cluster_observability_planes", "list_observability_planes",
		"Lists cluster-scoped observability planes shared by platform admins. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewClusterObservabilityPlane, t.PEToolset.ListClusterObservabilityPlanes)
}

func (t *Toolsets) RegisterGetClusterObservabilityPlane(s *mcp.Server, perms map[string]ToolPermission) {
	registerDeprecatedClusterResourceTool(s, perms, "get_cluster_observability_plane", "get_observability_plane",
		"Gets detailed information about a cluster-scoped observability plane including observer URL, "+
			"health status, and agent connection state.",
		authzcore.ActionViewClusterObservabilityPlane,
		"name", "Cluster observability plane name. Use list_cluster_observability_planes to discover valid names",
		t.PEToolset.GetClusterObservabilityPlane)
}
