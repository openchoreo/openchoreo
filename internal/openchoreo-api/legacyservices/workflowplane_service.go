// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// WorkflowPlaneService handles workflow plane-related business logic
type WorkflowPlaneService struct {
	k8sClient   client.Client
	wpClientMgr *kubernetesClient.KubeMultiClientManager
	logger      *slog.Logger
	authzPDP    authz.PDP
}

// NewWorkflowPlaneService creates a new workflow plane service
func NewWorkflowPlaneService(k8sClient client.Client, wpClientMgr *kubernetesClient.KubeMultiClientManager, logger *slog.Logger, authzPDP authz.PDP) *WorkflowPlaneService {
	return &WorkflowPlaneService{
		k8sClient:   k8sClient,
		wpClientMgr: wpClientMgr,
		logger:      logger,
		authzPDP:    authzPDP,
	}
}

// getWorkflowPlane retrieves the workflow plane for an namespace without authorization checks (internal use only)
func (s *WorkflowPlaneService) getWorkflowPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.WorkflowPlane, error) {
	// List all workflow planes in the namespace namespace
	var workflowPlanes openchoreov1alpha1.WorkflowPlaneList
	err := s.k8sClient.List(ctx, &workflowPlanes, client.InNamespace(namespaceName))
	if err != nil {
		s.logger.Error("Failed to list workflow planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list workflow planes: %w", err)
	}

	// Check if any workflow planes exist
	if len(workflowPlanes.Items) == 0 {
		s.logger.Warn("No workflow planes found", "namespace", namespaceName)
		return nil, fmt.Errorf("no workflow planes found for namespace: %s", namespaceName)
	}

	// Return the first workflow plane (0th index)
	workflowPlane := &workflowPlanes.Items[0]

	s.logger.Debug("Found workflow plane", "name", workflowPlane.Name, "namespace", namespaceName)

	return workflowPlane, nil
}

// GetWorkflowPlane retrieves the workflow plane for an namespace
func (s *WorkflowPlaneService) GetWorkflowPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.WorkflowPlane, error) {
	s.logger.Debug("Getting workflow plane", "namespace", namespaceName)
	workflowPlane, err := s.getWorkflowPlane(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowPlane, ResourceTypeWorkflowPlane, workflowPlane.Name,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	return workflowPlane, nil
}

// GetWorkflowPlaneClient creates and returns a Kubernetes client for the workflow plane cluster
// This method is deprecated and will be removed in a future version.
// Workflow plane operations should use the cluster gateway proxy instead.
func (s *WorkflowPlaneService) GetWorkflowPlaneClient(ctx context.Context, namespaceName string, gatewayURL string) (client.Client, error) {
	s.logger.Debug("Getting workflow plane client", "namespace", namespaceName)

	// Get the workflow plane first
	workflowPlane, err := s.getWorkflowPlane(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}

	// Use cluster agent proxy mode
	workflowPlaneClient, err := kubernetesClient.GetK8sClientFromWorkflowPlane(
		s.wpClientMgr,
		workflowPlane,
		gatewayURL,
	)
	if err != nil {
		s.logger.Error("Failed to create workflow plane client", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to create workflow plane client: %w", err)
	}

	s.logger.Debug("Created workflow plane client", "namespace", namespaceName, "cluster", workflowPlane.Name)
	return workflowPlaneClient, nil
}

// ListWorkflowPlanes retrieves all workflow planes for an namespace
func (s *WorkflowPlaneService) ListWorkflowPlanes(ctx context.Context, namespaceName string) ([]models.WorkflowPlaneResponse, error) {
	s.logger.Debug("Listing workflow planes", "namespace", namespaceName)

	// List all workflow planes in the namespace namespace
	var workflowPlanes openchoreov1alpha1.WorkflowPlaneList
	err := s.k8sClient.List(ctx, &workflowPlanes, client.InNamespace(namespaceName))
	if err != nil {
		s.logger.Error("Failed to list workflow planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list workflow planes: %w", err)
	}

	s.logger.Debug("Found workflow planes", "count", len(workflowPlanes.Items), "namespace", namespaceName)

	// Convert to response format
	workflowPlaneResponses := make([]models.WorkflowPlaneResponse, 0, len(workflowPlanes.Items))
	for i := range workflowPlanes.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkflowPlane, ResourceTypeWorkflowPlane, workflowPlanes.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized workflow plane", "namespace", namespaceName, "workflowPlane", workflowPlanes.Items[i].Name)
				continue
			}
			return nil, err
		}

		workflowPlaneResponses = append(workflowPlaneResponses, toWorkflowPlaneResponse(&workflowPlanes.Items[i]))
	}

	return workflowPlaneResponses, nil
}

// toWorkflowPlaneResponse converts a WorkflowPlane CR to a WorkflowPlaneResponse
func toWorkflowPlaneResponse(wp *openchoreov1alpha1.WorkflowPlane) models.WorkflowPlaneResponse {
	displayName := wp.Annotations[controller.AnnotationKeyDisplayName]
	description := wp.Annotations[controller.AnnotationKeyDescription]

	// Determine status from conditions
	status := statusUnknown
	if len(wp.Status.Conditions) > 0 {
		latestCondition := wp.Status.Conditions[len(wp.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	response := models.WorkflowPlaneResponse{
		Name:        wp.Name,
		Namespace:   wp.Namespace,
		DisplayName: displayName,
		Description: description,
		CreatedAt:   wp.CreationTimestamp.Time,
		Status:      status,
	}

	if wp.Spec.ObservabilityPlaneRef != nil {
		response.ObservabilityPlaneRef = &models.ObservabilityPlaneRef{
			Kind: string(wp.Spec.ObservabilityPlaneRef.Kind),
			Name: wp.Spec.ObservabilityPlaneRef.Name,
		}
	}

	if wp.Status.AgentConnection != nil {
		response.AgentConnection = toAgentConnectionStatusResponse(wp.Status.AgentConnection)
	}

	return response
}

// ArgoWorkflowExists checks whether the Argo Workflow resource referenced by the
// given RunReference still exists on the workflow plane. Returns true if it exists.
func (s *WorkflowPlaneService) ArgoWorkflowExists(
	ctx context.Context,
	namespaceName string,
	gatewayURL string,
	runReference *openchoreov1alpha1.ResourceReference,
) bool {
	if runReference == nil || runReference.Name == "" || runReference.Namespace == "" {
		return false
	}

	wpClient, err := s.GetWorkflowPlaneClient(ctx, namespaceName, gatewayURL)
	if err != nil {
		s.logger.Debug("Failed to get workflow plane client for workflow existence check", "error", err)
		return false
	}

	var argoWorkflow argoproj.Workflow
	if err := wpClient.Get(ctx, types.NamespacedName{
		Name:      runReference.Name,
		Namespace: runReference.Namespace,
	}, &argoWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false
		}
		s.logger.Debug("Failed to check argo workflow existence on workflow plane", "error", err)
		return false
	}

	return true
}
