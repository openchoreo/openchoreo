// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	deploymentpipelinesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline"
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	namespacesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
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
