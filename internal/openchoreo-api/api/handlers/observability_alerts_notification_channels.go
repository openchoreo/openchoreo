// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	observabilityalertsnotificationchannelsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityalertsnotificationchannel"
)

// ListObservabilityAlertsNotificationChannels returns a paginated list of observability alerts notification channels within a namespace.
func (h *Handler) ListObservabilityAlertsNotificationChannels(
	ctx context.Context,
	request gen.ListObservabilityAlertsNotificationChannelsRequestObject,
) (gen.ListObservabilityAlertsNotificationChannelsResponseObject, error) {
	h.logger.Debug("ListObservabilityAlertsNotificationChannels called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.ObservabilityAlertsNotificationChannelService.ListObservabilityAlertsNotificationChannels(ctx, request.NamespaceName, opts)
	if err != nil {
		h.logger.Error("Failed to list observability alerts notification channels", "error", err)
		return gen.ListObservabilityAlertsNotificationChannels500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ObservabilityAlertsNotificationChannel, gen.ObservabilityAlertsNotificationChannel](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert observability alerts notification channels", "error", err)
		return gen.ListObservabilityAlertsNotificationChannels500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListObservabilityAlertsNotificationChannels200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateObservabilityAlertsNotificationChannel creates a new observability alerts notification channel within a namespace.
func (h *Handler) CreateObservabilityAlertsNotificationChannel(
	ctx context.Context,
	request gen.CreateObservabilityAlertsNotificationChannelRequestObject,
) (gen.CreateObservabilityAlertsNotificationChannelResponseObject, error) {
	h.logger.Info("CreateObservabilityAlertsNotificationChannel called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateObservabilityAlertsNotificationChannel400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	ncCR, err := convert[gen.ObservabilityAlertsNotificationChannel, openchoreov1alpha1.ObservabilityAlertsNotificationChannel](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateObservabilityAlertsNotificationChannel400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	ncCR.Status = openchoreov1alpha1.ObservabilityAlertsNotificationChannelStatus{}

	created, err := h.services.ObservabilityAlertsNotificationChannelService.CreateObservabilityAlertsNotificationChannel(ctx, request.NamespaceName, &ncCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateObservabilityAlertsNotificationChannel403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, observabilityalertsnotificationchannelsvc.ErrObservabilityAlertsNotificationChannelAlreadyExists) {
			return gen.CreateObservabilityAlertsNotificationChannel409JSONResponse{ConflictJSONResponse: conflict("Observability alerts notification channel already exists")}, nil
		}
		h.logger.Error("Failed to create observability alerts notification channel", "error", err)
		return gen.CreateObservabilityAlertsNotificationChannel500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genNC, err := convert[openchoreov1alpha1.ObservabilityAlertsNotificationChannel, gen.ObservabilityAlertsNotificationChannel](*created)
	if err != nil {
		h.logger.Error("Failed to convert created observability alerts notification channel", "error", err)
		return gen.CreateObservabilityAlertsNotificationChannel500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Observability alerts notification channel created successfully", "namespaceName", request.NamespaceName, "channel", created.Name)
	return gen.CreateObservabilityAlertsNotificationChannel201JSONResponse(genNC), nil
}

// GetObservabilityAlertsNotificationChannel returns details of a specific observability alerts notification channel.
func (h *Handler) GetObservabilityAlertsNotificationChannel(
	ctx context.Context,
	request gen.GetObservabilityAlertsNotificationChannelRequestObject,
) (gen.GetObservabilityAlertsNotificationChannelResponseObject, error) {
	h.logger.Debug("GetObservabilityAlertsNotificationChannel called", "namespaceName", request.NamespaceName, "channel", request.ObservabilityAlertsNotificationChannelName)

	nc, err := h.services.ObservabilityAlertsNotificationChannelService.GetObservabilityAlertsNotificationChannel(ctx, request.NamespaceName, request.ObservabilityAlertsNotificationChannelName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetObservabilityAlertsNotificationChannel403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, observabilityalertsnotificationchannelsvc.ErrObservabilityAlertsNotificationChannelNotFound) {
			return gen.GetObservabilityAlertsNotificationChannel404JSONResponse{NotFoundJSONResponse: notFound("ObservabilityAlertsNotificationChannel")}, nil
		}
		h.logger.Error("Failed to get observability alerts notification channel", "error", err)
		return gen.GetObservabilityAlertsNotificationChannel500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genNC, err := convert[openchoreov1alpha1.ObservabilityAlertsNotificationChannel, gen.ObservabilityAlertsNotificationChannel](*nc)
	if err != nil {
		h.logger.Error("Failed to convert observability alerts notification channel", "error", err)
		return gen.GetObservabilityAlertsNotificationChannel500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetObservabilityAlertsNotificationChannel200JSONResponse(genNC), nil
}

// UpdateObservabilityAlertsNotificationChannel replaces an existing observability alerts notification channel (full update).
func (h *Handler) UpdateObservabilityAlertsNotificationChannel(
	ctx context.Context,
	request gen.UpdateObservabilityAlertsNotificationChannelRequestObject,
) (gen.UpdateObservabilityAlertsNotificationChannelResponseObject, error) {
	h.logger.Info("UpdateObservabilityAlertsNotificationChannel called", "namespaceName", request.NamespaceName, "channel", request.ObservabilityAlertsNotificationChannelName)

	if request.Body == nil {
		return gen.UpdateObservabilityAlertsNotificationChannel400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	ncCR, err := convert[gen.ObservabilityAlertsNotificationChannel, openchoreov1alpha1.ObservabilityAlertsNotificationChannel](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateObservabilityAlertsNotificationChannel400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	ncCR.Status = openchoreov1alpha1.ObservabilityAlertsNotificationChannelStatus{}

	// Ensure the name from the URL path is used
	ncCR.Name = request.ObservabilityAlertsNotificationChannelName

	updated, err := h.services.ObservabilityAlertsNotificationChannelService.UpdateObservabilityAlertsNotificationChannel(ctx, request.NamespaceName, &ncCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateObservabilityAlertsNotificationChannel403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, observabilityalertsnotificationchannelsvc.ErrObservabilityAlertsNotificationChannelNotFound) {
			return gen.UpdateObservabilityAlertsNotificationChannel404JSONResponse{NotFoundJSONResponse: notFound("ObservabilityAlertsNotificationChannel")}, nil
		}
		h.logger.Error("Failed to update observability alerts notification channel", "error", err)
		return gen.UpdateObservabilityAlertsNotificationChannel500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genNC, err := convert[openchoreov1alpha1.ObservabilityAlertsNotificationChannel, gen.ObservabilityAlertsNotificationChannel](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated observability alerts notification channel", "error", err)
		return gen.UpdateObservabilityAlertsNotificationChannel500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Observability alerts notification channel updated successfully", "namespaceName", request.NamespaceName, "channel", updated.Name)
	return gen.UpdateObservabilityAlertsNotificationChannel200JSONResponse(genNC), nil
}

// DeleteObservabilityAlertsNotificationChannel deletes an observability alerts notification channel by name.
func (h *Handler) DeleteObservabilityAlertsNotificationChannel(
	ctx context.Context,
	request gen.DeleteObservabilityAlertsNotificationChannelRequestObject,
) (gen.DeleteObservabilityAlertsNotificationChannelResponseObject, error) {
	h.logger.Info("DeleteObservabilityAlertsNotificationChannel called", "namespaceName", request.NamespaceName, "channel", request.ObservabilityAlertsNotificationChannelName)

	err := h.services.ObservabilityAlertsNotificationChannelService.DeleteObservabilityAlertsNotificationChannel(ctx, request.NamespaceName, request.ObservabilityAlertsNotificationChannelName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteObservabilityAlertsNotificationChannel403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, observabilityalertsnotificationchannelsvc.ErrObservabilityAlertsNotificationChannelNotFound) {
			return gen.DeleteObservabilityAlertsNotificationChannel404JSONResponse{NotFoundJSONResponse: notFound("ObservabilityAlertsNotificationChannel")}, nil
		}
		h.logger.Error("Failed to delete observability alerts notification channel", "error", err)
		return gen.DeleteObservabilityAlertsNotificationChannel500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Observability alerts notification channel deleted successfully", "namespaceName", request.NamespaceName, "channel", request.ObservabilityAlertsNotificationChannelName)
	return gen.DeleteObservabilityAlertsNotificationChannel204Response{}, nil
}
