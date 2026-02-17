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

// ObservabilityPlaneService handles observability plane-related business logic
type ObservabilityPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewObservabilityPlaneService creates a new observability plane service
func NewObservabilityPlaneService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ObservabilityPlaneService {
	return &ObservabilityPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListObservabilityPlanes retrieves all observability planes for an namespace
func (s *ObservabilityPlaneService) ListObservabilityPlanes(ctx context.Context, namespaceName string) ([]models.ObservabilityPlaneResponse, error) {
	s.logger.Debug("Listing observability planes", "namespace", namespaceName)

	// List all observability planes in the namespace namespace
	var observabilityPlanes openchoreov1alpha1.ObservabilityPlaneList
	err := s.k8sClient.List(ctx, &observabilityPlanes, client.InNamespace(namespaceName))
	if err != nil {
		s.logger.Error("Failed to list observability planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list observability planes: %w", err)
	}

	s.logger.Debug("Found observability planes", "count", len(observabilityPlanes.Items), "namespace", namespaceName)

	// Convert to response format
	observabilityPlaneResponses := make([]models.ObservabilityPlaneResponse, 0, len(observabilityPlanes.Items))
	for i := range observabilityPlanes.Items {
		if err := checkAuthorization(
			ctx,
			s.logger,
			s.authzPDP,
			SystemActionViewObservabilityPlane,
			ResourceTypeObservabilityPlane,
			observabilityPlanes.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName},
		); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized observability plane", "namespace", namespaceName, "observabilityPlane", observabilityPlanes.Items[i].Name)
				continue
			}
			return nil, err
		}

		observabilityPlaneResponses = append(observabilityPlaneResponses, toObservabilityPlaneResponse(&observabilityPlanes.Items[i]))
	}

	return observabilityPlaneResponses, nil
}

// toObservabilityPlaneResponse converts an ObservabilityPlane CR to an ObservabilityPlaneResponse
func toObservabilityPlaneResponse(op *openchoreov1alpha1.ObservabilityPlane) models.ObservabilityPlaneResponse {
	displayName := op.Annotations[controller.AnnotationKeyDisplayName]
	description := op.Annotations[controller.AnnotationKeyDescription]

	// Determine status from conditions
	status := statusUnknown
	if len(op.Status.Conditions) > 0 {
		latestCondition := op.Status.Conditions[len(op.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	response := models.ObservabilityPlaneResponse{
		Name:        op.Name,
		Namespace:   op.Namespace,
		DisplayName: displayName,
		Description: description,
		ObserverURL: op.Spec.ObserverURL,
		CreatedAt:   op.CreationTimestamp.Time,
		Status:      status,
	}

	if op.Status.AgentConnection != nil {
		response.AgentConnection = toAgentConnectionStatusResponse(op.Status.AgentConnection)
	}

	return response
}
