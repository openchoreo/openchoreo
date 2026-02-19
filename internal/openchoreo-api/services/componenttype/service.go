// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

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

// componentTypeService handles component type business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type componentTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*componentTypeService)(nil)

// NewService creates a new component type service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &componentTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *componentTypeService) CreateComponentType(ctx context.Context, namespaceName string, ct *openchoreov1alpha1.ComponentType) (*openchoreov1alpha1.ComponentType, error) {
	if ct == nil {
		return nil, fmt.Errorf("component type cannot be nil")
	}

	s.logger.Debug("Creating component type", "namespace", namespaceName, "componentType", ct.Name)

	exists, err := s.componentTypeExists(ctx, namespaceName, ct.Name)
	if err != nil {
		s.logger.Error("Failed to check component type existence", "error", err)
		return nil, fmt.Errorf("failed to check component type existence: %w", err)
	}
	if exists {
		s.logger.Warn("Component type already exists", "namespace", namespaceName, "componentType", ct.Name)
		return nil, ErrComponentTypeAlreadyExists
	}

	// Set defaults
	ct.TypeMeta = metav1.TypeMeta{
		Kind:       "ComponentType",
		APIVersion: "core.choreo.dev/v1alpha1",
	}
	ct.Namespace = namespaceName
	if ct.Labels == nil {
		ct.Labels = make(map[string]string)
	}
	ct.Labels[labels.LabelKeyNamespaceName] = namespaceName
	ct.Labels[labels.LabelKeyName] = ct.Name

	if err := s.k8sClient.Create(ctx, ct); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Component type already exists", "namespace", namespaceName, "componentType", ct.Name)
			return nil, ErrComponentTypeAlreadyExists
		}
		s.logger.Error("Failed to create component type CR", "error", err)
		return nil, fmt.Errorf("failed to create component type: %w", err)
	}

	s.logger.Debug("Component type created successfully", "namespace", namespaceName, "componentType", ct.Name)
	return ct, nil
}

func (s *componentTypeService) UpdateComponentType(ctx context.Context, namespaceName string, ct *openchoreov1alpha1.ComponentType) (*openchoreov1alpha1.ComponentType, error) {
	if ct == nil {
		return nil, fmt.Errorf("component type cannot be nil")
	}

	s.logger.Debug("Updating component type", "namespace", namespaceName, "componentType", ct.Name)

	existing := &openchoreov1alpha1.ComponentType{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: ct.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component type not found", "namespace", namespaceName, "componentType", ct.Name)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get component type", "error", err)
		return nil, fmt.Errorf("failed to get component type: %w", err)
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	ct.ResourceVersion = existing.ResourceVersion
	ct.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, ct); err != nil {
		s.logger.Error("Failed to update component type CR", "error", err)
		return nil, fmt.Errorf("failed to update component type: %w", err)
	}

	s.logger.Debug("Component type updated successfully", "namespace", namespaceName, "componentType", ct.Name)
	return ct, nil
}

func (s *componentTypeService) ListComponentTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentType], error) {
	s.logger.Debug("Listing component types", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var ctList openchoreov1alpha1.ComponentTypeList
	if err := s.k8sClient.List(ctx, &ctList, listOpts...); err != nil {
		s.logger.Error("Failed to list component types", "error", err)
		return nil, fmt.Errorf("failed to list component types: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.ComponentType]{
		Items:      ctList.Items,
		NextCursor: ctList.Continue,
	}
	if ctList.RemainingItemCount != nil {
		remaining := *ctList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed component types", "namespace", namespaceName, "count", len(ctList.Items))
	return result, nil
}

func (s *componentTypeService) GetComponentType(ctx context.Context, namespaceName, ctName string) (*openchoreov1alpha1.ComponentType, error) {
	s.logger.Debug("Getting component type", "namespace", namespaceName, "componentType", ctName)

	ct := &openchoreov1alpha1.ComponentType{}
	key := client.ObjectKey{
		Name:      ctName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component type not found", "namespace", namespaceName, "componentType", ctName)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get component type", "error", err)
		return nil, fmt.Errorf("failed to get component type: %w", err)
	}

	return ct, nil
}

func (s *componentTypeService) DeleteComponentType(ctx context.Context, namespaceName, ctName string) error {
	s.logger.Debug("Deleting component type", "namespace", namespaceName, "componentType", ctName)

	ct := &openchoreov1alpha1.ComponentType{}
	key := client.ObjectKey{
		Name:      ctName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component type not found", "namespace", namespaceName, "componentType", ctName)
			return ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get component type", "error", err)
		return fmt.Errorf("failed to get component type: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, ct); err != nil {
		s.logger.Error("Failed to delete component type CR", "error", err)
		return fmt.Errorf("failed to delete component type: %w", err)
	}

	s.logger.Debug("Component type deleted successfully", "namespace", namespaceName, "componentType", ctName)
	return nil
}

func (s *componentTypeService) componentTypeExists(ctx context.Context, namespaceName, ctName string) (bool, error) {
	ct := &openchoreov1alpha1.ComponentType{}
	key := client.ObjectKey{
		Name:      ctName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, ct)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of component type %s/%s: %w", namespaceName, ctName, err)
	}
	return true, nil
}
