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

// ---------------------------------------------------------------------------
// Authz CRDs — scope-collapsed canonical tools.
//
// Roles and role bindings each have one scope-collapsed family (5 CRUD + 1
// creation_schema). Plus two flat diagnostic tools (evaluate_authz, list_authz_actions).
// No deprecated cluster-prefixed aliases — these tools are net-new and ship
// scope-collapsed from day one.
// ---------------------------------------------------------------------------

const (
	scopedAuthzRoleNoun        = "authz role"
	scopedAuthzRoleBindingNoun = "authz role binding"
)

// ---------------------------------------------------------------------------
// AuthzRole / ClusterAuthzRole
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListAuthzRoles(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_authz_roles", scopedAuthzRoleNoun,
		"List authz roles. With scope=\"namespace\" (default) lists a namespace's AuthzRoles "+
			"(requires namespace_name); with scope=\"cluster\" lists platform-wide ClusterAuthzRoles. "+
			"Roles define what actions a subject is permitted to perform. Supports pagination via limit and cursor.",
		authzcore.ActionViewAuthzRole, authzcore.ActionViewClusterAuthzRole,
		scopedListHandlers{
			namespace: t.PEToolset.ListAuthzRoles,
			cluster:   t.PEToolset.ListClusterAuthzRoles,
		})
}

func (t *Toolsets) RegisterGetAuthzRole(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_authz_role", scopedAuthzRoleNoun,
		"Get the full definition of an authz role including its complete spec (actions, description). "+
			"Use scope=\"cluster\" for a platform-wide ClusterAuthzRole. Call before update_authz_role.",
		authzcore.ActionViewAuthzRole, authzcore.ActionViewClusterAuthzRole,
		"name", "Authz role name. Use list_authz_roles to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetAuthzRole,
			cluster:   t.PEToolset.GetClusterAuthzRole,
		})
}

func (t *Toolsets) RegisterGetAuthzRoleCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_authz_role_creation_schema", scopedAuthzRoleNoun,
		"Get the JSON schema for the AuthzRole spec. Use scope=\"cluster\" for the ClusterAuthzRole "+
			"variant. Use list_authz_actions to discover valid values for spec.actions[].",
		authzcore.ActionViewAuthzRole, authzcore.ActionViewClusterAuthzRole,
		scopedSchemaProviders{
			namespace: func() (any, error) { return AuthzRoleCreationSchema() },
			cluster:   func() (any, error) { return ClusterAuthzRoleCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateAuthzRole(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_authz_role", scopedAuthzRoleNoun,
		"Create an authz role. With scope=\"namespace\" (default) creates an AuthzRole in namespace_name; "+
			"with scope=\"cluster\" creates a platform-wide ClusterAuthzRole. spec.actions[] lists "+
			"permitted actions (e.g. \"component:create\", \"component:*\", \"*\"). Use list_authz_actions "+
			"to discover valid action names.",
		authzcore.ActionCreateAuthzRole, authzcore.ActionCreateClusterAuthzRole,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"AuthzRoleSpec — must include actions[] (min 1). Optional: description.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.AuthzRoleSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateAuthzRole(ctx, ns, &gen.CreateNamespaceRoleJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterAuthzRoleSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterAuthzRole(ctx, &gen.CreateClusterRoleJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateAuthzRole(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_authz_role", scopedAuthzRoleNoun,
		"Update an existing authz role (full-spec replacement). Use scope=\"cluster\" for a platform-wide "+
			"ClusterAuthzRole. Use get_authz_role to retrieve the current spec first.",
		authzcore.ActionUpdateAuthzRole, authzcore.ActionUpdateClusterAuthzRole,
		"Name of the authz role to update. Use list_authz_roles to discover valid names",
		"Full AuthzRoleSpec to replace the existing one. Use get_authz_role to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.AuthzRoleSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateAuthzRole(ctx, ns, &gen.UpdateNamespaceRoleJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterAuthzRoleSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterAuthzRole(ctx, &gen.UpdateClusterRoleJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterDeleteAuthzRole(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_authz_role", scopedAuthzRoleNoun,
		"Delete an authz role. Use scope=\"cluster\" for a platform-wide ClusterAuthzRole. "+
			"WARNING: deleting a role in use leaves any AuthzRoleBinding referencing it dangling; "+
			"call list_authz_role_bindings first to check.",
		authzcore.ActionDeleteAuthzRole, authzcore.ActionDeleteClusterAuthzRole,
		"name", "Name of the authz role to delete. Use list_authz_roles to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteAuthzRole,
			cluster:   t.PEToolset.DeleteClusterAuthzRole,
		})
}

