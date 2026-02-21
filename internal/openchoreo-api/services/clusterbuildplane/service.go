// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterbuildplane

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// clusterBuildPlaneService handles cluster build plane-related business logic without authorization checks.
type clusterBuildPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterBuildPlaneService)(nil)

// NewService creates a new cluster build plane service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterBuildPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterBuildPlaneService) ListClusterBuildPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterBuildPlane], error) {
	s.logger.Debug("Listing cluster build planes", "limit", opts.Limit, "cursor", opts.Cursor)

	var listOpts []client.ListOption
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var cbpList openchoreov1alpha1.ClusterBuildPlaneList
	if err := s.k8sClient.List(ctx, &cbpList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster build planes", "error", err)
		return nil, fmt.Errorf("failed to list cluster build planes: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterBuildPlane]{
		Items:      cbpList.Items,
		NextCursor: cbpList.Continue,
	}
	if cbpList.RemainingItemCount != nil {
		remaining := *cbpList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster build planes", "count", len(cbpList.Items))
	return result, nil
}

func (s *clusterBuildPlaneService) GetClusterBuildPlane(ctx context.Context, clusterBuildPlaneName string) (*openchoreov1alpha1.ClusterBuildPlane, error) {
	s.logger.Debug("Getting cluster build plane", "clusterBuildPlane", clusterBuildPlaneName)

	cbp := &openchoreov1alpha1.ClusterBuildPlane{}
	key := client.ObjectKey{
		Name: clusterBuildPlaneName,
	}

	if err := s.k8sClient.Get(ctx, key, cbp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster build plane not found", "clusterBuildPlane", clusterBuildPlaneName)
			return nil, ErrClusterBuildPlaneNotFound
		}
		s.logger.Error("Failed to get cluster build plane", "error", err)
		return nil, fmt.Errorf("failed to get cluster build plane: %w", err)
	}

	return cbp, nil
}

// CreateClusterBuildPlane creates a new cluster-scoped build plane.
func (s *clusterBuildPlaneService) CreateClusterBuildPlane(ctx context.Context, cbp *openchoreov1alpha1.ClusterBuildPlane) (*openchoreov1alpha1.ClusterBuildPlane, error) {
	if cbp == nil {
		return nil, ErrClusterBuildPlaneNil
	}
	s.logger.Debug("Creating cluster build plane", "clusterBuildPlane", cbp.Name)

	cbp.TypeMeta = metav1.TypeMeta{
		Kind:       "ClusterBuildPlane",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	if cbp.Labels == nil {
		cbp.Labels = make(map[string]string)
	}
	cbp.Labels[labels.LabelKeyName] = cbp.Name

	if err := s.k8sClient.Create(ctx, cbp); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, ErrClusterBuildPlaneAlreadyExists
		}
		s.logger.Error("Failed to create cluster build plane CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster build plane: %w", err)
	}

	s.logger.Debug("Cluster build plane created successfully", "clusterBuildPlane", cbp.Name)
	return cbp, nil
}

// UpdateClusterBuildPlane replaces an existing cluster-scoped build plane with the provided object.
func (s *clusterBuildPlaneService) UpdateClusterBuildPlane(ctx context.Context, cbp *openchoreov1alpha1.ClusterBuildPlane) (*openchoreov1alpha1.ClusterBuildPlane, error) {
	if cbp == nil {
		return nil, ErrClusterBuildPlaneNil
	}

	s.logger.Debug("Updating cluster build plane", "clusterBuildPlane", cbp.Name)

	existing := &openchoreov1alpha1.ClusterBuildPlane{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: cbp.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrClusterBuildPlaneNotFound
		}
		s.logger.Error("Failed to get cluster build plane", "error", err)
		return nil, fmt.Errorf("failed to get cluster build plane: %w", err)
	}

	cbp.ResourceVersion = existing.ResourceVersion
	if cbp.Labels == nil {
		cbp.Labels = make(map[string]string)
	}
	cbp.Labels[labels.LabelKeyName] = cbp.Name

	if err := s.k8sClient.Update(ctx, cbp); err != nil {
		s.logger.Error("Failed to update cluster build plane CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster build plane: %w", err)
	}

	s.logger.Debug("Cluster build plane updated successfully", "clusterBuildPlane", cbp.Name)
	return cbp, nil
}

// DeleteClusterBuildPlane removes a cluster-scoped build plane by name.
func (s *clusterBuildPlaneService) DeleteClusterBuildPlane(ctx context.Context, clusterBuildPlaneName string) error {
	s.logger.Debug("Deleting cluster build plane", "clusterBuildPlane", clusterBuildPlaneName)

	cbp := &openchoreov1alpha1.ClusterBuildPlane{}
	key := client.ObjectKey{Name: clusterBuildPlaneName}

	if err := s.k8sClient.Get(ctx, key, cbp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrClusterBuildPlaneNotFound
		}
		s.logger.Error("Failed to get cluster build plane", "error", err)
		return fmt.Errorf("failed to get cluster build plane: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, cbp); err != nil {
		s.logger.Error("Failed to delete cluster build plane CR", "error", err)
		return fmt.Errorf("failed to delete cluster build plane: %w", err)
	}

	s.logger.Debug("Cluster build plane deleted successfully", "clusterBuildPlane", clusterBuildPlaneName)
	return nil
}
