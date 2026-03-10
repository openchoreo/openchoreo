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

// ClusterWorkflowPlaneService handles cluster-scoped workflow plane-related business logic
type ClusterWorkflowPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewClusterWorkflowPlaneService creates a new cluster workflow plane service
func NewClusterWorkflowPlaneService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ClusterWorkflowPlaneService {
	return &ClusterWorkflowPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListClusterWorkflowPlanes lists all cluster-scoped workflow planes
func (s *ClusterWorkflowPlaneService) ListClusterWorkflowPlanes(ctx context.Context) ([]models.ClusterWorkflowPlaneResponse, error) {
	s.logger.Debug("Listing cluster workflow planes")

	var cbpList openchoreov1alpha1.ClusterWorkflowPlaneList
	if err := s.k8sClient.List(ctx, &cbpList); err != nil {
		s.logger.Error("Failed to list cluster workflow planes", "error", err)
		return nil, fmt.Errorf("failed to list cluster workflow planes: %w", err)
	}

	s.logger.Debug("Found cluster workflow planes", "count", len(cbpList.Items))

	workflowPlanes := make([]models.ClusterWorkflowPlaneResponse, 0, len(cbpList.Items))
	for i := range cbpList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterWorkflowPlane, ResourceTypeClusterWorkflowPlane, cbpList.Items[i].Name,
			authz.ResourceHierarchy{}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized cluster workflow plane", "clusterWorkflowPlane", cbpList.Items[i].Name)
				continue
			}
			return nil, fmt.Errorf("authorize %s %s for clusterWorkflowPlane %q: %w",
				SystemActionViewClusterWorkflowPlane, ResourceTypeClusterWorkflowPlane, cbpList.Items[i].Name, err)
		}
		workflowPlanes = append(workflowPlanes, toClusterWorkflowPlaneResponse(&cbpList.Items[i]))
	}

	return workflowPlanes, nil
}

// toClusterWorkflowPlaneResponse converts a ClusterWorkflowPlane CR to a ClusterWorkflowPlaneResponse
func toClusterWorkflowPlaneResponse(cwp *openchoreov1alpha1.ClusterWorkflowPlane) models.ClusterWorkflowPlaneResponse {
	displayName := cwp.Annotations[controller.AnnotationKeyDisplayName]
	description := cwp.Annotations[controller.AnnotationKeyDescription]

	status := statusUnknown
	if len(cwp.Status.Conditions) > 0 {
		latestCondition := cwp.Status.Conditions[len(cwp.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	response := models.ClusterWorkflowPlaneResponse{
		Name:        cwp.Name,
		PlaneID:     cwp.Spec.PlaneID,
		DisplayName: displayName,
		Description: description,
		CreatedAt:   cwp.CreationTimestamp.Time,
		Status:      status,
	}

	if cwp.Spec.ObservabilityPlaneRef != nil {
		response.ObservabilityPlaneRef = &models.ObservabilityPlaneRef{
			Kind: string(cwp.Spec.ObservabilityPlaneRef.Kind),
			Name: cwp.Spec.ObservabilityPlaneRef.Name,
		}
	}

	if cwp.Status.AgentConnection != nil {
		response.AgentConnection = toAgentConnectionStatusResponse(cwp.Status.AgentConnection)
	}

	return response
}
