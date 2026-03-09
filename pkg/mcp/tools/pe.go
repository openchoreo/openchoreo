// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ---------------------------------------------------------------------------
// PE Toolset — Environment management
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterPEListEnvironments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_environments",
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

func (t *Toolsets) RegisterCreateEnvironment(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_environment",
		Description: "Create a new environment in a namespace. Environments are deployment targets representing " +
			"pipeline stages (dev, staging, production) or isolated tenants.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"data_plane_ref": stringProperty("Associated data plane reference. Use list_dataplanes to discover valid names"),
			"is_production": map[string]any{
				"type":        "boolean",
				"description": "Whether this is a production environment",
			},
			"dns_prefix": stringProperty("Optional: DNS prefix for this environment"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
		DisplayName   string `json:"display_name"`
		Description   string `json:"description"`
		DataPlaneRef  string `json:"data_plane_ref"`
		IsProduction  bool   `json:"is_production"`
		DNSPrefix     string `json:"dns_prefix"`
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
			envReq.Spec.DataPlaneRef = &struct {
				Kind gen.EnvironmentSpecDataPlaneRefKind `json:"kind"`
				Name string                              `json:"name"`
			}{
				Kind: gen.EnvironmentSpecDataPlaneRefKindDataPlane,
				Name: args.DataPlaneRef,
			}
		}
		result, err := t.PEToolset.CreateEnvironment(ctx, args.NamespaceName, envReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateEnvironment(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "update_environment",
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

func (t *Toolsets) RegisterDeleteEnvironment(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "delete_environment",
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

func (t *Toolsets) RegisterCreateDeploymentPipeline(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_deployment_pipeline",
		Description: "Create a new deployment pipeline in a namespace. Deployment pipelines define the promotion " +
			"order between environments (e.g., dev → staging → production) with optional approval gates.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"promotion_paths": map[string]any{
				"type": "array",
				"description": "Promotion paths defining environment progression. " +
					"Each path has a source environment and target environments with optional approval requirements",
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
									"requires_approval": map[string]any{
										"type":        "boolean",
										"description": "Whether promotion to this environment requires approval",
									},
								},
							},
						},
					},
				},
			},
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName  string `json:"namespace_name"`
		Name           string `json:"name"`
		DisplayName    string `json:"display_name"`
		Description    string `json:"description"`
		PromotionPaths []struct {
			SourceEnvironmentRef  string `json:"source_environment_ref"`
			TargetEnvironmentRefs []struct {
				Name             string `json:"name"`
				RequiresApproval bool   `json:"requires_approval"`
			} `json:"target_environment_refs"`
		} `json:"promotion_paths"`
	}) (*mcp.CallToolResult, any, error) {
		annotations := map[string]string{}
		if args.DisplayName != "" {
			annotations["openchoreo.dev/display-name"] = args.DisplayName
		}
		if args.Description != "" {
			annotations["openchoreo.dev/description"] = args.Description
		}

		dpReq := &gen.CreateDeploymentPipelineJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        args.Name,
				Annotations: &annotations,
			},
		}

		if len(args.PromotionPaths) > 0 {
			paths := make([]gen.PromotionPath, 0, len(args.PromotionPaths))
			for _, p := range args.PromotionPaths {
				targets := make([]gen.TargetEnvironmentRef, 0, len(p.TargetEnvironmentRefs))
				for _, t := range p.TargetEnvironmentRefs {
					requiresApproval := t.RequiresApproval
					targets = append(targets, gen.TargetEnvironmentRef{
						Name:             t.Name,
						RequiresApproval: &requiresApproval,
					})
				}
				paths = append(paths, gen.PromotionPath{
					SourceEnvironmentRef:  p.SourceEnvironmentRef,
					TargetEnvironmentRefs: targets,
				})
			}
			dpReq.Spec = &gen.DeploymentPipelineSpec{
				PromotionPaths: &paths,
			}
		}

		result, err := t.PEToolset.CreateDeploymentPipeline(ctx, args.NamespaceName, dpReq)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — DataPlane read
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_dataplanes",
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

