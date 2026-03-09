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

func (h *MCPHandler) ListObservabilityPlanes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ObservabilityPlaneService.ListObservabilityPlanes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("observability_planes", result.Items, result.NextCursor, observabilityPlaneSummary), nil
}

func (h *MCPHandler) GetObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) (any, error) {
	op, err := h.services.ObservabilityPlaneService.GetObservabilityPlane(ctx, namespaceName, observabilityPlaneName)
	if err != nil {
		return nil, err
	}
	return observabilityPlaneDetail(op), nil
}

func (h *MCPHandler) GetDeploymentPipeline(ctx context.Context, namespaceName, pipelineName string) (any, error) {
	dp, err := h.services.DeploymentPipelineService.GetDeploymentPipeline(ctx, namespaceName, pipelineName)
	if err != nil {
		return nil, err
	}
	return deploymentPipelineDetail(dp), nil
}

func (h *MCPHandler) ListDeploymentPipelines(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.DeploymentPipelineService.ListDeploymentPipelines(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("deployment_pipelines", result.Items, result.NextCursor, deploymentPipelineSummary), nil
}

func (h *MCPHandler) CreateDeploymentPipeline(ctx context.Context, namespaceName string, req *gen.CreateDeploymentPipelineJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}

	dp := &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
	}

	if req.Spec != nil && req.Spec.PromotionPaths != nil {
		paths := make([]openchoreov1alpha1.PromotionPath, 0, len(*req.Spec.PromotionPaths))
		for _, p := range *req.Spec.PromotionPaths {
			targets := make([]openchoreov1alpha1.TargetEnvironmentRef, 0, len(p.TargetEnvironmentRefs))
			for _, t := range p.TargetEnvironmentRefs {
				ref := openchoreov1alpha1.TargetEnvironmentRef{
					Name: t.Name,
				}
				if t.RequiresApproval != nil {
					ref.RequiresApproval = *t.RequiresApproval
				}
				targets = append(targets, ref)
			}
			paths = append(paths, openchoreov1alpha1.PromotionPath{
				SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{
					Name: p.SourceEnvironmentRef.Name,
				},
				TargetEnvironmentRefs: targets,
			})
		}
		dp.Spec.PromotionPaths = paths
	}

	if displayName, ok := dp.Annotations[controller.AnnotationKeyDisplayName]; ok && displayName == "" {
		delete(dp.Annotations, controller.AnnotationKeyDisplayName)
	}
	if description, ok := dp.Annotations[controller.AnnotationKeyDescription]; ok && description == "" {
		delete(dp.Annotations, controller.AnnotationKeyDescription)
	}

	created, err := h.services.DeploymentPipelineService.CreateDeploymentPipeline(ctx, namespaceName, dp)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) ListBuildPlanes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.BuildPlaneService.ListBuildPlanes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("build_planes", result.Items, result.NextCursor, buildPlaneSummary), nil
}

func (h *MCPHandler) GetBuildPlane(ctx context.Context, namespaceName, buildPlaneName string) (any, error) {
	bp, err := h.services.BuildPlaneService.GetBuildPlane(ctx, namespaceName, buildPlaneName)
	if err != nil {
		return nil, err
	}
	return buildPlaneDetail(bp), nil
}

func (h *MCPHandler) GetResourceEvents(ctx context.Context, namespaceName, releaseBindingName, group, version, kind, name string) (any, error) {
	result, err := h.services.K8sResourcesService.GetResourceEvents(ctx, namespaceName, releaseBindingName, group, version, kind, name)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *MCPHandler) GetResourceLogs(ctx context.Context, namespaceName, releaseBindingName, podName string, sinceSeconds *int64) (any, error) {
	result, err := h.services.K8sResourcesService.GetResourceLogs(ctx, namespaceName, releaseBindingName, podName, sinceSeconds)
	if err != nil {
		return nil, err
	}
	return result, nil
}
