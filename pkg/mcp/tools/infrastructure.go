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
			"pipeline stages (dev, staging, production) or isolated tenants.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListEnvironments(ctx, args.NamespaceName)
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
		result, err := t.InfrastructureToolset.CreateEnvironment(ctx, args.NamespaceName, envReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_dataplanes",
		Description: "List all data planes in an namespace. Data planes are Kubernetes clusters or cluster " +
			"regions where component workloads actually execute.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListDataPlanes(ctx, args.NamespaceName)
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
		result, err := t.InfrastructureToolset.GetDataPlane(ctx, args.NamespaceName, args.DpName)
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
			"public_virtual_host":     stringProperty("Public virtual host for the data plane"),
			"namespace_virtual_host":  stringProperty("Namespace-specific virtual host"),
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
		}, []string{
			"namespace_name", "name", "cluster_agent_client_ca", "public_virtual_host", "namespace_virtual_host",
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName         string `json:"namespace_name"`
		Name                  string `json:"name"`
		DisplayName           string `json:"display_name"`
		Description           string `json:"description"`
		ClusterAgentClientCA  string `json:"cluster_agent_client_ca"`
		PublicVirtualHost     string `json:"public_virtual_host"`
		NamespaceVirtualHost  string `json:"namespace_virtual_host"`
		ObservabilityPlaneRef *struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"observability_plane_ref"`
	}) (*mcp.CallToolResult, any, error) {
		dataPlaneReq := &models.CreateDataPlaneRequest{
			Name:                    args.Name,
			DisplayName:             args.DisplayName,
			Description:             args.Description,
			ClusterAgentClientCA:    args.ClusterAgentClientCA,
			PublicVirtualHost:       args.PublicVirtualHost,
			OrganizationVirtualHost: args.NamespaceVirtualHost,
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
		result, err := t.InfrastructureToolset.CreateDataPlane(ctx, args.NamespaceName, dataPlaneReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentTypes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_types",
		Description: "List all available component types in an namespace. Component types define the " +
			"structure and capabilities of components (e.g., WebApplication, Service, ScheduledTask).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListComponentTypes(ctx, args.NamespaceName)
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
		result, err := t.InfrastructureToolset.GetComponentTypeSchema(ctx, args.NamespaceName, args.CtName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListWorkflows(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_workflows",
		Description: "List all available component-workflows in an namespace. Workflows define build and deployment " +
			"processes for components.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListWorkflows(ctx, args.NamespaceName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkflowSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_workflow_schema",
		Description: "Get the schema definition for a workflow. Returns the JSON schema showing workflow " +
			"configuration options and parameters.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"workflow_name":  stringProperty("Workflow name. Use list_workflows to discover valid names"),
		}, []string{"namespace_name", "workflow_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		WorkflowName  string `json:"workflow_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetWorkflowSchema(ctx, args.NamespaceName, args.WorkflowName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListTraits(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_traits",
		Description: "List all available traits in an namespace. Traits add capabilities to components " +
			"(e.g., autoscaling, ingress, service mesh).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListTraits(ctx, args.NamespaceName)
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
		result, err := t.InfrastructureToolset.GetTraitSchema(ctx, args.NamespaceName, args.TraitName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListObservabilityPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_observability_planes",
		Description: "List all ObservabilityPlanes in an namespace. ObservabilityPlanes provide monitoring, " +
			"logging, tracing, and metrics collection capabilities for deployed components.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListObservabilityPlanes(ctx, args.NamespaceName)
		return handleToolResult(result, err)
	})
}
func (t *Toolsets) RegisterListComponentWorkflowsOrgLevel(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_workflows_org_level",
		Description: "List all ComponentWorkflow templates available in an namespace. " +
			"ComponentWorkflows are reusable workflow templates that can be triggered on components.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
		}, []string{"namespace_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListComponentWorkflows(ctx, args.NamespaceName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentWorkflowSchemaOrgLevel(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_workflow_schema_org_level",
		Description: "Get the schema for a ComponentWorkflow template in an namespace. " +
			"Returns the JSON schema defining the input parameters and configuration for the workflow.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"cw_name":        defaultStringProperty(),
		}, []string{"namespace_name", "cw_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		CWName        string `json:"cw_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetComponentWorkflowSchema(ctx, args.NamespaceName, args.CWName)
		return handleToolResult(result, err)
	})
}

