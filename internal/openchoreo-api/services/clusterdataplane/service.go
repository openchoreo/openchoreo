// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

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

// clusterDataPlaneService handles cluster data plane-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type clusterDataPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterDataPlaneService)(nil)

// NewService creates a new cluster data plane service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterDataPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterDataPlaneService) ListClusterDataPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterDataPlane], error) {
	s.logger.Debug("Listing cluster data planes", "limit", opts.Limit, "cursor", opts.Cursor)

	var listOpts []client.ListOption
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var clusterDataPlaneList openchoreov1alpha1.ClusterDataPlaneList
	if err := s.k8sClient.List(ctx, &clusterDataPlaneList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster data planes", "error", err)
		return nil, fmt.Errorf("failed to list cluster data planes: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterDataPlane]{
		Items:      clusterDataPlaneList.Items,
		NextCursor: clusterDataPlaneList.Continue,
	}
	if clusterDataPlaneList.RemainingItemCount != nil {
		remaining := *clusterDataPlaneList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster data planes", "count", len(clusterDataPlaneList.Items))
	return result, nil
}

func (s *clusterDataPlaneService) GetClusterDataPlane(ctx context.Context, name string) (*openchoreov1alpha1.ClusterDataPlane, error) {
	s.logger.Debug("Getting cluster data plane", "clusterDataPlane", name)

	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	key := client.ObjectKey{Name: name}

	if err := s.k8sClient.Get(ctx, key, cdp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster data plane not found", "clusterDataPlane", name)
			return nil, ErrClusterDataPlaneNotFound
		}
		s.logger.Error("Failed to get cluster data plane", "error", err)
		return nil, fmt.Errorf("failed to get cluster data plane: %w", err)
	}

	return cdp, nil
}

func (s *clusterDataPlaneService) CreateClusterDataPlane(ctx context.Context, cdp *openchoreov1alpha1.ClusterDataPlane) (*openchoreov1alpha1.ClusterDataPlane, error) {
	if cdp == nil {
		return nil, ErrClusterDataPlaneNil
	}

	s.logger.Debug("Creating cluster data plane", "clusterDataPlane", cdp.Name)

	exists, err := s.clusterDataPlaneExists(ctx, cdp.Name)
	if err != nil {
		s.logger.Error("Failed to check cluster data plane existence", "error", err)
		return nil, fmt.Errorf("failed to check cluster data plane existence: %w", err)
	}
	if exists {
		s.logger.Warn("Cluster data plane already exists", "clusterDataPlane", cdp.Name)
		return nil, ErrClusterDataPlaneAlreadyExists
	}

	// Set defaults
	cdp.TypeMeta = metav1.TypeMeta{
		Kind:       "ClusterDataPlane",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	if cdp.Labels == nil {
		cdp.Labels = make(map[string]string)
	}
	cdp.Labels[labels.LabelKeyName] = cdp.Name

	if err := s.k8sClient.Create(ctx, cdp); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster data plane already exists", "clusterDataPlane", cdp.Name)
			return nil, ErrClusterDataPlaneAlreadyExists
		}
		s.logger.Error("Failed to create cluster data plane CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster data plane: %w", err)
	}

	s.logger.Debug("Cluster data plane created successfully", "clusterDataPlane", cdp.Name)
	return cdp, nil
}

func (s *clusterDataPlaneService) clusterDataPlaneExists(ctx context.Context, name string) (bool, error) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	key := client.ObjectKey{Name: name}

	err := s.k8sClient.Get(ctx, key, cdp)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of cluster data plane %s: %w", name, err)
	}
	return true, nil
}
