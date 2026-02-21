// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

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
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// buildPlaneService handles build plane-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type buildPlaneService struct {
	k8sClient   client.Client
	bpClientMgr *kubernetesClient.KubeMultiClientManager
	logger      *slog.Logger
}

var _ Service = (*buildPlaneService)(nil)

// NewService creates a new build plane service without authorization.
func NewService(k8sClient client.Client, bpClientMgr *kubernetesClient.KubeMultiClientManager, logger *slog.Logger) Service {
	return &buildPlaneService{
		k8sClient:   k8sClient,
		bpClientMgr: bpClientMgr,
		logger:      logger,
	}
}

func (s *buildPlaneService) ListBuildPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.BuildPlane], error) {
	s.logger.Debug("Listing build planes", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var buildPlaneList openchoreov1alpha1.BuildPlaneList
	if err := s.k8sClient.List(ctx, &buildPlaneList, listOpts...); err != nil {
		s.logger.Error("Failed to list build planes", "error", err)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.BuildPlane]{
		Items:      buildPlaneList.Items,
		NextCursor: buildPlaneList.Continue,
	}
	if buildPlaneList.RemainingItemCount != nil {
		remaining := *buildPlaneList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed build planes", "namespace", namespaceName, "count", len(buildPlaneList.Items))
	return result, nil
}

func (s *buildPlaneService) GetBuildPlane(ctx context.Context, namespaceName, buildPlaneName string) (*openchoreov1alpha1.BuildPlane, error) {
	s.logger.Debug("Getting build plane", "namespace", namespaceName, "buildPlane", buildPlaneName)

	buildPlane := &openchoreov1alpha1.BuildPlane{}
	key := client.ObjectKey{
		Name:      buildPlaneName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, buildPlane); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Build plane not found", "namespace", namespaceName, "buildPlane", buildPlaneName)
			return nil, ErrBuildPlaneNotFound
		}
		s.logger.Error("Failed to get build plane", "error", err)
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	return buildPlane, nil
}

// CreateBuildPlane creates a new build plane within a namespace.
func (s *buildPlaneService) CreateBuildPlane(ctx context.Context, namespaceName string, bp *openchoreov1alpha1.BuildPlane) (*openchoreov1alpha1.BuildPlane, error) {
	if bp == nil {
		return nil, ErrBuildPlaneNil
	}
	s.logger.Debug("Creating build plane", "namespace", namespaceName, "buildPlane", bp.Name)

	bp.TypeMeta = metav1.TypeMeta{
		Kind:       "BuildPlane",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	bp.Namespace = namespaceName
	if bp.Labels == nil {
		bp.Labels = make(map[string]string)
	}
	bp.Labels[labels.LabelKeyNamespaceName] = namespaceName
	bp.Labels[labels.LabelKeyName] = bp.Name

	if err := s.k8sClient.Create(ctx, bp); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, ErrBuildPlaneAlreadyExists
		}
		s.logger.Error("Failed to create build plane CR", "error", err)
		return nil, fmt.Errorf("failed to create build plane: %w", err)
	}

	s.logger.Debug("Build plane created successfully", "namespace", namespaceName, "buildPlane", bp.Name)
	return bp, nil
}

// UpdateBuildPlane replaces an existing build plane with the provided object.
func (s *buildPlaneService) UpdateBuildPlane(ctx context.Context, namespaceName string, bp *openchoreov1alpha1.BuildPlane) (*openchoreov1alpha1.BuildPlane, error) {
	if bp == nil {
		return nil, ErrBuildPlaneNil
	}

	s.logger.Debug("Updating build plane", "namespace", namespaceName, "buildPlane", bp.Name)

	existing := &openchoreov1alpha1.BuildPlane{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: bp.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrBuildPlaneNotFound
		}
		s.logger.Error("Failed to get build plane", "error", err)
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	bp.ResourceVersion = existing.ResourceVersion
	bp.Namespace = namespaceName
	if bp.Labels == nil {
		bp.Labels = make(map[string]string)
	}
	bp.Labels[labels.LabelKeyNamespaceName] = namespaceName
	bp.Labels[labels.LabelKeyName] = bp.Name

	if err := s.k8sClient.Update(ctx, bp); err != nil {
		s.logger.Error("Failed to update build plane CR", "error", err)
		return nil, fmt.Errorf("failed to update build plane: %w", err)
	}

	s.logger.Debug("Build plane updated successfully", "namespace", namespaceName, "buildPlane", bp.Name)
	return bp, nil
}

