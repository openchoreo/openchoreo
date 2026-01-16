// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// NamespaceService handles namespace-related business logic
type NamespaceService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewNamespaceService creates a new namespace service
func NewNamespaceService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *NamespaceService {
	return &NamespaceService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListNamespaces lists all namespaces
func (s *NamespaceService) ListNamespaces(ctx context.Context) ([]*models.NamespaceResponse, error) {
	s.logger.Debug("Listing namespaces")

	var namespaceList corev1.NamespaceList
	if err := s.k8sClient.List(ctx, &namespaceList); err != nil {
		s.logger.Error("Failed to list namespaces", "error", err)
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	namespaces := make([]*models.NamespaceResponse, 0, len(namespaceList.Items))
	for _, item := range namespaceList.Items {
		// Authorization check for each namespace
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewNamespace, ResourceTypeNamespace, item.Name,
			authz.ResourceHierarchy{Organization: item.Name}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized namespaces
				s.logger.Debug("Skipping unauthorized namespace", "namespace", item.Name)
				continue
			}
			// system failures, return the error
			return nil, err
		}
		namespaces = append(namespaces, s.toNamespaceResponse(&item))
	}

	s.logger.Debug("Listed namespaces", "count", len(namespaces))
	return namespaces, nil
}

// GetNamespace retrieves a specific namespace
func (s *NamespaceService) GetNamespace(ctx context.Context, namespaceName string) (*models.NamespaceResponse, error) {
	s.logger.Debug("Getting namespace", "namespace", namespaceName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewNamespace, ResourceTypeNamespace, namespaceName,
		authz.ResourceHierarchy{Organization: namespaceName}); err != nil {
		return nil, err
	}

	namespace := &corev1.Namespace{}
	key := client.ObjectKey{
		Name: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, namespace); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Namespace not found", "namespace", namespaceName)
			return nil, ErrNamespaceNotFound
		}
		s.logger.Error("Failed to get namespace", "error", err)
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	return s.toNamespaceResponse(namespace), nil
}

// toNamespaceResponse converts a Namespace to a NamespaceResponse
func (s *NamespaceService) toNamespaceResponse(namespace *corev1.Namespace) *models.NamespaceResponse {
	// Extract display name and description from annotations
	displayName := namespace.Annotations[controller.AnnotationKeyDisplayName]
	description := namespace.Annotations[controller.AnnotationKeyDescription]

	// Derive status from namespace phase
	status := statusUnknown
	switch namespace.Status.Phase {
	case corev1.NamespaceActive:
		status = statusReady
	case corev1.NamespaceTerminating:
		status = statusNotReady
	}

	return &models.NamespaceResponse{
		Name:        namespace.Name,
		DisplayName: displayName,
		Description: description,
		CreatedAt:   namespace.CreationTimestamp.Time,
		Status:      status,
	}
}
