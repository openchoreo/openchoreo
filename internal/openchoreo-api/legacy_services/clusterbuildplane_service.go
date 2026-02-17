// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacy_services

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

// ClusterBuildPlaneService handles cluster-scoped build plane-related business logic
type ClusterBuildPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewClusterBuildPlaneService creates a new cluster build plane service
func NewClusterBuildPlaneService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ClusterBuildPlaneService {
	return &ClusterBuildPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListClusterBuildPlanes lists all cluster-scoped build planes
func (s *ClusterBuildPlaneService) ListClusterBuildPlanes(ctx context.Context) ([]models.ClusterBuildPlaneResponse, error) {
	s.logger.Debug("Listing cluster build planes")

	var cbpList openchoreov1alpha1.ClusterBuildPlaneList
	if err := s.k8sClient.List(ctx, &cbpList); err != nil {
		s.logger.Error("Failed to list cluster build planes", "error", err)
		return nil, fmt.Errorf("failed to list cluster build planes: %w", err)
	}

	s.logger.Debug("Found cluster build planes", "count", len(cbpList.Items))

	buildPlanes := make([]models.ClusterBuildPlaneResponse, 0, len(cbpList.Items))
	for i := range cbpList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterBuildPlane, ResourceTypeClusterBuildPlane, cbpList.Items[i].Name,
			authz.ResourceHierarchy{}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized cluster build plane", "clusterBuildPlane", cbpList.Items[i].Name)
				continue
			}
			return nil, fmt.Errorf("authorize %s %s for clusterBuildPlane %q: %w",
				SystemActionViewClusterBuildPlane, ResourceTypeClusterBuildPlane, cbpList.Items[i].Name, err)
		}
		buildPlanes = append(buildPlanes, toClusterBuildPlaneResponse(&cbpList.Items[i]))
	}

	return buildPlanes, nil
}

// toClusterBuildPlaneResponse converts a ClusterBuildPlane CR to a ClusterBuildPlaneResponse
func toClusterBuildPlaneResponse(cbp *openchoreov1alpha1.ClusterBuildPlane) models.ClusterBuildPlaneResponse {
	displayName := cbp.Annotations[controller.AnnotationKeyDisplayName]
	description := cbp.Annotations[controller.AnnotationKeyDescription]

	status := statusUnknown
	if len(cbp.Status.Conditions) > 0 {
		latestCondition := cbp.Status.Conditions[len(cbp.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	response := models.ClusterBuildPlaneResponse{
		Name:        cbp.Name,
		PlaneID:     cbp.Spec.PlaneID,
		DisplayName: displayName,
		Description: description,
		CreatedAt:   cbp.CreationTimestamp.Time,
		Status:      status,
	}

	if cbp.Spec.ObservabilityPlaneRef != nil {
		response.ObservabilityPlaneRef = &models.ObservabilityPlaneRef{
			Kind: string(cbp.Spec.ObservabilityPlaneRef.Kind),
			Name: cbp.Spec.ObservabilityPlaneRef.Name,
		}
	}

	if cbp.Status.AgentConnection != nil {
		response.AgentConnection = toAgentConnectionStatusResponse(cbp.Status.AgentConnection)
	}

	return response
}
