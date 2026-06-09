// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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

func (h *MCPHandler) GetSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (any, error) {
	sr, err := h.services.SecretReferenceService.GetSecretReference(ctx, namespaceName, secretReferenceName)
	if err != nil {
		return nil, err
	}
	return secretReferenceDetail(sr), nil
}

func (h *MCPHandler) CreateSecretReference(ctx context.Context, namespaceName string, req *gen.CreateSecretReferenceJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	sr := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.SecretReferenceSpec, openchoreov1alpha1.SecretReferenceSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		sr.Spec = spec
	}

	created, err := h.services.SecretReferenceService.CreateSecretReference(ctx, namespaceName, sr)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateSecretReference(ctx context.Context, namespaceName string, req *gen.UpdateSecretReferenceJSONRequestBody) (any, error) {
	existing, err := h.services.SecretReferenceService.GetSecretReference(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		maps.Copy(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.SecretReferenceSpec, openchoreov1alpha1.SecretReferenceSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.SecretReferenceService.UpdateSecretReference(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteSecretReference(ctx context.Context, namespaceName, secretReferenceName string) (any, error) {
	if err := h.services.SecretReferenceService.DeleteSecretReference(ctx, namespaceName, secretReferenceName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      secretReferenceName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}
