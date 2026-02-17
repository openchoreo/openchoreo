// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// SecretReferenceService handles secret reference-related business logic
type SecretReferenceService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewSecretReferenceService creates a new secret reference service
func NewSecretReferenceService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *SecretReferenceService {
	return &SecretReferenceService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListSecretReferences lists all secret references for a namespace
func (s *SecretReferenceService) ListSecretReferences(ctx context.Context, namespaceName string) ([]*models.SecretReferenceResponse, error) {
	s.logger.Debug("Listing secret references", "namespace", namespaceName)

	// List secret references in the namespace
	var secretRefList openchoreov1alpha1.SecretReferenceList
	listOptions := &client.ListOptions{
		Namespace: namespaceName,
	}

	if err := s.k8sClient.List(ctx, &secretRefList, listOptions); err != nil {
		s.logger.Error("Failed to list secret references", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list secret references: %w", err)
	}

	// Check authorization for each secret reference
	secretReferences := make([]*models.SecretReferenceResponse, 0, len(secretRefList.Items))
	for i := range secretRefList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewSecretReference, ResourceTypeSecretReference, secretRefList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized secret reference", "namespace", namespaceName, "secretReference", secretRefList.Items[i].Name)
				continue
			}
			// Return other errors
			return nil, err
		}
		secretReferences = append(secretReferences, s.toSecretReferenceResponse(&secretRefList.Items[i]))
	}

	s.logger.Debug("Listed secret references", "count", len(secretReferences), "namespace", namespaceName)
	return secretReferences, nil
}

// toSecretReferenceResponse converts a SecretReference CR to a SecretReferenceResponse
func (s *SecretReferenceService) toSecretReferenceResponse(secretRef *openchoreov1alpha1.SecretReference) *models.SecretReferenceResponse {
	// Extract display name and description from annotations
	displayName := secretRef.Annotations[controller.AnnotationKeyDisplayName]
	description := secretRef.Annotations[controller.AnnotationKeyDescription]

	// Convert data sources
	dataInfo := make([]models.SecretDataSourceInfo, 0, len(secretRef.Spec.Data))
	for _, data := range secretRef.Spec.Data {
		dataInfo = append(dataInfo, models.SecretDataSourceInfo{
			SecretKey: data.SecretKey,
			RemoteRef: models.RemoteReferenceInfo{
				Key:      data.RemoteRef.Key,
				Property: data.RemoteRef.Property,
				Version:  data.RemoteRef.Version,
			},
		})
	}

	// Convert secret store references
	secretStores := make([]models.SecretStoreReference, 0, len(secretRef.Status.SecretStores))
	for _, store := range secretRef.Status.SecretStores {
		secretStores = append(secretStores, models.SecretStoreReference{
			Name:      store.Name,
			Namespace: store.Namespace,
			Kind:      store.Kind,
		})
	}

	// Get status from conditions
	status := statusUnknown
	if len(secretRef.Status.Conditions) > 0 {
		// Get the latest condition
		latestCondition := secretRef.Status.Conditions[len(secretRef.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	// Convert refresh interval
	refreshInterval := ""
	if secretRef.Spec.RefreshInterval != nil {
		refreshInterval = secretRef.Spec.RefreshInterval.Duration.String()
	}

	// Convert last refresh time
	var lastRefreshTime *time.Time
	if secretRef.Status.LastRefreshTime != nil {
		t := secretRef.Status.LastRefreshTime.Time
		lastRefreshTime = &t
	}

	return &models.SecretReferenceResponse{
		Name:            secretRef.Name,
		Namespace:       secretRef.Namespace,
		DisplayName:     displayName,
		Description:     description,
		SecretStores:    secretStores,
		RefreshInterval: refreshInterval,
		Data:            dataInfo,
		CreatedAt:       secretRef.CreationTimestamp.Time,
		LastRefreshTime: lastRefreshTime,
		Status:          status,
	}
}
