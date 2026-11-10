// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

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

// endpointReleaseBindingService handles endpoint release binding business logic without authorization checks.
type endpointReleaseBindingService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var endpointReleaseBindingTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "EndpointReleaseBinding",
}

var _ Service = (*endpointReleaseBindingService)(nil)

// NewService creates a new endpoint release binding service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &endpointReleaseBindingService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *endpointReleaseBindingService) CreateEndpointReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.EndpointReleaseBinding) (*openchoreov1alpha1.EndpointReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("endpoint release binding cannot be nil")
	}

	s.logger.Debug("Creating endpoint release binding", "namespace", namespaceName, "endpointReleaseBinding", rb.Name)

	if err := s.validateEndpointExists(ctx, namespaceName, rb.Spec.Owner.EndpointName); err != nil {
		return nil, err
	}

	exists, err := s.bindingExists(ctx, namespaceName, rb.Name)
	if err != nil {
		s.logger.Error("Failed to check binding existence", "error", err)
		return nil, fmt.Errorf("failed to check binding existence: %w", err)
	}
	if exists {
		s.logger.Warn("Endpoint release binding already exists", "namespace", namespaceName, "endpointReleaseBinding", rb.Name)
		return nil, ErrEndpointReleaseBindingAlreadyExists
	}

	// Set defaults
	rb.Namespace = namespaceName
	rb.Status = openchoreov1alpha1.EndpointReleaseBindingStatus{}
	if rb.Labels == nil {
		rb.Labels = make(map[string]string)
	}
	rb.Labels[labels.LabelKeyProjectName] = rb.Spec.Owner.ProjectName
	rb.Labels[labels.LabelKeyEndpointCRName] = rb.Spec.Owner.EndpointName

	if err := s.k8sClient.Create(ctx, rb); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Endpoint release binding already exists", "namespace", namespaceName, "endpointReleaseBinding", rb.Name)
			return nil, ErrEndpointReleaseBindingAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create endpoint release binding CR", "error", err)
		return nil, fmt.Errorf("failed to create endpoint release binding: %w", err)
	}

	s.logger.Debug("Endpoint release binding created successfully", "namespace", namespaceName, "endpointReleaseBinding", rb.Name)
	rb.TypeMeta = endpointReleaseBindingTypeMeta
	return rb, nil
}

func (s *endpointReleaseBindingService) UpdateEndpointReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.EndpointReleaseBinding) (*openchoreov1alpha1.EndpointReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("endpoint release binding cannot be nil")
	}

	s.logger.Debug("Updating endpoint release binding", "namespace", namespaceName, "endpointReleaseBinding", rb.Name)

	existing := &openchoreov1alpha1.EndpointReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: rb.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Endpoint release binding not found", "namespace", namespaceName, "endpointReleaseBinding", rb.Name)
			return nil, ErrEndpointReleaseBindingNotFound
		}
		s.logger.Error("Failed to get endpoint release binding", "error", err)
		return nil, fmt.Errorf("failed to get endpoint release binding: %w", err)
	}

	// Clear status from user input — status is server-managed
	rb.Status = openchoreov1alpha1.EndpointReleaseBindingStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = rb.Spec
	existing.Labels = rb.Labels
	existing.Annotations = rb.Annotations

	// Preserve special labels
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyProjectName] = existing.Spec.Owner.ProjectName
	existing.Labels[labels.LabelKeyEndpointCRName] = existing.Spec.Owner.EndpointName

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update endpoint release binding CR", "error", err)
		return nil, fmt.Errorf("failed to update endpoint release binding: %w", err)
	}

	s.logger.Debug("Endpoint release binding updated successfully", "namespace", namespaceName, "endpointReleaseBinding", rb.Name)
	existing.TypeMeta = endpointReleaseBindingTypeMeta
	return existing, nil
}

func (s *endpointReleaseBindingService) ListEndpointReleaseBindings(ctx context.Context, namespaceName, endpointName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointReleaseBinding], error) {
	s.logger.Debug("Listing endpoint release bindings", "namespace", namespaceName, "endpoint", endpointName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.EndpointReleaseBinding], error) {
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var rbList openchoreov1alpha1.EndpointReleaseBindingList
		if err := s.k8sClient.List(ctx, &rbList, listOpts...); err != nil {
			s.logger.Error("Failed to list endpoint release bindings", "error", err)
			return nil, fmt.Errorf("failed to list endpoint release bindings: %w", err)
		}

		for i := range rbList.Items {
			rbList.Items[i].TypeMeta = endpointReleaseBindingTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.EndpointReleaseBinding]{
			Items:      rbList.Items,
			NextCursor: rbList.Continue,
		}
		if rbList.RemainingItemCount != nil {
			remaining := *rbList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	if endpointName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(rb openchoreov1alpha1.EndpointReleaseBinding) bool {
				return rb.Spec.Owner.EndpointName == endpointName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *endpointReleaseBindingService) GetEndpointReleaseBinding(ctx context.Context, namespaceName, endpointReleaseBindingName string) (*openchoreov1alpha1.EndpointReleaseBinding, error) {
	s.logger.Debug("Getting endpoint release binding", "namespace", namespaceName, "endpointReleaseBinding", endpointReleaseBindingName)

	rb := &openchoreov1alpha1.EndpointReleaseBinding{}
	key := client.ObjectKey{
		Name:      endpointReleaseBindingName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Endpoint release binding not found", "namespace", namespaceName, "endpointReleaseBinding", endpointReleaseBindingName)
			return nil, ErrEndpointReleaseBindingNotFound
		}
		s.logger.Error("Failed to get endpoint release binding", "error", err)
		return nil, fmt.Errorf("failed to get endpoint release binding: %w", err)
	}

	rb.TypeMeta = endpointReleaseBindingTypeMeta
	return rb, nil
}

func (s *endpointReleaseBindingService) DeleteEndpointReleaseBinding(ctx context.Context, namespaceName, endpointReleaseBindingName string) error {
	s.logger.Debug("Deleting endpoint release binding", "namespace", namespaceName, "endpointReleaseBinding", endpointReleaseBindingName)

	rb := &openchoreov1alpha1.EndpointReleaseBinding{}
	rb.Name = endpointReleaseBindingName
	rb.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rb); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrEndpointReleaseBindingNotFound
		}
		s.logger.Error("Failed to delete endpoint release binding CR", "error", err)
		return fmt.Errorf("failed to delete endpoint release binding: %w", err)
	}

	s.logger.Debug("Endpoint release binding deleted successfully", "namespace", namespaceName, "endpointReleaseBinding", endpointReleaseBindingName)
	return nil
}

func (s *endpointReleaseBindingService) bindingExists(ctx context.Context, namespaceName, name string) (bool, error) {
	rb := &openchoreov1alpha1.EndpointReleaseBinding{}
	key := client.ObjectKey{Name: name, Namespace: namespaceName}

	err := s.k8sClient.Get(ctx, key, rb)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of endpoint release binding %s/%s: %w", namespaceName, name, err)
	}
	return true, nil
}

func (s *endpointReleaseBindingService) validateEndpointExists(ctx context.Context, namespaceName, endpointName string) error {
	r := &openchoreov1alpha1.Endpoint{}
	key := client.ObjectKey{
		Name:      endpointName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, r); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrEndpointNotFound
		}
		return fmt.Errorf("failed to validate endpoint: %w", err)
	}
	return nil
}
