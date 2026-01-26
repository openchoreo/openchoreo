// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

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

// CreateProject creates a new project in the given organization
func (s *ProjectService) CreateProject(ctx context.Context, orgName string, req *models.CreateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Debug("Creating project", "org", orgName, "project", req.Name)

	// Sanitize input
	req.Sanitize()

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateProject, ResourceTypeProject, req.Name,
		authz.ResourceHierarchy{Namespace: orgName, Project: req.Name}); err != nil {
		return nil, err
	}

	// Check if project already exists
	exists, err := s.projectExists(ctx, orgName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check project existence", "error", err)
		return nil, fmt.Errorf("failed to check project existence: %w", err)
	}
	if exists {
		s.logger.Warn("Project already exists", "org", orgName, "project", req.Name)
		return nil, ErrProjectAlreadyExists
	}

	// Create the project CR
	projectCR := s.buildProjectCR(orgName, req)
	if err := s.k8sClient.Create(ctx, projectCR); err != nil {
		s.logger.Error("Failed to create project CR", "error", err)
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	s.logger.Debug("Project created successfully", "org", orgName, "project", req.Name)
	return s.toProjectResponse(projectCR), nil
}

// ListProjects lists all projects in the given organization
func (s *ProjectService) ListProjects(ctx context.Context, orgName string, opts *models.ListOptions) (*models.ListResponse[*models.ProjectResponse], error) {
	if opts == nil {
		opts = &models.ListOptions{Limit: models.DefaultPageLimit}
	}
	s.logger.Debug("Listing projects", "org", orgName, "limit", opts.Limit, "continue", opts.Continue)

	var projectList openchoreov1alpha1.ProjectList
	listOpts := &client.ListOptions{
		Namespace: orgName,
		Limit:     int64(opts.Limit),
		Continue:  opts.Continue,
	}

	if err := s.k8sClient.List(ctx, &projectList, listOpts); err != nil {
		return nil, HandleListError(err, s.logger, opts.Continue, "projects")
	}

	projects := make([]*models.ProjectResponse, 0, len(projectList.Items))
	for _, item := range projectList.Items {
		// Authorization check for each project
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewProject, ResourceTypeProject, item.Name,
			authz.ResourceHierarchy{Namespace: orgName, Project: item.Name}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized projects silently (user doesn't have permission to see this project)
				s.logger.Debug("Skipping unauthorized project", "org", orgName, "project", item.Name)
				continue
			}
			// system failures, return the error
			return nil, err
		}
		projects = append(projects, s.toProjectResponse(&item))
	}

	s.logger.Debug("Listed projects", "org", orgName, "count", len(projects), "hasMore", projectList.Continue != "")
	return &models.ListResponse[*models.ProjectResponse]{
		Items: projects,
		Metadata: models.ResponseMetadata{
			ResourceVersion: projectList.ResourceVersion,
			Continue:        projectList.Continue,
			HasMore:         projectList.Continue != "",
		},
	}, nil
}

// GetProject retrieves a specific project
func (s *ProjectService) GetProject(ctx context.Context, orgName, projectName string) (*models.ProjectResponse, error) {
	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewProject, ResourceTypeProject, projectName,
		authz.ResourceHierarchy{Namespace: orgName, Project: projectName}); err != nil {
		return nil, err
	}
	return s.getProject(ctx, orgName, projectName)
}

// getProject is the internal helper without authorization (INTERNAL USE ONLY)
func (s *ProjectService) getProject(ctx context.Context, orgName, projectName string) (*models.ProjectResponse, error) {
	s.logger.Debug("Getting project", "org", orgName, "project", projectName)

	project := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, project); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project not found", "org", orgName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		s.logger.Error("Failed to get project", "error", err)
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	return s.toProjectResponse(project), nil
}

// projectExists checks if a project already exists in the organization
func (s *ProjectService) projectExists(ctx context.Context, orgName, projectName string) (bool, error) {
	project := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: orgName,
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
func (s *ProjectService) buildProjectCR(orgName string, req *models.CreateProjectRequest) *openchoreov1alpha1.Project {
	// Set default deployment pipeline if not provided
	deploymentPipeline := req.DeploymentPipeline
	if deploymentPipeline == "" {
		deploymentPipeline = defaultPipeline
	}

	return &openchoreov1alpha1.Project{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Project",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: orgName,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: req.DisplayName,
				controller.AnnotationKeyDescription: req.Description,
			},
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: orgName,
				labels.LabelKeyName:             req.Name,
			},
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: deploymentPipeline,
		},
	}
}

// DeleteProject deletes a project from the given organization
func (s *ProjectService) DeleteProject(ctx context.Context, orgName, projectName string) error {
	s.logger.Debug("Deleting project", "org", orgName, "project", projectName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionDeleteProject, ResourceTypeProject, projectName,
		authz.ResourceHierarchy{Namespace: orgName, Project: projectName}); err != nil {
		return err
	}

	// Get the project first to ensure it exists
	project := &openchoreov1alpha1.Project{}
	key := client.ObjectKey{
		Name:      projectName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, project); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Project not found", "org", orgName, "project", projectName)
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

	s.logger.Debug("Project deleted successfully", "org", orgName, "project", projectName)
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
		OrgName:            project.Namespace,
		DisplayName:        displayName,
		Description:        description,
		DeploymentPipeline: project.Spec.DeploymentPipelineRef,
		CreatedAt:          project.CreationTimestamp.Time,
		Status:             status,
	}

	// Include deletion timestamp if the project is marked for deletion
	if project.DeletionTimestamp != nil {
		response.DeletionTimestamp = &project.DeletionTimestamp.Time
	}

	return response
}
