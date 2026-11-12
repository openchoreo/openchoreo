// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// endpointBackendArg is one backend_refs entry as an MCP client sends it. It is
// flattened relative to the API shape: the only backend kind today is
// Component, so the component fields sit at the top level.
type endpointBackendArg struct {
	Name        string              `json:"name"`
	Component   string              `json:"component"`
	Endpoint    string              `json:"endpoint"`
	ReleaseLine string              `json:"release_line,omitempty"`
	Weight      *int32              `json:"weight,omitempty"`
	Match       *gen.EndpointMatch  `json:"match,omitempty"`
	Mirror      *gen.EndpointMirror `json:"mirror,omitempty"`
}

func endpointVisibilityProperty(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type": "string",
			"enum": []string{"namespace", "internal", "external"},
		},
	}
}

func endpointBackendRefsProperty(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": stringProperty("Unique backend name within the Endpoint (DNS label), e.g. \"stable\" or \"canary\""),
				"component": stringProperty(
					"Name of the Component that serves this backend, in the Endpoint's project"),
				"endpoint": stringProperty(
					"Name of the workload endpoint on that component. It must have internal or external visibility."),
				"release_line": stringProperty(
					"Optional: the component's release line to route to. Omit for its main line."),
				"weight": map[string]any{
					"type":        "integer",
					"description": "Optional: relative share of traffic (default 1; 0 sends none)",
				},
				"match":  endpointMatchProperty(),
				"mirror": endpointMirrorProperty(),
			},
			"required": []string{"name", "component", "endpoint"},
		},
	}
}

func endpointMatchProperty() map[string]any {
	return map[string]any{
		"type": "object",
		"description": "Optional: requests that meet all of these conditions go to this backend, whatever the " +
			"weights, e.g. a header testers send to reach a canary. Keep it when replacing backend_refs, or the " +
			"route is removed.",
		"properties": map[string]any{
			"path": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":  map[string]any{"type": "string", "enum": []string{"PathPrefix", "Exact"}},
					"value": stringProperty("Path relative to where the Endpoint is served, e.g. /next"),
				},
				"required": []string{"value"},
			},
			"headers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":  stringProperty("Header name"),
						"type":  map[string]any{"type": "string", "enum": []string{"Exact", "RegularExpression"}},
						"value": stringProperty("Header value, or an RE2 expression"),
					},
					"required": []string{"name", "value"},
				},
			},
			"cookies": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":  stringProperty("Cookie name"),
						"value": stringProperty("Cookie value"),
					},
					"required": []string{"name", "value"},
				},
			},
		},
	}
}

func endpointMirrorProperty() map[string]any {
	return map[string]any{
		"type": "object",
		"description": "Optional: copy a share of the weighted requests to this backend; its answers are " +
			"discarded. A mirror backend needs weight 0 and no match.",
		"properties": map[string]any{
			"percent": map[string]any{"type": "integer", "description": "Share of requests to copy, 0 to 100"},
		},
		"required": []string{"percent"},
	}
}

// toEndpointSpecVisibility converts the visibility argument, rejecting values
// the API does not accept.
func toEndpointSpecVisibility(raw []string) ([]gen.EndpointSpecVisibility, error) {
	out := make([]gen.EndpointSpecVisibility, 0, len(raw))
	for _, v := range raw {
		switch gen.EndpointSpecVisibility(v) {
		case gen.EndpointSpecVisibilityNamespace, gen.EndpointSpecVisibilityInternal, gen.EndpointSpecVisibilityExternal:
			out = append(out, gen.EndpointSpecVisibility(v))
		default:
			return nil, fmt.Errorf("visibility %q must be one of: namespace, internal, external", v)
		}
	}
	return out, nil
}

// toEndpointBackendRefs converts the backend_refs argument to the API shape.
func toEndpointBackendRefs(raw []endpointBackendArg) []gen.EndpointBackendRef {
	kind := gen.EndpointBackendRefKind("Component")
	out := make([]gen.EndpointBackendRef, 0, len(raw))
	for _, b := range raw {
		component := &gen.EndpointComponentBackend{Name: b.Component, Endpoint: b.Endpoint}
		if b.ReleaseLine != "" {
			component.ReleaseLine = &b.ReleaseLine
		}
		out = append(out, gen.EndpointBackendRef{
			Name:      b.Name,
			Kind:      &kind,
			Component: component,
			Weight:    b.Weight,
			Match:     b.Match,
			Mirror:    b.Mirror,
		})
	}
	return out
}

