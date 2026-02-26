// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListEnvironments(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.EnvironmentService.ListEnvironments(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("environments", result.Items, result.NextCursor, environmentSummary), nil
}

func (h *MCPHandler) GetEnvironment(ctx context.Context, namespaceName, envName string) (any, error) {
	env, err := h.services.EnvironmentService.GetEnvironment(ctx, namespaceName, envName)
	if err != nil {
		return nil, err
	}
	return environmentDetail(env), nil
}

func (h *MCPHandler) CreateEnvironment(ctx context.Context, namespaceName string, req *models.CreateEnvironmentRequest) (any, error) {
	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   namespaceName,
			Annotations: make(map[string]string),
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			IsProduction: req.IsProduction,
		},
	}

	if req.DisplayName != "" {
		env.Annotations[controller.AnnotationKeyDisplayName] = req.DisplayName
	}
	if req.Description != "" {
		env.Annotations[controller.AnnotationKeyDescription] = req.Description
	}
	if req.DataPlaneRef != nil {
		env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
			Kind: openchoreov1alpha1.DataPlaneRefKind(req.DataPlaneRef.Kind),
			Name: req.DataPlaneRef.Name,
		}
	}

	created, err := h.services.EnvironmentService.CreateEnvironment(ctx, namespaceName, env)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}
