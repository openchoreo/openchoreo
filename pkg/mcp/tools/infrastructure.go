// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

func (t *Toolsets) RegisterListEnvironments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_environments",
		Description: "List all environments in an organization. Environments are deployment targets representing " +
			"pipeline stages (dev, staging, production) or isolated tenants.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListEnvironments(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetEnvironments(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_environment",
		Description: "Get detailed information about an environment including associated data plane, deployed " +
			"components, resource quotas, and network configuration.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
			"env_name": stringProperty("Use list_environments to discover valid names"),
		}, []string{"org_name", "env_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
		EnvName string `json:"env_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetEnvironment(ctx, args.OrgName, args.EnvName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateEnvironment(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_environment",
		Description: "Create a new environment in an organization. Environments are deployment targets representing " +
			"pipeline stages (dev, staging, production) or isolated tenants.",
		InputSchema: createSchema(map[string]any{
			"org_name":       defaultStringProperty(),
			"name":           stringProperty("DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":   stringProperty("Human-readable display name"),
			"description":    stringProperty("Human-readable description"),
			"data_plane_ref": stringProperty("Associated data plane reference. Use list_dataplanes to discover valid names"),
			"is_production": map[string]any{
				"type":        "boolean",
				"description": "Whether this is a production environment",
			},
			"dns_prefix": stringProperty("Optional: DNS prefix for this environment"),
		}, []string{"org_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName      string `json:"org_name"`
		Name         string `json:"name"`
		DisplayName  string `json:"display_name"`
		Description  string `json:"description"`
		DataPlaneRef string `json:"data_plane_ref"`
		IsProduction bool   `json:"is_production"`
		DNSPrefix    string `json:"dns_prefix"`
	}) (*mcp.CallToolResult, any, error) {
		envReq := &models.CreateEnvironmentRequest{
			Name:         args.Name,
			DisplayName:  args.DisplayName,
			Description:  args.Description,
			DataPlaneRef: args.DataPlaneRef,
			IsProduction: args.IsProduction,
			DNSPrefix:    args.DNSPrefix,
		}
		result, err := t.InfrastructureToolset.CreateEnvironment(ctx, args.OrgName, envReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListDataPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_dataplanes",
		Description: "List all data planes in an organization. Data planes are Kubernetes clusters or cluster " +
			"regions where component workloads actually execute.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListDataPlanes(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetDataPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_dataplane",
		Description: "Get detailed information about a data plane including cluster details, capacity, health " +
			"status, associated environments, and network configuration.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
			"dp_name":  stringProperty("Use list_dataplanes to discover valid names"),
		}, []string{"org_name", "dp_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
		DpName  string `json:"dp_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetDataPlane(ctx, args.OrgName, args.DpName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateDataPlane(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "create_dataplane",
		Description: "Create a new data plane in an organization. Data planes are Kubernetes clusters or cluster " +
			"regions where component workloads actually execute.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
			"name": stringProperty(
				"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"display_name":              stringProperty("Human-readable display name"),
			"description":               stringProperty("Human-readable description"),
			"kubernetes_cluster_name":   stringProperty("Kubernetes cluster name"),
			"api_server_url":            stringProperty("Kubernetes API server URL"),
			"ca_cert":                   stringProperty("Kubernetes cluster CA certificate"),
			"client_cert":               stringProperty("Kubernetes client certificate"),
			"client_key":                stringProperty("Kubernetes client key"),
			"public_virtual_host":       stringProperty("Public virtual host for the data plane"),
			"organization_virtual_host": stringProperty("Organization-specific virtual host"),
			"observability_plane_ref":   stringProperty("Optional: Reference to an ObservabilityPlane resource for monitoring"),
		}, []string{
			"org_name", "name", "kubernetes_cluster_name", "api_server_url", "ca_cert",
			"client_cert", "client_key", "public_virtual_host", "organization_virtual_host",
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName                 string `json:"org_name"`
		Name                    string `json:"name"`
		DisplayName             string `json:"display_name"`
		Description             string `json:"description"`
		KubernetesClusterName   string `json:"kubernetes_cluster_name"`
		APIServerURL            string `json:"api_server_url"`
		CACert                  string `json:"ca_cert"`
		ClientCert              string `json:"client_cert"`
		ClientKey               string `json:"client_key"`
		PublicVirtualHost       string `json:"public_virtual_host"`
		OrganizationVirtualHost string `json:"organization_virtual_host"`
		ObservabilityPlaneRef   string `json:"observability_plane_ref"`
	}) (*mcp.CallToolResult, any, error) {
		dataPlaneReq := &models.CreateDataPlaneRequest{
			Name:                    args.Name,
			DisplayName:             args.DisplayName,
			Description:             args.Description,
			KubernetesClusterName:   args.KubernetesClusterName,
			APIServerURL:            args.APIServerURL,
			CACert:                  args.CACert,
			ClientCert:              args.ClientCert,
			ClientKey:               args.ClientKey,
			PublicVirtualHost:       args.PublicVirtualHost,
			OrganizationVirtualHost: args.OrganizationVirtualHost,
			ObservabilityPlaneRef:   args.ObservabilityPlaneRef,
		}
		result, err := t.InfrastructureToolset.CreateDataPlane(ctx, args.OrgName, dataPlaneReq)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListComponentTypes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_types",
		Description: "List all available component types in an organization. Component types define the " +
			"structure and capabilities of components (e.g., WebApplication, Service, ScheduledTask).",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListComponentTypes(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentTypeSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_type_schema",
		Description: "Get the schema definition for a component type. Returns the JSON schema showing " +
			"required fields, optional fields, and their types.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
			"ct_name":  stringProperty("Component type name. Use list_component_types to discover valid names"),
		}, []string{"org_name", "ct_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
		CtName  string `json:"ct_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetComponentTypeSchema(ctx, args.OrgName, args.CtName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListWorkflows(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_workflows",
		Description: "List all available component-workflows in an organization. Workflows define build and deployment " +
			"processes for components.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListWorkflows(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetWorkflowSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_workflow_schema",
		Description: "Get the schema definition for a workflow. Returns the JSON schema showing workflow " +
			"configuration options and parameters.",
		InputSchema: createSchema(map[string]any{
			"org_name":      defaultStringProperty(),
			"workflow_name": stringProperty("Workflow name. Use list_workflows to discover valid names"),
		}, []string{"org_name", "workflow_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName      string `json:"org_name"`
		WorkflowName string `json:"workflow_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetWorkflowSchema(ctx, args.OrgName, args.WorkflowName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListTraits(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_traits",
		Description: "List all available traits in an organization. Traits add capabilities to components " +
			"(e.g., autoscaling, ingress, service mesh).",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListTraits(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetTraitSchema(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_trait_schema",
		Description: "Get the schema definition for a trait. Returns the JSON schema showing trait " +
			"configuration options and parameters.",
		InputSchema: createSchema(map[string]any{
			"org_name":   defaultStringProperty(),
			"trait_name": stringProperty("Trait name. Use list_traits to discover valid names"),
		}, []string{"org_name", "trait_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName   string `json:"org_name"`
		TraitName string `json:"trait_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetTraitSchema(ctx, args.OrgName, args.TraitName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterListObservabilityPlanes(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_observability_planes",
		Description: "List all ObservabilityPlanes in an organization. ObservabilityPlanes provide monitoring, " +
			"logging, tracing, and metrics collection capabilities for deployed components.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListObservabilityPlanes(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}
func (t *Toolsets) RegisterListComponentWorkflowsOrgLevel(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "list_component_workflows_org_level",
		Description: "List all ComponentWorkflow templates available in an organization. " +
			"ComponentWorkflows are reusable workflow templates that can be triggered on components.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
		}, []string{"org_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.ListComponentWorkflows(ctx, args.OrgName)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetComponentWorkflowSchemaOrgLevel(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_component_workflow_schema_org_level",
		Description: "Get the schema for a ComponentWorkflow template in an organization. " +
			"Returns the JSON schema defining the input parameters and configuration for the workflow.",
		InputSchema: createSchema(map[string]any{
			"org_name": defaultStringProperty(),
			"cw_name":  defaultStringProperty(),
		}, []string{"org_name", "cw_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		OrgName string `json:"org_name"`
		CWName  string `json:"cw_name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.InfrastructureToolset.GetComponentWorkflowSchema(ctx, args.OrgName, args.CWName)
		return handleToolResult(result, err)
	})
}