//nolint:dupl // paginated list handlers share similar structure
func (t *Toolsets) RegisterListEndpoints(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_endpoints"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEndpoint}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List endpoints in a project. An Endpoint routes traffic to one or more component " +
			"workload endpoints, with relative weights, through an EndpointType or ClusterEndpointType " +
			"template. Supports pagination via limit and cursor.",
		InputSchema: createSchema(addPaginationProperties(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
		}), []string{"namespace_name", "project_name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		ProjectName   string `json:"project_name"`
		Limit         int    `json:"limit,omitempty"`
		Cursor        string `json:"cursor,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.EndpointToolset.ListEndpoints(
			ctx, args.NamespaceName, args.ProjectName, ListOpts{Limit: args.Limit, Cursor: args.Cursor})
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterGetEndpoint(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_endpoint"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEndpoint}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get the full definition of an endpoint including its type reference, visibility, " +
			"backends and weights, parameters, and the latest EndpointRelease pointer. Use list_endpoints " +
			"to discover valid names.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint name. Use list_endpoints to discover valid names"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.EndpointToolset.GetEndpoint(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterCreateEndpoint(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "create_endpoint"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionCreateEndpoint}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Create an endpoint in a project that routes traffic to component workload endpoints " +
			"through an EndpointType (namespace-scoped) or ClusterEndpointType (cluster-scoped) template. " +
			"Each backend names a component and one of its workload endpoints, which must have internal or " +
			"external visibility. Traffic is split by relative weight. Parameters must conform to the " +
			"template's parameters schema; use get_endpoint_type_schema with the matching scope to inspect it. " +
			"Bind a release to an environment with create_endpoint_release_binding to render its routes.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"project_name":   defaultStringProperty(),
			"name": stringProperty(
				"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)"),
			"type_name": stringProperty(
				"Name of the (Cluster)EndpointType to reference. Use list_endpoint_types to discover names."),
			"type_kind": stringProperty(
				"Optional: \"EndpointType\" (default, namespace-scoped) or \"ClusterEndpointType\" (cluster-scoped)."),
			"visibility": endpointVisibilityProperty(
				"Scopes to expose the backends to: one or more of namespace, internal, external."),
			"backend_refs": endpointBackendRefsProperty("Backends to route traffic to, with relative weights."),
			"component_name": stringProperty(
				"Optional: the component that owns the endpoint, so deleting the component deletes it. Every " +
					"backend must route to this component. Omit it to have the project own the endpoint."),
			"release_line_name": stringProperty(
				"Optional: the release line of component_name that owns the endpoint, so deleting the line deletes " +
					"it. Requires component_name, and every backend must route to this line."),
			"display_name": stringProperty("Human-readable display name"),
			"description":  stringProperty("Human-readable description"),
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: parameter values for the referenced (Cluster)EndpointType schema.",
			},
		}, []string{"namespace_name", "project_name", "name", "type_name", "visibility", "backend_refs"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName   string                 `json:"namespace_name"`
		ProjectName     string                 `json:"project_name"`
		ComponentName   string                 `json:"component_name"`
		ReleaseLineName string                 `json:"release_line_name"`
		Name            string                 `json:"name"`
		TypeName        string                 `json:"type_name"`
		TypeKind        string                 `json:"type_kind"`
		Visibility      []string               `json:"visibility"`
		BackendRefs     []endpointBackendArg   `json:"backend_refs"`
		DisplayName     string                 `json:"display_name"`
		Description     string                 `json:"description"`
		Parameters      map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		visibility, err := toEndpointSpecVisibility(args.Visibility)
		if err != nil {
			return nil, nil, err
		}
		annotations := annotationsFromDisplayDesc(args.DisplayName, args.Description)
		typeRef := gen.EndpointTypeRef{Name: args.TypeName}
		if args.TypeKind != "" {
			kind := gen.EndpointTypeRefKind(args.TypeKind)
			typeRef.Kind = &kind
		}
		body := &gen.CreateEndpointJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name, Annotations: &annotations},
			Spec: &gen.EndpointSpec{
				Owner: struct {
					ComponentName   *string `json:"componentName,omitempty"`
					ProjectName     string  `json:"projectName"`
					ReleaseLineName *string `json:"releaseLineName,omitempty"`
				}{ProjectName: args.ProjectName},
				Type:        typeRef,
				Visibility:  visibility,
				BackendRefs: toEndpointBackendRefs(args.BackendRefs),
			},
		}
		if args.ComponentName != "" {
			body.Spec.Owner.ComponentName = &args.ComponentName
		}
		if args.ReleaseLineName != "" {
			body.Spec.Owner.ReleaseLineName = &args.ReleaseLineName
		}
		if args.Parameters != nil {
			body.Spec.Parameters = &args.Parameters
		}
		result, err := t.EndpointToolset.CreateEndpoint(ctx, args.NamespaceName, args.ProjectName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterUpdateEndpoint(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "update_endpoint"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateEndpoint}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Update an existing endpoint (partial update). Only provided fields are applied; omitted " +
			"fields remain unchanged. backend_refs, when provided, replaces the whole backend list, so use it " +
			"to shift traffic weights. The project and spec.type are immutable. A line-owned endpoint whose new " +
			"backend_refs route to another line moves to its component, and a component-owned endpoint whose new " +
			"backend_refs route to another component moves to its project. component_name and release_line_name " +
			"narrow it again once every backend routes to them. Any change cuts a new EndpointRelease, which " +
			"bindings pick up once their pin is advanced.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint name to update. Use list_endpoints to discover valid names"),
			"visibility":     endpointVisibilityProperty("Optional: replace the visibility list."),
			"backend_refs":   endpointBackendRefsProperty("Optional: replace the whole backend list."),
			"component_name": stringProperty(
				"Optional: make this component the owner, so deleting it deletes the endpoint. " +
					"Every backend must route to it."),
			"release_line_name": stringProperty(
				"Optional: make this release line of the owning component the owner, so deleting the line " +
					"deletes the endpoint. Every backend must route to it."),
			"display_name": stringProperty("Optional: update the human-readable display name"),
			"description":  stringProperty("Optional: update the human-readable description"),
			"parameters": map[string]any{
				"type":        "object",
				"description": "Optional: replace parameter values for the referenced (Cluster)EndpointType schema.",
			},
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName   string                 `json:"namespace_name"`
		Name            string                 `json:"name"`
		Visibility      []string               `json:"visibility"`
		BackendRefs     []endpointBackendArg   `json:"backend_refs"`
		ComponentName   string                 `json:"component_name"`
		ReleaseLineName string                 `json:"release_line_name"`
		DisplayName     string                 `json:"display_name"`
		Description     string                 `json:"description"`
		Parameters      map[string]interface{} `json:"parameters"`
	}) (*mcp.CallToolResult, any, error) {
		body := &gen.UpdateEndpointJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: args.Name},
		}
		if args.DisplayName != "" || args.Description != "" {
			annotations := annotationsFromDisplayDesc(args.DisplayName, args.Description)
			body.Metadata.Annotations = &annotations
		}
		if args.Visibility != nil || args.BackendRefs != nil || args.Parameters != nil ||
			args.ComponentName != "" || args.ReleaseLineName != "" {
			body.Spec = &gen.EndpointSpec{}
			if args.Visibility != nil {
				visibility, err := toEndpointSpecVisibility(args.Visibility)
				if err != nil {
					return nil, nil, err
				}
				body.Spec.Visibility = visibility
			}
			if args.BackendRefs != nil {
				body.Spec.BackendRefs = toEndpointBackendRefs(args.BackendRefs)
			}
			if args.Parameters != nil {
				body.Spec.Parameters = &args.Parameters
			}
			if args.ComponentName != "" {
				body.Spec.Owner.ComponentName = &args.ComponentName
			}
			if args.ReleaseLineName != "" {
				body.Spec.Owner.ReleaseLineName = &args.ReleaseLineName
			}
		}
		result, err := t.EndpointToolset.UpdateEndpoint(ctx, args.NamespaceName, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterDeleteEndpoint(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "delete_endpoint"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionDeleteEndpoint}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Delete an endpoint. Its finalizer deletes the endpoint's EndpointReleaseBindings (removing " +
			"their routes from the data plane) and then its EndpointReleases.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint name to delete. Use list_endpoints to discover valid names"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.EndpointToolset.DeleteEndpoint(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}
