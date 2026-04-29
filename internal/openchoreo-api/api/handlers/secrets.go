// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	secretsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secret"
)

// CreateSecret creates a new secret across the control plane and target plane.
func (h *Handler) CreateSecret(
	ctx context.Context,
	request gen.CreateSecretRequestObject,
) (gen.CreateSecretResponseObject, error) {
	h.logger.Info("CreateSecret called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateSecret400JSONResponse{BadRequestJSONResponse: badRequest("request body is required")}, nil
	}

	params := &secretsvc.CreateSecretParams{
		SecretName: request.Body.SecretName,
		SecretType: corev1.SecretType(request.Body.SecretType),
		TargetPlane: openchoreov1alpha1.TargetPlaneRef{
			Kind: string(request.Body.TargetPlane.Kind),
			Name: request.Body.TargetPlane.Name,
		},
		Data: request.Body.Data,
	}

	result, err := h.services.SecretService.CreateSecret(ctx, request.NamespaceName, params)
	if err != nil {
		return mapCreateSecretError(h, err)
	}

	return gen.CreateSecret201JSONResponse(toSecretResponse(result)), nil
}

// UpdateSecret rotates the data of an existing secret.
func (h *Handler) UpdateSecret(
	ctx context.Context,
	request gen.UpdateSecretRequestObject,
) (gen.UpdateSecretResponseObject, error) {
	h.logger.Info("UpdateSecret called", "namespaceName", request.NamespaceName, "secretName", request.SecretName)

	if request.Body == nil {
		return gen.UpdateSecret400JSONResponse{BadRequestJSONResponse: badRequest("request body is required")}, nil
	}

	params := &secretsvc.UpdateSecretParams{Data: request.Body.Data}

	result, err := h.services.SecretService.UpdateSecret(ctx, request.NamespaceName, request.SecretName, params)
	if err != nil {
		return mapUpdateSecretError(h, err)
	}

	return gen.UpdateSecret200JSONResponse(toSecretResponse(result)), nil
}

// DeleteSecret removes a secret by name.
func (h *Handler) DeleteSecret(
	ctx context.Context,
	request gen.DeleteSecretRequestObject,
) (gen.DeleteSecretResponseObject, error) {
	h.logger.Info("DeleteSecret called", "namespaceName", request.NamespaceName, "secretName", request.SecretName)

	if err := h.services.SecretService.DeleteSecret(ctx, request.NamespaceName, request.SecretName); err != nil {
		return mapDeleteSecretError(h, err)
	}

	return gen.DeleteSecret204Response{}, nil
}

func toSecretResponse(info *secretsvc.SecretInfo) gen.SecretResponse {
	name := info.Name
	ns := info.Namespace
	secretType := gen.SecretType(info.SecretType)
	target := gen.TargetPlaneRef{
		Kind: gen.TargetPlaneRefKind(info.TargetPlane.Kind),
		Name: info.TargetPlane.Name,
	}
	keys := info.Keys
	return gen.SecretResponse{
		Name:        &name,
		Namespace:   &ns,
		SecretType:  &secretType,
		TargetPlane: &target,
		Keys:        &keys,
	}
}

func mapCreateSecretError(h *Handler, err error) (gen.CreateSecretResponseObject, error) {
	var validationErr *services.ValidationError
	switch {
	case errors.Is(err, services.ErrForbidden):
		return gen.CreateSecret403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case errors.Is(err, secretsvc.ErrSecretAlreadyExists):
		return gen.CreateSecret409JSONResponse{ConflictJSONResponse: conflict("secret already exists")}, nil
	case errors.Is(err, secretsvc.ErrPlaneNotFound):
		return gen.CreateSecret400JSONResponse{BadRequestJSONResponse: badRequest("target plane not found")}, nil
	case errors.Is(err, secretsvc.ErrSecretStoreNotConfigured):
		return gen.CreateSecret400JSONResponse{BadRequestJSONResponse: badRequest("secret store is not configured on the target plane")}, nil
	case errors.As(err, &validationErr):
		return gen.CreateSecret400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
	default:
		h.logger.Error("Failed to create secret", "error", err)
		return gen.CreateSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
}

func mapUpdateSecretError(h *Handler, err error) (gen.UpdateSecretResponseObject, error) {
	var validationErr *services.ValidationError
	switch {
	case errors.Is(err, services.ErrForbidden):
		return gen.UpdateSecret403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case errors.Is(err, secretsvc.ErrSecretNotFound):
		return gen.UpdateSecret404JSONResponse{NotFoundJSONResponse: notFound("secret")}, nil
	case errors.Is(err, secretsvc.ErrPlaneNotFound):
		return gen.UpdateSecret400JSONResponse{BadRequestJSONResponse: badRequest("target plane not found")}, nil
	case errors.Is(err, secretsvc.ErrSecretStoreNotConfigured):
		return gen.UpdateSecret400JSONResponse{BadRequestJSONResponse: badRequest("secret store is not configured on the target plane")}, nil
	case errors.As(err, &validationErr):
		return gen.UpdateSecret400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
	default:
		h.logger.Error("Failed to update secret", "error", err)
		return gen.UpdateSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
}

func mapDeleteSecretError(h *Handler, err error) (gen.DeleteSecretResponseObject, error) {
	var validationErr *services.ValidationError
	switch {
	case errors.Is(err, services.ErrForbidden):
		return gen.DeleteSecret403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case errors.Is(err, secretsvc.ErrSecretNotFound):
		return gen.DeleteSecret404JSONResponse{NotFoundJSONResponse: notFound("secret")}, nil
	case errors.Is(err, secretsvc.ErrPlaneNotFound):
		return gen.DeleteSecret400JSONResponse{BadRequestJSONResponse: badRequest("target plane not found")}, nil
	case errors.Is(err, secretsvc.ErrSecretStoreNotConfigured):
		return gen.DeleteSecret400JSONResponse{BadRequestJSONResponse: badRequest("secret store is not configured on the target plane")}, nil
	case errors.As(err, &validationErr):
		return gen.DeleteSecret400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
	default:
		h.logger.Error("Failed to delete secret", "error", err)
		return gen.DeleteSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
}
