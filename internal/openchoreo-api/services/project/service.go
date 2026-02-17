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
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	defaultPipeline = "default"

	statusReady    = "Ready"
	statusNotReady = "NotReady"
	statusUnknown  = "Unknown"
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

func (s *projectService) CreateProject(ctx context.Context, namespaceName string, req *models.CreateProjectRequest) (*models.ProjectResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("create project request cannot be nil")
	}

	s.logger.Debug("Creating project", "namespace", namespaceName, "project", req.Name)

	req.Sanitize()

	exists, err := s.ProjectExists(ctx, namespaceName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check project existence", "error", err)
		return nil, fmt.Errorf("failed to check project existence: %w", err)
	}
	if exists {
		s.logger.Warn("Project already exists", "namespace", namespaceName, "project", req.Name)
		return nil, ErrProjectAlreadyExists
	}

	projectCR := s.buildProjectCR(namespaceName, req)
	if err := s.k8sClient.Create(ctx, projectCR); err != nil {
		s.logger.Error("Failed to create project CR", "error", err)
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	s.logger.Debug("Project created successfully", "namespace", namespaceName, "project", req.Name)
	return s.toProjectResponse(projectCR), nil
}

func (s *projectService) ListProjects(ctx context.Context, namespaceName string) ([]*models.ProjectResponse, error) {
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
	for i := range projectList.Items {
		projects = append(projects, s.toProjectResponse(&projectList.Items[i]))
	}

	s.logger.Debug("Listed projects", "namespace", namespaceName, "count", len(projects))
	return projects, nil
}

func (s *projectService) GetProject(ctx context.Context, namespaceName, projectName string) (*models.ProjectResponse, error) {
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

func (s *projectService) ProjectExists(ctx context.Context, namespaceName, projectName string) (bool, error) {
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

func (s *projectService) buildProjectCR(namespaceName string, req *models.CreateProjectRequest) *openchoreov1alpha1.Project {
	deploymentPipeline := req.DeploymentPipeline
	if deploymentPipeline == "" {
		deploymentPipeline = defaultPipeline
	}

	projectSpec := openchoreov1alpha1.ProjectSpec{
		DeploymentPipelineRef: deploymentPipeline,
	}

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

func (s *projectService) toProjectResponse(project *openchoreov1alpha1.Project) *models.ProjectResponse {
	displayName := project.Annotations[controller.AnnotationKeyDisplayName]
	description := project.Annotations[controller.AnnotationKeyDescription]

	status := statusUnknown
	if len(project.Status.Conditions) > 0 {
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

	if project.Spec.BuildPlaneRef != nil {
		response.BuildPlaneRef = &models.BuildPlaneRef{
			Kind: string(project.Spec.BuildPlaneRef.Kind),
			Name: project.Spec.BuildPlaneRef.Name,
		}
	}

	if project.DeletionTimestamp != nil {
		response.DeletionTimestamp = &project.DeletionTimestamp.Time
	}

	return response
}
