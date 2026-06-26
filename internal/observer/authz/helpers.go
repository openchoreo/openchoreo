// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"fmt"
	"log/slog"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// FormatDualScopedResourceName returns the authz-engine identifier for a dual-scoped resource.
// Namespace-scoped resources use "{namespace}/{name}"; cluster-scoped resources use plain "{name}".
// An empty name returns "" so callers can omit the attribute when the scope is not provided.
func FormatDualScopedResourceName(namespace, name string, isClusterScoped bool) string {
	if name == "" {
		return ""
	}
	if isClusterScoped || namespace == "" {
		return name
	}
	return namespace + "/" + name
}

// ComponentScopeAuthz determines the authorization resource type, name, and hierarchy
// from a component search scope (namespace/project/component). The most specific
// non-empty field determines the resource type.
func ComponentScopeAuthz(namespace, project, component string) (ResourceType, string, authzcore.ResourceHierarchy) {
	if namespace == "" && project == "" && component == "" {
		return ResourceTypeUnknown, "", authzcore.ResourceHierarchy{}
	}
	if component != "" {
		return ResourceTypeComponent, component, authzcore.ResourceHierarchy{
			Namespace: namespace,
			Project:   project,
			Component: component,
		}
	}
	if project != "" {
		return ResourceTypeProject, project, authzcore.ResourceHierarchy{
			Namespace: namespace,
			Project:   project,
		}
	}
	return ResourceTypeNamespace, namespace, authzcore.ResourceHierarchy{
		Namespace: namespace,
	}
}

// searchScopeAuthz determines the authorization resource type, name, and hierarchy
// from a component/workflow search scope union, shared by logs and events queries.
func searchScopeAuthz(scope *types.SearchScope) (ResourceType, string, authzcore.ResourceHierarchy, error) {
	if scope == nil {
		return "", "", authzcore.ResourceHierarchy{}, fmt.Errorf("search scope is required")
	}
	if scope.Component != nil {
		c := scope.Component
		rt, rn, h := ComponentScopeAuthz(c.Namespace, c.Project, c.Component)
		return rt, rn, h, nil
	}
	if scope.Workflow != nil {
		w := scope.Workflow
		if w.WorkflowRunName != "" {
			return ResourceTypeWorkflowRun, w.WorkflowRunName,
				authzcore.ResourceHierarchy{Namespace: w.Namespace}, nil
		}
		return ResourceTypeNamespace, w.Namespace,
			authzcore.ResourceHierarchy{Namespace: w.Namespace}, nil
	}
	return "", "", authzcore.ResourceHierarchy{}, fmt.Errorf("invalid search scope")
}

// LogsScopeAuthz determines the authorization resource type, name, and hierarchy
// from a logs query request's search scope.
func LogsScopeAuthz(req *types.LogsQueryRequest) (ResourceType, string, authzcore.ResourceHierarchy, error) {
	if req == nil {
		return "", "", authzcore.ResourceHierarchy{}, fmt.Errorf("request is required")
	}
	return searchScopeAuthz(req.SearchScope)
}

// EventsScopeAuthz determines the authorization resource type, name, and hierarchy
// from an events query request's search scope.
func EventsScopeAuthz(req *types.EventsQueryRequest) (ResourceType, string, authzcore.ResourceHierarchy, error) {
	if req == nil {
		return "", "", authzcore.ResourceHierarchy{}, fmt.Errorf("request is required")
	}
	return searchScopeAuthz(req.SearchScope)
}

// RunsScopeAuthz determines the authorization resource type, name, and hierarchy
// for scheduled-task runs and retries queries. The runs/retries APIs accept a
// component-only scope, which we wrap into the shared SearchScope envelope to
// reuse the same authz pathway as events.
func RunsScopeAuthz(scope *types.ComponentSearchScope) (ResourceType, string, authzcore.ResourceHierarchy, error) {
	if scope == nil {
		return "", "", authzcore.ResourceHierarchy{}, fmt.Errorf("searchScope is required")
	}
	return searchScopeAuthz(&types.SearchScope{Component: scope})
}

// CheckAuthorization performs a complete authorization check for observer operations.
func CheckAuthorization(
	ctx context.Context,
	logger *slog.Logger,
	pdp authzcore.PDP,
	action Action,
	resourceType ResourceType,
	resourceID string,
	hierarchy authzcore.ResourceHierarchy,
	authzCtx authzcore.Context,
) error {
	if pdp == nil {
		logger.Debug("Authorization disabled, skipping check")
		return nil
	}

	// Extract SubjectContext from context (set by JWT middleware)
	authSubjectCtx, ok := auth.GetSubjectContextFromContext(ctx)
	if !ok || authSubjectCtx == nil {
		logger.Warn("No subject context found in request - authentication required",
			"action", action,
			"resource_type", resourceType,
			"resource_id", resourceID)
		return ErrAuthzUnauthorized
	}

	// Convert auth.SubjectContext to authz.SubjectContext
	authzSubjectCtx := authzcore.GetAuthzSubjectContext(authSubjectCtx)

	authzReq := &authzcore.EvaluateRequest{
		SubjectContext: authzSubjectCtx,
		Action:         string(action),
		Resource: authzcore.Resource{
			Type:      string(resourceType),
			ID:        resourceID,
			Hierarchy: hierarchy,
		},
		Context: authzCtx,
	}

	decision, err := pdp.Evaluate(ctx, authzReq)
	if err != nil {
		logger.Error("Failed to evaluate authorization",
			"error", err,
			"action", action,
			"resource_type", resourceType,
			"resource_id", resourceID)
		return fmt.Errorf("authorization evaluation failed: %w", err)
	}

	logger.Debug("Authorization decision received",
		"decision", decision.Decision,
		"action", action,
		"resource_type", resourceType,
		"resource_id", resourceID)

	if !decision.Decision {
		return ErrAuthzForbidden
	}

	return nil
}
