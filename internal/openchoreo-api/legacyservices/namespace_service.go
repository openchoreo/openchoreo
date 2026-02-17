// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
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

// ListNamespaces lists all control plane namespaces
// Only namespaces with the openchoreo.dev/controlplane-namespace=true label are returned
func (s *NamespaceService) ListNamespaces(ctx context.Context) ([]*models.NamespaceResponse, error) {
	s.logger.Debug("Listing control plane namespaces")

	var namespaceList corev1.NamespaceList
	// Filter to only include control plane namespaces
	labelSelector := client.MatchingLabels{
		labels.LabelKeyControlPlaneNamespace: labels.LabelValueTrue,
	}
	if err := s.k8sClient.List(ctx, &namespaceList, labelSelector); err != nil {
		s.logger.Error("Failed to list control plane namespaces", "error", err)
		return nil, fmt.Errorf("failed to list control plane namespaces: %w", err)
	}

	namespaces := make([]*models.NamespaceResponse, 0, len(namespaceList.Items))
	for _, item := range namespaceList.Items {
		// Authorization check for each namespace
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewNamespace, ResourceTypeNamespace, item.Name,
			authz.ResourceHierarchy{Namespace: item.Name}); err != nil {
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
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
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

// CreateNamespace creates a new control plane namespace
func (s *NamespaceService) CreateNamespace(ctx context.Context, req *models.CreateNamespaceRequest) (*models.NamespaceResponse, error) {
	s.logger.Debug("Creating namespace", "name", req.Name)

	// Authorization check - use system action for creating namespaces
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateNamespace, ResourceTypeNamespace, req.Name,
		authz.ResourceHierarchy{Namespace: req.Name}); err != nil {
		return nil, err
	}

	// Create namespace object with control plane label
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				labels.LabelKeyControlPlaneNamespace: labels.LabelValueTrue,
			},
			Annotations: make(map[string]string),
		},
	}

	// Add display name and description if provided
	if req.DisplayName != "" {
		namespace.Annotations[controller.AnnotationKeyDisplayName] = req.DisplayName
	}
	if req.Description != "" {
		namespace.Annotations[controller.AnnotationKeyDescription] = req.Description
	}

	// Create the namespace
	if err := s.k8sClient.Create(ctx, namespace); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Namespace already exists", "namespace", req.Name)
			return nil, ErrNamespaceAlreadyExists
		}
		s.logger.Error("Failed to create namespace", "namespace", req.Name, "error", err)
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}

	s.logger.Info("Namespace created successfully", "namespace", req.Name)
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
