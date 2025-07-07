package services

import (
	"context"
	"fmt"

	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ProjectService handles project-related business logic
type ProjectService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewProjectService creates a new project service
func NewProjectService(k8sClient client.Client, logger *slog.Logger) *ProjectService {
	return &ProjectService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// CreateProject creates a new project in the given organization
func (s *ProjectService) CreateProject(ctx context.Context, orgName string, req *models.CreateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Debug("Creating project", "org", orgName, "project", req.Name)

	// Sanitize input
	req.Sanitize()

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
func (s *ProjectService) ListProjects(ctx context.Context, orgName string) ([]*models.ProjectResponse, error) {
	s.logger.Debug("Listing projects", "org", orgName)

	var projectList choreov1.ProjectList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &projectList, listOpts...); err != nil {
		s.logger.Error("Failed to list projects", "error", err)
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	var projects []*models.ProjectResponse
	for _, item := range projectList.Items {
		projects = append(projects, s.toProjectResponse(&item))
	}

	s.logger.Debug("Listed projects", "org", orgName, "count", len(projects))
	return projects, nil
}

// GetProject retrieves a specific project
func (s *ProjectService) GetProject(ctx context.Context, orgName, projectName string) (*models.ProjectResponse, error) {
	s.logger.Debug("Getting project", "org", orgName, "project", projectName)

	project := &choreov1.Project{}
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
	project := &choreov1.Project{}
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
func (s *ProjectService) buildProjectCR(orgName string, req *models.CreateProjectRequest) *choreov1.Project {
	// Set default deployment pipeline if not provided
	deploymentPipeline := req.DeploymentPipeline
	if deploymentPipeline == "" {
		deploymentPipeline = "default-pipeline"
	}

	return &choreov1.Project{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Project",
			APIVersion: "core.choreo.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: orgName,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: req.Name,
				controller.AnnotationKeyDescription: fmt.Sprintf("Project for %s", req.Name),
			},
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: orgName,
				labels.LabelKeyName:             req.Name,
				"backstage.io/kubernetes-id":    req.Name,
			},
		},
		Spec: choreov1.ProjectSpec{
			DeploymentPipelineRef: deploymentPipeline,
		},
	}
}

// toProjectResponse converts a Project CR to a ProjectResponse
func (s *ProjectService) toProjectResponse(project *choreov1.Project) *models.ProjectResponse {
	// Extract repository info from annotations if available
	repositoryURL := project.Annotations["repository-url"]
	repositoryBranch := project.Annotations["repository-branch"]

	// Extract display name and description from annotations
	displayName := project.Annotations[controller.AnnotationKeyDisplayName]
	description := project.Annotations[controller.AnnotationKeyDescription]

	// Get status from conditions
	status := "Unknown"
	if len(project.Status.Conditions) > 0 {
		// Get the latest condition
		latestCondition := project.Status.Conditions[len(project.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = "Ready"
		} else {
			status = "NotReady"
		}
	}

	return &models.ProjectResponse{
		Name:               project.Name,
		OrgName:            project.Namespace,
		DisplayName:        displayName,
		Description:        description,
		RepositoryURL:      repositoryURL,
		RepositoryBranch:   repositoryBranch,
		DeploymentPipeline: project.Spec.DeploymentPipelineRef,
		CreatedAt:          project.CreationTimestamp.Time,
		Status:             status,
	}
}
