// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

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

// releaseBindingService handles release binding business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type releaseBindingService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*releaseBindingService)(nil)

// NewService creates a new release binding service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &releaseBindingService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *releaseBindingService) CreateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("release binding cannot be nil")
	}

	s.logger.Debug("Creating release binding", "namespace", namespaceName, "releaseBinding", rb.Name)

	// Validate that the referenced component exists
	if err := s.validateComponentExists(ctx, namespaceName, rb.Spec.Owner.ComponentName); err != nil {
		return nil, err
	}

	exists, err := s.releaseBindingExists(ctx, namespaceName, rb.Name)
	if err != nil {
		s.logger.Error("Failed to check release binding existence", "error", err)
		return nil, fmt.Errorf("failed to check release binding existence: %w", err)
	}
	if exists {
		s.logger.Warn("Release binding already exists", "namespace", namespaceName, "releaseBinding", rb.Name)
		return nil, ErrReleaseBindingAlreadyExists
	}

	// Set defaults
	rb.TypeMeta = metav1.TypeMeta{
		Kind:       "ReleaseBinding",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	rb.Namespace = namespaceName
	if rb.Labels == nil {
		rb.Labels = make(map[string]string)
	}
	rb.Labels[labels.LabelKeyNamespaceName] = namespaceName
	rb.Labels[labels.LabelKeyName] = rb.Name
	rb.Labels[labels.LabelKeyProjectName] = rb.Spec.Owner.ProjectName
	rb.Labels[labels.LabelKeyComponentName] = rb.Spec.Owner.ComponentName

	if err := s.k8sClient.Create(ctx, rb); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Release binding already exists", "namespace", namespaceName, "releaseBinding", rb.Name)
			return nil, ErrReleaseBindingAlreadyExists
		}
		s.logger.Error("Failed to create release binding CR", "error", err)
		return nil, fmt.Errorf("failed to create release binding: %w", err)
	}

	s.logger.Debug("Release binding created successfully", "namespace", namespaceName, "releaseBinding", rb.Name)
	return rb, nil
}

func (s *releaseBindingService) UpdateReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ReleaseBinding) (*openchoreov1alpha1.ReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("release binding cannot be nil")
	}

	s.logger.Debug("Updating release binding", "namespace", namespaceName, "releaseBinding", rb.Name)

	existing := &openchoreov1alpha1.ReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: rb.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Release binding not found", "namespace", namespaceName, "releaseBinding", rb.Name)
			return nil, ErrReleaseBindingNotFound
		}
		s.logger.Error("Failed to get release binding", "error", err)
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	// Validate that the referenced component exists
	if err := s.validateComponentExists(ctx, namespaceName, rb.Spec.Owner.ComponentName); err != nil {
		return nil, err
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	rb.ResourceVersion = existing.ResourceVersion
	rb.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, rb); err != nil {
		s.logger.Error("Failed to update release binding CR", "error", err)
		return nil, fmt.Errorf("failed to update release binding: %w", err)
	}

	s.logger.Debug("Release binding updated successfully", "namespace", namespaceName, "releaseBinding", rb.Name)
	return rb, nil
}

func (s *releaseBindingService) ListReleaseBindings(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBinding], error) {
	s.logger.Debug("Listing release bindings", "namespace", namespaceName, "component", componentName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBinding], error) {
		listOpts := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		if pageOpts.Limit > 0 {
			listOpts = append(listOpts, client.Limit(int64(pageOpts.Limit)))
		}
		if pageOpts.Cursor != "" {
			listOpts = append(listOpts, client.Continue(pageOpts.Cursor))
		}

		var rbList openchoreov1alpha1.ReleaseBindingList
		if err := s.k8sClient.List(ctx, &rbList, listOpts...); err != nil {
			s.logger.Error("Failed to list release bindings", "error", err)
			return nil, fmt.Errorf("failed to list release bindings: %w", err)
		}

		result := &services.ListResult[openchoreov1alpha1.ReleaseBinding]{
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
			func(rb openchoreov1alpha1.ReleaseBinding) bool {
				return rb.Spec.Owner.ComponentName == componentName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *releaseBindingService) GetReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) (*openchoreov1alpha1.ReleaseBinding, error) {
	s.logger.Debug("Getting release binding", "namespace", namespaceName, "releaseBinding", releaseBindingName)

	rb := &openchoreov1alpha1.ReleaseBinding{}
	key := client.ObjectKey{
		Name:      releaseBindingName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Release binding not found", "namespace", namespaceName, "releaseBinding", releaseBindingName)
			return nil, ErrReleaseBindingNotFound
		}
		s.logger.Error("Failed to get release binding", "error", err)
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	return rb, nil
}

func (s *releaseBindingService) DeleteReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string) error {
	s.logger.Debug("Deleting release binding", "namespace", namespaceName, "releaseBinding", releaseBindingName)

	rb := &openchoreov1alpha1.ReleaseBinding{}
	rb.Name = releaseBindingName
	rb.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rb); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrReleaseBindingNotFound
		}
		s.logger.Error("Failed to delete release binding CR", "error", err)
		return fmt.Errorf("failed to delete release binding: %w", err)
	}

	s.logger.Debug("Release binding deleted successfully", "namespace", namespaceName, "releaseBinding", releaseBindingName)
	return nil
}

func (s *releaseBindingService) releaseBindingExists(ctx context.Context, namespaceName, releaseBindingName string) (bool, error) {
	rb := &openchoreov1alpha1.ReleaseBinding{}
	key := client.ObjectKey{
		Name:      releaseBindingName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, rb)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of release binding %s/%s: %w", namespaceName, releaseBindingName, err)
	}
	return true, nil
}

func (s *releaseBindingService) validateComponentExists(ctx context.Context, namespaceName, componentName string) error {
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
