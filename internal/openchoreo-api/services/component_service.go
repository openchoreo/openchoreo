package services

import (
	"context"
	"fmt"

	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ComponentService handles component-related business logic
type ComponentService struct {
	k8sClient      client.Client
	projectService *ProjectService
	logger         *slog.Logger
}

// NewComponentService creates a new component service
func NewComponentService(k8sClient client.Client, projectService *ProjectService, logger *slog.Logger) *ComponentService {
	return &ComponentService{
		k8sClient:      k8sClient,
		projectService: projectService,
		logger:         logger,
	}
}

// CreateComponent creates a new component in the given project
func (s *ComponentService) CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (*models.ComponentResponse, error) {
	s.logger.Debug("Creating component", "org", orgName, "project", projectName, "component", req.Name)

	// Sanitize input
	req.Sanitize()

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if err == ErrProjectNotFound {
			s.logger.Warn("Project not found", "org", orgName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Check if component already exists
	exists, err := s.componentExists(ctx, orgName, projectName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check component existence", "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}
	if exists {
		s.logger.Warn("Component already exists", "org", orgName, "project", projectName, "component", req.Name)
		return nil, ErrComponentAlreadyExists
	}

	// Set default branch if not provided
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}

	// Create the component and related resources
	if err := s.createComponentResources(ctx, orgName, projectName, req, branch); err != nil {
		s.logger.Error("Failed to create component resources", "error", err)
		return nil, fmt.Errorf("failed to create component: %w", err)
	}

	s.logger.Debug("Component created successfully", "org", orgName, "project", projectName, "component", req.Name)

	// Return the created component
	return &models.ComponentResponse{
		Name:          req.Name,
		Description:   req.Description,
		Type:          req.Type,
		ProjectName:   projectName,
		OrgName:       orgName,
		RepositoryURL: req.RepositoryURL,
		Branch:        branch,
		CreatedAt:     metav1.Now().Time,
		Status:        "Creating",
	}, nil
}

// ListComponents lists all components in the given project
func (s *ComponentService) ListComponents(ctx context.Context, orgName, projectName string) ([]*models.ComponentResponse, error) {
	s.logger.Debug("Listing components", "org", orgName, "project", projectName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if err == ErrProjectNotFound {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	var componentList openchoreov1alpha1.ComponentV2List
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &componentList, listOpts...); err != nil {
		s.logger.Error("Failed to list components", "error", err)
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	var components []*models.ComponentResponse
	for _, item := range componentList.Items {
		// Only include components that belong to the specified project
		if item.Spec.Owner.ProjectName == projectName {
			components = append(components, s.toComponentResponse(&item))
		}
	}

	s.logger.Debug("Listed components", "org", orgName, "project", projectName, "count", len(components))
	return components, nil
}

// GetComponent retrieves a specific component
func (s *ComponentService) GetComponent(ctx context.Context, orgName, projectName, componentName string) (*models.ComponentResponse, error) {
	s.logger.Debug("Getting component", "org", orgName, "project", projectName, "component", componentName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if err == ErrProjectNotFound {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	component := &openchoreov1alpha1.ComponentV2{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "org", orgName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	return s.toComponentResponse(component), nil
}

// componentExists checks if a component already exists by name and namespace and belongs to the specified project
func (s *ComponentService) componentExists(ctx context.Context, orgName, projectName, componentName string) (bool, error) {
	component := &openchoreov1alpha1.ComponentV2{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}

	err := s.k8sClient.Get(ctx, key, component)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil // Not found, so doesn't exist
		}
		return false, fmt.Errorf("failed to check component existence: %w", err) // Some other error
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		return false, nil // Component exists but belongs to a different project
	}

	return true, nil // Found and belongs to the correct project
}

// createComponentResources creates the component and related Kubernetes resources
func (s *ComponentService) createComponentResources(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest, branch string) error {
	componentCR := &openchoreov1alpha1.ComponentV2{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Component",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: orgName,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: req.Name,
				controller.AnnotationKeyDescription: req.Description,
				"repository-url":                    req.RepositoryURL,
				"repository-branch":                 branch,
			},
		},
		Spec: openchoreov1alpha1.ComponentV2Spec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: projectName,
			},
			Type: openchoreov1alpha1.ComponentType(req.Type),
		},
	}

	if err := s.k8sClient.Create(ctx, componentCR); err != nil {
		return fmt.Errorf("failed to create component CR: %w", err)
	}

	return nil
}

// toComponentResponse converts a ComponentV2 CR to a ComponentResponse
func (s *ComponentService) toComponentResponse(component *openchoreov1alpha1.ComponentV2) *models.ComponentResponse {
	// Extract repository URL from annotations (stored during creation)
	repositoryURL := component.Annotations["repository-url"]
	if repositoryURL == "" {
		// Fallback if not in annotations
		repositoryURL = ""
	}

	// Extract project name from the component owner
	projectName := component.Spec.Owner.ProjectName

	// Extract branch info from annotations
	branch := component.Annotations["repository-branch"]
	if branch == "" {
		branch = "main" // default
	}

	// Get status - ComponentV2 doesn't have conditions yet, so default to Creating
	// This can be enhanced later when ComponentV2 adds status conditions
	status := "Creating"

	return &models.ComponentResponse{
		Name:          component.Name,
		Description:   component.Annotations[controller.AnnotationKeyDescription],
		Type:          string(component.Spec.Type),
		ProjectName:   projectName,
		OrgName:       component.Namespace,
		RepositoryURL: repositoryURL,
		Branch:        branch,
		CreatedAt:     component.CreationTimestamp.Time,
		Status:        status,
	}
}
