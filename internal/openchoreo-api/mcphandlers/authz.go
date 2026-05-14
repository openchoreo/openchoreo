// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// ---------------------------------------------------------------------------
// AuthzRole (namespace-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListAuthzRoles(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.AuthzService.ListNamespaceRoles(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("authz_roles", result.Items, result.NextCursor, authzRoleSummary), nil
}

func (h *MCPHandler) GetAuthzRole(ctx context.Context, namespaceName, roleName string) (any, error) {
	role, err := h.services.AuthzService.GetNamespaceRole(ctx, namespaceName, roleName)
	if err != nil {
		return nil, err
	}
	return authzRoleDetail(role), nil
}

func (h *MCPHandler) CreateAuthzRole(
	ctx context.Context, namespaceName string, req *gen.CreateNamespaceRoleJSONRequestBody,
) (any, error) {
	role, err := authzRoleFromRequest(req, namespaceName)
	if err != nil {
		return nil, err
	}
	created, err := h.services.AuthzService.CreateNamespaceRole(ctx, namespaceName, role)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateAuthzRole(
	ctx context.Context, namespaceName string, req *gen.UpdateNamespaceRoleJSONRequestBody,
) (any, error) {
	role, err := authzRoleFromRequest(req, namespaceName)
	if err != nil {
		return nil, err
	}
	updated, err := h.services.AuthzService.UpdateNamespaceRole(ctx, namespaceName, role)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteAuthzRole(ctx context.Context, namespaceName, roleName string) (any, error) {
	if err := h.services.AuthzService.DeleteNamespaceRole(ctx, namespaceName, roleName); err != nil {
		return nil, err
	}
	return map[string]any{"name": roleName, "namespace": namespaceName, "action": "deleted"}, nil
}

// ---------------------------------------------------------------------------
// ClusterAuthzRole (cluster-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListClusterAuthzRoles(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.AuthzService.ListClusterRoles(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_authz_roles", result.Items, result.NextCursor, clusterAuthzRoleSummary), nil
}

func (h *MCPHandler) GetClusterAuthzRole(ctx context.Context, roleName string) (any, error) {
	role, err := h.services.AuthzService.GetClusterRole(ctx, roleName)
	if err != nil {
		return nil, err
	}
	return clusterAuthzRoleDetail(role), nil
}

func (h *MCPHandler) CreateClusterAuthzRole(
	ctx context.Context, req *gen.CreateClusterRoleJSONRequestBody,
) (any, error) {
	role, err := clusterAuthzRoleFromRequest(req)
	if err != nil {
		return nil, err
	}
	created, err := h.services.AuthzService.CreateClusterRole(ctx, role)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateClusterAuthzRole(
	ctx context.Context, req *gen.UpdateClusterRoleJSONRequestBody,
) (any, error) {
	role, err := clusterAuthzRoleFromRequest(req)
	if err != nil {
		return nil, err
	}
	updated, err := h.services.AuthzService.UpdateClusterRole(ctx, role)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteClusterAuthzRole(ctx context.Context, roleName string) (any, error) {
	if err := h.services.AuthzService.DeleteClusterRole(ctx, roleName); err != nil {
		return nil, err
	}
	return map[string]any{"name": roleName, "action": "deleted"}, nil
}

// ---------------------------------------------------------------------------
// AuthzRoleBinding (namespace-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListAuthzRoleBindings(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.AuthzService.ListNamespaceRoleBindings(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList(
		"authz_role_bindings", result.Items, result.NextCursor, authzRoleBindingSummary,
	), nil
}

func (h *MCPHandler) GetAuthzRoleBinding(ctx context.Context, namespaceName, bindingName string) (any, error) {
	binding, err := h.services.AuthzService.GetNamespaceRoleBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}
	return authzRoleBindingDetail(binding), nil
}

func (h *MCPHandler) CreateAuthzRoleBinding(
	ctx context.Context, namespaceName string, req *gen.CreateNamespaceRoleBindingJSONRequestBody,
) (any, error) {
	binding, err := authzRoleBindingFromRequest(req, namespaceName)
	if err != nil {
		return nil, err
	}
	created, err := h.services.AuthzService.CreateNamespaceRoleBinding(ctx, namespaceName, binding)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateAuthzRoleBinding(
	ctx context.Context, namespaceName string, req *gen.UpdateNamespaceRoleBindingJSONRequestBody,
) (any, error) {
	binding, err := authzRoleBindingFromRequest(req, namespaceName)
	if err != nil {
		return nil, err
	}
	updated, err := h.services.AuthzService.UpdateNamespaceRoleBinding(ctx, namespaceName, binding)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteAuthzRoleBinding(ctx context.Context, namespaceName, bindingName string) (any, error) {
	if err := h.services.AuthzService.DeleteNamespaceRoleBinding(ctx, namespaceName, bindingName); err != nil {
		return nil, err
	}
	return map[string]any{"name": bindingName, "namespace": namespaceName, "action": "deleted"}, nil
}

// ---------------------------------------------------------------------------
// ClusterAuthzRoleBinding (cluster-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListClusterAuthzRoleBindings(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.AuthzService.ListClusterRoleBindings(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList(
		"cluster_authz_role_bindings", result.Items, result.NextCursor, clusterAuthzRoleBindingSummary,
	), nil
}

