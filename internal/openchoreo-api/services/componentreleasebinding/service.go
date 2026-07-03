// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentreleasebinding

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

// componentReleaseBindingService handles release binding business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type componentReleaseBindingService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var componentReleaseBindingTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ComponentReleaseBinding",
}

var _ Service = (*componentReleaseBindingService)(nil)

// NewService creates a new release binding service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &componentReleaseBindingService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *componentReleaseBindingService) CreateComponentReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ComponentReleaseBinding) (*openchoreov1alpha1.ComponentReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("release binding cannot be nil")
	}

	s.logger.Debug("Creating release binding", "namespace", namespaceName, "componentReleaseBinding", rb.Name)

	// Validate that the referenced component exists
	if err := s.validateComponentExists(ctx, namespaceName, rb.Spec.Owner.ComponentName); err != nil {
		return nil, err
	}

	exists, err := s.componentReleaseBindingExists(ctx, namespaceName, rb.Name)
	if err != nil {
		s.logger.Error("Failed to check release binding existence", "error", err)
		return nil, fmt.Errorf("failed to check release binding existence: %w", err)
	}
	if exists {
		s.logger.Warn("Release binding already exists", "namespace", namespaceName, "componentReleaseBinding", rb.Name)
		return nil, ErrComponentReleaseBindingAlreadyExists
	}

	// Set defaults
	rb.Namespace = namespaceName
	rb.Status = openchoreov1alpha1.ComponentReleaseBindingStatus{}
	if rb.Labels == nil {
		rb.Labels = make(map[string]string)
	}
	rb.Labels[labels.LabelKeyProjectName] = rb.Spec.Owner.ProjectName
	rb.Labels[labels.LabelKeyComponentName] = rb.Spec.Owner.ComponentName

	if err := s.k8sClient.Create(ctx, rb); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Release binding already exists", "namespace", namespaceName, "componentReleaseBinding", rb.Name)
			return nil, ErrComponentReleaseBindingAlreadyExists
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create release binding CR", "error", err)
		return nil, fmt.Errorf("failed to create release binding: %w", err)
	}

	s.logger.Debug("Release binding created successfully", "namespace", namespaceName, "componentReleaseBinding", rb.Name)
	rb.TypeMeta = componentReleaseBindingTypeMeta
	return rb, nil
}

func (s *componentReleaseBindingService) UpdateComponentReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ComponentReleaseBinding) (*openchoreov1alpha1.ComponentReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("release binding cannot be nil")
	}

	s.logger.Debug("Updating release binding", "namespace", namespaceName, "componentReleaseBinding", rb.Name)

	existing := &openchoreov1alpha1.ComponentReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: rb.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Release binding not found", "namespace", namespaceName, "componentReleaseBinding", rb.Name)
			return nil, ErrComponentReleaseBindingNotFound
		}
		s.logger.Error("Failed to get release binding", "error", err)
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	// Clear status from user input — status is server-managed
	rb.Status = openchoreov1alpha1.ComponentReleaseBindingStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = rb.Spec
	existing.Labels = rb.Labels
	existing.Annotations = rb.Annotations

	// Preserve special labels
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyProjectName] = existing.Spec.Owner.ProjectName
	existing.Labels[labels.LabelKeyComponentName] = existing.Spec.Owner.ComponentName

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			s.logger.Error("Release binding update rejected by validation", "error", err)
			return nil, vErr
		}
		s.logger.Error("Failed to update release binding CR", "error", err)
		return nil, fmt.Errorf("failed to update release binding: %w", err)
	}

	s.logger.Debug("Release binding updated successfully", "namespace", namespaceName, "componentReleaseBinding", rb.Name)
	existing.TypeMeta = componentReleaseBindingTypeMeta
	return existing, nil
}

func (s *componentReleaseBindingService) ListComponentComponentReleaseBindings(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentReleaseBinding], error) {
	s.logger.Debug("Listing release bindings", "namespace", namespaceName, "component", componentName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ComponentReleaseBinding], error) {
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var rbList openchoreov1alpha1.ComponentReleaseBindingList
		if err := s.k8sClient.List(ctx, &rbList, listOpts...); err != nil {
			s.logger.Error("Failed to list release bindings", "error", err)
			return nil, fmt.Errorf("failed to list release bindings: %w", err)
		}

		for i := range rbList.Items {
			rbList.Items[i].TypeMeta = componentReleaseBindingTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.ComponentReleaseBinding]{
			Items:      rbList.Items,
			NextCursor: rbList.Continue,
		}
		if rbList.RemainingItemCount != nil {
			remaining := *rbList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	// Apply component filter if specified
	if componentName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(rb openchoreov1alpha1.ComponentReleaseBinding) bool {
				return rb.Spec.Owner.ComponentName == componentName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *componentReleaseBindingService) GetComponentReleaseBinding(ctx context.Context, namespaceName, componentReleaseBindingName string) (*openchoreov1alpha1.ComponentReleaseBinding, error) {
	s.logger.Debug("Getting release binding", "namespace", namespaceName, "componentReleaseBinding", componentReleaseBindingName)

	rb := &openchoreov1alpha1.ComponentReleaseBinding{}
	key := client.ObjectKey{
		Name:      componentReleaseBindingName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Release binding not found", "namespace", namespaceName, "componentReleaseBinding", componentReleaseBindingName)
			return nil, ErrComponentReleaseBindingNotFound
		}
		s.logger.Error("Failed to get release binding", "error", err)
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	rb.TypeMeta = componentReleaseBindingTypeMeta
	return rb, nil
}

func (s *componentReleaseBindingService) DeleteComponentReleaseBinding(ctx context.Context, namespaceName, componentReleaseBindingName string) error {
	s.logger.Debug("Deleting release binding", "namespace", namespaceName, "componentReleaseBinding", componentReleaseBindingName)

	rb := &openchoreov1alpha1.ComponentReleaseBinding{}
	rb.Name = componentReleaseBindingName
	rb.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rb); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrComponentReleaseBindingNotFound
		}
		s.logger.Error("Failed to delete release binding CR", "error", err)
		return fmt.Errorf("failed to delete release binding: %w", err)
	}

	s.logger.Debug("Release binding deleted successfully", "namespace", namespaceName, "componentReleaseBinding", componentReleaseBindingName)
	return nil
}

func (s *componentReleaseBindingService) componentReleaseBindingExists(ctx context.Context, namespaceName, componentReleaseBindingName string) (bool, error) {
	rb := &openchoreov1alpha1.ComponentReleaseBinding{}
	key := client.ObjectKey{
		Name:      componentReleaseBindingName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, rb)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of release binding %s/%s: %w", namespaceName, componentReleaseBindingName, err)
	}
	return true, nil
}

func (s *componentReleaseBindingService) validateComponentExists(ctx context.Context, namespaceName, componentName string) error {
	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrComponentNotFound
		}
		return fmt.Errorf("failed to validate component: %w", err)
	}
	return nil
}
