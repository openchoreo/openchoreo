// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// clusterObservabilityPlaneService handles cluster observability plane-related business logic without authorization checks.
type clusterObservabilityPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterObservabilityPlaneService)(nil)

// NewService creates a new cluster observability plane service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterObservabilityPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterObservabilityPlaneService) ListClusterObservabilityPlanes(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterObservabilityPlane], error) {
	s.logger.Debug("Listing cluster observability planes", "limit", opts.Limit, "cursor", opts.Cursor)

	var listOpts []client.ListOption
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var copList openchoreov1alpha1.ClusterObservabilityPlaneList
	if err := s.k8sClient.List(ctx, &copList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster observability planes", "error", err)
		return nil, fmt.Errorf("failed to list cluster observability planes: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterObservabilityPlane]{
		Items:      copList.Items,
		NextCursor: copList.Continue,
	}
	if copList.RemainingItemCount != nil {
		remaining := *copList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster observability planes", "count", len(copList.Items))
	return result, nil
}

func (s *clusterObservabilityPlaneService) GetClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) (*openchoreov1alpha1.ClusterObservabilityPlane, error) {
	s.logger.Debug("Getting cluster observability plane", "clusterObservabilityPlane", clusterObservabilityPlaneName)

	cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
	key := client.ObjectKey{
		Name: clusterObservabilityPlaneName,
	}

	if err := s.k8sClient.Get(ctx, key, cop); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster observability plane not found", "clusterObservabilityPlane", clusterObservabilityPlaneName)
			return nil, ErrClusterObservabilityPlaneNotFound
		}
		s.logger.Error("Failed to get cluster observability plane", "error", err)
		return nil, fmt.Errorf("failed to get cluster observability plane: %w", err)
	}

	return cop, nil
}
