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
	clusterdataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterdataplane"
	clusterobservabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterobservabilityplane"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	dataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane"
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
	observabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	releasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/release"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
)

// errNotImplemented is returned for stub methods that are not yet implemented.
// TODO: Remove this error once all handler stubs are fully implemented.
var errNotImplemented = errors.New("not implemented")

// Handler implements gen.StrictServerInterface
type Handler struct {
	services                         *services.Services
	authzService                     authzsvc.Service
	projectService                   projectsvc.Service
	buildPlaneService                buildplanesvc.Service
	clusterBuildPlaneService         clusterbuildplanesvc.Service
	clusterDataPlaneService          clusterdataplanesvc.Service
	clusterObservabilityPlaneService clusterobservabilityplanesvc.Service
	dataPlaneService                 dataplanesvc.Service
	componentService                 componentsvc.Service
	componentReleaseService          componentreleasesvc.Service
	componentTypeService             componenttypesvc.Service
	environmentService               environmentsvc.Service
	observabilityPlaneService        observabilityplanesvc.Service
	releaseService                   releasesvc.Service
	releaseBindingService            releasebindingsvc.Service
	traitService                     traitsvc.Service
	workloadService                  workloadsvc.Service
	logger                           *slog.Logger
	Config                           *config.Config
}

// Compile-time check that Handler implements StrictServerInterface
var _ gen.StrictServerInterface = (*Handler)(nil)

// New creates a new Handler
func New(services *services.Services, authzService authzsvc.Service, projectService projectsvc.Service, buildPlaneService buildplanesvc.Service, clusterBuildPlaneService clusterbuildplanesvc.Service, clusterDataPlaneService clusterdataplanesvc.Service, clusterObservabilityPlaneService clusterobservabilityplanesvc.Service, dataPlaneService dataplanesvc.Service, componentService componentsvc.Service, componentReleaseService componentreleasesvc.Service, componentTypeService componenttypesvc.Service, environmentService environmentsvc.Service, observabilityPlaneService observabilityplanesvc.Service, releaseService releasesvc.Service, releaseBindingService releasebindingsvc.Service, traitService traitsvc.Service, workloadService workloadsvc.Service, logger *slog.Logger, cfg *config.Config) *Handler {
	return &Handler{
		services:                         services,
		authzService:                     authzService,
		projectService:                   projectService,
		buildPlaneService:                buildPlaneService,
		clusterBuildPlaneService:         clusterBuildPlaneService,
		clusterDataPlaneService:          clusterDataPlaneService,
		clusterObservabilityPlaneService: clusterObservabilityPlaneService,
		dataPlaneService:                 dataPlaneService,
		componentService:                 componentService,
		componentReleaseService:          componentReleaseService,
		componentTypeService:             componentTypeService,
		environmentService:               environmentService,
		observabilityPlaneService:        observabilityPlaneService,
		releaseService:                   releaseService,
		releaseBindingService:            releaseBindingService,
		traitService:                     traitService,
		workloadService:                  workloadService,
		logger:                           logger,
		Config:                           cfg,
	}
}
