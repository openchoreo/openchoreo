// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	clusterprojecttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterprojecttype"
	clusterresourcetypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterresourcetype"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	deploymentpipelinesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline"
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	namespacesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	projectreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectrelease"
	projectreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectreleasebinding"
	projecttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projecttype"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	resourcesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resource"
	resourcereleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcerelease"
	resourcereleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
	resourcetypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcetype"
	secretreferencesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secretreference"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
)

// newTestHandler creates an MCPHandler with the given services applied via functional options.
func newTestHandler(opts ...func(*handlerservices.Services)) *MCPHandler {
	svc := &handlerservices.Services{}
	for _, o := range opts {
		o(svc)
	}
	return NewMCPHandler(svc)
}

func withComponentService(s componentsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ComponentService = s }
}

func withComponentTypeService(s componenttypesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ComponentTypeService = s }
}

func withClusterComponentTypeService(s clustercomponenttypesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ClusterComponentTypeService = s }
}

func withEnvironmentService(s environmentsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.EnvironmentService = s }
}

func withNamespaceService(s namespacesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.NamespaceService = s }
}

func withProjectService(s projectsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ProjectService = s }
}

func withProjectTypeService(s projecttypesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ProjectTypeService = s }
}

func withClusterProjectTypeService(s clusterprojecttypesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ClusterProjectTypeService = s }
}

func withProjectReleaseService(s projectreleasesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ProjectReleaseService = s }
}

func withProjectReleaseBindingService(s projectreleasebindingsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ProjectReleaseBindingService = s }
}

func withReleaseBindingService(s releasebindingsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ReleaseBindingService = s }
}

func withWorkloadService(s workloadsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.WorkloadService = s }
}

func withWorkflowRunService(s workflowrunsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.WorkflowRunService = s }
}

func withDeploymentPipelineService(s deploymentpipelinesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.DeploymentPipelineService = s }
}

func withSecretReferenceService(s secretreferencesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.SecretReferenceService = s }
}

func withComponentReleaseService(s componentreleasesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ComponentReleaseService = s }
}

func withResourceTypeService(s resourcetypesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ResourceTypeService = s }
}

func withClusterResourceTypeService(s clusterresourcetypesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ClusterResourceTypeService = s }
}

func withResourceService(s resourcesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ResourceService = s }
}

func withResourceReleaseService(s resourcereleasesvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ResourceReleaseService = s }
}

func withResourceReleaseBindingService(s resourcereleasebindingsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.ResourceReleaseBindingService = s }
}
