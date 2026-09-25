// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

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

// clusterHookService handles cluster hook business logic without authorization checks.
type clusterHookService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterHookService)(nil)

var clusterHookTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterHook",
}

// NewService creates a new cluster hook service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterHookService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterHookService) CreateClusterHook(ctx context.Context, ch *openchoreov1alpha1.ClusterHook) (*openchoreov1alpha1.ClusterHook, error) {
	if ch == nil {
		return nil, fmt.Errorf("cluster hook cannot be nil")
	}

	s.logger.Debug("Creating cluster hook", "clusterHook", ch.Name)

	// Set defaults
	ch.Status = openchoreov1alpha1.HookStatus{}

	if err := s.k8sClient.Create(ctx, ch); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster hook already exists", "clusterHook", ch.Name)
			return nil, ErrClusterHookAlreadyExists
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create cluster hook CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster hook: %w", err)
	}

	s.logger.Debug("Cluster hook created successfully", "clusterHook", ch.Name)
	ch.TypeMeta = clusterHookTypeMeta
	return ch, nil
}

func (s *clusterHookService) UpdateClusterHook(ctx context.Context, ch *openchoreov1alpha1.ClusterHook) (*openchoreov1alpha1.ClusterHook, error) {
	if ch == nil {
		return nil, fmt.Errorf("cluster hook cannot be nil")
	}

	s.logger.Debug("Updating cluster hook", "clusterHook", ch.Name)

	existing := &openchoreov1alpha1.ClusterHook{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: ch.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster hook not found", "clusterHook", ch.Name)
			return nil, ErrClusterHookNotFound
		}
		s.logger.Error("Failed to get cluster hook", "error", err)
		return nil, fmt.Errorf("failed to get cluster hook: %w", err)
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = ch.Spec
	existing.Labels = ch.Labels
	existing.Annotations = ch.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to update cluster hook CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster hook: %w", err)
	}

	s.logger.Debug("Cluster hook updated successfully", "clusterHook", ch.Name)
	existing.TypeMeta = clusterHookTypeMeta
	return existing, nil
}

func (s *clusterHookService) ListClusterHooks(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterHook], error) {
	s.logger.Debug("Listing cluster hooks", "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}

	var hookList openchoreov1alpha1.ClusterHookList
	if err := s.k8sClient.List(ctx, &hookList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster hooks", "error", err)
		return nil, fmt.Errorf("failed to list cluster hooks: %w", err)
	}

	for i := range hookList.Items {
		hookList.Items[i].TypeMeta = clusterHookTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterHook]{
		Items:      hookList.Items,
		NextCursor: hookList.Continue,
	}
	if hookList.RemainingItemCount != nil {
		remaining := *hookList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster hooks", "count", len(hookList.Items))
	return result, nil
}

func (s *clusterHookService) GetClusterHook(ctx context.Context, clusterHookName string) (*openchoreov1alpha1.ClusterHook, error) {
	s.logger.Debug("Getting cluster hook", "clusterHook", clusterHookName)

	ch := &openchoreov1alpha1.ClusterHook{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: clusterHookName}, ch); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster hook not found", "clusterHook", clusterHookName)
			return nil, ErrClusterHookNotFound
		}
		s.logger.Error("Failed to get cluster hook", "error", err)
		return nil, fmt.Errorf("failed to get cluster hook: %w", err)
	}

	ch.TypeMeta = clusterHookTypeMeta
	return ch, nil
}

// DeleteClusterHook removes a cluster-scoped hook by name.
func (s *clusterHookService) DeleteClusterHook(ctx context.Context, clusterHookName string) error {
	s.logger.Debug("Deleting cluster hook", "clusterHook", clusterHookName)

	ch := &openchoreov1alpha1.ClusterHook{}
	ch.Name = clusterHookName

	if err := s.k8sClient.Delete(ctx, ch); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrClusterHookNotFound
		}
		s.logger.Error("Failed to delete cluster hook CR", "error", err)
		return fmt.Errorf("failed to delete cluster hook: %w", err)
	}

	s.logger.Debug("Cluster hook deleted successfully", "clusterHook", clusterHookName)
	return nil
}
