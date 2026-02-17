// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacy_services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ProjectService handles project-related business logic
type ProjectService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewProjectService creates a new project service
func NewProjectService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ProjectService {
	return &ProjectService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// CreateProject creates a new project in the given namespace
func (s *ProjectService) CreateProject(ctx context.Context, namespaceName string, req *models.CreateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Debug("Creating project", "namespace", namespaceName, "project", req.Name)

	// Sanitize input
	req.Sanitize()

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateProject, ResourceTypeProject, req.Name,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: req.Name}); err != nil {
		return nil, err
	}

	// Check if project already exists
	exists, err := s.projectExists(ctx, namespaceName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check project existence", "error", err)
		return nil, fmt.Errorf("failed to check project existence: %w", err)
	}
	if exists {
		s.logger.Warn("Project already exists", "namespace", namespaceName, "project", req.Name)
		return nil, ErrProjectAlreadyExists
	}

	// Create the project CR
	projectCR := s.buildProjectCR(namespaceName, req)
	if err := s.k8sClient.Create(ctx, projectCR); err != nil {
		s.logger.Error("Failed to create project CR", "error", err)
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	s.logger.Debug("Project created successfully", "namespace", namespaceName, "project", req.Name)
	return s.toProjectResponse(projectCR), nil
}

// ListProjects lists all projects in the given namespace
func (s *ProjectService) ListProjects(ctx context.Context, namespaceName string) ([]*models.ProjectResponse, error) {
	s.logger.Debug("Listing projects", "namespace", namespaceName)

	var projectList openchoreov1alpha1.ProjectList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &projectList, listOpts...); err != nil {
		s.logger.Error("Failed to list projects", "error", err)
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	projects := make([]*models.ProjectResponse, 0, len(projectList.Items))
	for _, item := range projectList.Items {
		// Authorization check for each project
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewProject, ResourceTypeProject, item.Name,
			authz.ResourceHierarchy{Namespace: namespaceName, Project: item.Name}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized projects silently (user doesn't have permission to see this project)
				s.logger.Debug("Skipping unauthorized project", "namespace", namespaceName, "project", item.Name)
				continue
			}
			// system failures, return the error
			return nil, err
		}
		projects = append(projects, s.toProjectResponse(&item))
	}

	s.logger.Debug("Listed projects", "namespace", namespaceName, "count", len(projects))
	return projects, nil
}

// GetProject retrieves a specific project
func (s *ProjectService) GetProject(ctx context.Context, namespaceName, projectName string) (*models.ProjectResponse, error) {
	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewProject, ResourceTypeProject, projectName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName}); err != nil {
		return nil, err
	}
	return s.getProject(ctx, namespaceName, projectName)
}

// getProject is the internal helper without authorization (INTERNAL USE ONLY)
func (s *ProjectService) getProject(ctx context.Context, namespaceName, projectName string) (*models.ProjectResponse, error) {
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

	return s.toProjectResponse(project), nil
}

// projectExists checks if a project already exists in the namespace
func (s *ProjectService) projectExists(ctx context.Context, namespaceName, projectName string) (bool, error) {
	project := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, project)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil // Not found, so doesn't exist
		}
		return false, err // Some other error
	}
	return true, nil // Found, so exists
}

// buildProjectCR builds a Project custom resource from the request
func (s *ProjectService) buildProjectCR(namespaceName string, req *models.CreateProjectRequest) *openchoreov1alpha1.Project {
	// Set default deployment pipeline if not provided
	deploymentPipeline := req.DeploymentPipeline
	if deploymentPipeline == "" {
		deploymentPipeline = defaultPipeline
	}

	projectSpec := openchoreov1alpha1.ProjectSpec{
		DeploymentPipelineRef: deploymentPipeline,
	}

	// Convert BuildPlaneRef if provided
	if req.BuildPlaneRef != nil {
		projectSpec.BuildPlaneRef = &openchoreov1alpha1.BuildPlaneRef{
			Kind: openchoreov1alpha1.BuildPlaneRefKind(req.BuildPlaneRef.Kind),
			Name: req.BuildPlaneRef.Name,
		}
	}

	return &openchoreov1alpha1.Project{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Project",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: namespaceName,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: req.DisplayName,
				controller.AnnotationKeyDescription: req.Description,
			},
			Labels: map[string]string{
				labels.LabelKeyNamespaceName: namespaceName,
				labels.LabelKeyName:          req.Name,
			},
		},
		Spec: projectSpec,
	}
}

// DeleteProject deletes a project from the given namespace
func (s *ProjectService) DeleteProject(ctx context.Context, namespaceName, projectName string) error {
	s.logger.Debug("Deleting project", "namespace", namespaceName, "project", projectName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionDeleteProject, ResourceTypeProject, projectName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName}); err != nil {
		return err
	}

	// Get the project first to ensure it exists
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

	// Delete the project CR
	if err := s.k8sClient.Delete(ctx, project); err != nil {
		s.logger.Error("Failed to delete project CR", "error", err)
		return fmt.Errorf("failed to delete project: %w", err)
	}

	s.logger.Debug("Project deleted successfully", "namespace", namespaceName, "project", projectName)
	return nil
}

// toProjectResponse converts a Project CR to a ProjectResponse
func (s *ProjectService) toProjectResponse(project *openchoreov1alpha1.Project) *models.ProjectResponse {
	// Extract display name and description from annotations
	displayName := project.Annotations[controller.AnnotationKeyDisplayName]
	description := project.Annotations[controller.AnnotationKeyDescription]

	// Get status from conditions
	status := statusUnknown
	if len(project.Status.Conditions) > 0 {
		// Get the latest condition
		latestCondition := project.Status.Conditions[len(project.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	response := &models.ProjectResponse{
		UID:                string(project.UID),
		Name:               project.Name,
		NamespaceName:      project.Namespace,
		DisplayName:        displayName,
		Description:        description,
		DeploymentPipeline: project.Spec.DeploymentPipelineRef,
		CreatedAt:          project.CreationTimestamp.Time,
		Status:             status,
	}

	// Convert BuildPlaneRef if present
	if project.Spec.BuildPlaneRef != nil {
		response.BuildPlaneRef = &models.BuildPlaneRef{
			Kind: string(project.Spec.BuildPlaneRef.Kind),
			Name: project.Spec.BuildPlaneRef.Name,
		}
	}

	// Include deletion timestamp if the project is marked for deletion
	if project.DeletionTimestamp != nil {
		response.DeletionTimestamp = &project.DeletionTimestamp.Time
	}

	return response
}
