// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/engines"
	"github.com/openchoreo/openchoreo/internal/controller/build/engines/argo"
)

// Builder handles the business logic for build operations
type Builder struct {
	client       client.Client
	k8sClientMgr *kubernetesClient.KubeMultiClientManager
	gatewayURL   string
	logger       logr.Logger
	buildEngines map[string]engines.BuildEngine
}

// NewBuilder creates a new build service
func NewBuilder(client client.Client, k8sClientMgr *kubernetesClient.KubeMultiClientManager, gatewayURL string) *Builder {
	service := &Builder{
		client:       client,
		k8sClientMgr: k8sClientMgr,
		gatewayURL:   gatewayURL,
		logger:       log.Log.WithName("build-service"),
		buildEngines: make(map[string]engines.BuildEngine),
	}

	// Register available build engines
	service.registerBuildEngines()

	return service
}

// registerBuildEngines registers all available build engines
func (s *Builder) registerBuildEngines() {
	// Register Argo engine
	argoEngine := argo.NewEngine()
	s.buildEngines[argoEngine.GetName()] = argoEngine

	// Future engines can be registered here:
	// tektonEngine := tekton.NewEngine()
	// s.buildEngines[tektonEngine.GetName()] = tektonEngine
}

func (s *Builder) EnsurePrerequisites(ctx context.Context, build *openchoreov1alpha1.Build, bpClient client.Client) error {
	// Determine build engine
	engineName := s.determineBuildEngine(build)
	buildEngine, exists := s.buildEngines[engineName]
	if !exists {
		return fmt.Errorf("unsupported build engine: %s", engineName)
	}

	// Ensure prerequisites using the selected build engine
	if err := buildEngine.EnsurePrerequisites(ctx, bpClient, build); err != nil {
		return fmt.Errorf("failed to ensure prerequisites: %w", err)
	}
	return nil
}

func (s *Builder) CreateBuild(ctx context.Context, build *openchoreov1alpha1.Build, bpClient client.Client) (*engines.BuildCreationResponse, error) {
	// Determine build engine
	engineName := s.determineBuildEngine(build)
	buildEngine, exists := s.buildEngines[engineName]
	if !exists {
		return nil, fmt.Errorf("unsupported build engine: %s", engineName)
	}

	// Create or get existing build
	buildInfo, err := buildEngine.CreateBuild(ctx, bpClient, build)
	if err != nil {
		return nil, fmt.Errorf("failed to create build: %w", err)
	}

	return &buildInfo, nil
}

// ProcessBuild handles the main build processing logic
func (s *Builder) ProcessBuild(ctx context.Context, build *openchoreov1alpha1.Build) error {
	logger := s.logger.WithValues("build", build.Name)

	// Get build plane and client
	bpClient, err := s.getBuildPlaneClient(ctx, build)
	if err != nil {
		return fmt.Errorf("failed to get build plane client: %w", err)
	}

	// Determine build engine (default to argo for now)
	engineName := s.determineBuildEngine(build)
	buildEngine, exists := s.buildEngines[engineName]
	if !exists {
		return fmt.Errorf("unsupported build engine: %s", engineName)
	}

	logger.Info("Using build engine", "engine", engineName)

	// Ensure prerequisites
	if err := buildEngine.EnsurePrerequisites(ctx, bpClient, build); err != nil {
		return fmt.Errorf("failed to ensure prerequisites: %w", err)
	}

	// Create or get existing build
	buildInfo, err := buildEngine.CreateBuild(ctx, bpClient, build)
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}

	logger.Info("Build processed", "buildID", buildInfo.ID, "created", buildInfo.Created)
	return nil
}

// GetBuildStatus returns the current status of a build
func (s *Builder) GetBuildStatus(ctx context.Context, build *openchoreov1alpha1.Build, bpClient client.Client) (engines.BuildStatus, error) {
	// Determine build engine
	engineName := s.determineBuildEngine(build)
	buildEngine, exists := s.buildEngines[engineName]
	if !exists {
		return engines.BuildStatus{}, fmt.Errorf("unsupported build engine: %s", engineName)
	}

	return buildEngine.GetBuildStatus(ctx, bpClient, build)
}

// ExtractBuildArtifacts extracts artifacts from a completed build
func (s *Builder) ExtractBuildArtifacts(ctx context.Context, build *openchoreov1alpha1.Build, bpClient client.Client) (*engines.BuildArtifacts, error) {
	// Determine build engine
	engineName := s.determineBuildEngine(build)
	buildEngine, exists := s.buildEngines[engineName]
	if !exists {
		return nil, fmt.Errorf("unsupported build engine: %s", engineName)
	}

	return buildEngine.ExtractBuildArtifacts(ctx, bpClient, build)
}

