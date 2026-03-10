// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// clusterWorkflowPlaneService handles cluster workflow plane-related business logic without authorization checks.
type clusterWorkflowPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterWorkflowPlaneService)(nil)

var clusterWorkflowPlaneTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterWorkflowPlane",
}

// NewService creates a new cluster workflow plane service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterWorkflowPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterWorkflowPlaneService) ListClusterWorkflowPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterWorkflowPlane], error) {
	s.logger.Debug("Listing cluster workflow planes", "limit", opts.Limit, "cursor", opts.Cursor)

	var listOpts []client.ListOption
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var cbpList openchoreov1alpha1.ClusterWorkflowPlaneList
	if err := s.k8sClient.List(ctx, &cbpList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster workflow planes", "error", err)
		return nil, fmt.Errorf("failed to list cluster workflow planes: %w", err)
	}

	for i := range cbpList.Items {
		cbpList.Items[i].TypeMeta = clusterWorkflowPlaneTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterWorkflowPlane]{
		Items:      cbpList.Items,
		NextCursor: cbpList.Continue,
	}
	if cbpList.RemainingItemCount != nil {
		remaining := *cbpList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster workflow planes", "count", len(cbpList.Items))
	return result, nil
}

func (s *clusterWorkflowPlaneService) GetClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) (*openchoreov1alpha1.ClusterWorkflowPlane, error) {
	s.logger.Debug("Getting cluster workflow plane", "clusterWorkflowPlane", clusterWorkflowPlaneName)

	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	key := client.ObjectKey{
		Name: clusterWorkflowPlaneName,
	}

	if err := s.k8sClient.Get(ctx, key, cwp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster workflow plane not found", "clusterWorkflowPlane", clusterWorkflowPlaneName)
			return nil, ErrClusterWorkflowPlaneNotFound
		}
		s.logger.Error("Failed to get cluster workflow plane", "error", err)
		return nil, fmt.Errorf("failed to get cluster workflow plane: %w", err)
	}

	cwp.TypeMeta = clusterWorkflowPlaneTypeMeta
	return cwp, nil
}

// CreateClusterWorkflowPlane creates a new cluster-scoped workflow plane.
func (s *clusterWorkflowPlaneService) CreateClusterWorkflowPlane(ctx context.Context, cwp *openchoreov1alpha1.ClusterWorkflowPlane) (*openchoreov1alpha1.ClusterWorkflowPlane, error) {
	if cwp == nil {
		return nil, ErrClusterWorkflowPlaneNil
	}
	s.logger.Debug("Creating cluster workflow plane", "clusterWorkflowPlane", cwp.Name)

	cwp.Status = openchoreov1alpha1.ClusterWorkflowPlaneStatus{}

	if err := s.k8sClient.Create(ctx, cwp); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, ErrClusterWorkflowPlaneAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create cluster workflow plane CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster workflow plane: %w", err)
	}

	s.logger.Debug("Cluster workflow plane created successfully", "clusterWorkflowPlane", cwp.Name)
	cwp.TypeMeta = clusterWorkflowPlaneTypeMeta
	return cwp, nil
}

// UpdateClusterWorkflowPlane replaces an existing cluster-scoped workflow plane with the provided object.
func (s *clusterWorkflowPlaneService) UpdateClusterWorkflowPlane(ctx context.Context, cwp *openchoreov1alpha1.ClusterWorkflowPlane) (*openchoreov1alpha1.ClusterWorkflowPlane, error) {
	if cwp == nil {
		return nil, ErrClusterWorkflowPlaneNil
	}

	s.logger.Debug("Updating cluster workflow plane", "clusterWorkflowPlane", cwp.Name)

	existing := &openchoreov1alpha1.ClusterWorkflowPlane{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: cwp.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrClusterWorkflowPlaneNotFound
		}
		s.logger.Error("Failed to get cluster workflow plane", "error", err)
		return nil, fmt.Errorf("failed to get cluster workflow plane: %w", err)
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	cwp.Status = openchoreov1alpha1.ClusterWorkflowPlaneStatus{}
	existing.Spec = cwp.Spec
	existing.Labels = cwp.Labels
	existing.Annotations = cwp.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update cluster workflow plane CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster workflow plane: %w", err)
	}

	s.logger.Debug("Cluster workflow plane updated successfully", "clusterWorkflowPlane", cwp.Name)
	existing.TypeMeta = clusterWorkflowPlaneTypeMeta
	return existing, nil
}

// DeleteClusterWorkflowPlane removes a cluster-scoped workflow plane by name.
func (s *clusterWorkflowPlaneService) DeleteClusterWorkflowPlane(ctx context.Context, clusterWorkflowPlaneName string) error {
	s.logger.Debug("Deleting cluster workflow plane", "clusterWorkflowPlane", clusterWorkflowPlaneName)

	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	cwp.Name = clusterWorkflowPlaneName

	if err := s.k8sClient.Delete(ctx, cwp); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrClusterWorkflowPlaneNotFound
		}
		s.logger.Error("Failed to delete cluster workflow plane CR", "error", err)
		return fmt.Errorf("failed to delete cluster workflow plane: %w", err)
	}

	s.logger.Debug("Cluster workflow plane deleted successfully", "clusterWorkflowPlane", clusterWorkflowPlaneName)
	return nil
}