func (t *Toolsets) RegisterGetDataPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_dataplane",
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
// PE Toolset — BuildPlane read
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListBuildPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_buildplanes",
		Description: "List all build planes in a namespace. Build planes are infrastructure that handles " +
			"continuous integration and container image building. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListBuildPlanes(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetBuildPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_buildplane",
		Description: "Get detailed information about a build plane including cluster details, health status, " +
			"and agent connection state.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"bp_name":        stringProperty("Use list_buildplanes to discover valid names"),
		}, []string{"namespace_name", "bp_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		BpName        string `json:"bp_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetBuildPlane(ctx, args.NamespaceName, args.BpName)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// PE Toolset — ObservabilityPlane read
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListObservabilityPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_observability_planes",
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

func (t *Toolsets) RegisterGetObservabilityPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_observability_plane",
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

func (t *Toolsets) RegisterListClusterDataPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_dataplanes",
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

func (t *Toolsets) RegisterGetClusterDataPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_dataplane",
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

func (t *Toolsets) RegisterListClusterBuildPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_buildplanes",
		Description: "List all cluster-scoped build planes. These are shared build infrastructure managed by " +
			"platform admins, not scoped to any namespace. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{}), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Limit  int    `json:"limit,omitempty"`
		Cursor string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListClusterBuildPlanes(ctx, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListClusterObservabilityPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_observability_planes",
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

func (t *Toolsets) RegisterPEListComponentTypes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_types",
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

func (t *Toolsets) RegisterPEGetComponentTypeSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_type_schema",
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
		result, err := t.PEToolset.GetComponentTypeSchema(ctx, args.NamespaceName, args.CtName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEListTraits(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_traits",
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

func (t *Toolsets) RegisterPEGetTraitSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_trait_schema",
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
		result, err := t.PEToolset.GetTraitSchema(ctx, args.NamespaceName, args.TraitName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEListWorkflows(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_workflows",
		Description: "List all workflows in a namespace. Workflows are reusable templates that define " +
			"automated processes such as CI/CD pipelines executed on the build plane. " +
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

func (t *Toolsets) RegisterPEGetWorkflowSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_workflow_schema",
		Description: "Get the parameter schema for a specific workflow. Use this to inspect what parameters " +
			"a workflow accepts before configuring a component's workflow field or triggering a workflow run. " +
			"Use list_workflows to discover valid workflow names.",
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

// ---------------------------------------------------------------------------
// PE Toolset — Diagnostics
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterGetResourceEvents(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_resource_events",
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

func (t *Toolsets) RegisterGetResourceLogs(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_resource_logs",
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

func (t *Toolsets) RegisterGetDeploymentPipeline(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_deployment_pipeline",
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

func (t *Toolsets) RegisterListDeploymentPipelines(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_deployment_pipelines",
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

func (t *Toolsets) RegisterListEnvironments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_environments",
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

func (t *Toolsets) RegisterCreateWorkflowRun(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_workflow_run",
		Description: "Create a new workflow run by specifying a workflow name and optional parameters. " +
			"Workflows define automated processes like CI/CD pipelines that execute on the build plane.",
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

func (t *Toolsets) RegisterListWorkflowRuns(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_workflow_runs",
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

func (t *Toolsets) RegisterGetWorkflowRun(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_workflow_run",
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

func (t *Toolsets) RegisterListWorkflows(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_workflows",
		Description: "List all workflows in a namespace. Workflows are reusable templates that define " +
			"automated processes such as CI/CD pipelines executed on the build plane. " +
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

func (t *Toolsets) RegisterGetWorkflowSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_workflow_schema",
		Description: "Get the parameter schema for a specific workflow. Use this to inspect what parameters " +
			"a workflow accepts before configuring a component's workflow field or triggering a workflow run. " +
			"Use list_workflows to discover valid workflow names.",
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

func (t *Toolsets) RegisterListClusterWorkflows(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_workflows",
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

func (t *Toolsets) RegisterGetClusterWorkflow(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_workflow",
		Description: "Get detailed information about a cluster-scoped workflow including its name, " +
			"display name, and description.",
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

func (t *Toolsets) RegisterGetClusterWorkflowSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_workflow_schema",
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

func (t *Toolsets) RegisterPEListClusterComponentTypes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_component_types",
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

func (t *Toolsets) RegisterPEGetClusterComponentType(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_component_type",
		Description: "Get detailed information about a cluster-scoped component type including workload type, " +
			"allowed workflows, allowed traits, and description.",
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

func (t *Toolsets) RegisterPEGetClusterComponentTypeSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_component_type_schema",
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

func (t *Toolsets) RegisterPEListClusterTraits(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_traits",
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

func (t *Toolsets) RegisterPEGetClusterTrait(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_trait",
		Description: "Get detailed information about a cluster-scoped trait including its name, " +
			"display name, and description.",
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

func (t *Toolsets) RegisterPEGetClusterTraitSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_trait_schema",
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

func (t *Toolsets) RegisterListComponentTypes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_types",
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

func (t *Toolsets) RegisterGetComponentTypeSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_type_schema",
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

func (t *Toolsets) RegisterListTraits(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_traits",
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

func (t *Toolsets) RegisterGetTraitSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_trait_schema",
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

func (t *Toolsets) RegisterListClusterComponentTypes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_component_types",
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

func (t *Toolsets) RegisterGetClusterComponentType(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_component_type",
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

func (t *Toolsets) RegisterGetClusterComponentTypeSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_component_type_schema",
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

func (t *Toolsets) RegisterListClusterTraits(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_traits",
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

func (t *Toolsets) RegisterGetClusterTrait(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_trait",
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

func (t *Toolsets) RegisterGetClusterTraitSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_cluster_trait_schema",
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
