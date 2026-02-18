// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	defaultPipeline = "default"
)

// projectService handles project-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type projectService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*projectService)(nil)

// NewService creates a new project service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &projectService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *projectService) CreateProject(ctx context.Context, namespaceName string, project *openchoreov1alpha1.Project) (*openchoreov1alpha1.Project, error) {
	if project == nil {
		return nil, fmt.Errorf("project cannot be nil")
	}

	s.logger.Debug("Creating project", "namespace", namespaceName, "project", project.Name)

	exists, err := s.projectExists(ctx, namespaceName, project.Name)
	if err != nil {
		s.logger.Error("Failed to check project existence", "error", err)
		return nil, fmt.Errorf("failed to check project existence: %w", err)
	}
	if exists {
		s.logger.Warn("Project already exists", "namespace", namespaceName, "project", project.Name)
		return nil, ErrProjectAlreadyExists
	}

	// Set defaults
	project.TypeMeta = metav1.TypeMeta{
		Kind:       "Project",
		APIVersion: "core.choreo.dev/v1alpha1",
	}
	project.Namespace = namespaceName
	if project.Labels == nil {
		project.Labels = make(map[string]string)
	}
	project.Labels[labels.LabelKeyNamespaceName] = namespaceName
	project.Labels[labels.LabelKeyName] = project.Name

	if project.Spec.DeploymentPipelineRef == "" {
		project.Spec.DeploymentPipelineRef = defaultPipeline
	}

	if err := s.k8sClient.Create(ctx, project); err != nil {
		s.logger.Error("Failed to create project CR", "error", err)
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	s.logger.Debug("Project created successfully", "namespace", namespaceName, "project", project.Name)
	return project, nil
}

func (s *projectService) UpdateProject(ctx context.Context, namespaceName string, project *openchoreov1alpha1.Project) (*openchoreov1alpha1.Project, error) {
	if project == nil {
		return nil, fmt.Errorf("project cannot be nil")
	}

	s.logger.Debug("Updating project", "namespace", namespaceName, "project", project.Name)

	existing := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      project.Name,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project not found", "namespace", namespaceName, "project", project.Name)
			return nil, ErrProjectNotFound
		}
		s.logger.Error("Failed to get project", "error", err)
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	// Update mutable fields
	existing.Spec = project.Spec
	existing.Annotations = project.Annotations
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	for k, v := range project.Labels {
		existing.Labels[k] = v
	}

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		s.logger.Error("Failed to update project CR", "error", err)
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	s.logger.Debug("Project updated successfully", "namespace", namespaceName, "project", project.Name)
	return existing, nil
}

func (s *projectService) ListProjects(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Project], error) {
	s.logger.Debug("Listing projects", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var projectList openchoreov1alpha1.ProjectList
	if err := s.k8sClient.List(ctx, &projectList, listOpts...); err != nil {
		s.logger.Error("Failed to list projects", "error", err)
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.Project]{
		Items:      projectList.Items,
		NextCursor: projectList.Continue,
	}
	if projectList.RemainingItemCount != nil {
		remaining := *projectList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed projects", "namespace", namespaceName, "count", len(projectList.Items))
	return result, nil
}

func (s *projectService) GetProject(ctx context.Context, namespaceName, projectName string) (*openchoreov1alpha1.Project, error) {
	s.logger.Debug("Getting project", "namespace", namespaceName, "project", projectName)

	project := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, project); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		s.logger.Error("Failed to get project", "error", err)
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	return project, nil
}

func (s *projectService) DeleteProject(ctx context.Context, namespaceName, projectName string) error {
	s.logger.Debug("Deleting project", "namespace", namespaceName, "project", projectName)

	project := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, project); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			return ErrProjectNotFound
		}
		s.logger.Error("Failed to get project", "error", err)
		return fmt.Errorf("failed to get project: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, project); err != nil {
		s.logger.Error("Failed to delete project CR", "error", err)
		return fmt.Errorf("failed to delete project: %w", err)
	}

	s.logger.Debug("Project deleted successfully", "namespace", namespaceName, "project", projectName)
	return nil
}

func (s *projectService) projectExists(ctx context.Context, namespaceName, projectName string) (bool, error) {
	project := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, project)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of project %s/%s: %w", namespaceName, projectName, err)
	}
	return true, nil
}
