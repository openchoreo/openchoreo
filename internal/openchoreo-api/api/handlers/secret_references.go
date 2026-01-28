// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListSecretReferences returns a list of secret references
func (h *Handler) ListSecretReferences(
	ctx context.Context,
	request gen.ListSecretReferencesRequestObject,
) (gen.ListSecretReferencesResponseObject, error) {
	h.logger.Debug("ListSecretReferences called", "namespaceName", request.NamespaceName)

	secretRefs, err := h.services.SecretReferenceService.ListSecretReferences(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list secret references", "error", err)
		return gen.ListSecretReferences500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.SecretReference, 0, len(secretRefs))
	for _, sr := range secretRefs {
		items = append(items, toGenSecretReference(sr))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListSecretReferences200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenSecretReference converts models.SecretReferenceResponse to gen.SecretReference
func toGenSecretReference(sr *models.SecretReferenceResponse) gen.SecretReference {
	result := gen.SecretReference{
		Name:        sr.Name,
		Namespace:   sr.Namespace,
		DisplayName: ptr.To(sr.DisplayName),
		Description: ptr.To(sr.Description),
		CreatedAt:   sr.CreatedAt,
		Status:      ptr.To(sr.Status),
	}
	if sr.RefreshInterval != "" {
		result.RefreshInterval = ptr.To(sr.RefreshInterval)
	}
	if sr.LastRefreshTime != nil {
		result.LastRefreshTime = sr.LastRefreshTime
	}
	if len(sr.SecretStores) > 0 {
		secretStores := make([]gen.SecretStoreReference, 0, len(sr.SecretStores))
		for _, ss := range sr.SecretStores {
			secretStores = append(secretStores, gen.SecretStoreReference{
				Kind: ss.Kind,
				Name: ss.Name,
			})
		}
		result.SecretStores = ptr.To(secretStores)
	}
	if len(sr.Data) > 0 {
		data := make([]gen.SecretDataSource, 0, len(sr.Data))
		for _, d := range sr.Data {
			dataSource := gen.SecretDataSource{
				SecretKey: d.SecretKey,
				RemoteRef: gen.RemoteReference{Key: d.RemoteRef.Key},
			}
			if d.RemoteRef.Property != "" {
				dataSource.RemoteRef.Property = ptr.To(d.RemoteRef.Property)
			}
			if d.RemoteRef.Version != "" {
				dataSource.RemoteRef.Version = ptr.To(d.RemoteRef.Version)
			}
			data = append(data, dataSource)
		}
		result.Data = ptr.To(data)
	}
	return result
}