func (h *MCPHandler) GetClusterAuthzRoleBinding(ctx context.Context, bindingName string) (any, error) {
	binding, err := h.services.AuthzService.GetClusterRoleBinding(ctx, bindingName)
	if err != nil {
		return nil, err
	}
	return clusterAuthzRoleBindingDetail(binding), nil
}

func (h *MCPHandler) CreateClusterAuthzRoleBinding(
	ctx context.Context, req *gen.CreateClusterRoleBindingJSONRequestBody,
) (any, error) {
	binding, err := clusterAuthzRoleBindingFromRequest(req)
	if err != nil {
		return nil, err
	}
	created, err := h.services.AuthzService.CreateClusterRoleBinding(ctx, binding)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateClusterAuthzRoleBinding(
	ctx context.Context, req *gen.UpdateClusterRoleBindingJSONRequestBody,
) (any, error) {
	binding, err := clusterAuthzRoleBindingFromRequest(req)
	if err != nil {
		return nil, err
	}
	updated, err := h.services.AuthzService.UpdateClusterRoleBinding(ctx, binding)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteClusterAuthzRoleBinding(ctx context.Context, bindingName string) (any, error) {
	if err := h.services.AuthzService.DeleteClusterRoleBinding(ctx, bindingName); err != nil {
		return nil, err
	}
	return map[string]any{"name": bindingName, "action": "deleted"}, nil
}

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

func (h *MCPHandler) EvaluateAuthz(ctx context.Context, requests []gen.EvaluateRequest) (any, error) {
	callerSubject, _ := auth.GetSubjectContextFromContext(ctx)

	internal := make([]authzcore.EvaluateRequest, len(requests))
	for i, r := range requests {
		var authzCtx authzcore.Context
		if r.Context != nil {
			converted, err := convertSpec[gen.AuthzContext, authzcore.Context](*r.Context)
			if err != nil {
				return nil, err
			}
			authzCtx = converted
		}
		subject := resolveEvaluateSubject(r.SubjectContext, callerSubject)
		internal[i] = authzcore.EvaluateRequest{
			Action: r.Action,
			Resource: authzcore.Resource{
				Type: r.Resource.Type,
				ID:   derefString(r.Resource.Id),
				Hierarchy: authzcore.ResourceHierarchy{
					Namespace: derefString(r.Resource.Hierarchy.Namespace),
					Project:   derefString(r.Resource.Hierarchy.Project),
					Component: derefString(r.Resource.Hierarchy.Component),
					Resource:  derefString(r.Resource.Hierarchy.Resource),
				},
			},
			SubjectContext: subject,
			Context:        authzCtx,
		}
	}

	decisions, err := h.services.AuthzService.Evaluate(ctx, internal)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(decisions))
	for i, d := range decisions {
		entry := map[string]any{"decision": d.Decision}
		if d.Context != nil && d.Context.Reason != "" {
			entry["reason"] = d.Context.Reason
		}
		out[i] = entry
	}
	return map[string]any{"decisions": out}, nil
}

func (h *MCPHandler) ListAuthzActions(ctx context.Context) (any, error) {
	actions, err := h.services.AuthzService.ListActions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(actions))
	for i, a := range actions {
		entry := map[string]any{
			"name":         a.Name,
			"lowest_scope": string(a.LowestScope),
		}
		if len(a.Conditions) > 0 {
			conds := make([]map[string]any, len(a.Conditions))
			for j, c := range a.Conditions {
				conds[j] = map[string]any{"key": c.Key, "description": c.Description}
			}
			entry["conditions"] = conds
		}
		out[i] = entry
	}
	return map[string]any{"actions": out}, nil
}

// ---------------------------------------------------------------------------
// Helpers — request → CRD
// ---------------------------------------------------------------------------

// metadataFromRequest builds a metav1.ObjectMeta from a gen.ObjectMeta request
// payload, applying the standard cleanAnnotations pass. Shared by the four
// authz CRD builders; other PE handlers inline this because each touches only
// one CRD shape.
func metadataFromRequest(meta gen.ObjectMeta, namespace string) metav1.ObjectMeta {
	annotations := map[string]string{}
	if meta.Annotations != nil {
		maps.Copy(annotations, *meta.Annotations)
	}
	cleanAnnotations(annotations)
	out := metav1.ObjectMeta{Name: meta.Name}
	if namespace != "" {
		out.Namespace = namespace
	}
	if len(annotations) > 0 {
		out.Annotations = annotations
	}
	return out
}

