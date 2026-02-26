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

// ClusterDataPlane operations

func (h *MCPHandler) ListClusterDataPlanes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterDataPlaneService.ListClusterDataPlanes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_data_planes", result.Items, result.NextCursor, clusterDataPlaneSummary), nil
}

func (h *MCPHandler) GetClusterDataPlane(ctx context.Context, cdpName string) (any, error) {
	cdp, err := h.services.ClusterDataPlaneService.GetClusterDataPlane(ctx, cdpName)
	if err != nil {
		return nil, err
	}
	return clusterDataPlaneDetail(cdp), nil
}

func (h *MCPHandler) CreateClusterDataPlane(ctx context.Context, req *models.CreateClusterDataPlaneRequest) (any, error) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Annotations: make(map[string]string),
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: req.PlaneID,
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: req.ClusterAgentClientCA,
				},
			},
		},
	}

	if req.DisplayName != "" {
		cdp.Annotations[controller.AnnotationKeyDisplayName] = req.DisplayName
	}
	if req.Description != "" {
		cdp.Annotations[controller.AnnotationKeyDescription] = req.Description
	}
	if req.ObservabilityPlaneRef != nil {
		cdp.Spec.ObservabilityPlaneRef = &openchoreov1alpha1.ClusterObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKind(req.ObservabilityPlaneRef.Kind),
			Name: req.ObservabilityPlaneRef.Name,
		}
	}

	created, err := h.services.ClusterDataPlaneService.CreateClusterDataPlane(ctx, cdp)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

// ClusterBuildPlane operations

func (h *MCPHandler) ListClusterBuildPlanes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterBuildPlaneService.ListClusterBuildPlanes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_build_planes", result.Items, result.NextCursor, clusterBuildPlaneSummary), nil
}

// ClusterObservabilityPlane operations

func (h *MCPHandler) ListClusterObservabilityPlanes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterObservabilityPlaneService.ListClusterObservabilityPlanes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_observability_planes", result.Items, result.NextCursor, clusterObservabilityPlaneSummary), nil
}
