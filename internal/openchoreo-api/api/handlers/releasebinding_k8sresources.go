// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	k8sresourcessvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
)

// GetReleaseBindingK8sResourceTree returns all live Kubernetes resources deployed by the releases
// owned by a release binding.
func (h *Handler) GetReleaseBindingK8sResourceTree(
	ctx context.Context,
	request gen.GetReleaseBindingK8sResourceTreeRequestObject,
) (gen.GetReleaseBindingK8sResourceTreeResponseObject, error) {
	h.logger.Debug("GetReleaseBindingK8sResourceTree called",
		"namespace", request.NamespaceName,
		"releaseBinding", request.ReleaseBindingName)

	result, err := h.services.K8sResourcesService.GetResourceTree(ctx, request.NamespaceName, request.ReleaseBindingName)
	if err != nil {
		return h.handleK8sResourceTreeError(err)
	}

	genReleases := make([]gen.ReleaseResourceTree, 0, len(result.Releases))
	for _, r := range result.Releases {
		nodes, err := convertList[models.ResourceNode, gen.ResourceNode](r.Nodes)
		if err != nil {
			h.logger.Error("Failed to convert resource nodes", "error", err)
			return gen.GetReleaseBindingK8sResourceTree500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
		}
		genReleases = append(genReleases, gen.ReleaseResourceTree{
			Name:        r.Name,
			TargetPlane: gen.ReleaseResourceTreeTargetPlane(r.TargetPlane),
			Nodes:       nodes,
		})
	}

	return gen.GetReleaseBindingK8sResourceTree200JSONResponse{
		Releases: genReleases,
	}, nil
}

// GetReleaseBindingK8sResourceEvents returns Kubernetes events for a specific resource
// in the release binding's resource tree.
func (h *Handler) GetReleaseBindingK8sResourceEvents(
	ctx context.Context,
	request gen.GetReleaseBindingK8sResourceEventsRequestObject,
) (gen.GetReleaseBindingK8sResourceEventsResponseObject, error) {
	h.logger.Debug("GetReleaseBindingK8sResourceEvents called",
		"namespace", request.NamespaceName,
		"releaseBinding", request.ReleaseBindingName,
		"kind", request.Params.Kind,
		"name", request.Params.Name)

	group := ""
	if request.Params.Group != nil {
		group = *request.Params.Group
	}

	resp, err := h.services.K8sResourcesService.GetResourceEvents(
		ctx,
		request.NamespaceName,
		request.ReleaseBindingName,
		group,
		request.Params.Version,
		request.Params.Kind,
		request.Params.Name,
	)
	if err != nil {
		return h.handleK8sResourceEventsError(err)
	}

	result, err := convert[models.ResourceEventsResponse, gen.ResourceEventsResponse](*resp)
	if err != nil {
		h.logger.Error("Failed to convert resource events response", "error", err)
		return gen.GetReleaseBindingK8sResourceEvents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetReleaseBindingK8sResourceEvents200JSONResponse(result), nil
}

// GetReleaseBindingK8sResourceLogs returns logs for a specific pod in the release binding's resource tree.
func (h *Handler) GetReleaseBindingK8sResourceLogs(
	ctx context.Context,
	request gen.GetReleaseBindingK8sResourceLogsRequestObject,
) (gen.GetReleaseBindingK8sResourceLogsResponseObject, error) {
	h.logger.Debug("GetReleaseBindingK8sResourceLogs called",
		"namespace", request.NamespaceName,
		"releaseBinding", request.ReleaseBindingName,
		"podName", request.Params.PodName)

	resp, err := h.services.K8sResourcesService.GetResourceLogs(
		ctx,
		request.NamespaceName,
		request.ReleaseBindingName,
		request.Params.PodName,
		request.Params.SinceSeconds,
	)
	if err != nil {
		return h.handleK8sResourceLogsError(err)
	}

	result, err := convert[models.ResourcePodLogsResponse, gen.ResourcePodLogsResponse](*resp)
	if err != nil {
		h.logger.Error("Failed to convert resource pod logs response", "error", err)
		return gen.GetReleaseBindingK8sResourceLogs500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetReleaseBindingK8sResourceLogs200JSONResponse(result), nil
}

func (h *Handler) handleK8sResourceTreeError(err error) (gen.GetReleaseBindingK8sResourceTreeResponseObject, error) {
	if errors.Is(err, services.ErrForbidden) {
		return gen.GetReleaseBindingK8sResourceTree403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseBindingNotFound) {
		return gen.GetReleaseBindingK8sResourceTree404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseNotFound) {
		return gen.GetReleaseBindingK8sResourceTree404JSONResponse{NotFoundJSONResponse: notFound("Release")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound) {
		return gen.GetReleaseBindingK8sResourceTree404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
	}
	h.logger.Error("Failed to get k8s resource tree", "error", err)
	return gen.GetReleaseBindingK8sResourceTree500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

func (h *Handler) handleK8sResourceEventsError(err error) (gen.GetReleaseBindingK8sResourceEventsResponseObject, error) {
	if errors.Is(err, services.ErrForbidden) {
		return gen.GetReleaseBindingK8sResourceEvents403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseBindingNotFound) {
		return gen.GetReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseNotFound) {
		return gen.GetReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("Release")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound) {
		return gen.GetReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrResourceNotFound) {
		return gen.GetReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
	}
	h.logger.Error("Failed to get k8s resource events", "error", err)
	return gen.GetReleaseBindingK8sResourceEvents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

func (h *Handler) handleK8sResourceLogsError(err error) (gen.GetReleaseBindingK8sResourceLogsResponseObject, error) {
	if errors.Is(err, services.ErrForbidden) {
		return gen.GetReleaseBindingK8sResourceLogs403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseBindingNotFound) {
		return gen.GetReleaseBindingK8sResourceLogs404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseNotFound) {
		return gen.GetReleaseBindingK8sResourceLogs404JSONResponse{NotFoundJSONResponse: notFound("Release")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound) {
		return gen.GetReleaseBindingK8sResourceLogs404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrResourceNotFound) {
		return gen.GetReleaseBindingK8sResourceLogs404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
	}
	h.logger.Error("Failed to get k8s resource logs", "error", err)
	return gen.GetReleaseBindingK8sResourceLogs500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}
