// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlerservices

import (
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	authzsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/authz"
	buildplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/buildplane"
	clusterbuildplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterbuildplane"
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	clusterdataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterdataplane"
	clusterobservabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterobservabilityplane"
	clustertraitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustertrait"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	dataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane"
	deploymentpipelinesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline"
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
	namespacesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace"
	observabilityalertsnotificationchannelsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityalertsnotificationchannel"
	observabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	releasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/release"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	secretreferencesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secretreference"
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
)

// Services aggregates all K8s-native API service interfaces.
type Services struct {
	AuthzService                                  authzsvc.Service
	ProjectService                                projectsvc.Service
	BuildPlaneService                             buildplanesvc.Service
	ClusterBuildPlaneService                      clusterbuildplanesvc.Service
	ClusterComponentTypeService                   clustercomponenttypesvc.Service
	ClusterDataPlaneService                       clusterdataplanesvc.Service
	ClusterObservabilityPlaneService              clusterobservabilityplanesvc.Service
	ClusterTraitService                           clustertraitsvc.Service
	DataPlaneService                              dataplanesvc.Service
	DeploymentPipelineService                     deploymentpipelinesvc.Service
	NamespaceService                              namespacesvc.Service
	ComponentService                              componentsvc.Service
	ComponentReleaseService                       componentreleasesvc.Service
	ComponentTypeService                          componenttypesvc.Service
	EnvironmentService                            environmentsvc.Service
	ObservabilityAlertsNotificationChannelService observabilityalertsnotificationchannelsvc.Service
	ObservabilityPlaneService                     observabilityplanesvc.Service
	ReleaseService                                releasesvc.Service
	ReleaseBindingService                         releasebindingsvc.Service
	SecretReferenceService                        secretreferencesvc.Service
	TraitService                                  traitsvc.Service
	WorkloadService                               workloadsvc.Service
}

// NewServices creates all K8s-native API services with authorization wrappers.
func NewServices(k8sClient client.Client, pap authzcore.PAP, pdp authzcore.PDP, logger *slog.Logger) *Services {
	return &Services{
		AuthzService:                                  authzsvc.NewServiceWithAuthz(pap, pdp, k8sClient, logger.With("component", "authz-service")),
		ProjectService:                                projectsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "project-service")),
		BuildPlaneService:                             buildplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "buildplane-service")),
		ClusterBuildPlaneService:                      clusterbuildplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clusterbuildplane-service")),
		ClusterComponentTypeService:                   clustercomponenttypesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clustercomponenttype-service")),
		ClusterDataPlaneService:                       clusterdataplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clusterdataplane-service")),
		ClusterObservabilityPlaneService:              clusterobservabilityplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clusterobservabilityplane-service")),
		ClusterTraitService:                           clustertraitsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clustertrait-service")),
		DataPlaneService:                              dataplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "dataplane-service")),
		DeploymentPipelineService:                     deploymentpipelinesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "deploymentpipeline-service")),
		NamespaceService:                              namespacesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "namespace-service")),
		ComponentService:                              componentsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "component-service")),
		ComponentReleaseService:                       componentreleasesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "componentrelease-service")),
		ComponentTypeService:                          componenttypesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "componenttype-service")),
		EnvironmentService:                            environmentsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "environment-service")),
		ObservabilityAlertsNotificationChannelService: observabilityalertsnotificationchannelsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "observabilityalertsnotificationchannel-service")),
		ObservabilityPlaneService:                     observabilityplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "observabilityplane-service")),
		ReleaseService:                                releasesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "release-service")),
		ReleaseBindingService:                         releasebindingsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "releasebinding-service")),
		SecretReferenceService:                        secretreferencesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "secretreference-service")),
		TraitService:                                  traitsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "trait-service")),
		WorkloadService:                               workloadsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "workload-service")),
	}
}
