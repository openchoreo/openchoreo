// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"fmt"
	"log/slog"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// ComponentScopeAuthz determines the authorization resource type, name, and hierarchy
// from a component search scope (namespace/project/component).
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

// CheckAuthorization performs a complete authorization check for RCA agent operations.
func CheckAuthorization(
	ctx context.Context,
	logger *slog.Logger,
	pdp authzcore.PDP,
	action Action,
	resourceType ResourceType,
	resourceID string,
	hierarchy authzcore.ResourceHierarchy,
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
		Context: authzcore.Context{},
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
