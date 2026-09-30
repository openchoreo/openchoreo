// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"net/http"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	hooksvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/hook"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ListHooks returns a paginated list of hooks within a namespace.
func (h *Handler) ListHooks(
	ctx context.Context,
	request gen.ListHooksRequestObject,
) (gen.ListHooksResponseObject, error) {
	h.logger.Debug("ListHooks called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.HookService.ListHooks(ctx, request.NamespaceName, opts)
	if err != nil {
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			return gen.ListHooks400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list hooks", "error", err)
		return gen.ListHooks500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Hook, gen.Hook](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert hooks", "error", err)
		return gen.ListHooks500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListHooks200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateHook creates a new hook within a namespace.
func (h *Handler) CreateHook(
	ctx context.Context,
	request gen.CreateHookRequestObject,
) (gen.CreateHookResponseObject, error) {
	h.logger.Info("CreateHook called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateHook400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	hCR, err := convert[gen.Hook, openchoreov1alpha1.Hook](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateHook400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.HookService.CreateHook(ctx, request.NamespaceName, &hCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, hooksvc.ErrHookAlreadyExists) {
			return gen.CreateHook409JSONResponse{ConflictJSONResponse: conflict("Hook already exists")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.CreateHook422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.CreateHook400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create hook", "error", err)
		return gen.CreateHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(created.UID), Name: created.Name})

	genHook, err := convert[openchoreov1alpha1.Hook, gen.Hook](*created)
	if err != nil {
		h.logger.Error("Failed to convert created hook", "error", err)
		return gen.CreateHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Hook created successfully", "namespaceName", request.NamespaceName, "hook", created.Name)
	return gen.CreateHook201JSONResponse(genHook), nil
}

// GetHook returns details of a specific hook.
func (h *Handler) GetHook(
	ctx context.Context,
	request gen.GetHookRequestObject,
) (gen.GetHookResponseObject, error) {
	h.logger.Debug("GetHook called", "namespaceName", request.NamespaceName, "hookName", request.HookName)

	t, err := h.services.HookService.GetHook(ctx, request.NamespaceName, request.HookName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, hooksvc.ErrHookNotFound) {
			return gen.GetHook404JSONResponse{NotFoundJSONResponse: notFound("Hook")}, nil
		}
		h.logger.Error("Failed to get hook", "error", err)
		return gen.GetHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genHook, err := convert[openchoreov1alpha1.Hook, gen.Hook](*t)
	if err != nil {
		h.logger.Error("Failed to convert hook", "error", err)
		return gen.GetHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetHook200JSONResponse(genHook), nil
}

// UpdateHook replaces an existing hook (full update).
func (h *Handler) UpdateHook(
	ctx context.Context,
	request gen.UpdateHookRequestObject,
) (gen.UpdateHookResponseObject, error) {
	h.logger.Info("UpdateHook called", "namespaceName", request.NamespaceName, "hookName", request.HookName)

	if request.Body == nil {
		return gen.UpdateHook400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	hCR, err := convert[gen.Hook, openchoreov1alpha1.Hook](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateHook400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	// Ensure the name from the URL path is used
	hCR.Name = request.HookName

	updated, err := h.services.HookService.UpdateHook(ctx, request.NamespaceName, &hCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, hooksvc.ErrHookNotFound) {
			return gen.UpdateHook404JSONResponse{NotFoundJSONResponse: notFound("Hook")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.UpdateHook422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.UpdateHook400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update hook", "error", err)
		return gen.UpdateHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(updated.UID), Name: updated.Name})

	genHook, err := convert[openchoreov1alpha1.Hook, gen.Hook](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated hook", "error", err)
		return gen.UpdateHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Hook updated successfully", "namespaceName", request.NamespaceName, "hook", updated.Name)
	return gen.UpdateHook200JSONResponse(genHook), nil
}

// DeleteHook deletes a hook by name.
func (h *Handler) DeleteHook(
	ctx context.Context,
	request gen.DeleteHookRequestObject,
) (gen.DeleteHookResponseObject, error) {
	h.logger.Info("DeleteHook called", "namespaceName", request.NamespaceName, "hookName", request.HookName)

	err := h.services.HookService.DeleteHook(ctx, request.NamespaceName, request.HookName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, hooksvc.ErrHookNotFound) {
			return gen.DeleteHook404JSONResponse{NotFoundJSONResponse: notFound("Hook")}, nil
		}
		h.logger.Error("Failed to delete hook", "error", err)
		return gen.DeleteHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Hook deleted successfully", "namespaceName", request.NamespaceName, "hook", request.HookName)
	return gen.DeleteHook204Response{}, nil
}
