// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListNamespaces(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.NamespaceService.ListNamespaces(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("namespaces", result.Items, result.NextCursor, namespaceSummary), nil
}

func (h *MCPHandler) CreateNamespace(ctx context.Context, req *gen.CreateNamespaceJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		annotations = *req.Metadata.Annotations
	}

	resourceLabels := map[string]string{}
	if req.Metadata.Labels != nil {
		for key, value := range *req.Metadata.Labels {
			resourceLabels[key] = value
		}
	}
	resourceLabels[labels.LabelKeyControlPlaneNamespace] = labels.LabelValueTrue

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Labels:      resourceLabels,
			Annotations: annotations,
		},
	}

	if displayName, ok := ns.Annotations[controller.AnnotationKeyDisplayName]; ok && displayName == "" {
		delete(ns.Annotations, controller.AnnotationKeyDisplayName)
	}
	if description, ok := ns.Annotations[controller.AnnotationKeyDescription]; ok && description == "" {
		delete(ns.Annotations, controller.AnnotationKeyDescription)
	}

	created, err := h.services.NamespaceService.CreateNamespace(ctx, ns)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) ListSecretReferences(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.SecretReferenceService.ListSecretReferences(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("secret_references", result.Items, result.NextCursor, secretReferenceSummary), nil
}
