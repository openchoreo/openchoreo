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

func (h *MCPHandler) CreateDataPlane(ctx context.Context, namespaceName string, req *models.CreateDataPlaneRequest) (any, error) {
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   namespaceName,
			Annotations: make(map[string]string),
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: req.ClusterAgentClientCA,
				},
			},
		},
	}

	if req.DisplayName != "" {
		dp.Annotations[controller.AnnotationKeyDisplayName] = req.DisplayName
	}
	if req.Description != "" {
		dp.Annotations[controller.AnnotationKeyDescription] = req.Description
	}
	if req.ObservabilityPlaneRef != nil {
		dp.Spec.ObservabilityPlaneRef = &openchoreov1alpha1.ObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ObservabilityPlaneRefKind(req.ObservabilityPlaneRef.Kind),
			Name: req.ObservabilityPlaneRef.Name,
		}
	}

	created, err := h.services.DataPlaneService.CreateDataPlane(ctx, namespaceName, dp)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}
