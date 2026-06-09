// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

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

var observabilityPlaneTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ObservabilityPlane",
}

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

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

	var opList openchoreov1alpha1.ObservabilityPlaneList
	if err := s.k8sClient.List(ctx, &opList, listOpts...); err != nil {
		s.logger.Error("Failed to list observability planes", "error", err)
		return nil, fmt.Errorf("failed to list observability planes: %w", err)
	}

	for i := range opList.Items {
		opList.Items[i].TypeMeta = observabilityPlaneTypeMeta
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

	op.TypeMeta = observabilityPlaneTypeMeta
	return op, nil
}

// CreateObservabilityPlane creates a new observability plane within a namespace.
func (s *observabilityPlaneService) CreateObservabilityPlane(ctx context.Context, namespaceName string, op *openchoreov1alpha1.ObservabilityPlane) (*openchoreov1alpha1.ObservabilityPlane, error) {
	if op == nil {
		return nil, fmt.Errorf("observability plane cannot be nil")
	}
	s.logger.Debug("Creating observability plane", "namespace", namespaceName, "observabilityPlane", op.Name)

	op.Status = openchoreov1alpha1.ObservabilityPlaneStatus{}
	op.Namespace = namespaceName
	if err := s.k8sClient.Create(ctx, op); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, ErrObservabilityPlaneAlreadyExists
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create observability plane CR", "error", err)
		return nil, fmt.Errorf("failed to create observability plane: %w", err)
	}

	s.logger.Debug("Observability plane created successfully", "namespace", namespaceName, "observabilityPlane", op.Name)
	op.TypeMeta = observabilityPlaneTypeMeta
	return op, nil
}

// UpdateObservabilityPlane replaces an existing observability plane with the provided object.
func (s *observabilityPlaneService) UpdateObservabilityPlane(ctx context.Context, namespaceName string, op *openchoreov1alpha1.ObservabilityPlane) (*openchoreov1alpha1.ObservabilityPlane, error) {
	if op == nil {
		return nil, fmt.Errorf("observability plane cannot be nil")
	}
	s.logger.Debug("Updating observability plane", "namespace", namespaceName, "observabilityPlane", op.Name)

	existing := &openchoreov1alpha1.ObservabilityPlane{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: op.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrObservabilityPlaneNotFound
		}
		s.logger.Error("Failed to get observability plane", "error", err)
		return nil, fmt.Errorf("failed to get observability plane: %w", err)
	}

	// Clear status from user input — status is server-managed
	op.Status = openchoreov1alpha1.ObservabilityPlaneStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = op.Spec
	existing.Labels = op.Labels
	existing.Annotations = op.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to update observability plane CR", "error", err)
		return nil, fmt.Errorf("failed to update observability plane: %w", err)
	}

	s.logger.Debug("Observability plane updated successfully", "namespace", namespaceName, "observabilityPlane", op.Name)
	existing.TypeMeta = observabilityPlaneTypeMeta
	return existing, nil
}

// DeleteObservabilityPlane removes an observability plane by name within a namespace.
func (s *observabilityPlaneService) DeleteObservabilityPlane(ctx context.Context, namespaceName, observabilityPlaneName string) error {
	s.logger.Debug("Deleting observability plane", "namespace", namespaceName, "observabilityPlane", observabilityPlaneName)

	op := &openchoreov1alpha1.ObservabilityPlane{}
	op.Name = observabilityPlaneName
	op.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, op); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrObservabilityPlaneNotFound
		}
		s.logger.Error("Failed to delete observability plane CR", "error", err)
		return fmt.Errorf("failed to delete observability plane: %w", err)
	}

	s.logger.Debug("Observability plane deleted successfully", "namespace", namespaceName, "observabilityPlane", observabilityPlaneName)
	return nil
}