// ---------------------------------------------------------------------------
// AuthzRoleBinding / ClusterAuthzRoleBinding
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterListAuthzRoleBindings(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedListTool(s, perms, "list_authz_role_bindings", scopedAuthzRoleBindingNoun,
		"List authz role bindings. With scope=\"namespace\" (default) lists AuthzRoleBindings in namespace_name; "+
			"with scope=\"cluster\" lists platform-wide ClusterAuthzRoleBindings. Bindings connect a subject "+
			"(entitlement claim) to one or more roles with optional per-mapping scope. "+
			"Supports pagination via limit and cursor.",
		authzcore.ActionViewAuthzRoleBinding, authzcore.ActionViewClusterAuthzRoleBinding,
		scopedListHandlers{
			namespace: t.PEToolset.ListAuthzRoleBindings,
			cluster:   t.PEToolset.ListClusterAuthzRoleBindings,
		})
}

func (t *Toolsets) RegisterGetAuthzRoleBinding(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "get_authz_role_binding", scopedAuthzRoleBindingNoun,
		"Get the full definition of an authz role binding including its entitlement, role mappings, and "+
			"effect. Use scope=\"cluster\" for a platform-wide ClusterAuthzRoleBinding. Call before "+
			"update_authz_role_binding.",
		authzcore.ActionViewAuthzRoleBinding, authzcore.ActionViewClusterAuthzRoleBinding,
		"name", "Authz role binding name. Use list_authz_role_bindings to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.GetAuthzRoleBinding,
			cluster:   t.PEToolset.GetClusterAuthzRoleBinding,
		})
}