// RegisterListClusterDataPlanes registers the "list_cluster_dataplanes" MCP tool
// which lists all cluster-scoped data planes (shared infrastructure managed by
// platform admins, not scoped to any namespace). This tool is only registered
// when InfrastructureToolset also implements ClusterPlaneHandler; otherwise it
// is a no-op. Added in v0.12.0 (non-breaking, additive).
func (t *Toolsets) RegisterListClusterDataPlanes(s *mcp.Server) {
	cp, ok := t.InfrastructureToolset.(ClusterPlaneHandler)
	if !ok {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_dataplanes",
		Description: "List all cluster-scoped data planes. These are shared infrastructure managed by " +
			"platform admins, not scoped to any namespace.",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		result, err := cp.ListClusterDataPlanes(ctx)
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
	cp, ok := t.InfrastructureToolset.(ClusterPlaneHandler)
	if !ok {
		return
	}
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
		result, err := cp.GetClusterDataPlane(ctx, args.CdpName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateClusterDataPlane(s *mcp.Server) {
	cp, ok := t.InfrastructureToolset.(ClusterPlaneHandler)
	if !ok {
		return
	}
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
			"name", "plane_id", "cluster_agent_client_ca", "public_virtual_host", "organization_virtual_host",
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name                    string `json:"name"`
		DisplayName             string `json:"display_name"`
		Description             string `json:"description"`
		PlaneID                 string `json:"plane_id"`
		ClusterAgentClientCA    string `json:"cluster_agent_client_ca"`
		PublicVirtualHost       string `json:"public_virtual_host"`
		OrganizationVirtualHost string `json:"organization_virtual_host"`
		PublicHTTPPort          *int32 `json:"public_http_port"`
		PublicHTTPSPort         *int32 `json:"public_https_port"`
		OrganizationHTTPPort    *int32 `json:"organization_http_port"`
		OrganizationHTTPSPort   *int32 `json:"organization_https_port"`
		ObservabilityPlaneRef   *struct {
			Name string `json:"name"`
		} `json:"observability_plane_ref"`
	}) (*mcp.CallToolResult, any, error) {
		cdpReq := &models.CreateClusterDataPlaneRequest{
			Name:                    args.Name,
			DisplayName:             args.DisplayName,
			Description:             args.Description,
			PlaneID:                 args.PlaneID,
			ClusterAgentClientCA:    args.ClusterAgentClientCA,
			PublicVirtualHost:       args.PublicVirtualHost,
			OrganizationVirtualHost: args.OrganizationVirtualHost,
			PublicHTTPPort:          args.PublicHTTPPort,
			PublicHTTPSPort:         args.PublicHTTPSPort,
			OrganizationHTTPPort:    args.OrganizationHTTPPort,
			OrganizationHTTPSPort:   args.OrganizationHTTPSPort,
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
		result, err := cp.CreateClusterDataPlane(ctx, cdpReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListClusterBuildPlanes(s *mcp.Server) {
	cp, ok := t.InfrastructureToolset.(ClusterPlaneHandler)
	if !ok {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_buildplanes",
		Description: "List all cluster-scoped build planes. These are shared build infrastructure managed by " +
			"platform admins, not scoped to any namespace.",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		result, err := cp.ListClusterBuildPlanes(ctx)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListClusterObservabilityPlanes(s *mcp.Server) {
	cp, ok := t.InfrastructureToolset.(ClusterPlaneHandler)
	if !ok {
		return
	}
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_cluster_observability_planes",
		Description: "List all cluster-scoped observability planes. These are shared observability infrastructure " +
			"managed by platform admins, not scoped to any namespace.",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		result, err := cp.ListClusterObservabilityPlanes(ctx)
		return handleToolResult(result, err)
	})
}
