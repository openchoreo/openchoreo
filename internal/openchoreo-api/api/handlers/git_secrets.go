// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	gitsecretsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/gitsecret"
)

// ListGitSecrets returns all git secrets in a namespace.
func (h *Handler) ListGitSecrets(
	ctx context.Context,
	request gen.ListGitSecretsRequestObject,
) (gen.ListGitSecretsResponseObject, error) {
	h.logger.Debug("ListGitSecrets called", "namespaceName", request.NamespaceName)

	items, err := h.services.GitSecretService.ListGitSecrets(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list git secrets", "error", err)
		return gen.ListGitSecrets500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	respItems := make([]gen.GitSecretResponse, 0, len(items))
	for _, item := range items {
		name := item.Name
		ns := item.Namespace
		respItems = append(respItems, gen.GitSecretResponse{
			Name:      &name,
			Namespace: &ns,
		})
	}

	totalCount := len(respItems)
	return gen.ListGitSecrets200JSONResponse(gen.GitSecretListResponse{
		Items:      respItems,
		TotalCount: &totalCount,
	}), nil
}

// CreateGitSecret creates a new git secret.
func (h *Handler) CreateGitSecret(
	ctx context.Context,
	request gen.CreateGitSecretRequestObject,
) (gen.CreateGitSecretResponseObject, error) {
	h.logger.Info("CreateGitSecret called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateGitSecret400JSONResponse{BadRequestJSONResponse: badRequest("request body is required")}, nil
	}

	params := &gitsecretsvc.CreateGitSecretParams{
		SecretName: request.Body.SecretName,
		SecretType: string(request.Body.SecretType),
	}
	if request.Body.Username != nil {
		params.Username = *request.Body.Username
	}
	if request.Body.Token != nil {
		params.Token = *request.Body.Token
	}
	if request.Body.SshKey != nil {
		params.SSHKey = *request.Body.SshKey
	}
	if request.Body.SshKeyId != nil {
		params.SSHKeyID = *request.Body.SshKeyId
	}

	result, err := h.services.GitSecretService.CreateGitSecret(ctx, request.NamespaceName, params)
	if err != nil {
		return mapCreateGitSecretError(h, err)
	}

	name := result.Name
	ns := result.Namespace
	return gen.CreateGitSecret201JSONResponse(gen.GitSecretResponse{
		Name:      &name,
		Namespace: &ns,
	}), nil
}

// DeleteGitSecret deletes a git secret by name.
func (h *Handler) DeleteGitSecret(
	ctx context.Context,
	request gen.DeleteGitSecretRequestObject,
) (gen.DeleteGitSecretResponseObject, error) {
	h.logger.Info("DeleteGitSecret called", "namespaceName", request.NamespaceName, "gitSecretName", request.GitSecretName)

	err := h.services.GitSecretService.DeleteGitSecret(ctx, request.NamespaceName, request.GitSecretName)
	if err != nil {
		return mapDeleteGitSecretError(h, err)
	}

	return gen.DeleteGitSecret204Response{}, nil
}

func mapCreateGitSecretError(h *Handler, err error) (gen.CreateGitSecretResponseObject, error) {
	var validationErr *services.ValidationError
	switch {
	case errors.Is(err, services.ErrForbidden):
		return gen.CreateGitSecret403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case errors.Is(err, gitsecretsvc.ErrGitSecretAlreadyExists):
		return gen.CreateGitSecret409JSONResponse{ConflictJSONResponse: conflict("git secret already exists")}, nil
	case errors.Is(err, gitsecretsvc.ErrInvalidSecretType):
		return gen.CreateGitSecret400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
	case errors.As(err, &validationErr):
		return gen.CreateGitSecret400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
	case errors.Is(err, gitsecretsvc.ErrBuildPlaneNotFound):
		h.logger.Error("Failed to create git secret", "error", err)
		return gen.CreateGitSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	case errors.Is(err, gitsecretsvc.ErrSecretStoreNotConfigured):
		h.logger.Error("Failed to create git secret", "error", err)
		return gen.CreateGitSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	default:
		h.logger.Error("Failed to create git secret", "error", err)
		return gen.CreateGitSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
}

func mapDeleteGitSecretError(h *Handler, err error) (gen.DeleteGitSecretResponseObject, error) {
	switch {
	case errors.Is(err, services.ErrForbidden):
		return gen.DeleteGitSecret403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	case errors.Is(err, gitsecretsvc.ErrGitSecretNotFound):
		return gen.DeleteGitSecret404JSONResponse{NotFoundJSONResponse: notFound("git secret")}, nil
	case errors.Is(err, gitsecretsvc.ErrBuildPlaneNotFound):
		h.logger.Error("Failed to delete git secret", "error", err)
		return gen.DeleteGitSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	default:
		h.logger.Error("Failed to delete git secret", "error", err)
		return gen.DeleteGitSecret500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}
}