func (t *Toolsets) RegisterGetAuthzRoleBindingCreationSchema(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSchemaTool(s, perms, "get_authz_role_binding_creation_schema", scopedAuthzRoleBindingNoun,
		"Get the JSON schema for the role binding spec. The schema differs between scopes: cluster "+
			"bindings reference only ClusterAuthzRole and allow scope.namespace; namespace bindings "+
			"can reference either AuthzRole or ClusterAuthzRole. Both expose scope.project / "+
			"scope.component / scope.resource — scope.component and scope.resource are sibling "+
			"sub-scopes under project and are mutually exclusive. Optional conditions[] applies "+
			"CEL-based ABAC restrictions to specific actions.",
		authzcore.ActionViewAuthzRoleBinding, authzcore.ActionViewClusterAuthzRoleBinding,
		scopedSchemaProviders{
			namespace: func() (any, error) { return AuthzRoleBindingCreationSchema() },
			cluster:   func() (any, error) { return ClusterAuthzRoleBindingCreationSchema() },
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterCreateAuthzRoleBinding(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "create_authz_role_binding", scopedAuthzRoleBindingNoun,
		"Create an authz role binding. With scope=\"namespace\" (default) creates an AuthzRoleBinding in "+
			"namespace_name (can reference both AuthzRole and ClusterAuthzRole); with scope=\"cluster\" "+
			"creates a ClusterAuthzRoleBinding (ClusterAuthzRole only, scope can include namespace). "+
			"Use get_authz_role_binding_creation_schema for the per-scope spec shape.",
		authzcore.ActionCreateAuthzRoleBinding, authzcore.ActionCreateClusterAuthzRoleBinding,
		"DNS-compatible identifier (lowercase, alphanumeric, hyphens only, max 63 chars)",
		"Binding spec — must include entitlement and roleMappings[]. Optional: effect (allow|deny, default allow).",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.AuthzRoleBindingSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateAuthzRoleBinding(ctx, ns, &gen.CreateNamespaceRoleBindingJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterAuthzRoleBindingSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.CreateClusterAuthzRoleBinding(ctx, &gen.CreateClusterRoleBindingJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

//nolint:dupl // create/update register helpers share a near-identical shape per resource
func (t *Toolsets) RegisterUpdateAuthzRoleBinding(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedWriteTool(s, perms, "update_authz_role_binding", scopedAuthzRoleBindingNoun,
		"Update an existing authz role binding (full-spec replacement). Use scope=\"cluster\" for a "+
			"platform-wide ClusterAuthzRoleBinding. Use get_authz_role_binding to retrieve the current spec first.",
		authzcore.ActionUpdateAuthzRoleBinding, authzcore.ActionUpdateClusterAuthzRoleBinding,
		"Name of the role binding to update. Use list_authz_role_bindings to discover valid names",
		"Full binding spec to replace the existing one. Use get_authz_role_binding to retrieve the current spec first.",
		scopedWriteHandlers{
			namespace: func(ctx context.Context, ns, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.AuthzRoleBindingSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateAuthzRoleBinding(ctx, ns, &gen.UpdateNamespaceRoleBindingJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
			cluster: func(ctx context.Context, name string, anns map[string]string, specRaw map[string]any) (any, error) {
				spec, err := buildSpec[gen.ClusterAuthzRoleBindingSpec](specRaw)
				if err != nil {
					return nil, err
				}
				return t.PEToolset.UpdateClusterAuthzRoleBinding(ctx, &gen.UpdateClusterRoleBindingJSONRequestBody{
					Metadata: gen.ObjectMeta{Name: name, Annotations: &anns}, Spec: spec,
				})
			},
		})
}

func (t *Toolsets) RegisterDeleteAuthzRoleBinding(s *mcp.Server, perms map[string]ToolPermission) {
	registerScopedSingleResourceTool(s, perms, "delete_authz_role_binding", scopedAuthzRoleBindingNoun,
		"Delete an authz role binding. Use scope=\"cluster\" for a platform-wide ClusterAuthzRoleBinding. "+
			"Revokes only what this binding granted; the referenced role remains.",
		authzcore.ActionDeleteAuthzRoleBinding, authzcore.ActionDeleteClusterAuthzRoleBinding,
		"name", "Name of the role binding to delete. Use list_authz_role_bindings to discover valid names",
		scopedSingleResourceHandlers{
			namespace: t.PEToolset.DeleteAuthzRoleBinding,
			cluster:   t.PEToolset.DeleteClusterAuthzRoleBinding,
		})
}

// ---------------------------------------------------------------------------
// Diagnostics — flat (not scope-collapsed)
// ---------------------------------------------------------------------------

func (t *Toolsets) RegisterEvaluateAuthz(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "evaluate_authz"
	// Gate on authzrolebinding:view: a caller inspecting authorization decisions should be
	// able to see the bindings that produced them. The service layer is itself ungated for
	// the caller's own subject; this gate controls tools/list visibility only.
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewAuthzRoleBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Evaluate one or more authorization requests and return allow/deny decisions. " +
			"Used to debug \"why am I getting 403?\" — pass the action and resource the caller tried to " +
			"perform; the response includes the matching binding chain when a decision is denied. " +
			"Each request must include action, resource.type, and subject_context (use the caller's own " +
			"identity to ask \"can I do X?\").",
		InputSchema: createSchema(map[string]any{
			"requests": map[string]any{
				"type": "array",
				"description": "List of evaluation requests. Each: {action, " +
					"resource: {type, id?, hierarchy?}, subject_context: " +
					"{type, entitlement_claim, entitlement_values[]}, context?}.",
				"items":    map[string]any{"type": "object"},
				"minItems": 1,
			},
		}, []string{"requests"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Requests []map[string]interface{} `json:"requests"`
	}) (*mcp.CallToolResult, any, error) {
		if len(args.Requests) == 0 {
			return nil, nil, fmt.Errorf("requests must contain at least one item")
		}
		typed := make([]gen.EvaluateRequest, 0, len(args.Requests))
		for i, raw := range args.Requests {
			var r gen.EvaluateRequest
			if err := decodeSpecStrict(raw, &r); err != nil {
				return nil, nil, fmt.Errorf("requests[%d]: %w", i, err)
			}
			typed = append(typed, r)
		}
		return handleToolResult(t.PEToolset.EvaluateAuthz(ctx, typed))
	})
}

func (t *Toolsets) RegisterListAuthzActions(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "list_authz_actions"
	// Gate on authzrole:view: the catalog is read alongside role authoring. The service
	// layer is ungated; this gate controls tools/list visibility only.
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewAuthzRole}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "List all authorization actions defined in the system. Each entry has a name (e.g. " +
			"\"component:view\") and the lowest scope it applies at (cluster|namespace|project|component). " +
			"Use this to discover valid values for AuthzRole.spec.actions[] before calling create_authz_role.",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return handleToolResult(t.PEToolset.ListAuthzActions(ctx))
	})
}
