// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	secretreferencesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secretreference"
)

// ListSecretReferences returns a paginated list of secret references within a namespace.
func (h *Handler) ListSecretReferences(
	ctx context.Context,
	request gen.ListSecretReferencesRequestObject,
) (gen.ListSecretReferencesResponseObject, error) {
	h.logger.Debug("ListSecretReferences called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.secretReferenceService.ListSecretReferences(ctx, request.NamespaceName, opts)
	if err != nil {
		h.logger.Error("Failed to list secret references", "error", err)
		return gen.ListSecretReferences500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.SecretReference, gen.SecretReference](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert secret references", "error", err)
		return gen.ListSecretReferences500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListSecretReferences200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateSecretReference creates a new secret reference within a namespace.
func (h *Handler) CreateSecretReference(
	ctx context.Context,
	request gen.CreateSecretReferenceRequestObject,
) (gen.CreateSecretReferenceResponseObject, error) {
	h.logger.Info("CreateSecretReference called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateSecretReference400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	srCR, err := convert[gen.SecretReference, openchoreov1alpha1.SecretReference](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateSecretReference400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	srCR.Status = openchoreov1alpha1.SecretReferenceStatus{}

	created, err := h.secretReferenceService.CreateSecretReference(ctx, request.NamespaceName, &srCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateSecretReference403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, secretreferencesvc.ErrSecretReferenceAlreadyExists) {
			return gen.CreateSecretReference409JSONResponse{ConflictJSONResponse: conflict("Secret reference already exists")}, nil
		}
		h.logger.Error("Failed to create secret reference", "error", err)
		return gen.CreateSecretReference500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genSR, err := convert[openchoreov1alpha1.SecretReference, gen.SecretReference](*created)
	if err != nil {
		h.logger.Error("Failed to convert created secret reference", "error", err)
		return gen.CreateSecretReference500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Secret reference created successfully", "namespaceName", request.NamespaceName, "secretReference", created.Name)
	return gen.CreateSecretReference201JSONResponse(genSR), nil
}

// GetSecretReference returns details of a specific secret reference.
func (h *Handler) GetSecretReference(
	ctx context.Context,
	request gen.GetSecretReferenceRequestObject,
) (gen.GetSecretReferenceResponseObject, error) {
	h.logger.Debug("GetSecretReference called", "namespaceName", request.NamespaceName, "secretReferenceName", request.SecretReferenceName)

	sr, err := h.secretReferenceService.GetSecretReference(ctx, request.NamespaceName, request.SecretReferenceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetSecretReference403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, secretreferencesvc.ErrSecretReferenceNotFound) {
			return gen.GetSecretReference404JSONResponse{NotFoundJSONResponse: notFound("SecretReference")}, nil
		}
		h.logger.Error("Failed to get secret reference", "error", err)
		return gen.GetSecretReference500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genSR, err := convert[openchoreov1alpha1.SecretReference, gen.SecretReference](*sr)
	if err != nil {
		h.logger.Error("Failed to convert secret reference", "error", err)
		return gen.GetSecretReference500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetSecretReference200JSONResponse(genSR), nil
}

// UpdateSecretReference replaces an existing secret reference (full update).
func (h *Handler) UpdateSecretReference(
	ctx context.Context,
	request gen.UpdateSecretReferenceRequestObject,
) (gen.UpdateSecretReferenceResponseObject, error) {
	h.logger.Info("UpdateSecretReference called", "namespaceName", request.NamespaceName, "secretReferenceName", request.SecretReferenceName)

	if request.Body == nil {
		return gen.UpdateSecretReference400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	srCR, err := convert[gen.SecretReference, openchoreov1alpha1.SecretReference](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateSecretReference400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	srCR.Status = openchoreov1alpha1.SecretReferenceStatus{}

	// Ensure the name from the URL path is used
	srCR.Name = request.SecretReferenceName

	updated, err := h.secretReferenceService.UpdateSecretReference(ctx, request.NamespaceName, &srCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateSecretReference403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, secretreferencesvc.ErrSecretReferenceNotFound) {
			return gen.UpdateSecretReference404JSONResponse{NotFoundJSONResponse: notFound("SecretReference")}, nil
		}
		h.logger.Error("Failed to update secret reference", "error", err)
		return gen.UpdateSecretReference500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genSR, err := convert[openchoreov1alpha1.SecretReference, gen.SecretReference](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated secret reference", "error", err)
		return gen.UpdateSecretReference500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Secret reference updated successfully", "namespaceName", request.NamespaceName, "secretReference", updated.Name)
	return gen.UpdateSecretReference200JSONResponse(genSR), nil
}

// DeleteSecretReference deletes a secret reference by name.
func (h *Handler) DeleteSecretReference(
	ctx context.Context,
	request gen.DeleteSecretReferenceRequestObject,
) (gen.DeleteSecretReferenceResponseObject, error) {
	h.logger.Info("DeleteSecretReference called", "namespaceName", request.NamespaceName, "secretReferenceName", request.SecretReferenceName)

	err := h.secretReferenceService.DeleteSecretReference(ctx, request.NamespaceName, request.SecretReferenceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteSecretReference403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, secretreferencesvc.ErrSecretReferenceNotFound) {
			return gen.DeleteSecretReference404JSONResponse{NotFoundJSONResponse: notFound("SecretReference")}, nil
		}
		h.logger.Error("Failed to delete secret reference", "error", err)
		return gen.DeleteSecretReference500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Secret reference deleted successfully", "namespaceName", request.NamespaceName, "secretReference", request.SecretReferenceName)
	return gen.DeleteSecretReference204Response{}, nil
}