func authzRoleFromRequest(req *gen.AuthzRole, namespace string) (*openchoreov1alpha1.AuthzRole, error) {
	role := &openchoreov1alpha1.AuthzRole{ObjectMeta: metadataFromRequest(req.Metadata, namespace)}
	if req.Spec != nil {
		spec, err := convertSpec[gen.AuthzRoleSpec, openchoreov1alpha1.AuthzRoleSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		role.Spec = spec
	}
	return role, nil
}

func clusterAuthzRoleFromRequest(req *gen.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	role := &openchoreov1alpha1.ClusterAuthzRole{ObjectMeta: metadataFromRequest(req.Metadata, "")}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ClusterAuthzRoleSpec, openchoreov1alpha1.ClusterAuthzRoleSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		role.Spec = spec
	}
	return role, nil
}

func authzRoleBindingFromRequest(
	req *gen.AuthzRoleBinding, namespace string,
) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	binding := &openchoreov1alpha1.AuthzRoleBinding{ObjectMeta: metadataFromRequest(req.Metadata, namespace)}
	if req.Spec != nil {
		spec, err := convertSpec[gen.AuthzRoleBindingSpec, openchoreov1alpha1.AuthzRoleBindingSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		binding.Spec = spec
	}
	return binding, nil
}

func clusterAuthzRoleBindingFromRequest(
	req *gen.ClusterAuthzRoleBinding,
) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	binding := &openchoreov1alpha1.ClusterAuthzRoleBinding{ObjectMeta: metadataFromRequest(req.Metadata, "")}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ClusterAuthzRoleBindingSpec, openchoreov1alpha1.ClusterAuthzRoleBindingSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		binding.Spec = spec
	}
	return binding, nil
}

// ---------------------------------------------------------------------------
// Helpers — CRD → response shape
// ---------------------------------------------------------------------------

func authzRoleSummary(r openchoreov1alpha1.AuthzRole) map[string]any {
	m := extractCommonMeta(&r)
	m["actions"] = r.Spec.Actions
	return m
}

func authzRoleDetail(r *openchoreov1alpha1.AuthzRole) map[string]any {
	m := extractCommonMeta(r)
	if spec := specToMap(r.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

func clusterAuthzRoleSummary(r openchoreov1alpha1.ClusterAuthzRole) map[string]any {
	m := extractCommonMeta(&r)
	m["actions"] = r.Spec.Actions
	return m
}

func clusterAuthzRoleDetail(r *openchoreov1alpha1.ClusterAuthzRole) map[string]any {
	m := extractCommonMeta(r)
	if spec := specToMap(r.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

func authzRoleBindingSummary(b openchoreov1alpha1.AuthzRoleBinding) map[string]any {
	m := extractCommonMeta(&b)
	m["entitlement"] = map[string]any{"claim": b.Spec.Entitlement.Claim, "value": b.Spec.Entitlement.Value}
	m["effect"] = string(b.Spec.Effect)
	m["roleMappingCount"] = len(b.Spec.RoleMappings)
	return m
}

func authzRoleBindingDetail(b *openchoreov1alpha1.AuthzRoleBinding) map[string]any {
	m := extractCommonMeta(b)
	if spec := specToMap(b.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

func clusterAuthzRoleBindingSummary(b openchoreov1alpha1.ClusterAuthzRoleBinding) map[string]any {
	m := extractCommonMeta(&b)
	m["entitlement"] = map[string]any{"claim": b.Spec.Entitlement.Claim, "value": b.Spec.Entitlement.Value}
	m["effect"] = string(b.Spec.Effect)
	m["roleMappingCount"] = len(b.Spec.RoleMappings)
	return m
}

func clusterAuthzRoleBindingDetail(b *openchoreov1alpha1.ClusterAuthzRoleBinding) map[string]any {
	m := extractCommonMeta(b)
	if spec := specToMap(b.Spec); len(spec) > 0 {
		m["spec"] = spec
	}
	return m
}

// ---------------------------------------------------------------------------
// Helpers — diagnostics
// ---------------------------------------------------------------------------

// resolveEvaluateSubject returns the subject context to evaluate against.
// Unlike the HTTP /evaluates endpoint, which requires the client to supply a
// subject, the MCP tool defaults to the caller when the request omits one — the
// "can I do X?" case is overwhelmingly the common one for agents.
func resolveEvaluateSubject(genSubject gen.SubjectContext, caller *auth.SubjectContext) *authzcore.SubjectContext {
	if genSubject.EntitlementClaim != "" {
		return &authzcore.SubjectContext{
			Type:              string(genSubject.Type),
			EntitlementClaim:  genSubject.EntitlementClaim,
			EntitlementValues: genSubject.EntitlementValues,
		}
	}
	if caller == nil {
		return &authzcore.SubjectContext{}
	}
	return authzcore.GetAuthzSubjectContext(caller)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
