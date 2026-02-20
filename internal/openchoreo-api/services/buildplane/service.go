// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// buildPlaneService handles build plane-related business logic without authorization checks.
type buildPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*buildPlaneService)(nil)

// NewService creates a new build plane service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &buildPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
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
