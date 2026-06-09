// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"net/http"

	corev1 "k8s.io/api/core/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	secretsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secret"
)

const secretsDisabledMessage = "Secret API is disabled on this server"

// dereferenceLabels converts the optional labels map from a request body into a
// plain map, returning nil when the field is absent.
func dereferenceLabels(labels *map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	return *labels
}

// secretsEnabled reports whether the Secret API endpoints are enabled.
func (h *Handler) secretsEnabled() bool {
	return h.Config != nil && h.Config.SecretManagement.Enabled
}

// ListSecrets returns a paginated list of secrets managed by the Secret API.
func (h *Handler) ListSecrets(
	ctx context.Context,
	request gen.ListSecretsRequestObject,
) (gen.ListSecretsResponseObject, error) {
	if !h.secretsEnabled() {
		return gen.ListSecrets501JSONResponse{NotImplementedJSONResponse: notImplemented(secretsDisabledMessage)}, nil
	}
	h.logger.Debug("ListSecrets called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, nil)

	result, err := h.services.SecretService.ListSecrets(ctx, request.NamespaceName, opts)
	if err != nil {
		return mapListSecretsError(h, err)
	}

	items, err := convertList[corev1.Secret, gen.Secret](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert secrets", "error", err)
		return gen.ListSecrets500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.ListSecrets200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateSecret creates a new secret across the control plane and target plane.
func (h *Handler) CreateSecret(
	ctx context.Context,
	request gen.CreateSecretRequestObject,
) (gen.CreateSecretResponseObject, error) {
	if !h.secretsEnabled() {
		return gen.CreateSecret501JSONResponse{NotImplementedJSONResponse: notImplemented(secretsDisabledMessage)}, nil
	}
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
		Data:   request.Body.Data,
		Labels: dereferenceLabels(request.Body.Labels),
	}

	result, err := h.services.SecretService.CreateSecret(ctx, request.NamespaceName, params)
	if err != nil {
		return mapCreateSecretError(h, err)
	}

	out, err := convert[corev1.Secret, gen.Secret](*result)
	if err != nil {
		h.logger.Error("Failed to convert created secret", "error", err)
		return gen.CreateSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.CreateSecret201JSONResponse(out), nil
}

// GetSecret returns a secret, including its data from the target plane.
func (h *Handler) GetSecret(
	ctx context.Context,
	request gen.GetSecretRequestObject,
) (gen.GetSecretResponseObject, error) {
	if !h.secretsEnabled() {
		return gen.GetSecret501JSONResponse{NotImplementedJSONResponse: notImplemented(secretsDisabledMessage)}, nil
	}
	h.logger.Debug("GetSecret called", "namespaceName", request.NamespaceName, "secretName", request.SecretName)

	result, err := h.services.SecretService.GetSecret(ctx, request.NamespaceName, request.SecretName)
	if err != nil {
		return mapGetSecretError(h, err)
	}

	out, err := convert[corev1.Secret, gen.Secret](*result)
	if err != nil {
		h.logger.Error("Failed to convert secret", "error", err)
		return gen.GetSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.GetSecret200JSONResponse(out), nil
}

// UpdateSecret replaces a secret's data with the supplied final state.
func (h *Handler) UpdateSecret(
	ctx context.Context,
	request gen.UpdateSecretRequestObject,
) (gen.UpdateSecretResponseObject, error) {
	if !h.secretsEnabled() {
		return gen.UpdateSecret501JSONResponse{NotImplementedJSONResponse: notImplemented(secretsDisabledMessage)}, nil
	}
	h.logger.Info("UpdateSecret called", "namespaceName", request.NamespaceName, "secretName", request.SecretName)

	if request.Body == nil {
		return gen.UpdateSecret400JSONResponse{BadRequestJSONResponse: badRequest("request body is required")}, nil
	}

	params := &secretsvc.UpdateSecretParams{
		Data:   request.Body.Data,
		Labels: dereferenceLabels(request.Body.Labels),
	}

	result, err := h.services.SecretService.UpdateSecret(ctx, request.NamespaceName, request.SecretName, params)
	if err != nil {
		return mapUpdateSecretError(h, err)
	}

	out, err := convert[corev1.Secret, gen.Secret](*result)
	if err != nil {
		h.logger.Error("Failed to convert updated secret", "error", err)
		return gen.UpdateSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
	return gen.UpdateSecret200JSONResponse(out), nil
}

// DeleteSecret removes a secret by name.
func (h *Handler) DeleteSecret(
	ctx context.Context,
	request gen.DeleteSecretRequestObject,
) (gen.DeleteSecretResponseObject, error) {
	if !h.secretsEnabled() {
		return gen.DeleteSecret501JSONResponse{NotImplementedJSONResponse: notImplemented(secretsDisabledMessage)}, nil
	}
	h.logger.Info("DeleteSecret called", "namespaceName", request.NamespaceName, "secretName", request.SecretName)

	if err := h.services.SecretService.DeleteSecret(ctx, request.NamespaceName, request.SecretName); err != nil {
		return mapDeleteSecretError(h, err)
	}

	return gen.DeleteSecret204Response{}, nil
}

func mapCreateSecretError(h *Handler, err error) (gen.CreateSecretResponseObject, error) {
	var validationErr *services.ValidationError
	switch {
	case errors.Is(err, services.ErrForbidden):
		return gen.CreateSecret403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case errors.Is(err, secretsvc.ErrSecretAlreadyExists):
		return gen.CreateSecret409JSONResponse{ConflictJSONResponse: conflict("secret already exists")}, nil
	case errors.Is(err, secretsvc.ErrPlaneNotFound):
		return gen.CreateSecret422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent("target plane not found")}, nil
	case errors.Is(err, secretsvc.ErrSecretStoreNotConfigured):
		return gen.CreateSecret400JSONResponse{BadRequestJSONResponse: badRequest("secret store is not configured on the target plane")}, nil
	case errors.As(err, &validationErr):
		if validationErr.StatusCode == http.StatusUnprocessableEntity {
			return gen.CreateSecret422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
		}
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
		return gen.UpdateSecret422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent("target plane not found")}, nil
	case errors.Is(err, secretsvc.ErrSecretStoreNotConfigured):
		return gen.UpdateSecret400JSONResponse{BadRequestJSONResponse: badRequest("secret store is not configured on the target plane")}, nil
	case errors.As(err, &validationErr):
		if validationErr.StatusCode == http.StatusUnprocessableEntity {
			return gen.UpdateSecret422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
		}
		return gen.UpdateSecret400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
	default:
		h.logger.Error("Failed to update secret", "error", err)
		return gen.UpdateSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
}

func mapGetSecretError(h *Handler, err error) (gen.GetSecretResponseObject, error) {
	switch {
	case errors.Is(err, services.ErrForbidden):
		return gen.GetSecret403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case errors.Is(err, secretsvc.ErrSecretNotFound):
		return gen.GetSecret404JSONResponse{NotFoundJSONResponse: notFound("secret")}, nil
	default:
		h.logger.Error("Failed to get secret", "error", err)
		return gen.GetSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
}

func mapListSecretsError(h *Handler, err error) (gen.ListSecretsResponseObject, error) {
	var validationErr *services.ValidationError
	switch {
	case errors.Is(err, services.ErrForbidden):
		return gen.ListSecrets403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case errors.As(err, &validationErr):
		return gen.ListSecrets400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
	default:
		h.logger.Error("Failed to list secrets", "error", err)
		return gen.ListSecrets500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
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
		return gen.DeleteSecret422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent("target plane not found")}, nil
	case errors.Is(err, secretsvc.ErrSecretStoreNotConfigured):
		return gen.DeleteSecret400JSONResponse{BadRequestJSONResponse: badRequest("secret store is not configured on the target plane")}, nil
	case errors.As(err, &validationErr):
		return gen.DeleteSecret400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
	default:
		h.logger.Error("Failed to delete secret", "error", err)
		return gen.DeleteSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
}
