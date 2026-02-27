// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (t *Toolsets) RegisterListEnvironments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_environments",
		Description: "List all environments in an namespace. Environments are deployment targets representing " +
			"pipeline stages (dev, staging, production) or isolated tenants. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
		}), []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListEnvironments(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetEnvironments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_environment",
		Description: "Get detailed information about an environment including associated data plane, deployed " +
			"components, resource quotas, and network configuration.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"env_name":       stringProperty("Use list_environments to discover valid names"),
		}, []string{"namespace_name", "env_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		EnvName       string `json:"env_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetEnvironment(ctx, args.NamespaceName, args.EnvName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateEnvironment(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_environment",
		Description: "Create a new environment in an namespace. Environments are deployment targets representing " +
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
		// Convert DataPlaneRef string to object (default to DataPlane kind)
		var dataPlaneRef *models.DataPlaneRef
		if args.DataPlaneRef != "" {
			dataPlaneRef = &models.DataPlaneRef{
				Kind: "DataPlane",
				Name: args.DataPlaneRef,
			}
		}

		envReq := &models.CreateEnvironmentRequest{
			Name:         args.Name,
			DisplayName:  args.DisplayName,
			Description:  args.Description,
			DataPlaneRef: dataPlaneRef,
			IsProduction: args.IsProduction,
			DNSPrefix:    args.DNSPrefix,
		}
		result, err := t.PEToolset.CreateEnvironment(ctx, args.NamespaceName, envReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_dataplanes",
		Description: "List all data planes in an namespace. Data planes are Kubernetes clusters or cluster " +
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

func (t *Toolsets) RegisterCreateDataPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_dataplane",
		Description: "Create a new data plane in an namespace. Uses cluster agent for communication.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty(
				"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":            stringProperty("Human-readable display name"),
			"description":             stringProperty("Human-readable description"),
			"cluster_agent_client_ca": stringProperty("CA certificate to verify cluster agent's client certificate"),
			"observability_plane_ref": map[string]any{
				"type":        "object",
				"description": "Optional: Reference to an ObservabilityPlane or ClusterObservabilityPlane resource",
				"required":    []string{"name"},
				"properties": map[string]any{
					"kind": map[string]any{
						"type": "string",
						"description": "ObservabilityPlane or ClusterObservabilityPlane. " +
							"Defaults to ObservabilityPlane when omitted.",
						"enum": []string{"ObservabilityPlane", "ClusterObservabilityPlane"},
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Name of the observability plane resource",
					},
				},
			},
		}, []string{"namespace_name", "name", "cluster_agent_client_ca"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName         string `json:"namespace_name"`
		Name                  string `json:"name"`
		DisplayName           string `json:"display_name"`
		Description           string `json:"description"`
		ClusterAgentClientCA  string `json:"cluster_agent_client_ca"`
		ObservabilityPlaneRef *struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"observability_plane_ref"`
	}) (*mcp.CallToolResult, any, error) {
		dataPlaneReq := &models.CreateDataPlaneRequest{
			Name:                 args.Name,
			DisplayName:          args.DisplayName,
			Description:          args.Description,
			ClusterAgentClientCA: args.ClusterAgentClientCA,
		}
		if args.ObservabilityPlaneRef != nil {
			if args.ObservabilityPlaneRef.Name == "" {
				return nil, nil, fmt.Errorf("observability_plane_ref.name is required when observability_plane_ref is provided")
			}
			kind := args.ObservabilityPlaneRef.Kind
			if kind == "" {
				kind = "ObservabilityPlane"
			} else if kind != "ObservabilityPlane" && kind != "ClusterObservabilityPlane" {
				return nil, nil, fmt.Errorf(
					"observability_plane_ref.kind must be 'ObservabilityPlane' or "+
						"'ClusterObservabilityPlane', got '%s'", kind)
			}
			dataPlaneReq.ObservabilityPlaneRef = &models.ObservabilityPlaneRef{
				Kind: kind,
				Name: args.ObservabilityPlaneRef.Name,
			}
		}
		result, err := t.PEToolset.CreateDataPlane(ctx, args.NamespaceName, dataPlaneReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentTypes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_types",
		Description: "List all available component types in an namespace. Component types define the " +
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
		Description: "List all available traits in an namespace. Traits add capabilities to components " +
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
		result, err := t.InfrastructureToolset.GetDeploymentPipeline(ctx, args.NamespaceName, args.PipelineName)
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
		result, err := t.InfrastructureToolset.ListDeploymentPipelines(
			ctx, args.NamespaceName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

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

func (t *Toolsets) RegisterGetObserverURL(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_observer_url",
		Description: "Get the observer URL for an environment. The observer URL provides access to monitoring, " +
			"logging, and observability data for components deployed in the environment.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"env_name":       stringProperty("Use list_environments to discover valid names"),
		}, []string{"namespace_name", "env_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		EnvName       string `json:"env_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetObserverURL(ctx, args.NamespaceName, args.EnvName)
		return handleToolResult(result, err)
	})
}

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
		result, err := t.ComponentToolset.CreateWorkflowRun(ctx, args.NamespaceName, args.WorkflowName, args.Parameters)
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
		result, err := t.ComponentToolset.ListWorkflowRuns(
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
		result, err := t.ComponentToolset.GetWorkflowRun(ctx, args.NamespaceName, args.RunName)
		return handleToolResult(result, err)
	})
}

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
		result, err := t.ComponentToolset.ListWorkflows(
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
		result, err := t.ComponentToolset.GetWorkflowSchema(ctx, args.NamespaceName, args.WorkflowName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListObservabilityPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_observability_planes",
		Description: "List all ObservabilityPlanes in an namespace. ObservabilityPlanes provide monitoring, " +
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

// RegisterListClusterDataPlanes registers the "list_cluster_dataplanes" MCP tool
// which lists all cluster-scoped data planes (shared infrastructure managed by
// platform admins, not scoped to any namespace). This tool is only registered
// when InfrastructureToolset also implements ClusterPlaneHandler; otherwise it
// is a no-op. Added in v0.12.0 (non-breaking, additive).
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

// RegisterGetClusterDataPlane registers the "get_cluster_dataplane" MCP tool
// which returns detailed information about a specific cluster-scoped data plane
// including cluster details, capacity, health status, and network configuration.
// Requires a "cdp_name" parameter (discoverable via list_cluster_dataplanes).
// Only registered when InfrastructureToolset also implements ClusterPlaneHandler;
// otherwise it is a no-op. Added in v0.12.0 (non-breaking, additive).
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

func (t *Toolsets) RegisterCreateClusterDataPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_cluster_dataplane",
		Description: "Create a new cluster-scoped data plane. Cluster data planes are shared infrastructure " +
			"managed by platform admins. Uses cluster agent for communication.",
		InputSchema: createSchema(map[string]any{
			"name": stringProperty(
				"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":              stringProperty("Human-readable display name"),
			"description":               stringProperty("Human-readable description"),
			"plane_id":                  stringProperty("Logical plane identifier for the physical cluster"),
			"cluster_agent_client_ca":   stringProperty("CA certificate to verify cluster agent's client certificate"),
			"public_virtual_host":       stringProperty("Public virtual host for the data plane"),
			"organization_virtual_host": stringProperty("Organization-specific virtual host"),
			"public_http_port": map[string]any{
				"type":        "integer",
				"description": "Public HTTP port",
			},
			"public_https_port": map[string]any{
				"type":        "integer",
				"description": "Public HTTPS port",
			},
			"organization_http_port": map[string]any{
				"type":        "integer",
				"description": "Organization HTTP port",
			},
			"organization_https_port": map[string]any{
				"type":        "integer",
				"description": "Organization HTTPS port",
			},
			"observability_plane_ref": map[string]any{
				"type":        "object",
				"description": "Optional: Reference to a ClusterObservabilityPlane resource",
				"required":    []string{"name"},
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Name of the ClusterObservabilityPlane resource",
					},
				},
			},
		}, []string{
			"name", "plane_id", "cluster_agent_client_ca",
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name                  string `json:"name"`
		DisplayName           string `json:"display_name"`
		Description           string `json:"description"`
		PlaneID               string `json:"plane_id"`
		ClusterAgentClientCA  string `json:"cluster_agent_client_ca"`
		ObservabilityPlaneRef *struct {
			Name string `json:"name"`
		} `json:"observability_plane_ref"`
	}) (*mcp.CallToolResult, any, error) {
		cdpReq := &models.CreateClusterDataPlaneRequest{
			Name:                 args.Name,
			DisplayName:          args.DisplayName,
			Description:          args.Description,
			PlaneID:              args.PlaneID,
			ClusterAgentClientCA: args.ClusterAgentClientCA,
		}
		if args.ObservabilityPlaneRef != nil {
			if args.ObservabilityPlaneRef.Name == "" {
				return nil, nil, fmt.Errorf("observability_plane_ref.name is required when observability_plane_ref is provided")
			}
			cdpReq.ObservabilityPlaneRef = &models.ObservabilityPlaneRef{
				Kind: "ClusterObservabilityPlane",
				Name: args.ObservabilityPlaneRef.Name,
			}
		}
		if err := cdpReq.Validate(); err != nil {
			return nil, nil, fmt.Errorf("invalid cluster dataplane request: %w", err)
		}
		result, err := t.PEToolset.CreateClusterDataPlane(ctx, cdpReq)
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

// RegisterListClusterComponentTypes registers the "list_cluster_component_types" MCP tool
// which lists all cluster-scoped component types (shared templates managed by platform
// admins, not scoped to any namespace).
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

// RegisterGetClusterComponentType registers the "get_cluster_component_type" MCP tool
// which returns detailed information about a specific cluster-scoped component type.
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

// RegisterGetClusterComponentTypeSchema registers the "get_cluster_component_type_schema" MCP tool
// which returns the JSON schema for a specific cluster-scoped component type.
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

// RegisterListClusterTraits registers the "list_cluster_traits" MCP tool
// which lists all cluster-scoped traits (shared trait definitions managed by platform
// admins, not scoped to any namespace).
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

// RegisterGetClusterTrait registers the "get_cluster_trait" MCP tool
// which returns detailed information about a specific cluster-scoped trait.
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

// RegisterGetClusterTraitSchema registers the "get_cluster_trait_schema" MCP tool
// which returns the JSON schema for a specific cluster-scoped trait.
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
