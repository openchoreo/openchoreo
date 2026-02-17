// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ClusterObservabilityPlaneService handles cluster-scoped observability plane-related business logic
type ClusterObservabilityPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewClusterObservabilityPlaneService creates a new cluster observability plane service
func NewClusterObservabilityPlaneService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ClusterObservabilityPlaneService {
	return &ClusterObservabilityPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListClusterObservabilityPlanes lists all cluster-scoped observability planes
func (s *ClusterObservabilityPlaneService) ListClusterObservabilityPlanes(ctx context.Context) ([]models.ClusterObservabilityPlaneResponse, error) {
	s.logger.Debug("Listing cluster observability planes")

	var copList openchoreov1alpha1.ClusterObservabilityPlaneList
	if err := s.k8sClient.List(ctx, &copList); err != nil {
		s.logger.Error("Failed to list cluster observability planes", "error", err)
		return nil, fmt.Errorf("failed to list cluster observability planes: %w", err)
	}

	s.logger.Debug("Found cluster observability planes", "count", len(copList.Items))

	observabilityPlanes := make([]models.ClusterObservabilityPlaneResponse, 0, len(copList.Items))
	for i := range copList.Items {
		if err := checkAuthorization(
			ctx,
			s.logger,
			s.authzPDP,
			SystemActionViewClusterObservabilityPlane,
			ResourceTypeClusterObservabilityPlane,
			copList.Items[i].Name,
			authz.ResourceHierarchy{},
		); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized cluster observability plane", "clusterObservabilityPlane", copList.Items[i].Name)
				continue
			}
			return nil, fmt.Errorf("authorize %s %s for clusterObservabilityPlane %q: %w",
				SystemActionViewClusterObservabilityPlane, ResourceTypeClusterObservabilityPlane, copList.Items[i].Name, err)
		}
		observabilityPlanes = append(observabilityPlanes, toClusterObservabilityPlaneResponse(&copList.Items[i]))
	}

	return observabilityPlanes, nil
}

// toClusterObservabilityPlaneResponse converts a ClusterObservabilityPlane CR to a ClusterObservabilityPlaneResponse
func toClusterObservabilityPlaneResponse(cop *openchoreov1alpha1.ClusterObservabilityPlane) models.ClusterObservabilityPlaneResponse {
	displayName := cop.Annotations[controller.AnnotationKeyDisplayName]
	description := cop.Annotations[controller.AnnotationKeyDescription]

	status := statusUnknown
	if len(cop.Status.Conditions) > 0 {
		latestCondition := cop.Status.Conditions[len(cop.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	response := models.ClusterObservabilityPlaneResponse{
		Name:        cop.Name,
		PlaneID:     cop.Spec.PlaneID,
		DisplayName: displayName,
		Description: description,
		ObserverURL: cop.Spec.ObserverURL,
		RCAAgentURL: cop.Spec.RCAAgentURL,
		CreatedAt:   cop.CreationTimestamp.Time,
		Status:      status,
	}

	if cop.Status.AgentConnection != nil {
		response.AgentConnection = toAgentConnectionStatusResponse(cop.Status.AgentConnection)
	}

	return response
}
