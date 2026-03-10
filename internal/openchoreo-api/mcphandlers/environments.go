// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListEnvironments(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.EnvironmentService.ListEnvironments(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("environments", result.Items, result.NextCursor, environmentSummary), nil
}

func (h *MCPHandler) CreateEnvironment(ctx context.Context, namespaceName string, req *gen.CreateEnvironmentJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}

	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{},
	}

	if req.Spec != nil && req.Spec.IsProduction != nil {
		env.Spec.IsProduction = *req.Spec.IsProduction
	}
	if req.Spec != nil && req.Spec.DataPlaneRef != nil {
		env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
			Kind: openchoreov1alpha1.DataPlaneRefKind(req.Spec.DataPlaneRef.Kind),
			Name: req.Spec.DataPlaneRef.Name,
		}
	}

	if displayName, ok := env.Annotations[controller.AnnotationKeyDisplayName]; ok && displayName == "" {
		delete(env.Annotations, controller.AnnotationKeyDisplayName)
	}
	if description, ok := env.Annotations[controller.AnnotationKeyDescription]; ok && description == "" {
		delete(env.Annotations, controller.AnnotationKeyDescription)
	}

	created, err := h.services.EnvironmentService.CreateEnvironment(ctx, namespaceName, env)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateEnvironment(ctx context.Context, namespaceName string, req *gen.UpdateEnvironmentJSONRequestBody) (any, error) {
	// Fetch the existing environment first to preserve immutable fields
	existing, err := h.services.EnvironmentService.GetEnvironment(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	// Apply mutable updates
	if req.Metadata.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		maps.Copy(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil && req.Spec.IsProduction != nil {
		existing.Spec.IsProduction = *req.Spec.IsProduction
	}

	updated, err := h.services.EnvironmentService.UpdateEnvironment(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteEnvironment(ctx context.Context, namespaceName, envName string) (any, error) {
	if err := h.services.EnvironmentService.DeleteEnvironment(ctx, namespaceName, envName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      envName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}
