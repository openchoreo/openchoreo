// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

type promotionPathInputTarget struct {
	Name string `json:"name"`
}

type promotionPathInput struct {
	SourceEnvironmentRef  string                     `json:"source_environment_ref"`
	TargetEnvironmentRefs []promotionPathInputTarget `json:"target_environment_refs"`
}

func buildAnnotations(displayName, description string) map[string]string {
	annotations := map[string]string{}
	if displayName != "" {
		annotations["openchoreo.dev/display-name"] = displayName
	}
	if description != "" {
		annotations["openchoreo.dev/description"] = description
	}
	return annotations
}

func buildDeploymentPipelineSpecFromInput(promotionPaths []promotionPathInput) *gen.DeploymentPipelineSpec {
	if len(promotionPaths) == 0 {
		return nil
	}

	paths := make([]gen.PromotionPath, 0, len(promotionPaths))
	for _, p := range promotionPaths {
		targets := make([]gen.TargetEnvironmentRef, 0, len(p.TargetEnvironmentRefs))
		for _, t := range p.TargetEnvironmentRefs {
			targets = append(targets, gen.TargetEnvironmentRef{
				Name: t.Name,
			})
		}
		kind := gen.PromotionPathSourceEnvironmentRefKindEnvironment
		paths = append(paths, gen.PromotionPath{
			SourceEnvironmentRef: struct {
				Kind *gen.PromotionPathSourceEnvironmentRefKind `json:"kind,omitempty"`
				Name string                                     `json:"name"`
			}{
				Kind: &kind,
				Name: p.SourceEnvironmentRef,
			},
			TargetEnvironmentRefs: targets,
		})
	}

	return &gen.DeploymentPipelineSpec{
		PromotionPaths: &paths,
	}
}

func buildSpec[T any](specInput map[string]interface{}) (*T, error) {
	specJSON, err := json.Marshal(specInput)
	if err != nil {
		return nil, err
	}

	var spec T
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

// ---------------------------------------------------------------------------
// PE Toolset — Environment management
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListEnvironments(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_environments"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEnvironment}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all environments in a namespace. Environments are deployment targets representing " +
			"pipeline stages (dev, staging, production) or isolated tenants. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListEnvironments(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateEnvironment(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_environment"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateEnvironment}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new environment in a namespace. Environments are deployment targets representing " +
			"pipeline stages (dev, qa, prod) or isolated tenants.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"data_plane_ref": stringProperty("Associated data plane reference name." +
				" Use list_dataplanes to discover namespace-scoped names," +
				" or list_cluster_dataplanes to discover cluster-scoped names"),
			"data_plane_ref_kind": map[string]any{
				"type": "string",
				"enum": []string{
					string(gen.EnvironmentSpecDataPlaneRefKindDataPlane),
					string(gen.EnvironmentSpecDataPlaneRefKindClusterDataPlane),
				},
				"description": "Kind of the data plane reference." +
					" Use 'DataPlane' for namespace-scoped (default) or 'ClusterDataPlane' for cluster-scoped",
			},
			"is_production": map[string]any{
				"type":        "boolean",
				"description": "Whether this is a production environment",
			},
		}, []string{"namespace_name", "name", "data_plane_ref"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName    string `json:"namespace_name"`
		Name             string `json:"name"`
		DisplayName      string `json:"display_name"`
		Description      string `json:"description"`
		DataPlaneRef     string `json:"data_plane_ref"`
		DataPlaneRefKind string `json:"data_plane_ref_kind"`
		IsProduction     bool   `json:"is_production"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}

		envReq := &gen.CreateEnvironmentJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: &gen.EnvironmentSpec{
				IsProduction: &args.IsProduction,
			},
		}
		if args.DataPlaneRef != "" {
			kind := gen.EnvironmentSpecDataPlaneRefKindDataPlane
			if args.DataPlaneRefKind == string(gen.EnvironmentSpecDataPlaneRefKindClusterDataPlane) {
				kind = gen.EnvironmentSpecDataPlaneRefKindClusterDataPlane
			}
			envReq.Spec.DataPlaneRef = &struct {
				Kind gen.EnvironmentSpecDataPlaneRefKind `json:"kind"`
				Name string                              `json:"name"`
			}{
				Kind: kind,
				Name: args.DataPlaneRef,
			}
		}
		result, err := t.PEToolset.CreateEnvironment(ctx, args.NamespaceName, envReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateEnvironment(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_environment"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateEnvironment}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing environment in a namespace. Allows modifying display name, description, " +
			"and production flag. Data plane reference is immutable after creation.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Name of the environment to update. Use list_environments to discover valid names"),
			"display_name":   stringProperty("Updated human-readable display name"),
			"description":    stringProperty("Updated human-readable description"),
			"is_production": map[string]any{
				"type":        "boolean",
				"description": "Whether this is a production environment",
			},
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
		DisplayName   string `json:"display_name"`
		Description   string `json:"description"`
		IsProduction  *bool  `json:"is_production"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}

		envReq := &gen.UpdateEnvironmentJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: &gen.EnvironmentSpec{
				IsProduction: args.IsProduction,
			},
		}
		result, err := t.PEToolset.UpdateEnvironment(ctx, args.NamespaceName, envReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteEnvironment(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_environment"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteEnvironment}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete an environment from a namespace. " +
			"This will remove the deployment target and any associated resources.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"env_name":       stringProperty("Name of the environment to delete. Use list_environments to discover valid names"),
		}, []string{"namespace_name", "env_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		EnvName       string `json:"env_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteEnvironment(ctx, args.NamespaceName, args.EnvName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Deployment pipeline management
// ---------------------------------------------------------------------------

// promotionPathsSchema returns the JSON schema for a promotion_paths array field.
// The description parameter allows callers to customize the field description.
func promotionPathsSchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type":     "object",
			"required": []string{"source_environment_ref", "target_environment_refs"},
			"properties": map[string]any{
				"source_environment_ref": stringProperty("Source environment name"),
				"target_environment_refs": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"name"},
						"properties": map[string]any{
							"name": stringProperty("Target environment name"),
						},
					},
				},
			},
		},
	}
}

