// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListNamespaces(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.NamespaceService.ListNamespaces(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("namespaces", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetNamespace(ctx context.Context, name string) (any, error) {
	return h.services.NamespaceService.GetNamespace(ctx, name)
}

func (h *MCPHandler) CreateNamespace(ctx context.Context, req *models.CreateNamespaceRequest) (any, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				labels.LabelKeyControlPlaneNamespace: labels.LabelValueTrue,
			},
			Annotations: make(map[string]string),
		},
	}

	if req.DisplayName != "" {
		ns.Annotations[controller.AnnotationKeyDisplayName] = req.DisplayName
	}
	if req.Description != "" {
		ns.Annotations[controller.AnnotationKeyDescription] = req.Description
	}

	return h.services.NamespaceService.CreateNamespace(ctx, ns)
}

func (h *MCPHandler) ListSecretReferences(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.SecretReferenceService.ListSecretReferences(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("secret_references", result.Items, result.NextCursor), nil
}
