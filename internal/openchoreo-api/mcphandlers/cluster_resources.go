// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
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

func (h *MCPHandler) CreateClusterDataPlane(ctx context.Context, req *gen.CreateClusterDataPlaneJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		annotations = *req.Metadata.Annotations
	}

	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Annotations: annotations,
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{},
	}

	if req.Spec != nil && req.Spec.PlaneID != nil {
		cdp.Spec.PlaneID = *req.Spec.PlaneID
	}
	if req.Spec != nil && req.Spec.ClusterAgent != nil && req.Spec.ClusterAgent.ClientCA != nil {
		if req.Spec.ClusterAgent.ClientCA.Value != nil {
			cdp.Spec.ClusterAgent.ClientCA.Value = *req.Spec.ClusterAgent.ClientCA.Value
		}
		if req.Spec.ClusterAgent.ClientCA.SecretRef != nil &&
			req.Spec.ClusterAgent.ClientCA.SecretRef.Name != nil &&
			req.Spec.ClusterAgent.ClientCA.SecretRef.Key != nil {
			secretNamespace := ""
			if req.Spec.ClusterAgent.ClientCA.SecretRef.Namespace != nil {
				secretNamespace = *req.Spec.ClusterAgent.ClientCA.SecretRef.Namespace
			}
			cdp.Spec.ClusterAgent.ClientCA.SecretRef = &openchoreov1alpha1.SecretKeyReference{
				Name:      *req.Spec.ClusterAgent.ClientCA.SecretRef.Name,
				Namespace: secretNamespace,
				Key:       *req.Spec.ClusterAgent.ClientCA.SecretRef.Key,
			}
		}
	}
	if req.Spec != nil && req.Spec.ObservabilityPlaneRef != nil {
		cdp.Spec.ObservabilityPlaneRef = &openchoreov1alpha1.ClusterObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKind(req.Spec.ObservabilityPlaneRef.Kind),
			Name: req.Spec.ObservabilityPlaneRef.Name,
		}
	}

	if displayName, ok := cdp.Annotations[controller.AnnotationKeyDisplayName]; ok && displayName == "" {
		delete(cdp.Annotations, controller.AnnotationKeyDisplayName)
	}
	if description, ok := cdp.Annotations[controller.AnnotationKeyDescription]; ok && description == "" {
		delete(cdp.Annotations, controller.AnnotationKeyDescription)
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
