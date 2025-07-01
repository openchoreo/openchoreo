package services

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"golang.org/x/exp/slog"
)

type Services struct {
	ProjectService   *ProjectService
	ComponentService *ComponentService
}

// NewServices creates and initializes all services
func NewServices(k8sClient client.Client, logger *slog.Logger) *Services {
	// Create project service
	projectService := NewProjectService(k8sClient, logger.With("service", "project"))

	// Create component service (depends on project service)
	componentService := NewComponentService(k8sClient, projectService, logger.With("service", "component"))

	return &Services{
		ProjectService:   projectService,
		ComponentService: componentService,
	}
}
