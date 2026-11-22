// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	k8sresourcessvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
)

// GetEndpointReleaseBindingK8sResourceTree returns the live Kubernetes
// resources deployed by the releases an endpoint release binding owns.
func (h *Handler) GetEndpointReleaseBindingK8sResourceTree(
	ctx context.Context,
	request gen.GetEndpointReleaseBindingK8sResourceTreeRequestObject,
) (gen.GetEndpointReleaseBindingK8sResourceTreeResponseObject, error) {
	h.logger.Debug("GetEndpointReleaseBindingK8sResourceTree called",
		"namespace", request.NamespaceName,
		"endpointReleaseBinding", request.EndpointReleaseBindingName)

	result, err := h.services.K8sResourcesService.GetEndpointResourceTree(ctx, request.NamespaceName, request.EndpointReleaseBindingName)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden):
			return gen.GetEndpointReleaseBindingK8sResourceTree403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case errors.Is(err, k8sresourcessvc.ErrEndpointReleaseBindingNotFound):
			return gen.GetEndpointReleaseBindingK8sResourceTree404JSONResponse{NotFoundJSONResponse: notFound("EndpointReleaseBinding")}, nil
		case errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound):
			return gen.GetEndpointReleaseBindingK8sResourceTree404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
		}
		h.logger.Error("Failed to get k8s resource tree", "error", err)
		return gen.GetEndpointReleaseBindingK8sResourceTree500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genReleases := make([]gen.ReleaseResourceTree, 0, len(result.RenderedReleases))
	for _, r := range result.RenderedReleases {
		nodes, err := convertList[models.ResourceNode, gen.ResourceNode](r.Nodes)
		if err != nil {
			h.logger.Error("Failed to convert resource nodes", "error", err)
			return gen.GetEndpointReleaseBindingK8sResourceTree500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
		}
		entry := gen.ReleaseResourceTree{
			Name:        r.Name,
			TargetPlane: gen.ReleaseResourceTreeTargetPlane(r.TargetPlane),
			Nodes:       nodes,
		}
		if r.Release != nil {
			genRelease, err := convert[openchoreov1alpha1.RenderedRelease, gen.RenderedRelease](*r.Release)
			if err != nil {
				h.logger.Error("Failed to convert rendered release", "error", err)
				return gen.GetEndpointReleaseBindingK8sResourceTree500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
			}
			entry.RenderedRelease = &genRelease
		}
		genReleases = append(genReleases, entry)
	}

	return gen.GetEndpointReleaseBindingK8sResourceTree200JSONResponse{RenderedReleases: genReleases}, nil
}

// GetEndpointReleaseBindingK8sResourceEvents returns Kubernetes events for a
// resource in an endpoint release binding's resource tree.
func (h *Handler) GetEndpointReleaseBindingK8sResourceEvents(
	ctx context.Context,
	request gen.GetEndpointReleaseBindingK8sResourceEventsRequestObject,
) (gen.GetEndpointReleaseBindingK8sResourceEventsResponseObject, error) {
	h.logger.Debug("GetEndpointReleaseBindingK8sResourceEvents called",
		"namespace", request.NamespaceName,
		"endpointReleaseBinding", request.EndpointReleaseBindingName,
		"kind", request.Params.Kind,
		"name", request.Params.Name)

	group := ""
	if request.Params.Group != nil {
		group = *request.Params.Group
	}
	resp, err := h.services.K8sResourcesService.GetEndpointResourceEvents(ctx, request.NamespaceName,
		request.EndpointReleaseBindingName, group, request.Params.Version, request.Params.Kind, request.Params.Name)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden):
			return gen.GetEndpointReleaseBindingK8sResourceEvents403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case errors.Is(err, k8sresourcessvc.ErrEndpointReleaseBindingNotFound):
			return gen.GetEndpointReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("EndpointReleaseBinding")}, nil
		case errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound):
			return gen.GetEndpointReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
		case errors.Is(err, k8sresourcessvc.ErrResourceNotFound):
			return gen.GetEndpointReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
		}
		// An incomplete walk or an ambiguous identity is not "no such
		// resource", so it is a server error, as for a release binding.
		h.logger.Error("Failed to get k8s resource events", "error", err)
		return gen.GetEndpointReleaseBindingK8sResourceEvents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	result, err := convert[models.ResourceEventsResponse, gen.ResourceEventsResponse](*resp)
	if err != nil {
		h.logger.Error("Failed to convert resource events response", "error", err)
		return gen.GetEndpointReleaseBindingK8sResourceEvents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.GetEndpointReleaseBindingK8sResourceEvents200JSONResponse(result), nil
}
