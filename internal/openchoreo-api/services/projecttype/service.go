// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// projectTypeService handles project type business logic without authorization checks.
type projectTypeService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var projectTypeTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ProjectType",
}

var _ Service = (*projectTypeService)(nil)

// NewService creates a new project type service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &projectTypeService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *projectTypeService) CreateProjectType(ctx context.Context, namespaceName string, pt *openchoreov1alpha1.ProjectType) (*openchoreov1alpha1.ProjectType, error) {
	if pt == nil {
		return nil, fmt.Errorf("project type cannot be nil")
	}

	s.logger.Debug("Creating project type", "namespace", namespaceName, "projectType", pt.Name)

	exists, err := s.projectTypeExists(ctx, namespaceName, pt.Name)
	if err != nil {
		s.logger.Error("Failed to check project type existence", "error", err)
		return nil, fmt.Errorf("failed to check project type existence: %w", err)
	}
	if exists {
		s.logger.Warn("Project type already exists", "namespace", namespaceName, "projectType", pt.Name)
		return nil, ErrProjectTypeAlreadyExists
	}

	pt.Namespace = namespaceName
	pt.Status = openchoreov1alpha1.ProjectTypeStatus{}
	if err := s.k8sClient.Create(ctx, pt); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Project type already exists", "namespace", namespaceName, "projectType", pt.Name)
			return nil, ErrProjectTypeAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create project type CR", "error", err)
		return nil, fmt.Errorf("failed to create project type: %w", err)
	}

	s.logger.Debug("Project type created successfully", "namespace", namespaceName, "projectType", pt.Name)
	pt.TypeMeta = projectTypeTypeMeta
	return pt, nil
}

func (s *projectTypeService) UpdateProjectType(ctx context.Context, namespaceName string, pt *openchoreov1alpha1.ProjectType) (*openchoreov1alpha1.ProjectType, error) {
	if pt == nil {
		return nil, fmt.Errorf("project type cannot be nil")
	}

	s.logger.Debug("Updating project type", "namespace", namespaceName, "projectType", pt.Name)

	existing := &openchoreov1alpha1.ProjectType{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: pt.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project type not found", "namespace", namespaceName, "projectType", pt.Name)
			return nil, ErrProjectTypeNotFound
		}
		s.logger.Error("Failed to get project type", "error", err)
		return nil, fmt.Errorf("failed to get project type: %w", err)
	}

	pt.Status = openchoreov1alpha1.ProjectTypeStatus{}
	existing.Spec = pt.Spec
	existing.Labels = pt.Labels
	existing.Annotations = pt.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update project type CR", "error", err)
		return nil, fmt.Errorf("failed to update project type: %w", err)
	}

	s.logger.Debug("Project type updated successfully", "namespace", namespaceName, "projectType", pt.Name)
	existing.TypeMeta = projectTypeTypeMeta
	return existing, nil
}

func (s *projectTypeService) ListProjectTypes(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ProjectType], error) {
	s.logger.Debug("Listing project types", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

	var ptList openchoreov1alpha1.ProjectTypeList
	if err := s.k8sClient.List(ctx, &ptList, listOpts...); err != nil {
		s.logger.Error("Failed to list project types", "error", err)
		return nil, fmt.Errorf("failed to list project types: %w", err)
	}

	for i := range ptList.Items {
		ptList.Items[i].TypeMeta = projectTypeTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ProjectType]{
		Items:      ptList.Items,
		NextCursor: ptList.Continue,
	}
	if ptList.RemainingItemCount != nil {
		remaining := *ptList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed project types", "namespace", namespaceName, "count", len(ptList.Items))
	return result, nil
}

func (s *projectTypeService) GetProjectType(ctx context.Context, namespaceName, ptName string) (*openchoreov1alpha1.ProjectType, error) {
	s.logger.Debug("Getting project type", "namespace", namespaceName, "projectType", ptName)

	pt := &openchoreov1alpha1.ProjectType{}
	key := client.ObjectKey{
		Name:      ptName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, pt); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project type not found", "namespace", namespaceName, "projectType", ptName)
			return nil, ErrProjectTypeNotFound
		}
		s.logger.Error("Failed to get project type", "error", err)
		return nil, fmt.Errorf("failed to get project type: %w", err)
	}

	pt.TypeMeta = projectTypeTypeMeta
	return pt, nil
}

func (s *projectTypeService) DeleteProjectType(ctx context.Context, namespaceName, ptName string) error {
	s.logger.Debug("Deleting project type", "namespace", namespaceName, "projectType", ptName)

	pt := &openchoreov1alpha1.ProjectType{}
	pt.Name = ptName
	pt.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, pt); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrProjectTypeNotFound
		}
		s.logger.Error("Failed to delete project type CR", "error", err)
		return fmt.Errorf("failed to delete project type: %w", err)
	}

	s.logger.Debug("Project type deleted successfully", "namespace", namespaceName, "projectType", ptName)
	return nil
}

func (s *projectTypeService) GetProjectTypeSchema(ctx context.Context, namespaceName, ptName string) (map[string]any, error) {
	s.logger.Debug("Getting project type schema", "namespace", namespaceName, "projectType", ptName)

	pt, err := s.GetProjectType(ctx, namespaceName, ptName)
	if err != nil {
		return nil, err
	}

	rawSchema, err := schema.SectionToRawJSONSchema(pt.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved project type schema successfully", "namespace", namespaceName, "projectType", ptName)
	return rawSchema, nil
}

func (s *projectTypeService) projectTypeExists(ctx context.Context, namespaceName, ptName string) (bool, error) {
	pt := &openchoreov1alpha1.ProjectType{}
	key := client.ObjectKey{
		Name:      ptName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, pt)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of project type %s/%s: %w", namespaceName, ptName, err)
	}
	return true, nil
}
