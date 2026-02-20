// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
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
	observabilityalertsnotificationchannelsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityalertsnotificationchannel"
	observabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	releasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/release"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	secretreferencesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secretreference"
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
)

// errNotImplemented is returned for stub methods that are not yet implemented.
// TODO: Remove this error once all handler stubs are fully implemented.
var errNotImplemented = errors.New("not implemented")

// Handler implements gen.StrictServerInterface
type Handler struct {
	services                                      *services.Services
	authzService                                  authzsvc.Service
	projectService                                projectsvc.Service
	buildPlaneService                             buildplanesvc.Service
	clusterBuildPlaneService                      clusterbuildplanesvc.Service
	clusterComponentTypeService                   clustercomponenttypesvc.Service
	clusterDataPlaneService                       clusterdataplanesvc.Service
	clusterTraitService                           clustertraitsvc.Service
	clusterObservabilityPlaneService              clusterobservabilityplanesvc.Service
	dataPlaneService                              dataplanesvc.Service
	deploymentPipelineService                     deploymentpipelinesvc.Service
	componentService                              componentsvc.Service
	componentReleaseService                       componentreleasesvc.Service
	componentTypeService                          componenttypesvc.Service
	environmentService                            environmentsvc.Service
	observabilityAlertsNotificationChannelService observabilityalertsnotificationchannelsvc.Service
	observabilityPlaneService                     observabilityplanesvc.Service
	releaseService                                releasesvc.Service
	releaseBindingService                         releasebindingsvc.Service
	secretReferenceService                        secretreferencesvc.Service
	traitService                                  traitsvc.Service
	workloadService                               workloadsvc.Service
	logger                                        *slog.Logger
	Config                                        *config.Config
}

// Compile-time check that Handler implements StrictServerInterface
var _ gen.StrictServerInterface = (*Handler)(nil)

// New creates a new Handler
func New(services *services.Services, authzService authzsvc.Service, projectService projectsvc.Service, buildPlaneService buildplanesvc.Service, clusterBuildPlaneService clusterbuildplanesvc.Service, clusterComponentTypeService clustercomponenttypesvc.Service, clusterDataPlaneService clusterdataplanesvc.Service, clusterObservabilityPlaneService clusterobservabilityplanesvc.Service, clusterTraitService clustertraitsvc.Service, dataPlaneService dataplanesvc.Service, deploymentPipelineService deploymentpipelinesvc.Service, componentService componentsvc.Service, componentReleaseService componentreleasesvc.Service, componentTypeService componenttypesvc.Service, environmentService environmentsvc.Service, observabilityAlertsNotificationChannelService observabilityalertsnotificationchannelsvc.Service, observabilityPlaneService observabilityplanesvc.Service, releaseService releasesvc.Service, releaseBindingService releasebindingsvc.Service, secretReferenceService secretreferencesvc.Service, traitService traitsvc.Service, workloadService workloadsvc.Service, logger *slog.Logger, cfg *config.Config) *Handler {
	return &Handler{
		services:                                      services,
		authzService:                                  authzService,
		projectService:                                projectService,
		buildPlaneService:                             buildPlaneService,
		clusterBuildPlaneService:                      clusterBuildPlaneService,
		clusterComponentTypeService:                   clusterComponentTypeService,
		clusterDataPlaneService:                       clusterDataPlaneService,
		clusterTraitService:                           clusterTraitService,
		clusterObservabilityPlaneService:              clusterObservabilityPlaneService,
		dataPlaneService:                              dataPlaneService,
		deploymentPipelineService:                     deploymentPipelineService,
		componentService:                              componentService,
		componentReleaseService:                       componentReleaseService,
		componentTypeService:                          componentTypeService,
		environmentService:                            environmentService,
		observabilityAlertsNotificationChannelService: observabilityAlertsNotificationChannelService,
		observabilityPlaneService:                     observabilityPlaneService,
		releaseService:                                releaseService,
		releaseBindingService:                         releaseBindingService,
		secretReferenceService:                        secretReferenceService,
		traitService:                                  traitService,
		workloadService:                               workloadService,
		logger:                                        logger,
		Config:                                        cfg,
	}
}
