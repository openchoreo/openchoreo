// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"github.com/openchoreo/openchoreo/internal/controller/build/engines"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/engines/argo"
)

// BuildService handles the business logic for build operations
type BuildService struct {
	client       client.Client
	k8sClientMgr *kubernetesClient.KubeMultiClientManager
	logger       logr.Logger
	buildEngines map[string]engines.BuildEngine
}

// NewBuildService creates a new build service
func NewBuildService(client client.Client, k8sClientMgr *kubernetesClient.KubeMultiClientManager) *BuildService {
	service := &BuildService{
		client:       client,
		k8sClientMgr: k8sClientMgr,
		logger:       log.Log.WithName("build-service"),
		buildEngines: make(map[string]engines.BuildEngine),
	}

	// Register available build engines
	service.registerBuildEngines()

	return service
}

// registerBuildEngines registers all available build engines
func (s *BuildService) registerBuildEngines() {
	// Register Argo engine
	argoEngine := argo.NewEngine()
	s.buildEngines[argoEngine.GetName()] = argoEngine

	// Future engines can be registered here:
	// tektonEngine := tekton.NewEngine()
	// s.buildEngines[tektonEngine.GetName()] = tektonEngine
}

// ProcessBuild handles the main build processing logic
func (s *BuildService) ProcessBuild(ctx context.Context, build *openchoreov1alpha1.Build) error {
	logger := s.logger.WithValues("build", build.Name)

	// Get build plane and client
	buildPlane, bpClient, err := s.getBuildPlaneClient(ctx, build)
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
func (s *BuildService) GetBuildStatus(ctx context.Context, build *openchoreov1alpha1.Build) (engines.BuildStatus, error) {
	// Get build plane client
	_, bpClient, err := s.getBuildPlaneClient(ctx, build)
	if err != nil {
		return engines.BuildStatus{}, fmt.Errorf("failed to get build plane client: %w", err)
	}

	// Determine build engine
	engineName := s.determineBuildEngine(build)
	buildEngine, exists := s.buildEngines[engineName]
	if !exists {
		return engines.BuildStatus{}, fmt.Errorf("unsupported build engine: %s", engineName)
	}

	return buildEngine.GetBuildStatus(ctx, bpClient, build)
}

// ExtractBuildArtifacts extracts artifacts from a completed build
func (s *BuildService) ExtractBuildArtifacts(ctx context.Context, build *openchoreov1alpha1.Build) (*engines.BuildArtifacts, error) {
	// Get build plane client
	_, bpClient, err := s.getBuildPlaneClient(ctx, build)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	// Determine build engine
	engineName := s.determineBuildEngine(build)
	buildEngine, exists := s.buildEngines[engineName]
	if !exists {
		return nil, fmt.Errorf("unsupported build engine: %s", engineName)
	}

	return buildEngine.ExtractBuildArtifacts(ctx, bpClient, build)
}

// CreateWorkloadFromArtifacts creates a workload CR from build artifacts
func (s *BuildService) CreateWorkloadFromArtifacts(ctx context.Context, build *openchoreov1alpha1.Build, artifacts *engines.BuildArtifacts) error {
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
func (s *BuildService) UpdateBuildStatusConditions(build *openchoreov1alpha1.Build, status engines.BuildStatus, artifacts *engines.BuildArtifacts) {
	switch status.Phase {
	case engines.BuildPhaseRunning:
		s.setBuildInProgressCondition(build)
	case engines.BuildPhaseSucceeded:
		s.setBuildCompletedCondition(build, "Build completed successfully")
		if artifacts != nil && artifacts.Image != "" {
			build.Status.ImageStatus.Image = artifacts.Image
		}
	case engines.BuildPhaseFailed, engines.BuildPhaseError:
		s.setBuildFailedCondition(build, "BuildFailed", status.Message)
	}
}

// determineBuildEngine determines which build engine to use based on build spec
func (s *BuildService) determineBuildEngine(build *openchoreov1alpha1.Build) string {
	// For now, always use argo. In the future, this could be determined by:
	// - build.Spec.TemplateRef.Engine
	// - build annotations
	// - organization/project defaults
	// - build plane configuration
	if build.Spec.TemplateRef.Engine != "" {
		return build.Spec.TemplateRef.Engine
	}
	return "argo" // default
}

// getBuildPlaneClient gets the build plane and its client
func (s *BuildService) getBuildPlaneClient(ctx context.Context, build *openchoreov1alpha1.Build) (*openchoreov1alpha1.BuildPlane, client.Client, error) {
	buildPlane, err := controller.GetBuildPlane(ctx, s.client, build)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot retrieve the build plane: %w", err)
	}

	bpClient, err := kubernetesClient.GetK8sClient(s.k8sClientMgr, buildPlane.Namespace, buildPlane.Name, buildPlane.Spec.KubernetesCluster)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	return buildPlane, bpClient, nil
}

// Helper methods for setting conditions
func (s *BuildService) setBuildInProgressCondition(build *openchoreov1alpha1.Build) {
	condition := NewBuildInProgressCondition(build.Generation)
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}

func (s *BuildService) setBuildCompletedCondition(build *openchoreov1alpha1.Build, message string) {
	condition := NewBuildCompletedCondition(build.Generation)
	if message != "" {
		condition.Message = message
	}
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}

func (s *BuildService) setBuildFailedCondition(build *openchoreov1alpha1.Build, reason, message string) {
	condition := NewBuildFailedCondition(build.Generation)
	if reason != "" {
		condition.Reason = reason
	}
	if message != "" {
		condition.Message = message
	}
	meta.SetStatusCondition(&build.Status.Conditions, condition)
}
