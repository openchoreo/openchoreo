package services

import (
	"golang.org/x/exp/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Services struct {
	ProjectService      *ProjectService
	ComponentService    *ComponentService
	OrganizationService *OrganizationService
	EnvironmentService  *EnvironmentService
	DataPlaneService    *DataPlaneService
}

// NewServices creates and initializes all services
func NewServices(k8sClient client.Client, logger *slog.Logger) *Services {
	// Create project service
	projectService := NewProjectService(k8sClient, logger.With("service", "project"))

	// Create component service (depends on project service)
	componentService := NewComponentService(k8sClient, projectService, logger.With("service", "component"))

	// Create organization service
	organizationService := NewOrganizationService(k8sClient, logger.With("service", "organization"))

	// Create environment service
	environmentService := NewEnvironmentService(k8sClient, logger.With("service", "environment"))

	// Create dataplane service
	dataplaneService := NewDataPlaneService(k8sClient, logger.With("service", "dataplane"))

	return &Services{
		ProjectService:      projectService,
		ComponentService:    componentService,
		OrganizationService: organizationService,
		EnvironmentService:  environmentService,
		DataPlaneService:    dataplaneService,
	}
}
