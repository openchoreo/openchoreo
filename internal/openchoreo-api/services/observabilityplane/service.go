// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// observabilityPlaneService handles observability plane-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type observabilityPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*observabilityPlaneService)(nil)

// NewService creates a new observability plane service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &observabilityPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *observabilityPlaneService) ListObservabilityPlanes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ObservabilityPlane], error) {
	s.logger.Debug("Listing observability planes", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var opList openchoreov1alpha1.ObservabilityPlaneList
	if err := s.k8sClient.List(ctx, &opList, listOpts...); err != nil {
		s.logger.Error("Failed to list observability planes", "error", err)
		return nil, fmt.Errorf("failed to list observability planes: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.ObservabilityPlane]{
		Items:      opList.Items,
		NextCursor: opList.Continue,
	}
	if opList.RemainingItemCount != nil {
		remaining := *opList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed observability planes", "namespace", namespaceName, "count", len(opList.Items))
	return result, nil
}

func (s *observabilityPlaneService) GetObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) (*openchoreov1alpha1.ObservabilityPlane, error) {
	s.logger.Debug("Getting observability plane", "namespace", namespaceName, "observabilityPlane", observabilityPlaneName)

	op := &openchoreov1alpha1.ObservabilityPlane{}
	key := client.ObjectKey{
		Name:      observabilityPlaneName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, op); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Observability plane not found", "namespace", namespaceName, "observabilityPlane", observabilityPlaneName)
			return nil, ErrObservabilityPlaneNotFound
		}
		s.logger.Error("Failed to get observability plane", "error", err)
		return nil, fmt.Errorf("failed to get observability plane: %w", err)
	}

	return op, nil
}