//nolint:dupl
func (t *Toolsets) RegisterCreateDeploymentPipeline(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_deployment_pipeline"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateDeploymentPipeline}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new deployment pipeline in a namespace. Deployment pipelines define the promotion " +
			"order between environments (e.g., dev → staging → production) with optional approval gates.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"promotion_paths": promotionPathsSchema(
				"Promotion paths defining environment progression. " +
					"Each path has a source environment and target environments with optional approval requirements",
			),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName  string               `json:"namespace_name"`
		Name           string               `json:"name"`
		DisplayName    string               `json:"display_name"`
		Description    string               `json:"description"`
		PromotionPaths []promotionPathInput `json:"promotion_paths"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)

		dpReq := &gen.CreateDeploymentPipelineJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
		}

		if spec := buildDeploymentPipelineSpecFromInput(args.PromotionPaths); spec != nil {
			dpReq.Spec = spec
		}

		result, err := t.PEToolset.CreateDeploymentPipeline(ctx, args.NamespaceName, dpReq)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Component releases
// ---------------------------------------------------------------------------

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterPEListComponentReleases(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_component_releases"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponentRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all releases for a component. Releases are immutable snapshots of a component at a " +
			"specific build, ready for deployment to environments. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": defaultStringProperty(),
		}), []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListComponentReleases(
			ctx, args.NamespaceName, args.ComponentName,
			ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPECreateComponentRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_component_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateComponentRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new release from the latest build of a component. Releases are immutable " +
			"snapshots that can be deployed to environments through release bindings. The component " +
			"must have at least one successful build or workload. If the source repository does not " +
			"contain a workload descriptor (workload.yaml), use update_workload to configure the " +
			"workload before creating a release.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": defaultStringProperty(),
			"release_name":   stringProperty("Optional release name. If omitted, a name will be auto-generated"),
		}, []string{"namespace_name", "component_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.CreateComponentRelease(
			ctx, args.NamespaceName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetComponentRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_component_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponentRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a specific component release including build information, " +
			"image tags, and deployment status.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"release_name":   stringProperty("Use list_component_releases to discover valid names"),
		}, []string{"namespace_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetComponentRelease(ctx, args.NamespaceName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetComponentReleaseSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_component_release_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponentRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the release schema for a component. Returns the JSON schema showing the configuration " +
			"options available when creating or deploying releases for this component.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"component_name": stringProperty("Use list_components to discover valid names"),
			"release_name":   stringProperty("Use list_component_releases to discover valid names"),
		}, []string{"namespace_name", "component_name", "release_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ComponentName string `json:"component_name"`
		ReleaseName   string `json:"release_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetComponentReleaseSchema(
			ctx, args.NamespaceName, args.ComponentName, args.ReleaseName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — DataPlane read
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_dataplanes"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewDataPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all data planes in a namespace. Data planes are Kubernetes clusters or cluster " +
			"regions where component workloads actually execute. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListDataPlanes(ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetDataPlane(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_dataplane"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewDataPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a data plane including cluster details, capacity, health " +
			"status, associated environments, and network configuration.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"dp_name":        stringProperty("Use list_dataplanes to discover valid names"),
		}, []string{"namespace_name", "dp_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		DpName        string `json:"dp_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetDataPlane(ctx, args.NamespaceName, args.DpName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — WorkflowPlane read
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListWorkflowPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_workflowplanes"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflowPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all workflow planes in a namespace. Workflow planes are infrastructure that handles " +
			"continuous integration and container image building. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListWorkflowPlanes(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkflowPlane(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workflowplane"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflowPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a workflow plane including cluster details, health status, " +
			"and agent connection state.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"wp_name":        stringProperty("Use list_workflowplanes to discover valid names"),
		}, []string{"namespace_name", "wp_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		WpName        string `json:"wp_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetWorkflowPlane(ctx, args.NamespaceName, args.WpName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — ObservabilityPlane read
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListObservabilityPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_observability_planes"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewObservabilityPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all ObservabilityPlanes in a namespace. ObservabilityPlanes provide monitoring, " +
			"logging, tracing, and metrics collection capabilities for deployed components. " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListObservabilityPlanes(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetObservabilityPlane(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_observability_plane"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewObservabilityPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about an observability plane including observer URL, " +
			"health status, and agent connection state.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"op_name":        stringProperty("Use list_observability_planes to discover valid names"),
		}, []string{"namespace_name", "op_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		OpName        string `json:"op_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetObservabilityPlane(ctx, args.NamespaceName, args.OpName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Cluster-scoped plane read
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListClusterDataPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_cluster_dataplanes"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterDataPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all cluster-scoped data planes. These are shared infrastructure managed by " +
			"platform admins, not scoped to any namespace. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListClusterDataPlanes(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetClusterDataPlane(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_dataplane"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterDataPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a cluster-scoped data plane including cluster details, " +
			"capacity, health status, and network configuration.",
		InputSchema: createSchema(map[string]any{
			"cdp_name": stringProperty("Use list_cluster_dataplanes to discover valid names"),
		}, []string{"cdp_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CdpName string `json:"cdp_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetClusterDataPlane(ctx, args.CdpName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListClusterWorkflowPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_cluster_workflowplanes"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterWorkflowPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all cluster-scoped workflow planes. These are shared workflow infrastructure managed by " +
			"platform admins, not scoped to any namespace. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListClusterWorkflowPlanes(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListClusterObservabilityPlanes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_cluster_observability_planes"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterObservabilityPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all cluster-scoped observability planes. These are shared observability infrastructure " +
			"managed by platform admins, not scoped to any namespace. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListClusterObservabilityPlanes(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Platform standards read (dual with Component toolset)
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_component_types"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all available component types in a namespace. Component types define the " +
			"structure and capabilities of components (e.g., WebApplication, Service, ScheduledTask). " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListComponentTypes(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetComponentTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_component_type_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the parameter schema for a component type. Returns the JSON schema showing " +
			"the parameters developers can configure when using this component type.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"ct_name":        stringProperty("Component type name. Use list_component_types to discover valid names"),
		}, []string{"namespace_name", "ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		CtName        string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetComponentTypeSchema(ctx, args.NamespaceName, args.CtName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a component type including its complete spec. " +
			"Use this before updating a component type to retrieve the current spec.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"ct_name":        stringProperty("Component type name. Use list_component_types to discover valid names"),
		}, []string{"namespace_name", "ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		CtName        string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetComponentType(ctx, args.NamespaceName, args.CtName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEListTraits(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_traits"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all available traits in a namespace. Traits add capabilities to components " +
			"(e.g., autoscaling, ingress, service mesh). Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListTraits(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetTraitSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_trait_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the parameter schema for a trait. Returns the JSON schema showing the " +
			"parameters developers can configure when using this trait.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"trait_name":     stringProperty("Trait name. Use list_traits to discover valid names"),
		}, []string{"namespace_name", "trait_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		TraitName     string `json:"trait_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetTraitSchema(ctx, args.NamespaceName, args.TraitName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a trait including its complete spec. " +
			"Use this before updating a trait to retrieve the current spec.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"trait_name":     stringProperty("Trait name. Use list_traits to discover valid names"),
		}, []string{"namespace_name", "trait_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		TraitName     string `json:"trait_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetTrait(ctx, args.NamespaceName, args.TraitName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEListWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_workflows"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all workflows in a namespace. Workflows are reusable templates that define " +
			"automated processes such as CI/CD pipelines executed on the workflow plane. " +
			"Use this to discover available workflow names for use with create_component or create_workflow_run. " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListWorkflows(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workflow_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the parameter schema for a specific workflow. Use this to inspect what parameters " +
			"a workflow accepts before configuring a component's workflow field or triggering a workflow run.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workflow_name":  stringProperty("Name of the workflow. Use list_workflows to discover valid names"),
		}, []string{"namespace_name", "workflow_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		WorkflowName  string `json:"workflow_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetWorkflowSchema(ctx, args.NamespaceName, args.WorkflowName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workflow"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a workflow including its complete spec. " +
			"Use this before updating a workflow to retrieve the current spec.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workflow_name":  stringProperty("Name of the workflow. Use list_workflows to discover valid names"),
		}, []string{"namespace_name", "workflow_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		WorkflowName  string `json:"workflow_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetWorkflow(ctx, args.NamespaceName, args.WorkflowName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Diagnostics
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterGetResourceEvents(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_resource_events"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get Kubernetes events for a specific resource in a deployment. Useful for diagnosing " +
			"deployment issues, scheduling problems, or container startup failures.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"release_binding_name": stringProperty(
				"Name of the release binding (deployment). Use list_release_bindings to discover valid names"),
			"group":         stringProperty("API group of the resource (e.g., 'apps', '' for core resources)"),
			"version":       stringProperty("API version of the resource (e.g., 'v1')"),
			"kind":          stringProperty("Kind of the resource (e.g., 'Deployment', 'Pod', 'Service')"),
			"resource_name": stringProperty("Name of the specific Kubernetes resource"),
		}, []string{"namespace_name", "release_binding_name", "group", "version", "kind", "resource_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName      string `json:"namespace_name"`
		ReleaseBindingName string `json:"release_binding_name"`
		Group              string `json:"group"`
		Version            string `json:"version"`
		Kind               string `json:"kind"`
		ResourceName       string `json:"resource_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetResourceEvents(
			ctx, args.NamespaceName, args.ReleaseBindingName,
			args.Group, args.Version, args.Kind, args.ResourceName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetResourceLogs(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_resource_logs"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewLogs}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get container logs from a specific pod in a deployment. Useful for debugging " +
			"application errors, startup failures, and runtime issues.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"release_binding_name": stringProperty(
				"Name of the release binding (deployment). Use list_release_bindings to discover valid names"),
			"pod_name": stringProperty("Name of the pod. Use get_resource_events to discover pod names"),
			"since_seconds": map[string]any{
				"type":        "integer",
				"description": "Return logs from the last N seconds. Defaults to all available logs",
			},
		}, []string{"namespace_name", "release_binding_name", "pod_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName      string `json:"namespace_name"`
		ReleaseBindingName string `json:"release_binding_name"`
		PodName            string `json:"pod_name"`
		SinceSeconds       *int64 `json:"since_seconds"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetResourceLogs(
			ctx, args.NamespaceName, args.ReleaseBindingName, args.PodName, args.SinceSeconds)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// Deployment Toolset — Deployment pipelines (developer-facing read)
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterGetDeploymentPipeline(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_deployment_pipeline"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewDeploymentPipeline}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a deployment pipeline including its stages, promotion " +
			"order, and associated environments.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"pipeline_name":  stringProperty("Use list_deployment_pipelines to discover valid names"),
		}, []string{"namespace_name", "pipeline_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		PipelineName  string `json:"pipeline_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetDeploymentPipeline(ctx, args.NamespaceName, args.PipelineName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListDeploymentPipelines(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_deployment_pipelines"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewDeploymentPipeline}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all deployment pipelines in a namespace. Deployment pipelines define the promotion " +
			"order between environments (e.g., dev → staging → production). Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.ListDeploymentPipelines(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// Deployment Toolset — Environments (developer-facing read)
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListEnvironments(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_environments"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEnvironment}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all environments in a namespace. Environments are deployment targets representing " +
			"pipeline stages (dev, staging, production) or isolated tenants. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.ListEnvironments(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// Build Toolset — WorkflowRun operations
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterCreateWorkflowRun(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_workflow_run"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateWorkflowRun}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new workflow run by specifying a workflow name and optional parameters. " +
			"Workflows define automated processes like CI/CD pipelines that execute on the workflow plane.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workflow_name":  stringProperty("Name of the Workflow CR to execute"),
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: Parameters to pass to the workflow execution",
			},
		}, []string{"namespace_name", "workflow_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		WorkflowName  string                 `json:"workflow_name"`
		Parameters    map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.CreateWorkflowRun(ctx, args.NamespaceName, args.WorkflowName, args.Parameters)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListWorkflowRuns(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_workflow_runs"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflowRun}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List workflow runs in a namespace, optionally filtered by project and component. " +
			"Shows execution history including status, timestamps, and workflow references. " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   stringProperty("Optional: filter by project name"),
			"component_name": stringProperty("Optional: filter by component name"),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		ComponentName string `json:"component_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.ListWorkflowRuns(
			ctx, args.NamespaceName, args.ProjectName, args.ComponentName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkflowRun(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workflow_run"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflowRun}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a specific workflow run including its status, tasks, " +
			"timestamps, and referenced resources.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"run_name":       stringProperty("Use list_workflow_runs to discover valid names"),
		}, []string{"namespace_name", "run_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		RunName       string `json:"run_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.GetWorkflowRun(ctx, args.NamespaceName, args.RunName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// Build Toolset — Workflow read
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_workflows"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all workflows in a namespace. Workflows are reusable templates that define " +
			"automated processes such as CI/CD pipelines executed on the workflow plane. " +
			"Use this to discover available workflow names for use with create_component or create_workflow_run. " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.ListWorkflows(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workflow_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the parameter schema for a specific workflow. Use this to inspect what parameters " +
			"a workflow accepts before configuring a component's workflow field or triggering a workflow run.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workflow_name":  stringProperty("Name of the workflow. Use list_workflows to discover valid names"),
		}, []string{"namespace_name", "workflow_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		WorkflowName  string `json:"workflow_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.GetWorkflowSchema(ctx, args.NamespaceName, args.WorkflowName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// Build Toolset — Cluster-scoped workflows
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListClusterWorkflows(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_cluster_workflows"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all cluster-scoped workflows. These are shared workflow definitions managed by platform " +
			"admins that can be referenced by components across all namespaces via ClusterComponentType. " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.ListClusterWorkflows(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_workflow"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a cluster-scoped workflow including its complete spec. " +
			"Use this before updating a cluster workflow to retrieve the current spec.",
		InputSchema: createSchema(map[string]any{
			"cwf_name": stringProperty("Cluster workflow name. Use list_cluster_workflows to discover valid names"),
		}, []string{"cwf_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CwfName string `json:"cwf_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.GetClusterWorkflow(ctx, args.CwfName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetClusterWorkflowSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_workflow_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the schema definition for a cluster-scoped workflow. Returns the JSON schema " +
			"showing workflow configuration options and parameters.",
		InputSchema: createSchema(map[string]any{
			"cwf_name": stringProperty("Cluster workflow name. Use list_cluster_workflows to discover valid names"),
		}, []string{"cwf_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CwfName string `json:"cwf_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.GetClusterWorkflowSchema(ctx, args.CwfName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Cluster-scoped platform standards
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListClusterComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_cluster_component_types"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all cluster-scoped component types. These are shared component type templates managed " +
			"by platform admins that define the structure and capabilities of components across all namespaces. " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListClusterComponentTypes(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a cluster-scoped component type including its complete spec. " +
			"Use this before updating a cluster component type to retrieve the current spec.",
		InputSchema: createSchema(map[string]any{
			"cct_name": stringProperty("Cluster component type name. Use list_cluster_component_types to discover valid names"),
		}, []string{"cct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CctName string `json:"cct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetClusterComponentType(ctx, args.CctName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetClusterComponentTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_component_type_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the schema definition for a cluster-scoped component type. Returns the JSON schema " +
			"showing required fields, optional fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"cct_name": stringProperty("Cluster component type name. Use list_cluster_component_types to discover valid names"),
		}, []string{"cct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CctName string `json:"cct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetClusterComponentTypeSchema(ctx, args.CctName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEListClusterTraits(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_cluster_traits"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all cluster-scoped traits. These are shared trait definitions managed by platform " +
			"admins that add capabilities to components across all namespaces (e.g., autoscaling, ingress). " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListClusterTraits(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of a cluster-scoped trait including its complete spec. " +
			"Use this before updating a cluster trait to retrieve the current spec.",
		InputSchema: createSchema(map[string]any{
			"ct_name": stringProperty("Cluster trait name. Use list_cluster_traits to discover valid names"),
		}, []string{"ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CtName string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetClusterTrait(ctx, args.CtName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetClusterTraitSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_trait_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the schema definition for a cluster-scoped trait. Returns the JSON schema " +
			"showing trait configuration options and parameters.",
		InputSchema: createSchema(map[string]any{
			"ct_name": stringProperty("Cluster trait name. Use list_cluster_traits to discover valid names"),
		}, []string{"ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CtName string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetClusterTraitSchema(ctx, args.CtName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// Component Toolset — Platform standards (read-only, developer-facing)
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_component_types"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all available component types in a namespace. Component types define the " +
			"structure and capabilities of components (e.g., WebApplication, Service, ScheduledTask). " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListComponentTypes(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_component_type_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the schema definition for a component type. Returns the JSON schema showing " +
			"required fields, optional fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"ct_name":        stringProperty("Component type name. Use list_component_types to discover valid names"),
		}, []string{"namespace_name", "ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		CtName        string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetComponentTypeSchema(ctx, args.NamespaceName, args.CtName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListTraits(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_traits"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all available traits in a namespace. Traits add capabilities to components " +
			"(e.g., autoscaling, ingress, service mesh). Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListTraits(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetTraitSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_trait_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the schema definition for a trait. Returns the JSON schema showing trait " +
			"configuration options and parameters.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"trait_name":     stringProperty("Trait name. Use list_traits to discover valid names"),
		}, []string{"namespace_name", "trait_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		TraitName     string `json:"trait_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetTraitSchema(ctx, args.NamespaceName, args.TraitName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListClusterComponentTypes(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_cluster_component_types"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all cluster-scoped component types. These are shared component type templates managed " +
			"by platform admins that define the structure and capabilities of components across all namespaces. " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListClusterComponentTypes(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a cluster-scoped component type including workload type, " +
			"allowed workflows, allowed traits, and description.",
		InputSchema: createSchema(map[string]any{
			"cct_name": stringProperty("Cluster component type name. Use list_cluster_component_types to discover valid names"),
		}, []string{"cct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CctName string `json:"cct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetClusterComponentType(ctx, args.CctName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetClusterComponentTypeSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_component_type_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the schema definition for a cluster-scoped component type. Returns the JSON schema " +
			"showing required fields, optional fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"cct_name": stringProperty("Cluster component type name. Use list_cluster_component_types to discover valid names"),
		}, []string{"cct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CctName string `json:"cct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetClusterComponentTypeSchema(ctx, args.CctName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListClusterTraits(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_cluster_traits"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all cluster-scoped traits. These are shared trait definitions managed by platform " +
			"admins that add capabilities to components across all namespaces (e.g., autoscaling, ingress). " +
			"Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.ListClusterTraits(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a cluster-scoped trait including its name, " +
			"display name, and description.",
		InputSchema: createSchema(map[string]any{
			"ct_name": stringProperty("Cluster trait name. Use list_cluster_traits to discover valid names"),
		}, []string{"ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CtName string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetClusterTrait(ctx, args.CtName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetClusterTraitSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_trait_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the schema definition for a cluster-scoped trait. Returns the JSON schema " +
			"showing trait configuration options and parameters.",
		InputSchema: createSchema(map[string]any{
			"ct_name": stringProperty("Cluster trait name. Use list_cluster_traits to discover valid names"),
		}, []string{"ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CtName string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.ComponentToolset.GetClusterTraitSchema(ctx, args.CtName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Deployment pipeline write operations
// ---------------------------------------------------------------------------

//nolint:dupl
func (t *Toolsets) RegisterUpdateDeploymentPipeline(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_deployment_pipeline"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateDeploymentPipeline}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing deployment pipeline in a namespace. Allows modifying promotion paths " +
			"between environments and approval requirements. Use list_environments to discover valid environment names.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty(
				"Name of the deployment pipeline to update. Use list_deployment_pipelines to discover valid names"),
			"display_name": stringProperty("Updated human-readable display name"),
			"description":  stringProperty("Updated human-readable description"),
			"promotion_paths": promotionPathsSchema(
				"Updated promotion paths defining environment progression. " +
					"Each path has a source environment and target environments with optional approval requirements",
			),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName  string               `json:"namespace_name"`
		Name           string               `json:"name"`
		DisplayName    string               `json:"display_name"`
		Description    string               `json:"description"`
		PromotionPaths []promotionPathInput `json:"promotion_paths"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		dpReq := &gen.UpdateDeploymentPipelineJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
		}
		if spec := buildDeploymentPipelineSpecFromInput(args.PromotionPaths); spec != nil {
			dpReq.Spec = spec
		}
		result, err := t.PEToolset.UpdateDeploymentPipeline(ctx, args.NamespaceName, dpReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteDeploymentPipeline(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_deployment_pipeline"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteDeploymentPipeline}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Delete a deployment pipeline from a namespace.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"pipeline_name": stringProperty(
				"Name of the deployment pipeline to delete. Use list_deployment_pipelines to discover valid names"),
		}, []string{"namespace_name", "pipeline_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		PipelineName  string `json:"pipeline_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteDeploymentPipeline(ctx, args.NamespaceName, args.PipelineName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Cluster-scoped plane read (Get additions)
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterGetClusterWorkflowPlane(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_workflowplane"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterWorkflowPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a cluster-scoped workflow plane including cluster details, " +
			"health status, and agent connection state.",
		InputSchema: createSchema(map[string]any{
			"cwp_name": stringProperty("Cluster workflow plane name. Use list_cluster_workflowplanes to discover valid names"),
		}, []string{"cwp_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CwpName string `json:"cwp_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetClusterWorkflowPlane(ctx, args.CwpName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetClusterObservabilityPlane(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_observability_plane"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewClusterObservabilityPlane}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get detailed information about a cluster-scoped observability plane including " +
			"observer URL, health status, and agent connection state.",
		InputSchema: createSchema(map[string]any{
			"cop_name": stringProperty(
				"Cluster observability plane name. Use list_cluster_observability_planes to discover valid names"),
		}, []string{"cop_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CopName string `json:"cop_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetClusterObservabilityPlane(ctx, args.CopName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Component type creation schema tools
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterGetComponentTypeCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_component_type_creation_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the spec schema for creating a namespace-scoped component type. " +
			"Returns the full JSON schema showing all required and optional fields, their types, " +
			"and descriptions. Call this before create_component_type to understand the spec structure.",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		schema, err := ComponentTypeCreationSchema()
		return handleToolResult(schema, err)
	})
}

func (t *Toolsets) RegisterGetClusterComponentTypeCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_cluster_component_type_creation_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the spec schema for creating a cluster-scoped component type. " +
			"Returns the full JSON schema showing all required and optional fields, their types, " +
			"and descriptions. Call this before create_cluster_component_type to understand the spec structure.",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		schema, err := ClusterComponentTypeCreationSchema()
		return handleToolResult(schema, err)
	})
}

func (t *Toolsets) RegisterGetTraitCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_trait_creation_schema"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the spec schema for creating a namespace-scoped trait. " +
			"Returns the full JSON schema showing all required and optional fields, their types, " +
			"and descriptions. Call this before create_trait to understand the spec structure.",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		schema, err := TraitCreationSchema()
		return handleToolResult(schema, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Platform standards write (namespace-scoped)
// ---------------------------------------------------------------------------

//nolint:dupl // create/update component type handlers share similar structure
func (t *Toolsets) RegisterCreateComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new component type in a namespace. Component types define the structure, " +
			"workload type, allowed traits, and allowed workflows for components.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"spec": map[string]any{
				"type":        "object",
				"description": "Use get_component_type_creation_schema to check the schema",
			},
		}, []string{"namespace_name", "name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		Spec          map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		spec, err := buildSpec[gen.ComponentTypeSpec](args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		ctReq := &gen.CreateComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: spec,
		}
		result, err := t.PEToolset.CreateComponentType(ctx, args.NamespaceName, ctReq)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update component type handlers share similar structure
func (t *Toolsets) RegisterUpdateComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Update an existing component type in a namespace (full replacement).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty(
				"Name of the component type to update. Use list_component_types to discover valid names"),
			"display_name": stringProperty("Updated human-readable display name"),
			"description":  stringProperty("Updated human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Full component type spec to replace the existing one. " +
					"Use get_component_type to retrieve the current spec first.",
			},
		}, []string{"namespace_name", "name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		Spec          map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		spec, err := buildSpec[gen.ComponentTypeSpec](args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		ctReq := &gen.UpdateComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: spec,
		}
		result, err := t.PEToolset.UpdateComponentType(ctx, args.NamespaceName, ctReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Delete a component type from a namespace.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"ct_name": stringProperty(
				"Name of the component type to delete. Use list_component_types to discover valid names"),
		}, []string{"namespace_name", "ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		CtName        string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteComponentType(ctx, args.NamespaceName, args.CtName)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update trait handlers share similar structure
func (t *Toolsets) RegisterCreateTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new trait in a namespace. Traits add capabilities to components by creating " +
			"additional Kubernetes resources or patching existing ones (e.g., autoscaling, ingress, service mesh).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"spec": map[string]any{
				"type":        "object",
				"description": "Use get_trait_creation_schema to check the schema",
			},
		}, []string{"namespace_name", "name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		Spec          map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		spec, err := buildSpec[gen.TraitSpec](args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		traitReq := &gen.CreateTraitJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: spec,
		}
		result, err := t.PEToolset.CreateTrait(ctx, args.NamespaceName, traitReq)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update trait handlers share similar structure
func (t *Toolsets) RegisterUpdateTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Update an existing trait in a namespace (full replacement).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Name of the trait to update. Use list_traits to discover valid names"),
			"display_name":   stringProperty("Updated human-readable display name"),
			"description":    stringProperty("Updated human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Full trait spec to replace the existing one. " +
					"Use get_trait to retrieve the current spec first.",
			},
		}, []string{"namespace_name", "name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		Spec          map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		spec, err := buildSpec[gen.TraitSpec](args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		traitReq := &gen.UpdateTraitJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: spec,
		}
		result, err := t.PEToolset.UpdateTrait(ctx, args.NamespaceName, traitReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Delete a trait from a namespace.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"trait_name":     stringProperty("Name of the trait to delete. Use list_traits to discover valid names"),
		}, []string{"namespace_name", "trait_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		TraitName     string `json:"trait_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteTrait(ctx, args.NamespaceName, args.TraitName)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update workflow handlers share similar structure
func (t *Toolsets) RegisterPECreateWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_workflow"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new workflow in a namespace. Workflows are reusable CI/CD pipeline templates " +
			"that execute on the workflow plane.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Workflow specification. Required field: runTemplate (Argo Workflow template definition). " +
					"Use get_workflow_schema on an existing workflow to see the full structure.",
			},
		}, []string{"namespace_name", "name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		Spec          map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		spec, err := buildSpec[gen.WorkflowSpec](args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		wfReq := &gen.CreateWorkflowJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: spec,
		}
		result, err := t.PEToolset.CreateWorkflow(ctx, args.NamespaceName, wfReq)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update workflow handlers share similar structure
func (t *Toolsets) RegisterPEUpdateWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_workflow"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Update an existing workflow in a namespace (full replacement).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Name of the workflow to update. Use list_workflows to discover valid names"),
			"display_name":   stringProperty("Updated human-readable display name"),
			"description":    stringProperty("Updated human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Full workflow spec to replace the existing one. " +
					"Use get_workflow to retrieve the current spec first.",
			},
		}, []string{"namespace_name", "name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		DisplayName   string                 `json:"display_name"`
		Description   string                 `json:"description"`
		Spec          map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		spec, err := buildSpec[gen.WorkflowSpec](args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		wfReq := &gen.UpdateWorkflowJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: spec,
		}
		result, err := t.PEToolset.UpdateWorkflow(ctx, args.NamespaceName, wfReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEDeleteWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_workflow"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Delete a workflow from a namespace.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workflow_name":  stringProperty("Name of the workflow to delete. Use list_workflows to discover valid names"),
		}, []string{"namespace_name", "workflow_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		WorkflowName  string `json:"workflow_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteWorkflow(ctx, args.NamespaceName, args.WorkflowName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — Platform standards write (cluster-scoped)
// ---------------------------------------------------------------------------

//nolint:dupl // create/update cluster component type handlers share similar structure
func (t *Toolsets) RegisterCreateClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_cluster_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new cluster-scoped component type. Cluster component types are the platform-wide " +
			"golden path templates available to all namespaces.",
		InputSchema: createSchema(map[string]any{
			"name":         stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name": stringProperty("Human-readable display name"),
			"description":  stringProperty("Human-readable description"),
			"spec": map[string]any{
				"type":        "object",
				"description": "Use get_cluster_component_type_creation_schema to check the schema",
			},
		}, []string{"name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string                 `json:"name"`
		DisplayName string                 `json:"display_name"`
		Description string                 `json:"description"`
		Spec        map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}
		specJSON, err := json.Marshal(args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		var spec gen.ClusterComponentTypeSpec
		if err := json.Unmarshal(specJSON, &spec); err != nil {
			return handleToolResult(nil, err)
		}
		cctReq := &gen.CreateClusterComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: &spec,
		}
		result, err := t.PEToolset.CreateClusterComponentType(ctx, cctReq)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update cluster component type handlers share similar structure
func (t *Toolsets) RegisterUpdateClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_cluster_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Update an existing cluster-scoped component type (full replacement).",
		InputSchema: createSchema(map[string]any{
			"name": stringProperty(
				"Name of the cluster component type to update. Use list_cluster_component_types to discover valid names"),
			"display_name": stringProperty("Updated human-readable display name"),
			"description":  stringProperty("Updated human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Full cluster component type spec to replace the existing one. " +
					"Use get_cluster_component_type to retrieve the current spec first.",
			},
		}, []string{"name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string                 `json:"name"`
		DisplayName string                 `json:"display_name"`
		Description string                 `json:"description"`
		Spec        map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}
		specJSON, err := json.Marshal(args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		var spec gen.ClusterComponentTypeSpec
		if err := json.Unmarshal(specJSON, &spec); err != nil {
			return handleToolResult(nil, err)
		}
		cctReq := &gen.UpdateClusterComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: &spec,
		}
		result, err := t.PEToolset.UpdateClusterComponentType(ctx, cctReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteClusterComponentType(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_cluster_component_type"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteClusterComponentType}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Delete a cluster-scoped component type.",
		InputSchema: createSchema(map[string]any{
			"cct_name": stringProperty(
				"Name of the cluster component type to delete. Use list_cluster_component_types to discover valid names"),
		}, []string{"cct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CctName string `json:"cct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteClusterComponentType(ctx, args.CctName)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update cluster trait handlers share similar structure
func (t *Toolsets) RegisterCreateClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_cluster_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new cluster-scoped trait. Cluster traits are platform-wide capability definitions " +
			"available to all namespaces (e.g., autoscaling, ingress, service mesh).",
		InputSchema: createSchema(map[string]any{
			"name":         stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name": stringProperty("Human-readable display name"),
			"description":  stringProperty("Human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Cluster trait specification defining what resources the trait creates or patches. " +
					"Use get_cluster_trait_schema on an existing trait to see the full structure.",
			},
		}, []string{"name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string                 `json:"name"`
		DisplayName string                 `json:"display_name"`
		Description string                 `json:"description"`
		Spec        map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}
		specJSON, err := json.Marshal(args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		var spec gen.ClusterTraitSpec
		if err := json.Unmarshal(specJSON, &spec); err != nil {
			return handleToolResult(nil, err)
		}
		ctReq := &gen.CreateClusterTraitJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: &spec,
		}
		result, err := t.PEToolset.CreateClusterTrait(ctx, ctReq)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update cluster trait handlers share similar structure
func (t *Toolsets) RegisterUpdateClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_cluster_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing cluster-scoped trait (full replacement). " +
			"Use get_cluster_trait to retrieve the current definition first.",
		InputSchema: createSchema(map[string]any{
			"name": stringProperty(
				"Name of the cluster trait to update. Use list_cluster_traits to discover valid names"),
			"display_name": stringProperty("Updated human-readable display name"),
			"description":  stringProperty("Updated human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Full cluster trait spec to replace the existing one. " +
					"Use get_cluster_trait to retrieve the current spec first.",
			},
		}, []string{"name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string                 `json:"name"`
		DisplayName string                 `json:"display_name"`
		Description string                 `json:"description"`
		Spec        map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}
		specJSON, err := json.Marshal(args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		var spec gen.ClusterTraitSpec
		if err := json.Unmarshal(specJSON, &spec); err != nil {
			return handleToolResult(nil, err)
		}
		ctReq := &gen.UpdateClusterTraitJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: &spec,
		}
		result, err := t.PEToolset.UpdateClusterTrait(ctx, ctReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteClusterTrait(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_cluster_trait"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteClusterTrait}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Delete a cluster-scoped trait.",
		InputSchema: createSchema(map[string]any{
			"ct_name": stringProperty("Name of the cluster trait to delete. Use list_cluster_traits to discover valid names"),
		}, []string{"ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CtName string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteClusterTrait(ctx, args.CtName)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update cluster workflow handlers share similar structure
func (t *Toolsets) RegisterCreateClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_cluster_workflow"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateClusterWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create a new cluster-scoped workflow. Cluster workflows are platform-wide CI/CD pipeline " +
			"templates available to all namespaces.",
		InputSchema: createSchema(map[string]any{
			"name":         stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name": stringProperty("Human-readable display name"),
			"description":  stringProperty("Human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Cluster workflow specification. Required field: runTemplate (Argo Workflow template definition). " +
					"Use get_cluster_workflow_schema on an existing workflow to see the full structure.",
			},
		}, []string{"name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string                 `json:"name"`
		DisplayName string                 `json:"display_name"`
		Description string                 `json:"description"`
		Spec        map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		spec, err := buildSpec[gen.ClusterWorkflowSpec](args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		cwfReq := &gen.CreateClusterWorkflowJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: spec,
		}
		result, err := t.PEToolset.CreateClusterWorkflow(ctx, cwfReq)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // create/update cluster workflow handlers share similar structure
func (t *Toolsets) RegisterUpdateClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_cluster_workflow"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateClusterWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing cluster-scoped workflow (full replacement). " +
			"Use get_cluster_workflow to retrieve the current definition first.",
		InputSchema: createSchema(map[string]any{
			"name": stringProperty(
				"Name of the cluster workflow to update. Use list_cluster_workflows to discover valid names"),
			"display_name": stringProperty("Updated human-readable display name"),
			"description":  stringProperty("Updated human-readable description"),
			"spec": map[string]any{
				"type": "object",
				"description": "Full cluster workflow spec to replace the existing one. " +
					"Use get_cluster_workflow to retrieve the current spec first.",
			},
		}, []string{"name", "spec"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name        string                 `json:"name"`
		DisplayName string                 `json:"display_name"`
		Description string                 `json:"description"`
		Spec        map[string]interface{} `json:"spec"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := buildAnnotations(args.DisplayName, args.Description)
		spec, err := buildSpec[gen.ClusterWorkflowSpec](args.Spec)
		if err != nil {
			return handleToolResult(nil, err)
		}
		cwfReq := &gen.UpdateClusterWorkflowJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
			Spec: spec,
		}
		result, err := t.PEToolset.UpdateClusterWorkflow(ctx, cwfReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteClusterWorkflow(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_cluster_workflow"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteClusterWorkflow}
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Delete a cluster-scoped workflow.",
		InputSchema: createSchema(map[string]any{
			"cwf_name": stringProperty(
				"Name of the cluster workflow to delete. Use list_cluster_workflows to discover valid names"),
		}, []string{"cwf_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CwfName string `json:"cwf_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteClusterWorkflow(ctx, args.CwfName)
		return handleToolResult(result, err)
	})
}
