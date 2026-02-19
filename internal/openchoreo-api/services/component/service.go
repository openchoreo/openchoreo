// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

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

// componentService handles component-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type componentService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*componentService)(nil)

// NewService creates a new component service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &componentService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *componentService) CreateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	if component == nil {
		return nil, fmt.Errorf("component cannot be nil")
	}

	s.logger.Debug("Creating component", "namespace", namespaceName, "component", component.Name)

	exists, err := s.componentExists(ctx, namespaceName, component.Name)
	if err != nil {
		s.logger.Error("Failed to check component existence", "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}
	if exists {
		s.logger.Warn("Component already exists", "namespace", namespaceName, "component", component.Name)
		return nil, ErrComponentAlreadyExists
	}

	// Set defaults
	component.TypeMeta = metav1.TypeMeta{
		Kind:       "Component",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	component.Namespace = namespaceName
	if component.Labels == nil {
		component.Labels = make(map[string]string)
	}
	component.Labels[labels.LabelKeyNamespaceName] = namespaceName
	component.Labels[labels.LabelKeyName] = component.Name
	component.Labels[labels.LabelKeyProjectName] = component.Spec.Owner.ProjectName

	if err := s.k8sClient.Create(ctx, component); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Component already exists", "namespace", namespaceName, "component", component.Name)
			return nil, ErrComponentAlreadyExists
		}
		s.logger.Error("Failed to create component CR", "error", err)
		return nil, fmt.Errorf("failed to create component: %w", err)
	}

	s.logger.Debug("Component created successfully", "namespace", namespaceName, "component", component.Name)
	return component, nil
}

func (s *componentService) UpdateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	if component == nil {
		return nil, fmt.Errorf("component cannot be nil")
	}

	s.logger.Debug("Updating component", "namespace", namespaceName, "component", component.Name)

	existing := &openchoreov1alpha1.Component{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: component.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "component", component.Name)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	component.ResourceVersion = existing.ResourceVersion
	component.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, component); err != nil {
		s.logger.Error("Failed to update component CR", "error", err)
		return nil, fmt.Errorf("failed to update component: %w", err)
	}

	s.logger.Debug("Component updated successfully", "namespace", namespaceName, "component", component.Name)
	return component, nil
}

func (s *componentService) ListComponents(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error) {
	s.logger.Debug("Listing components", "namespace", namespaceName, "project", projectName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var componentList openchoreov1alpha1.ComponentList
	if err := s.k8sClient.List(ctx, &componentList, listOpts...); err != nil {
		s.logger.Error("Failed to list components", "error", err)
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	items := componentList.Items
	if projectName != "" {
		filtered := make([]openchoreov1alpha1.Component, 0, len(items))
		for _, c := range items {
			if c.Spec.Owner.ProjectName == projectName {
				filtered = append(filtered, c)
			}
		}
		items = filtered
	}

	result := &services.ListResult[openchoreov1alpha1.Component]{
		Items:      items,
		NextCursor: componentList.Continue,
	}
	if componentList.RemainingItemCount != nil {
		remaining := *componentList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed components", "namespace", namespaceName, "count", len(items))
	return result, nil
}

func (s *componentService) GetComponent(ctx context.Context, namespaceName, componentName string) (*openchoreov1alpha1.Component, error) {
	s.logger.Debug("Getting component", "namespace", namespaceName, "component", componentName)

	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	return component, nil
}

func (s *componentService) DeleteComponent(ctx context.Context, namespaceName, componentName string) error {
	s.logger.Debug("Deleting component", "namespace", namespaceName, "component", componentName)

	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "component", componentName)
			return ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return fmt.Errorf("failed to get component: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, component); err != nil {
		s.logger.Error("Failed to delete component CR", "error", err)
		return fmt.Errorf("failed to delete component: %w", err)
	}

	s.logger.Debug("Component deleted successfully", "namespace", namespaceName, "component", componentName)
	return nil
}

func (s *componentService) componentExists(ctx context.Context, namespaceName, componentName string) (bool, error) {
	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, component)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of component %s/%s: %w", namespaceName, componentName, err)
	}
	return true, nil
}