// DeleteBuildPlane removes a build plane by name within a namespace.
func (s *buildPlaneService) DeleteBuildPlane(ctx context.Context, namespaceName, buildPlaneName string) error {
	s.logger.Debug("Deleting build plane", "namespace", namespaceName, "buildPlane", buildPlaneName)

	bp := &openchoreov1alpha1.BuildPlane{}
	key := client.ObjectKey{
		Name:      buildPlaneName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, bp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrBuildPlaneNotFound
		}
		s.logger.Error("Failed to get build plane", "error", err)
		return fmt.Errorf("failed to get build plane: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, bp); err != nil {
		s.logger.Error("Failed to delete build plane CR", "error", err)
		return fmt.Errorf("failed to delete build plane: %w", err)
	}

	s.logger.Debug("Build plane deleted successfully", "namespace", namespaceName, "buildPlane", buildPlaneName)
	return nil
}

// getFirstBuildPlane retrieves the first build plane in a namespace.
// Used internally by GetBuildPlaneClient and ArgoWorkflowExists.
func (s *buildPlaneService) getFirstBuildPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.BuildPlane, error) {
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	if err := s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(namespaceName)); err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	if len(buildPlanes.Items) == 0 {
		s.logger.Warn("No build planes found", "namespace", namespaceName)
		return nil, fmt.Errorf("no build planes found for namespace: %s", namespaceName)
	}

	return &buildPlanes.Items[0], nil
}

// GetBuildPlaneClient creates and returns a Kubernetes client for the build plane cluster.
// This method is deprecated and will be removed in a future version.
// Build plane operations should use the cluster gateway proxy instead.
func (s *buildPlaneService) GetBuildPlaneClient(ctx context.Context, namespaceName, gatewayURL string) (client.Client, error) {
	s.logger.Debug("Getting build plane client", "namespace", namespaceName)

	buildPlane, err := s.getFirstBuildPlane(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	buildPlaneClient, err := kubernetesClient.GetK8sClientFromBuildPlane(
		s.bpClientMgr,
		buildPlane,
		gatewayURL,
	)
	if err != nil {
		s.logger.Error("Failed to create build plane client", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to create build plane client: %w", err)
	}

	s.logger.Debug("Created build plane client", "namespace", namespaceName, "cluster", buildPlane.Name)
	return buildPlaneClient, nil
}

// ArgoWorkflowExists checks whether the Argo Workflow resource referenced by the
// given RunReference still exists on the build plane. Returns true if it exists.
func (s *buildPlaneService) ArgoWorkflowExists(
	ctx context.Context,
	namespaceName, gatewayURL string,
	runReference *openchoreov1alpha1.ResourceReference,
) bool {
	if runReference == nil || runReference.Name == "" || runReference.Namespace == "" {
		return false
	}

	bpClient, err := s.GetBuildPlaneClient(ctx, namespaceName, gatewayURL)
	if err != nil {
		s.logger.Debug("Failed to get build plane client for workflow existence check", "error", err)
		return false
	}

	var argoWorkflow argoproj.Workflow
	if err := bpClient.Get(ctx, types.NamespacedName{
		Name:      runReference.Name,
		Namespace: runReference.Namespace,
	}, &argoWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false
		}
		s.logger.Debug("Failed to check argo workflow existence on build plane", "error", err)
		return false
	}

	return true
}
