// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
)

// ListGitSecrets handles GET /api/v1/namespaces/{namespaceName}/git-secrets
func (h *Handler) ListGitSecrets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	secrets, err := h.services.GitSecretService.ListGitSecrets(ctx, namespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to list git secrets", "namespace", namespaceName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		h.logger.Error("Failed to list git secrets", "error", err, "namespace", namespaceName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list git secrets", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, models.ListResponse[models.GitSecretResponse]{
		Items:      secrets,
		TotalCount: len(secrets),
		Page:       1,
		PageSize:   len(secrets),
	})
}

// CreateGitSecret handles POST /api/v1/namespaces/{namespaceName}/git-secrets
func (h *Handler) CreateGitSecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	var req models.CreateGitSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	if err := req.Validate(); err != nil {
		h.logger.Error("Request validation failed", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	setAuditResource(ctx, "git_secret", req.SecretName, req.SecretName)
	addAuditMetadata(ctx, "organization", namespaceName)

	secret, err := h.services.GitSecretService.CreateGitSecret(ctx, namespaceName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to create git secret", "namespace", namespaceName, "secret", req.SecretName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrInvalidSecretType) {
			writeErrorResponse(w, http.StatusBadRequest, services.ErrInvalidSecretType.Error(), services.CodeInvalidSecretType)
			return
		}
		if errors.Is(err, services.ErrInvalidCredentials) {
			writeErrorResponse(w, http.StatusBadRequest, services.ErrInvalidCredentials.Error(), services.CodeInvalidCredentials)
			return
		}
		if errors.Is(err, services.ErrGitSecretAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, "Git secret already exists", services.CodeGitSecretExists)
			return
		}
		if errors.Is(err, services.ErrBuildPlaneNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Build plane not found", services.CodeBuildPlaneNotFound)
			return
		}
		if errors.Is(err, services.ErrSecretStoreNotConfigured) {
			writeErrorResponse(w, http.StatusInternalServerError, "Build plane secret store is not configured", services.CodeSecretStoreNotConfigured)
			return
		}
		h.logger.Error("Failed to create git secret", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create git secret", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, secret)
}

// DeleteGitSecret handles DELETE /api/v1/namespaces/{namespaceName}/git-secrets/{secretName}
func (h *Handler) DeleteGitSecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	namespaceName := r.PathValue("namespaceName")
	secretName := r.PathValue("secretName")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}
	if secretName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Secret name is required", services.CodeInvalidInput)
		return
	}

	setAuditResource(ctx, "git_secret", secretName, secretName)
	addAuditMetadata(ctx, "organization", namespaceName)

	err := h.services.GitSecretService.DeleteGitSecret(ctx, namespaceName, secretName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to delete git secret", "namespace", namespaceName, "secret", secretName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrGitSecretNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Git secret not found", services.CodeGitSecretNotFound)
			return
		}
		if errors.Is(err, services.ErrBuildPlaneNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Build plane not found", services.CodeBuildPlaneNotFound)
			return
		}
		h.logger.Error("Failed to delete git secret", "error", err, "namespace", namespaceName, "secret", secretName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete git secret", services.CodeInternalError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
