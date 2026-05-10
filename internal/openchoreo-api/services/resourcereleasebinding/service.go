// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

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

// resourceReleaseBindingService handles resource release binding business logic without authorization checks.
type resourceReleaseBindingService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var resourceReleaseBindingTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ResourceReleaseBinding",
}

var _ Service = (*resourceReleaseBindingService)(nil)

// NewService creates a new resource release binding service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &resourceReleaseBindingService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *resourceReleaseBindingService) CreateResourceReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.ResourceReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("resource release binding cannot be nil")
	}

	s.logger.Debug("Creating resource release binding", "namespace", namespaceName, "resourceReleaseBinding", rb.Name)

	if err := s.validateResourceExists(ctx, namespaceName, rb.Spec.Owner.ResourceName); err != nil {
		return nil, err
	}

	exists, err := s.bindingExists(ctx, namespaceName, rb.Name)
	if err != nil {
		s.logger.Error("Failed to check binding existence", "error", err)
		return nil, fmt.Errorf("failed to check binding existence: %w", err)
	}
	if exists {
		s.logger.Warn("Resource release binding already exists", "namespace", namespaceName, "resourceReleaseBinding", rb.Name)
		return nil, ErrResourceReleaseBindingAlreadyExists
	}

	// Set defaults
	rb.Namespace = namespaceName
	rb.Status = openchoreov1alpha1.ResourceReleaseBindingStatus{}
	if rb.Labels == nil {
		rb.Labels = make(map[string]string)
	}
	rb.Labels[labels.LabelKeyProjectName] = rb.Spec.Owner.ProjectName
	rb.Labels[labels.LabelKeyResourceName] = rb.Spec.Owner.ResourceName

	if err := s.k8sClient.Create(ctx, rb); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Resource release binding already exists", "namespace", namespaceName, "resourceReleaseBinding", rb.Name)
			return nil, ErrResourceReleaseBindingAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create resource release binding CR", "error", err)
		return nil, fmt.Errorf("failed to create resource release binding: %w", err)
	}

	s.logger.Debug("Resource release binding created successfully", "namespace", namespaceName, "resourceReleaseBinding", rb.Name)
	rb.TypeMeta = resourceReleaseBindingTypeMeta
	return rb, nil
}

func (s *resourceReleaseBindingService) UpdateResourceReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ResourceReleaseBinding) (*openchoreov1alpha1.ResourceReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("resource release binding cannot be nil")
	}

	s.logger.Debug("Updating resource release binding", "namespace", namespaceName, "resourceReleaseBinding", rb.Name)

	existing := &openchoreov1alpha1.ResourceReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: rb.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Resource release binding not found", "namespace", namespaceName, "resourceReleaseBinding", rb.Name)
			return nil, ErrResourceReleaseBindingNotFound
		}
		s.logger.Error("Failed to get resource release binding", "error", err)
		return nil, fmt.Errorf("failed to get resource release binding: %w", err)
	}

	// Clear status from user input — status is server-managed
	rb.Status = openchoreov1alpha1.ResourceReleaseBindingStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = rb.Spec
	existing.Labels = rb.Labels
	existing.Annotations = rb.Annotations

	// Preserve special labels
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyProjectName] = existing.Spec.Owner.ProjectName
	existing.Labels[labels.LabelKeyResourceName] = existing.Spec.Owner.ResourceName

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update resource release binding CR", "error", err)
		return nil, fmt.Errorf("failed to update resource release binding: %w", err)
	}

	s.logger.Debug("Resource release binding updated successfully", "namespace", namespaceName, "resourceReleaseBinding", rb.Name)
	existing.TypeMeta = resourceReleaseBindingTypeMeta
	return existing, nil
}

func (s *resourceReleaseBindingService) ListResourceReleaseBindings(ctx context.Context, namespaceName, resourceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceReleaseBinding], error) {
	s.logger.Debug("Listing resource release bindings", "namespace", namespaceName, "resource", resourceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceReleaseBinding], error) {
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var rbList openchoreov1alpha1.ResourceReleaseBindingList
		if err := s.k8sClient.List(ctx, &rbList, listOpts...); err != nil {
			s.logger.Error("Failed to list resource release bindings", "error", err)
			return nil, fmt.Errorf("failed to list resource release bindings: %w", err)
		}

		for i := range rbList.Items {
			rbList.Items[i].TypeMeta = resourceReleaseBindingTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.ResourceReleaseBinding]{
			Items:      rbList.Items,
			NextCursor: rbList.Continue,
		}
		if rbList.RemainingItemCount != nil {
			remaining := *rbList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	if resourceName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(rb openchoreov1alpha1.ResourceReleaseBinding) bool {
				return rb.Spec.Owner.ResourceName == resourceName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *resourceReleaseBindingService) GetResourceReleaseBinding(ctx context.Context, namespaceName, resourceReleaseBindingName string) (*openchoreov1alpha1.ResourceReleaseBinding, error) {
	s.logger.Debug("Getting resource release binding", "namespace", namespaceName, "resourceReleaseBinding", resourceReleaseBindingName)

	rb := &openchoreov1alpha1.ResourceReleaseBinding{}
	key := client.ObjectKey{
		Name:      resourceReleaseBindingName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Resource release binding not found", "namespace", namespaceName, "resourceReleaseBinding", resourceReleaseBindingName)
			return nil, ErrResourceReleaseBindingNotFound
		}
		s.logger.Error("Failed to get resource release binding", "error", err)
		return nil, fmt.Errorf("failed to get resource release binding: %w", err)
	}

	rb.TypeMeta = resourceReleaseBindingTypeMeta
	return rb, nil
}

func (s *resourceReleaseBindingService) DeleteResourceReleaseBinding(ctx context.Context, namespaceName, resourceReleaseBindingName string) error {
	s.logger.Debug("Deleting resource release binding", "namespace", namespaceName, "resourceReleaseBinding", resourceReleaseBindingName)

	rb := &openchoreov1alpha1.ResourceReleaseBinding{}
	rb.Name = resourceReleaseBindingName
	rb.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rb); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrResourceReleaseBindingNotFound
		}
		s.logger.Error("Failed to delete resource release binding CR", "error", err)
		return fmt.Errorf("failed to delete resource release binding: %w", err)
	}

	s.logger.Debug("Resource release binding deleted successfully", "namespace", namespaceName, "resourceReleaseBinding", resourceReleaseBindingName)
	return nil
}

func (s *resourceReleaseBindingService) bindingExists(ctx context.Context, namespaceName, name string) (bool, error) {
	rb := &openchoreov1alpha1.ResourceReleaseBinding{}
	key := client.ObjectKey{Name: name, Namespace: namespaceName}

	err := s.k8sClient.Get(ctx, key, rb)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of resource release binding %s/%s: %w", namespaceName, name, err)
	}
	return true, nil
}

func (s *resourceReleaseBindingService) validateResourceExists(ctx context.Context, namespaceName, resourceName string) error {
	r := &openchoreov1alpha1.Resource{}
	key := client.ObjectKey{
		Name:      resourceName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, r); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrResourceNotFound
		}
		return fmt.Errorf("failed to validate resource: %w", err)
	}
	return nil
}
