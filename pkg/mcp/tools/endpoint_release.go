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
// EndpointReleases — list/get on the pe toolset, delete on the deployment
// toolset. There is no create: the Endpoint controller cuts a release whenever
// the Endpoint or its type changes.
// ---------------------------------------------------------------------------

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterPEListEndpointReleases(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_endpoint_releases"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEndpointRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all releases for an endpoint. EndpointReleases are immutable snapshots of " +
			"Endpoint.spec plus the referenced (Cluster)EndpointType.spec at the time they were cut, " +
			"created by the Endpoint controller. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"endpoint_name":  defaultStringProperty(),
		}), []string{"namespace_name", "endpoint_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		EndpointName  string `json:"endpoint_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.ListEndpointReleases(
			ctx, args.NamespaceName, args.EndpointName,
			ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterPEGetEndpointRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_endpoint_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEndpointRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of an endpoint release including the embedded EndpointType " +
			"snapshot and the captured visibility, backends, and parameter values.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint release name. Use list_endpoint_releases to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.PEToolset.GetEndpointRelease(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteEndpointRelease(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_endpoint_release"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteEndpointRelease}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete an endpoint release. EndpointReleases are normally cleaned up by the owning " +
			"Endpoint's finalizer; this is for ad-hoc cleanup of releases no binding pins any more.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint release name. Use list_endpoint_releases to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.DeleteEndpointRelease(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

// ---------------------------------------------------------------------------
// EndpointReleaseBindings — deployment toolset.
// ---------------------------------------------------------------------------

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListEndpointReleaseBindings(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_endpoint_release_bindings"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEndpointReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List EndpointReleaseBindings for an endpoint. Each binding pins an EndpointRelease " +
			"to an environment, resolves its backends there, and renders its routes. Supports pagination " +
			"via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"endpoint_name":  defaultStringProperty(),
		}), []string{"namespace_name", "endpoint_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		EndpointName  string `json:"endpoint_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.ListEndpointReleaseBindings(
			ctx, args.NamespaceName, args.EndpointName,
			ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetEndpointReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_endpoint_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEndpointReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of an EndpointReleaseBinding including the release pin, " +
			"per-env configuration overrides, conditions, and the resolution of each backend in the " +
			"environment (resolved, or the reason it is not, such as NotDeployed).",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name": stringProperty(
				"Binding name. Use list_endpoint_release_bindings to discover valid names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.GetEndpointReleaseBinding(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateEndpointReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_endpoint_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateEndpointReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create an EndpointReleaseBinding that pins an EndpointRelease to an environment and " +
			"renders its routes there. spec.owner and spec.environment are immutable after creation. A " +
			"backend that should receive traffic but is not deployed in the environment blocks rendering " +
			"until it is; advance the pin with update_endpoint_release_binding.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name."),
			"project_name":   stringProperty("Name of the project that owns the endpoint."),
			"endpoint_name":  stringProperty("Name of the parent Endpoint."),
			"environment":    stringProperty("Target environment name."),
			"endpoint_release": stringProperty(
				"Optional: name of the EndpointRelease to pin. Leave unset for a pending binding."),
			"endpoint_type_environment_configs": map[string]any{
				"type": "object",
				"description": "Optional: per-env values for the referenced EndpointType's " +
					"environmentConfigs schema.",
			},
		}, []string{"namespace_name", "name", "project_name", "endpoint_name", "environment"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName                  string                 `json:"namespace_name"`
		Name                           string                 `json:"name"`
		ProjectName                    string                 `json:"project_name"`
		EndpointName                   string                 `json:"endpoint_name"`
		Environment                    string                 `json:"environment"`
		EndpointRelease                string                 `json:"endpoint_release"`
		EndpointTypeEnvironmentConfigs map[string]interface{} `json:"endpoint_type_environment_configs"`
	}) (*mcp.CallToolResult, any, error) {
		spec := gen.EndpointReleaseBindingSpec{Environment: args.Environment}
		spec.Owner.ProjectName = args.ProjectName
		spec.Owner.EndpointName = args.EndpointName
		if args.EndpointRelease != "" {
			spec.EndpointRelease = &args.EndpointRelease
		}
		if args.EndpointTypeEnvironmentConfigs != nil {
			spec.EndpointTypeEnvironmentConfigs = &args.EndpointTypeEnvironmentConfigs
		}
		body := &gen.CreateEndpointReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     &spec,
		}
		result, err := t.DeploymentToolset.CreateEndpointReleaseBinding(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

//nolint:dupl // binding update handlers share similar structure
func (t *Toolsets) RegisterUpdateEndpointReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_endpoint_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateEndpointReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing EndpointReleaseBinding (partial update). spec.owner and " +
			"spec.environment cannot be changed. Use this to advance the release pin (promote a new " +
			"EndpointRelease into an environment) or update environment configuration overrides.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name. Use list_endpoint_release_bindings to discover valid names."),
			"endpoint_release": stringProperty(
				"Optional: name of the EndpointRelease to pin. Use list_endpoint_releases to discover names."),
			"endpoint_type_environment_configs": map[string]any{
				"type":        "object",
				"description": "Optional: replace per-env values for the referenced EndpointType's environmentConfigs.",
			},
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName                  string                 `json:"namespace_name"`
		Name                           string                 `json:"name"`
		EndpointRelease                string                 `json:"endpoint_release"`
		EndpointTypeEnvironmentConfigs map[string]interface{} `json:"endpoint_type_environment_configs"`
	}) (*mcp.CallToolResult, any, error) {
		spec := &gen.EndpointReleaseBindingSpec{}
		if args.EndpointRelease != "" {
			spec.EndpointRelease = &args.EndpointRelease
		}
		if args.EndpointTypeEnvironmentConfigs != nil {
			spec.EndpointTypeEnvironmentConfigs = &args.EndpointTypeEnvironmentConfigs
		}
		body := &gen.UpdateEndpointReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
			Spec:     spec,
		}
		result, err := t.DeploymentToolset.UpdateEndpointReleaseBinding(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteEndpointReleaseBinding(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_endpoint_release_binding"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteEndpointReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete an EndpointReleaseBinding. Its finalizer deletes the underlying RenderedRelease " +
			"first, removing the endpoint's routes from that environment's data plane.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Binding name to delete. Use list_endpoint_release_bindings to discover names."),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.DeploymentToolset.DeleteEndpointReleaseBinding(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}
