// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// workflowPlaneClient bundles a workflow plane k8s client with the identity
// information required to route gateway calls (pod logs, events) to the
// correct agent proxy URL. For namespace-scoped WorkflowPlanes the
// planeNamespace is the CR namespace; for cluster-scoped ClusterWorkflowPlanes
// it is "_cluster".
type workflowPlaneClient struct {
	client         client.Client
	planeID        string
	planeNamespace string
	planeName      string
}

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
		// Treat absence of any WorkflowPlane in the namespace as a Kubernetes-style NotFound error
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "workflowplanes"}, namespaceName)
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

// getClusterWorkflowPlane retrieves the default-named ClusterWorkflowPlane (cluster-scoped) without auth checks.
func (s *WorkflowPlaneService) getClusterWorkflowPlane(ctx context.Context) (*openchoreov1alpha1.ClusterWorkflowPlane, error) {
	var list openchoreov1alpha1.ClusterWorkflowPlaneList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		s.logger.Error("Failed to list cluster workflow planes", "error", err)
		return nil, fmt.Errorf("failed to list cluster workflow planes: %w", err)
	}
	if len(list.Items) == 0 {
		s.logger.Warn("No cluster workflow planes found")
		return nil, fmt.Errorf("no cluster workflow planes found")
	}
	// Prefer the default-named plane if present to avoid relying on list ordering.
	for i := range list.Items {
		if list.Items[i].Name == controller.DefaultPlaneName {
			cwp := &list.Items[i]
			s.logger.Debug("Found default-named cluster workflow plane", "name", cwp.Name)
			return cwp, nil
		}
	}

	// No default-named plane found; return the first item.
	s.logger.Warn(
		"No default-named cluster workflow plane found, falling back to first item",
		"defaultName", controller.DefaultPlaneName,
		"count", len(list.Items),
	)
	cwp := &list.Items[0]
	s.logger.Debug("Falling back to first cluster workflow plane", "name", cwp.Name)
	return cwp, nil
}

// GetClusterWorkflowPlaneClient creates and returns a Kubernetes client for the cluster workflow plane.
func (s *WorkflowPlaneService) GetClusterWorkflowPlaneClient(ctx context.Context, gatewayURL string) (client.Client, error) {
	cwp, err := s.getClusterWorkflowPlane(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster workflow plane: %w", err)
	}
	c, err := kubernetesClient.GetK8sClientFromClusterWorkflowPlane(s.wpClientMgr, cwp, gatewayURL)
	if err != nil {
		s.logger.Error("Failed to create cluster workflow plane client", "error", err)
		return nil, fmt.Errorf("failed to create cluster workflow plane client: %w", err)
	}
	s.logger.Debug("Created cluster workflow plane client", "cluster", cwp.Name)
	return c, nil
}

// getWorkflowPlaneClientWithFallback tries to get a namespace-scoped WorkflowPlane client first,
// falling back to the cluster-scoped ClusterWorkflowPlane when none exists. Returns a
// workflowPlaneClient struct with the client and plane identity needed for gateway calls.
func (s *WorkflowPlaneService) getWorkflowPlaneClientWithFallback(ctx context.Context, namespaceName string, gatewayURL string) (*workflowPlaneClient, error) {
	// Try namespace-scoped WorkflowPlane first
	wp, err := s.getWorkflowPlane(ctx, namespaceName)
	if err == nil {
		planeID := wp.Spec.PlaneID
		if planeID == "" {
			planeID = wp.Name
		}
		c, err := kubernetesClient.GetK8sClientFromWorkflowPlane(s.wpClientMgr, wp, gatewayURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create workflow plane client: %w", err)
		}
		return &workflowPlaneClient{
			client:         c,
			planeID:        planeID,
			planeNamespace: wp.Namespace,
			planeName:      wp.Name,
		}, nil
	}

	// Only fall back to the cluster-scoped plane when the namespace-scoped plane is actually not found.
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get namespace workflow plane: %w", err)
	}

	s.logger.Debug("No namespace-scoped workflow plane, falling back to cluster workflow plane", "namespace", namespaceName, "error", err)

	// Fall back to cluster-scoped ClusterWorkflowPlane
	cwp, err := s.getClusterWorkflowPlane(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get any workflow plane for namespace %s: %w", namespaceName, err)
	}
	planeID := cwp.Spec.PlaneID
	if planeID == "" {
		planeID = cwp.Name
	}
	c, err := kubernetesClient.GetK8sClientFromClusterWorkflowPlane(s.wpClientMgr, cwp, gatewayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster workflow plane client: %w", err)
	}
	return &workflowPlaneClient{
		client:         c,
		planeID:        planeID,
		planeNamespace: "_cluster",
		planeName:      cwp.Name,
	}, nil
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

	wpc, err := s.getWorkflowPlaneClientWithFallback(ctx, namespaceName, gatewayURL)
	if err != nil {
		s.logger.Debug("Failed to get workflow plane client for workflow existence check", "error", err)
		return false
	}

	var argoWorkflow argoproj.Workflow
	if err := wpc.client.Get(ctx, types.NamespacedName{
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
