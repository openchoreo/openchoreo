// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// namespaceService handles namespace business logic without authorization checks.
type namespaceService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*namespaceService)(nil)

// NewService creates a new namespace service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &namespaceService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *namespaceService) CreateNamespace(ctx context.Context, ns *corev1.Namespace) (*corev1.Namespace, error) {
	if ns == nil {
		return nil, fmt.Errorf("namespace cannot be nil")
	}

	s.logger.Debug("Creating namespace", "namespace", ns.Name)

	exists, err := s.namespaceExists(ctx, ns.Name)
	if err != nil {
		s.logger.Error("Failed to check namespace existence", "error", err)
		return nil, fmt.Errorf("failed to check namespace existence: %w", err)
	}
	if exists {
		s.logger.Warn("Namespace already exists", "namespace", ns.Name)
		return nil, ErrNamespaceAlreadyExists
	}

	// Ensure the control plane label is set
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	ns.Labels[labels.LabelKeyControlPlaneNamespace] = labels.LabelValueTrue

	// Ensure annotations map exists
	if ns.Annotations == nil {
		ns.Annotations = make(map[string]string)
	}

	if err := s.k8sClient.Create(ctx, ns); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Namespace already exists", "namespace", ns.Name)
			return nil, ErrNamespaceAlreadyExists
		}
		s.logger.Error("Failed to create namespace", "error", err)
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}

	s.logger.Debug("Namespace created successfully", "namespace", ns.Name)
	return ns, nil
}

func (s *namespaceService) UpdateNamespace(ctx context.Context, ns *corev1.Namespace) (*corev1.Namespace, error) {
	if ns == nil {
		return nil, fmt.Errorf("namespace cannot be nil")
	}

	s.logger.Debug("Updating namespace", "namespace", ns.Name)

	existing := &corev1.Namespace{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: ns.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Namespace not found", "namespace", ns.Name)
			return nil, ErrNamespaceNotFound
		}
		s.logger.Error("Failed to get namespace", "error", err)
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	// Ensure the control plane label is always preserved
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyControlPlaneNamespace] = labels.LabelValueTrue

	// Update annotations on the existing namespace (only displayName and description are mutable)
	if existing.Annotations == nil {
		existing.Annotations = make(map[string]string)
	}
	if ns.Annotations != nil {
		if v, ok := ns.Annotations[controller.AnnotationKeyDisplayName]; ok {
			existing.Annotations[controller.AnnotationKeyDisplayName] = v
		}
		if v, ok := ns.Annotations[controller.AnnotationKeyDescription]; ok {
			existing.Annotations[controller.AnnotationKeyDescription] = v
		}
	}

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		s.logger.Error("Failed to update namespace", "error", err)
		return nil, fmt.Errorf("failed to update namespace: %w", err)
	}

	s.logger.Debug("Namespace updated successfully", "namespace", ns.Name)
	return existing, nil
}

func (s *namespaceService) ListNamespaces(ctx context.Context, opts services.ListOptions) (*services.ListResult[corev1.Namespace], error) {
	s.logger.Debug("Listing control plane namespaces", "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		// Only include control plane namespaces
		client.MatchingLabels{
			labels.LabelKeyControlPlaneNamespace: labels.LabelValueTrue,
		},
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var nsList corev1.NamespaceList
	if err := s.k8sClient.List(ctx, &nsList, listOpts...); err != nil {
		s.logger.Error("Failed to list control plane namespaces", "error", err)
		return nil, fmt.Errorf("failed to list control plane namespaces: %w", err)
	}

	result := &services.ListResult[corev1.Namespace]{
		Items:      nsList.Items,
		NextCursor: nsList.Continue,
	}
	if nsList.RemainingItemCount != nil {
		remaining := *nsList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed namespaces", "count", len(nsList.Items))
	return result, nil
}

func (s *namespaceService) GetNamespace(ctx context.Context, namespaceName string) (*corev1.Namespace, error) {
	s.logger.Debug("Getting namespace", "namespace", namespaceName)

	ns := &corev1.Namespace{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Namespace not found", "namespace", namespaceName)
			return nil, ErrNamespaceNotFound
		}
		s.logger.Error("Failed to get namespace", "error", err)
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	return ns, nil
}

func (s *namespaceService) DeleteNamespace(ctx context.Context, namespaceName string) error {
	s.logger.Debug("Deleting namespace", "namespace", namespaceName)

	ns := &corev1.Namespace{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Namespace not found", "namespace", namespaceName)
			return ErrNamespaceNotFound
		}
		s.logger.Error("Failed to get namespace", "error", err)
		return fmt.Errorf("failed to get namespace: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, ns); err != nil {
		s.logger.Error("Failed to delete namespace", "error", err)
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	s.logger.Debug("Namespace deleted successfully", "namespace", namespaceName)
	return nil
}

func (s *namespaceService) namespaceExists(ctx context.Context, namespaceName string) (bool, error) {
	ns := &corev1.Namespace{}
	err := s.k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of namespace %s: %w", namespaceName, err)
	}
	return true, nil
}