// CreateWorkloadFromArtifacts creates a workload CR from build artifacts
func (s *Builder) CreateWorkloadFromArtifacts(ctx context.Context, build *openchoreov1alpha1.Build, artifacts *engines.BuildArtifacts) error {
	logger := s.logger.WithValues("build", build.Name)

	if artifacts.WorkloadCR == "" {
		logger.Info("No workload CR found in build artifacts, skipping workload creation")
		return nil
	}

	// Parse the YAML into a Workload object
	workload := &openchoreov1alpha1.Workload{}
	if err := yaml.Unmarshal([]byte(artifacts.WorkloadCR), workload); err != nil {
		return fmt.Errorf("failed to unmarshal workload CR: %w", err)
	}

	// Set the namespace to match the build
	workload.Namespace = build.Namespace

	// Try to create the workload CR
	if err := s.client.Create(ctx, workload); err != nil {
		if client.IgnoreAlreadyExists(err) == nil {
			logger.Info("Workload CR already exists", "name", workload.Name, "namespace", workload.Namespace)
			return nil
		}
		return fmt.Errorf("failed to create workload CR: %w", err)
	}

	logger.Info("Successfully created workload CR", "name", workload.Name, "namespace", workload.Namespace)
	return nil
}

// UpdateBuildStatusConditions updates build status based on current build status
func (s *Builder) UpdateBuildStatusConditions(build *openchoreov1alpha1.Build, status engines.BuildStatus, artifacts *engines.BuildArtifacts) {
	switch status.Phase {
	case engines.BuildPhaseRunning:
		s.setBuildInProgressCondition(build)
	case engines.BuildPhaseSucceeded:
		s.setBuildCompletedCondition(build, "Build completed successfully")
		if artifacts != nil && artifacts.Image != "" {
			build.Status.ImageStatus.Image = artifacts.Image
		}
	case engines.BuildPhaseFailed, engines.BuildPhaseUnknown:
		s.setBuildFailedCondition(build, "BuildFailed", status.Message)
	}
}

// determineBuildEngine determines which build engine to use based on build spec
func (s *Builder) determineBuildEngine(build *openchoreov1alpha1.Build) string {
	// For now, always use argo. In the future, this could be determined by:
	// - build.Spec.TemplateRef.Engine
	if build.Spec.TemplateRef.Engine != "" {
		return build.Spec.TemplateRef.Engine
	}
	return "argo" // default
}

// getBuildPlaneClient gets the build plane and its client
func (s *Builder) getBuildPlaneClient(ctx context.Context, build *openchoreov1alpha1.Build) (client.Client, error) {
	project, err := controller.FindProjectByName(ctx, s.client, build.Namespace, build.Spec.Owner.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the project for build plane lookup: %w", err)
	}

	buildPlaneResult, err := controller.GetBuildPlaneOrClusterBuildPlaneOfProject(ctx, s.client, project)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the build plane: %w", err)
	}
	if buildPlaneResult == nil {
		return nil, fmt.Errorf("no build plane found for project '%s'", project.Name)
	}

	bpClient, err := buildPlaneResult.GetK8sClient(s.k8sClientMgr, s.gatewayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	return bpClient, nil
}

// GetBuildPlaneClient gets the build plane client for a given BuildPlaneResult - public method for controller access
func (s *Builder) GetBuildPlaneClient(buildPlaneResult *controller.BuildPlaneResult) (client.Client, error) {
	bpClient, err := buildPlaneResult.GetK8sClient(s.k8sClientMgr, s.gatewayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	return bpClient, nil
}

func (s *Builder) setBuildInProgressCondition(build *openchoreov1alpha1.Build) {
	condition := NewBuildInProgressCondition(build.Generation)
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}

func (s *Builder) setBuildCompletedCondition(build *openchoreov1alpha1.Build, message string) {
	condition := NewBuildCompletedCondition(build.Generation)
	if message != "" {
		condition.Message = message
	}
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}

func (s *Builder) setBuildFailedCondition(build *openchoreov1alpha1.Build, reason, message string) {
	condition := NewBuildFailedCondition(build.Generation)
	if reason != "" {
		condition.Reason = reason
	}
	if message != "" {
		condition.Message = message
	}
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}
