// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresource"
)

// QueryK8sResources handles POST /api/v1alpha1/k8sresources/query.
func (h *Handler) QueryK8sResources(
	ctx context.Context,
	request gen.QueryK8sResourcesRequestObject,
) (gen.QueryK8sResourcesResponseObject, error) {
	if request.Body == nil {
		return gen.QueryK8sResources400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	body := request.Body

	// Build service request
	queryReq := &k8sresource.QueryRequest{
		PlaneType:       string(body.PlaneType),
		PlaneName:       body.PlaneName,
		K8sResourcePath: body.K8sResourcePath,
		Project:         body.Project,
		Component:       body.Component,
	}
	if body.PlaneNamespace != nil {
		queryReq.PlaneNamespace = *body.PlaneNamespace
	}
	if body.Environment != nil {
		queryReq.Environment = *body.Environment
	}

	resp, err := h.services.K8sResourceService.QueryK8sResources(ctx, queryReq)
	if err != nil {
		return h.handleK8sResourceError(err)
	}

	// Build response envelope
	result := gen.QueryK8sResources200JSONResponse{
		Data: resp.Data,
		Metadata: struct {
			K8sStatusCode  int     `json:"k8sStatusCode"`
			PlaneName      string  `json:"planeName"`
			PlaneNamespace *string `json:"planeNamespace,omitempty"`
			PlaneType      string  `json:"planeType"`
		}{
			K8sStatusCode: resp.K8sStatusCode,
			PlaneName:     resp.PlaneName,
			PlaneType:     resp.PlaneType,
		},
	}
	if resp.PlaneNamespace != "" {
		result.Metadata.PlaneNamespace = &resp.PlaneNamespace
	}

	return result, nil
}

func (h *Handler) handleK8sResourceError(err error) (gen.QueryK8sResourcesResponseObject, error) {
	if errors.Is(err, services.ErrForbidden) {
		return gen.QueryK8sResources403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}
	if errors.Is(err, k8sresource.ErrPlaneNotFound) {
		return gen.QueryK8sResources404JSONResponse{NotFoundJSONResponse: notFound("Plane")}, nil
	}
	if errors.Is(err, k8sresource.ErrInvalidK8sPath) || errors.Is(err, k8sresource.ErrMissingRequired) {
		return gen.QueryK8sResources400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
	}
	if errors.Is(err, k8sresource.ErrGatewayUnavailable) || errors.Is(err, k8sresource.ErrGatewayError) {
		return gen.QueryK8sResources502JSONResponse{BadGatewayJSONResponse: badGateway(err.Error())}, nil
	}
	h.logger.Error("Failed to query K8s resources", "error", err)
	return gen.QueryK8sResources500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}
