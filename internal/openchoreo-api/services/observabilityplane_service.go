// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ObservabilityPlaneService handles observability plane-related business logic
type ObservabilityPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewObservabilityPlaneService creates a new observability plane service
func NewObservabilityPlaneService(k8sClient client.Client, logger *slog.Logger) *ObservabilityPlaneService {
	return &ObservabilityPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ListObservabilityPlanes retrieves all observability planes for an organization
func (s *ObservabilityPlaneService) ListObservabilityPlanes(ctx context.Context, orgName string) ([]models.ObservabilityPlaneResponse, error) {
	s.logger.Debug("Listing observability planes", "org", orgName)

	// List all observability planes in the organization namespace
	var observabilityPlanes openchoreov1alpha1.ObservabilityPlaneList
	err := s.k8sClient.List(ctx, &observabilityPlanes, client.InNamespace(orgName))
	if err != nil {
		s.logger.Error("Failed to list observability planes", "error", err, "org", orgName)
		return nil, fmt.Errorf("failed to list observability planes: %w", err)
	}

	s.logger.Debug("Found observability planes", "count", len(observabilityPlanes.Items), "org", orgName)

	// Convert to response format
	observabilityPlaneResponses := make([]models.ObservabilityPlaneResponse, 0, len(observabilityPlanes.Items))
	for _, observabilityPlane := range observabilityPlanes.Items {
		displayName := observabilityPlane.Annotations[controller.AnnotationKeyDisplayName]
		description := observabilityPlane.Annotations[controller.AnnotationKeyDescription]

		// Determine status from conditions
		status := ""

		observabilityPlaneResponse := models.ObservabilityPlaneResponse{
			Name:        observabilityPlane.Name,
			Namespace:   observabilityPlane.Namespace,
			DisplayName: displayName,
			Description: description,
			CreatedAt:   observabilityPlane.CreationTimestamp.Time,
			Status:      status,
		}

		observabilityPlaneResponses = append(observabilityPlaneResponses, observabilityPlaneResponse)
	}

	return observabilityPlaneResponses, nil
}
