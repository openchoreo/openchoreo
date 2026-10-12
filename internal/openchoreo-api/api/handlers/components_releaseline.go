// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"net/http"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcerrors "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ListComponentReleaseLines returns the release lines of a component.
func (h *Handler) ListComponentReleaseLines(
	ctx context.Context,
	request gen.ListComponentReleaseLinesRequestObject,
) (gen.ListComponentReleaseLinesResponseObject, error) {
	h.logger.Debug("ListComponentReleaseLines called", "namespaceName", request.NamespaceName, "component", request.ComponentName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ComponentService.ListReleaseLines(ctx, request.NamespaceName, request.ComponentName, opts)
	if err != nil {
		switch {
		case errors.Is(err, svcerrors.ErrForbidden):
			return gen.ListComponentReleaseLines403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case errors.Is(err, componentsvc.ErrComponentNotFound):
			return gen.ListComponentReleaseLines404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if validationErr, ok := errors.AsType[*svcerrors.ValidationError](err); ok {
			return gen.ListComponentReleaseLines400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list release lines", "error", err)
		return gen.ListComponentReleaseLines500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ReleaseLine, gen.ReleaseLine](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert release lines", "error", err)
		return gen.ListComponentReleaseLines500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.ListComponentReleaseLines200JSONResponse{Items: items, Pagination: ToPagination(result)}, nil
}

// GetComponentReleaseLine returns a release line of a component.
func (h *Handler) GetComponentReleaseLine(
	ctx context.Context,
	request gen.GetComponentReleaseLineRequestObject,
) (gen.GetComponentReleaseLineResponseObject, error) {
	h.logger.Debug("GetComponentReleaseLine called", "namespaceName", request.NamespaceName, "component", request.ComponentName, "releaseLine", request.ReleaseLineName)

	rl, err := h.services.ComponentService.GetReleaseLine(ctx, request.NamespaceName, request.ComponentName, request.ReleaseLineName)
	if err != nil {
		switch {
		case errors.Is(err, svcerrors.ErrForbidden):
			return gen.GetComponentReleaseLine403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case errors.Is(err, componentsvc.ErrComponentNotFound):
			return gen.GetComponentReleaseLine404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		case errors.Is(err, componentsvc.ErrReleaseLineNotFound):
			return gen.GetComponentReleaseLine404JSONResponse{NotFoundJSONResponse: notFound("ReleaseLine")}, nil
		}
		h.logger.Error("Failed to get release line", "error", err)
		return gen.GetComponentReleaseLine500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genLine, err := convert[openchoreov1alpha1.ReleaseLine, gen.ReleaseLine](*rl)
	if err != nil {
		h.logger.Error("Failed to convert release line", "error", err)
		return gen.GetComponentReleaseLine500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.GetComponentReleaseLine200JSONResponse(genLine), nil
}

// CreateComponentReleaseLine creates a release line for a component.
func (h *Handler) CreateComponentReleaseLine(
	ctx context.Context,
	request gen.CreateComponentReleaseLineRequestObject,
) (gen.CreateComponentReleaseLineResponseObject, error) {
	h.logger.Info("CreateComponentReleaseLine called", "namespaceName", request.NamespaceName, "component", request.ComponentName)

	if request.Body == nil {
		return gen.CreateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}
	rlCR, err := convert[gen.ReleaseLine, openchoreov1alpha1.ReleaseLine](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	if rlCR.Namespace != "" && rlCR.Namespace != request.NamespaceName {
		return gen.CreateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest("Namespace in body does not match path")}, nil
	}

	created, err := h.services.ComponentService.CreateReleaseLine(ctx, request.NamespaceName, request.ComponentName, &rlCR)
	if err != nil {
		switch {
		case errors.Is(err, svcerrors.ErrForbidden):
			return gen.CreateComponentReleaseLine403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case errors.Is(err, componentsvc.ErrComponentNotFound):
			return gen.CreateComponentReleaseLine404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		case errors.Is(err, componentsvc.ErrReleaseLinesNotEnabled):
			return gen.CreateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
		case errors.Is(err, componentsvc.ErrReleaseLineAlreadyExists):
			return gen.CreateComponentReleaseLine409JSONResponse{ConflictJSONResponse: conflict("A release line with this name already exists in the namespace")}, nil
		}
		if validationErr, ok := errors.AsType[*svcerrors.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.CreateComponentReleaseLine422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.CreateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create release line", "error", err)
		return gen.CreateComponentReleaseLine500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(created.UID), Name: created.Name})

	genLine, err := convert[openchoreov1alpha1.ReleaseLine, gen.ReleaseLine](*created)
	if err != nil {
		h.logger.Error("Failed to convert created release line", "error", err)
		return gen.CreateComponentReleaseLine500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.CreateComponentReleaseLine201JSONResponse(genLine), nil
}

// UpdateComponentReleaseLine replaces the spec of a component's release line.
func (h *Handler) UpdateComponentReleaseLine(
	ctx context.Context,
	request gen.UpdateComponentReleaseLineRequestObject,
) (gen.UpdateComponentReleaseLineResponseObject, error) {
	h.logger.Info("UpdateComponentReleaseLine called", "namespaceName", request.NamespaceName, "component", request.ComponentName, "releaseLine", request.ReleaseLineName)

	if request.Body == nil {
		return gen.UpdateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}
	rlCR, err := convert[gen.ReleaseLine, openchoreov1alpha1.ReleaseLine](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	if rlCR.Namespace != "" && rlCR.Namespace != request.NamespaceName {
		return gen.UpdateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest("Namespace in body does not match path")}, nil
	}
	rlCR.Name = request.ReleaseLineName

	updated, err := h.services.ComponentService.UpdateReleaseLine(ctx, request.NamespaceName, request.ComponentName, &rlCR)
	if err != nil {
		switch {
		case errors.Is(err, svcerrors.ErrForbidden):
			return gen.UpdateComponentReleaseLine403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case errors.Is(err, componentsvc.ErrComponentNotFound):
			return gen.UpdateComponentReleaseLine404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		case errors.Is(err, componentsvc.ErrReleaseLineNotFound):
			return gen.UpdateComponentReleaseLine404JSONResponse{NotFoundJSONResponse: notFound("ReleaseLine")}, nil
		}
		if validationErr, ok := errors.AsType[*svcerrors.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.UpdateComponentReleaseLine422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.UpdateComponentReleaseLine400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update release line", "error", err)
		return gen.UpdateComponentReleaseLine500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(updated.UID), Name: updated.Name})

	genLine, err := convert[openchoreov1alpha1.ReleaseLine, gen.ReleaseLine](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated release line", "error", err)
		return gen.UpdateComponentReleaseLine500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.UpdateComponentReleaseLine200JSONResponse(genLine), nil
}

// DeleteComponentReleaseLine deletes a component's release line.
func (h *Handler) DeleteComponentReleaseLine(
	ctx context.Context,
	request gen.DeleteComponentReleaseLineRequestObject,
) (gen.DeleteComponentReleaseLineResponseObject, error) {
	h.logger.Info("DeleteComponentReleaseLine called", "namespaceName", request.NamespaceName, "component", request.ComponentName, "releaseLine", request.ReleaseLineName)

	if err := h.services.ComponentService.DeleteReleaseLine(ctx, request.NamespaceName, request.ComponentName, request.ReleaseLineName); err != nil {
		switch {
		case errors.Is(err, svcerrors.ErrForbidden):
			return gen.DeleteComponentReleaseLine403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		case errors.Is(err, componentsvc.ErrComponentNotFound):
			return gen.DeleteComponentReleaseLine404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		case errors.Is(err, componentsvc.ErrReleaseLineNotFound):
			return gen.DeleteComponentReleaseLine404JSONResponse{NotFoundJSONResponse: notFound("ReleaseLine")}, nil
		}
		h.logger.Error("Failed to delete release line", "error", err)
		return gen.DeleteComponentReleaseLine500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.DeleteComponentReleaseLine204Response{}, nil
}
