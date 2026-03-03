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

func (h *MCPHandler) ListDataPlanes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.DataPlaneService.ListDataPlanes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("data_planes", result.Items, result.NextCursor, dataplaneSummary), nil
}

func (h *MCPHandler) GetDataPlane(ctx context.Context, namespaceName, dpName string) (any, error) {
	dp, err := h.services.DataPlaneService.GetDataPlane(ctx, namespaceName, dpName)
	if err != nil {
		return nil, err
	}
	return dataplaneDetail(dp), nil
}

func (h *MCPHandler) CreateDataPlane(ctx context.Context, namespaceName string, req *gen.CreateDataPlaneJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}

	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{},
	}

	if req.Spec != nil && req.Spec.ClusterAgent != nil && req.Spec.ClusterAgent.ClientCA != nil {
		if req.Spec.ClusterAgent.ClientCA.Value != nil {
			dp.Spec.ClusterAgent.ClientCA.Value = *req.Spec.ClusterAgent.ClientCA.Value
		}
		if req.Spec.ClusterAgent.ClientCA.SecretRef != nil &&
			req.Spec.ClusterAgent.ClientCA.SecretRef.Name != nil &&
			req.Spec.ClusterAgent.ClientCA.SecretRef.Key != nil {
			secretNamespace := ""
			if req.Spec.ClusterAgent.ClientCA.SecretRef.Namespace != nil {
				secretNamespace = *req.Spec.ClusterAgent.ClientCA.SecretRef.Namespace
			}
			dp.Spec.ClusterAgent.ClientCA.SecretRef = &openchoreov1alpha1.SecretKeyReference{
				Name:      *req.Spec.ClusterAgent.ClientCA.SecretRef.Name,
				Namespace: secretNamespace,
				Key:       *req.Spec.ClusterAgent.ClientCA.SecretRef.Key,
			}
		}
	}
	if req.Spec != nil && req.Spec.ObservabilityPlaneRef != nil {
		dp.Spec.ObservabilityPlaneRef = &openchoreov1alpha1.ObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ObservabilityPlaneRefKind(req.Spec.ObservabilityPlaneRef.Kind),
			Name: req.Spec.ObservabilityPlaneRef.Name,
		}
	}

	if displayName, ok := dp.Annotations[controller.AnnotationKeyDisplayName]; ok && displayName == "" {
		delete(dp.Annotations, controller.AnnotationKeyDisplayName)
	}
	if description, ok := dp.Annotations[controller.AnnotationKeyDescription]; ok && description == "" {
		delete(dp.Annotations, controller.AnnotationKeyDescription)
	}

	created, err := h.services.DataPlaneService.CreateDataPlane(ctx, namespaceName, dp)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}
