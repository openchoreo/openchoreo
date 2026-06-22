// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

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

// projectReleaseBindingService handles project release binding business logic without authorization checks.
type projectReleaseBindingService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var projectReleaseBindingTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ProjectReleaseBinding",
}

var _ Service = (*projectReleaseBindingService)(nil)

// NewService creates a new project release binding service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &projectReleaseBindingService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *projectReleaseBindingService) CreateProjectReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ProjectReleaseBinding) (*openchoreov1alpha1.ProjectReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("project release binding cannot be nil")
	}

	s.logger.Debug("Creating project release binding", "namespace", namespaceName, "projectReleaseBinding", rb.Name)

	if err := s.validateProjectExists(ctx, namespaceName, rb.Spec.Owner.ProjectName); err != nil {
		return nil, err
	}

	exists, err := s.bindingExists(ctx, namespaceName, rb.Name)
	if err != nil {
		s.logger.Error("Failed to check binding existence", "error", err)
		return nil, fmt.Errorf("failed to check binding existence: %w", err)
	}
	if exists {
		s.logger.Warn("Project release binding already exists", "namespace", namespaceName, "projectReleaseBinding", rb.Name)
		return nil, ErrProjectReleaseBindingAlreadyExists
	}

	// Set defaults
	rb.Namespace = namespaceName
	rb.Status = openchoreov1alpha1.ProjectReleaseBindingStatus{}
	if rb.Labels == nil {
		rb.Labels = make(map[string]string)
	}
	rb.Labels[labels.LabelKeyProjectName] = rb.Spec.Owner.ProjectName

	if err := s.k8sClient.Create(ctx, rb); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Project release binding already exists", "namespace", namespaceName, "projectReleaseBinding", rb.Name)
			return nil, ErrProjectReleaseBindingAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create project release binding CR", "error", err)
		return nil, fmt.Errorf("failed to create project release binding: %w", err)
	}

	s.logger.Debug("Project release binding created successfully", "namespace", namespaceName, "projectReleaseBinding", rb.Name)
	rb.TypeMeta = projectReleaseBindingTypeMeta
	return rb, nil
}

func (s *projectReleaseBindingService) UpdateProjectReleaseBinding(ctx context.Context, namespaceName string, rb *openchoreov1alpha1.ProjectReleaseBinding) (*openchoreov1alpha1.ProjectReleaseBinding, error) {
	if rb == nil {
		return nil, fmt.Errorf("project release binding cannot be nil")
	}

	s.logger.Debug("Updating project release binding", "namespace", namespaceName, "projectReleaseBinding", rb.Name)

	existing := &openchoreov1alpha1.ProjectReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: rb.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project release binding not found", "namespace", namespaceName, "projectReleaseBinding", rb.Name)
			return nil, ErrProjectReleaseBindingNotFound
		}
		s.logger.Error("Failed to get project release binding", "error", err)
		return nil, fmt.Errorf("failed to get project release binding: %w", err)
	}

	// Clear status from user input — status is server-managed
	rb.Status = openchoreov1alpha1.ProjectReleaseBindingStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = rb.Spec
	existing.Labels = rb.Labels
	existing.Annotations = rb.Annotations

	// Preserve special labels
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyProjectName] = existing.Spec.Owner.ProjectName

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update project release binding CR", "error", err)
		return nil, fmt.Errorf("failed to update project release binding: %w", err)
	}

	s.logger.Debug("Project release binding updated successfully", "namespace", namespaceName, "projectReleaseBinding", rb.Name)
	existing.TypeMeta = projectReleaseBindingTypeMeta
	return existing, nil
}

func (s *projectReleaseBindingService) ListProjectReleaseBindings(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectReleaseBinding], error) {
	s.logger.Debug("Listing project release bindings", "namespace", namespaceName, "project", projectName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectReleaseBinding], error) {
		commonOpts, err := services.BuildListOptions(pageOpts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var rbList openchoreov1alpha1.ProjectReleaseBindingList
		if err := s.k8sClient.List(ctx, &rbList, listOpts...); err != nil {
			s.logger.Error("Failed to list project release bindings", "error", err)
			return nil, fmt.Errorf("failed to list project release bindings: %w", err)
		}

		for i := range rbList.Items {
			rbList.Items[i].TypeMeta = projectReleaseBindingTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.ProjectReleaseBinding]{
			Items:      rbList.Items,
			NextCursor: rbList.Continue,
		}
		if rbList.RemainingItemCount != nil {
			remaining := *rbList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	if projectName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(rb openchoreov1alpha1.ProjectReleaseBinding) bool {
				return rb.Spec.Owner.ProjectName == projectName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *projectReleaseBindingService) GetProjectReleaseBinding(ctx context.Context, namespaceName, projectReleaseBindingName string) (*openchoreov1alpha1.ProjectReleaseBinding, error) {
	s.logger.Debug("Getting project release binding", "namespace", namespaceName, "projectReleaseBinding", projectReleaseBindingName)

	rb := &openchoreov1alpha1.ProjectReleaseBinding{}
	key := client.ObjectKey{
		Name:      projectReleaseBindingName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project release binding not found", "namespace", namespaceName, "projectReleaseBinding", projectReleaseBindingName)
			return nil, ErrProjectReleaseBindingNotFound
		}
		s.logger.Error("Failed to get project release binding", "error", err)
		return nil, fmt.Errorf("failed to get project release binding: %w", err)
	}

	rb.TypeMeta = projectReleaseBindingTypeMeta
	return rb, nil
}

func (s *projectReleaseBindingService) DeleteProjectReleaseBinding(ctx context.Context, namespaceName, projectReleaseBindingName string) error {
	s.logger.Debug("Deleting project release binding", "namespace", namespaceName, "projectReleaseBinding", projectReleaseBindingName)

	rb := &openchoreov1alpha1.ProjectReleaseBinding{}
	rb.Name = projectReleaseBindingName
	rb.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, rb); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrProjectReleaseBindingNotFound
		}
		s.logger.Error("Failed to delete project release binding CR", "error", err)
		return fmt.Errorf("failed to delete project release binding: %w", err)
	}

	s.logger.Debug("Project release binding deleted successfully", "namespace", namespaceName, "projectReleaseBinding", projectReleaseBindingName)
	return nil
}

func (s *projectReleaseBindingService) bindingExists(ctx context.Context, namespaceName, name string) (bool, error) {
	rb := &openchoreov1alpha1.ProjectReleaseBinding{}
	key := client.ObjectKey{Name: name, Namespace: namespaceName}

	err := s.k8sClient.Get(ctx, key, rb)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of project release binding %s/%s: %w", namespaceName, name, err)
	}
	return true, nil
}

func (s *projectReleaseBindingService) validateProjectExists(ctx context.Context, namespaceName, projectName string) error {
	p := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, p); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrProjectNotFound
		}
		return fmt.Errorf("failed to validate project: %w", err)
	}
	return nil
}
