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
			"name":           stringProperty("Name of the environment to delete. Use list_environments to discover valid names"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteEnvironment(ctx, args.NamespaceName, args.Name)
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
// PE Toolset — Diagnostics
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterGetResourceTree(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_resource_tree"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List rendered Kubernetes resources (Deployment, Service, Pod, etc.) under a release binding, " +
			"with their group/version/kind/name, parent refs, and health.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"release_binding_name": stringProperty(
				"Name of the release binding (deployment). Use list_release_bindings to discover valid names"),
		}, []string{"namespace_name", "release_binding_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName      string `json:"namespace_name"`
		ReleaseBindingName string `json:"release_binding_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetResourceTree(ctx, args.NamespaceName, args.ReleaseBindingName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetResourceEvents(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_resource_events"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get Kubernetes events for a specific rendered resource in a deployment. " +
			"Useful for diagnosing scheduling problems and container startup failures. " +
			"Call get_resource_tree first to discover the exact group/version/kind/name.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"release_binding_name": stringProperty(
				"Name of the release binding (deployment). Use list_release_bindings to discover valid names"),
			"group":   stringProperty("API group of the resource (e.g., 'apps', '' for core resources)"),
			"version": stringProperty("API version of the resource (e.g., 'v1')"),
			"kind":    stringProperty("Kind of the resource (e.g., 'Deployment', 'Pod', 'Service')"),
			"resource_name": stringProperty(
				"Rendered K8s resource name (usually differs from the component name). " +
					"Use get_resource_tree to discover it."),
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
			"pod_name": stringProperty("Name of the pod. Use get_resource_tree to discover pod names under the binding"),
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
			"name":           stringProperty("Use list_deployment_pipelines to discover valid names"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetDeploymentPipeline(ctx, args.NamespaceName, args.Name)
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
			"name":           stringProperty("Name of the Workflow CR to execute"),
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: Parameters to pass to the workflow execution",
			},
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string                 `json:"namespace_name"`
		Name          string                 `json:"name"`
		Parameters    map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.CreateWorkflowRun(ctx, args.NamespaceName, args.Name, args.Parameters)
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

func (t *Toolsets) RegisterGetWorkflowRunStatus(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workflow_run_status"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflowRun}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the overall status and per-step status of a specific workflow run. " +
			"Returns the run-level phase and a breakdown of each task with its phase and start/finish timestamps. " +
			"Useful for monitoring CI/CD pipeline progress without fetching full logs.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"run_name":       stringProperty("Use list_workflow_runs to discover valid names"),
		}, []string{"namespace_name", "run_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		RunName       string `json:"run_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.GetWorkflowRunStatus(ctx, args.NamespaceName, args.RunName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkflowRunLogs(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workflow_run_logs"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflowRun}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get live logs for a specific workflow run from the workflow plane. " +
			"Useful for debugging CI/CD pipeline failures and inspecting task output. " +
			"Logs are fetched live; no archived logs are returned for completed runs. " +
			"Optionally filter by task name and limit to recent activity via since_seconds.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"run_name":       stringProperty("Use list_workflow_runs to discover valid names"),
			"task":           stringProperty("Optional: filter logs by task name within the workflow run"),
			"since_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional: return logs newer than a relative duration in seconds",
				"minimum":     0,
			},
		}, []string{"namespace_name", "run_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		RunName       string `json:"run_name"`
		Task          string `json:"task"`
		SinceSeconds  *int64 `json:"since_seconds"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.GetWorkflowRunLogs(
			ctx, args.NamespaceName, args.RunName, args.Task, args.SinceSeconds)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkflowRunEvents(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_workflow_run_events"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewWorkflowRun}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get Kubernetes events for a specific workflow run. " +
			"Useful for diagnosing scheduling problems, pod startup failures, and other workflow run issues. " +
			"Optionally filter by task name within the run.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"run_name":       stringProperty("Use list_workflow_runs to discover valid names"),
			"task":           stringProperty("Optional: filter events by task name within the workflow run"),
		}, []string{"namespace_name", "run_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		RunName       string `json:"run_name"`
		Task          string `json:"task"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.BuildToolset.GetWorkflowRunEvents(
			ctx, args.NamespaceName, args.RunName, args.Task)
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
			"name": stringProperty(
				"Name of the deployment pipeline to delete. Use list_deployment_pipelines to discover valid names"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.DeleteDeploymentPipeline(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}
