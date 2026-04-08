// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

var workflowPlaneTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "WorkflowPlane",
}

// workflowPlaneService handles workflow plane-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type workflowPlaneService struct {
	k8sClient           client.Client
	planeClientProvider kubernetesClient.WorkflowPlaneClientProvider
	logger              *slog.Logger
}

var _ Service = (*workflowPlaneService)(nil)

// NewService creates a new workflow plane service without authorization.
func NewService(k8sClient client.Client, planeClientProvider kubernetesClient.WorkflowPlaneClientProvider, logger *slog.Logger) Service {
	return &workflowPlaneService{
		k8sClient:           k8sClient,
		planeClientProvider: planeClientProvider,
		logger:              logger,
	}
}

func (s *workflowPlaneService) ListWorkflowPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.WorkflowPlane], error) {
	s.logger.Debug("Listing workflow planes", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var workflowPlaneList openchoreov1alpha1.WorkflowPlaneList
	if err := s.k8sClient.List(ctx, &workflowPlaneList, listOpts...); err != nil {
		s.logger.Error("Failed to list workflow planes", "error", err)
		return nil, fmt.Errorf("failed to list workflow planes: %w", err)
	}

	for i := range workflowPlaneList.Items {
		workflowPlaneList.Items[i].TypeMeta = workflowPlaneTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.WorkflowPlane]{
		Items:      workflowPlaneList.Items,
		NextCursor: workflowPlaneList.Continue,
	}
	if workflowPlaneList.RemainingItemCount != nil {
		remaining := *workflowPlaneList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed workflow planes", "namespace", namespaceName, "count", len(workflowPlaneList.Items))
	return result, nil
}

func (s *workflowPlaneService) GetWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) (*openchoreov1alpha1.WorkflowPlane, error) {
	s.logger.Debug("Getting workflow plane", "namespace", namespaceName, "workflowPlane", workflowPlaneName)

	workflowPlane := &openchoreov1alpha1.WorkflowPlane{}
	key := client.ObjectKey{
		Name:      workflowPlaneName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, workflowPlane); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow plane not found", "namespace", namespaceName, "workflowPlane", workflowPlaneName)
			return nil, ErrWorkflowPlaneNotFound
		}
		s.logger.Error("Failed to get workflow plane", "error", err)
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}

	workflowPlane.TypeMeta = workflowPlaneTypeMeta
	return workflowPlane, nil
}

// CreateWorkflowPlane creates a new workflow plane within a namespace.
func (s *workflowPlaneService) CreateWorkflowPlane(ctx context.Context, namespaceName string, wp *openchoreov1alpha1.WorkflowPlane) (*openchoreov1alpha1.WorkflowPlane, error) {
	if wp == nil {
		return nil, ErrWorkflowPlaneNil
	}
	s.logger.Debug("Creating workflow plane", "namespace", namespaceName, "workflowPlane", wp.Name)

	wp.Status = openchoreov1alpha1.WorkflowPlaneStatus{}
	wp.Namespace = namespaceName
	if err := s.k8sClient.Create(ctx, wp); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, ErrWorkflowPlaneAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create workflow plane CR", "error", err)
		return nil, fmt.Errorf("failed to create workflow plane: %w", err)
	}

	s.logger.Debug("Workflow plane created successfully", "namespace", namespaceName, "workflowPlane", wp.Name)
	wp.TypeMeta = workflowPlaneTypeMeta
	return wp, nil
}

// UpdateWorkflowPlane replaces an existing workflow plane with the provided object.
func (s *workflowPlaneService) UpdateWorkflowPlane(ctx context.Context, namespaceName string, wp *openchoreov1alpha1.WorkflowPlane) (*openchoreov1alpha1.WorkflowPlane, error) {
	if wp == nil {
		return nil, ErrWorkflowPlaneNil
	}

	s.logger.Debug("Updating workflow plane", "namespace", namespaceName, "workflowPlane", wp.Name)

	existing := &openchoreov1alpha1.WorkflowPlane{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: wp.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrWorkflowPlaneNotFound
		}
		s.logger.Error("Failed to get workflow plane", "error", err)
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}

	// Clear status from user input — status is server-managed
	wp.Status = openchoreov1alpha1.WorkflowPlaneStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = wp.Spec
	existing.Labels = wp.Labels
	existing.Annotations = wp.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update workflow plane CR", "error", err)
		return nil, fmt.Errorf("failed to update workflow plane: %w", err)
	}

	s.logger.Debug("Workflow plane updated successfully", "namespace", namespaceName, "workflowPlane", wp.Name)
	existing.TypeMeta = workflowPlaneTypeMeta
	return existing, nil
}

// DeleteWorkflowPlane removes a workflow plane by name within a namespace.
func (s *workflowPlaneService) DeleteWorkflowPlane(ctx context.Context, namespaceName, workflowPlaneName string) error {
	s.logger.Debug("Deleting workflow plane", "namespace", namespaceName, "workflowPlane", workflowPlaneName)

	wp := &openchoreov1alpha1.WorkflowPlane{}
	wp.Name = workflowPlaneName
	wp.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, wp); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrWorkflowPlaneNotFound
		}
		s.logger.Error("Failed to delete workflow plane CR", "error", err)
		return fmt.Errorf("failed to delete workflow plane: %w", err)
	}

	s.logger.Debug("Workflow plane deleted successfully", "namespace", namespaceName, "workflowPlane", workflowPlaneName)
	return nil
}

// getFirstWorkflowPlane retrieves the first workflow plane in a namespace.
// Used internally by GetWorkflowPlaneClient and ArgoWorkflowExists.
func (s *workflowPlaneService) getFirstWorkflowPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.WorkflowPlane, error) {
	var workflowPlanes openchoreov1alpha1.WorkflowPlaneList
	if err := s.k8sClient.List(ctx, &workflowPlanes, client.InNamespace(namespaceName)); err != nil {
		s.logger.Error("Failed to list workflow planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list workflow planes: %w", err)
	}

	if len(workflowPlanes.Items) == 0 {
		s.logger.Warn("No workflow planes found", "namespace", namespaceName)
		return nil, fmt.Errorf("no workflow planes found for namespace: %s", namespaceName)
	}

	return &workflowPlanes.Items[0], nil
}

// GetWorkflowPlaneClient creates and returns a Kubernetes client for the workflow plane cluster.
// This method is deprecated and will be removed in a future version.
// Workflow plane operations should use the cluster gateway proxy instead.
func (s *workflowPlaneService) GetWorkflowPlaneClient(ctx context.Context, namespaceName string) (client.Client, error) {
	s.logger.Debug("Getting workflow plane client", "namespace", namespaceName)

	workflowPlane, err := s.getFirstWorkflowPlane(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}

	workflowPlaneClient, err := s.planeClientProvider.WorkflowPlaneClient(workflowPlane)
	if err != nil {
		s.logger.Error("Failed to create workflow plane client", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to create workflow plane client: %w", err)
	}

	s.logger.Debug("Created workflow plane client", "namespace", namespaceName, "cluster", workflowPlane.Name)
	return workflowPlaneClient, nil
}

// ArgoWorkflowExists checks whether the Argo Workflow resource referenced by the
// given RunReference still exists on the workflow plane. Returns true if it exists.
func (s *workflowPlaneService) ArgoWorkflowExists(
	ctx context.Context,
	namespaceName string,
	runReference *openchoreov1alpha1.ResourceReference,
) bool {
	if runReference == nil || runReference.Name == "" || runReference.Namespace == "" {
		return false
	}

	wpClient, err := s.GetWorkflowPlaneClient(ctx, namespaceName)
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
