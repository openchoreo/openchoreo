// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlerservices

import (
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	authzsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/authz"
	autobuildsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/autobuild"
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	clusterdataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterdataplane"
	clusterobservabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterobservabilityplane"
	clustertraitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustertrait"
	clusterworkflowsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflow"
	clusterworkflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflowplane"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	dataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane"
	deploymentpipelinesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline"
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
	gitsecretsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/gitsecret"
	k8sresourcessvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
	namespacesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace"
	observabilityalertsnotificationchannelsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityalertsnotificationchannel"
	observabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	secretreferencesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secretreference"
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
	workflowsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflow"
	workflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowplane"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
)

// Services aggregates all K8s-native API service interfaces.
type Services struct {
	AutoBuildService                              autobuildsvc.Service
	AuthzService                                  authzsvc.Service
	ProjectService                                projectsvc.Service
	WorkflowPlaneService                          workflowplanesvc.Service
	ClusterWorkflowPlaneService                   clusterworkflowplanesvc.Service
	ClusterComponentTypeService                   clustercomponenttypesvc.Service
	ClusterDataPlaneService                       clusterdataplanesvc.Service
	ClusterObservabilityPlaneService              clusterobservabilityplanesvc.Service
	ClusterTraitService                           clustertraitsvc.Service
	ClusterWorkflowService                        clusterworkflowsvc.Service
	DataPlaneService                              dataplanesvc.Service
	DeploymentPipelineService                     deploymentpipelinesvc.Service
	NamespaceService                              namespacesvc.Service
	ComponentService                              componentsvc.Service
	ComponentReleaseService                       componentreleasesvc.Service
	ComponentTypeService                          componenttypesvc.Service
	EnvironmentService                            environmentsvc.Service
	GitSecretService                              gitsecretsvc.Service
	ObservabilityAlertsNotificationChannelService observabilityalertsnotificationchannelsvc.Service
	ObservabilityPlaneService                     observabilityplanesvc.Service
	K8sResourcesService                           k8sresourcessvc.Service
	ReleaseBindingService                         releasebindingsvc.Service
	SecretReferenceService                        secretreferencesvc.Service
	TraitService                                  traitsvc.Service
	WorkflowService                               workflowsvc.Service
	WorkflowRunService                            workflowrunsvc.Service
	WorkloadService                               workloadsvc.Service
}

// NewServices creates all K8s-native API services with authorization wrappers.
func NewServices(k8sClient client.Client, pap authzcore.PAP, pdp authzcore.PDP, wpClientMgr *kubernetesClient.KubeMultiClientManager, gatewayURL string, logger *slog.Logger, gwClient *gatewayClient.Client, webhookProcessor autobuildsvc.WebhookProcessor) *Services {
	return &Services{
		AutoBuildService:                              autobuildsvc.NewService(k8sClient, webhookProcessor, logger.With("component", "autobuild-service")),
		AuthzService:                                  authzsvc.NewServiceWithAuthz(pap, pdp, logger.With("component", "authz-service")),
		ProjectService:                                projectsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "project-service")),
		WorkflowPlaneService:                          workflowplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "workflowplane-service")),
		ClusterWorkflowPlaneService:                   clusterworkflowplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clusterworkflowplane-service")),
		ClusterComponentTypeService:                   clustercomponenttypesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clustercomponenttype-service")),
		ClusterDataPlaneService:                       clusterdataplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clusterdataplane-service")),
		ClusterObservabilityPlaneService:              clusterobservabilityplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clusterobservabilityplane-service")),
		ClusterTraitService:                           clustertraitsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clustertrait-service")),
		ClusterWorkflowService:                        clusterworkflowsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "clusterworkflow-service")),
		DataPlaneService:                              dataplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "dataplane-service")),
		DeploymentPipelineService:                     deploymentpipelinesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "deploymentpipeline-service")),
		NamespaceService:                              namespacesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "namespace-service")),
		ComponentService:                              componentsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "component-service")),
		ComponentReleaseService:                       componentreleasesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "componentrelease-service")),
		ComponentTypeService:                          componenttypesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "componenttype-service")),
		EnvironmentService:                            environmentsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "environment-service")),
		GitSecretService:                              gitsecretsvc.NewServiceWithAuthz(k8sClient, wpClientMgr, pdp, logger.With("component", "gitsecret-service"), gatewayURL),
		ObservabilityAlertsNotificationChannelService: observabilityalertsnotificationchannelsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "observabilityalertsnotificationchannel-service")),
		ObservabilityPlaneService:                     observabilityplanesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "observabilityplane-service")),
		K8sResourcesService:                           k8sresourcessvc.NewServiceWithAuthz(k8sClient, gwClient, pdp, logger.With("component", "k8sresources-service")),
		ReleaseBindingService:                         releasebindingsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "releasebinding-service")),
		SecretReferenceService:                        secretreferencesvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "secretreference-service")),
		TraitService:                                  traitsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "trait-service")),
		WorkflowService:                               workflowsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "workflow-service")),
		WorkflowRunService:                            workflowrunsvc.NewServiceWithAuthz(k8sClient, wpClientMgr, gwClient, pdp, logger.With("component", "workflowrun-service")),
		WorkloadService:                               workloadsvc.NewServiceWithAuthz(k8sClient, pdp, logger.With("component", "workload-service")),
	}
}
