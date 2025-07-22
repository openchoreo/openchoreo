// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// BuildService handles build-related business logic
type BuildService struct {
	k8sClient         client.Client
	logger            *slog.Logger
	buildPlaneService *BuildPlaneService
	BPClientMgr       *kubernetesClient.KubeMultiClientManager
}

// NewBuildService creates a new build service
func NewBuildService(k8sClient client.Client, buildPlaneService *BuildPlaneService, BPClientMgr *kubernetesClient.KubeMultiClientManager, logger *slog.Logger) *BuildService {
	return &BuildService{
		k8sClient:         k8sClient,
		logger:            logger,
		buildPlaneService: buildPlaneService,
		BPClientMgr:       BPClientMgr,
	}
}

// ListBuildTemplates retrieves cluster workflow templates (argo) available for an organization in the buildplane
func (s *BuildService) ListBuildTemplates(ctx context.Context, orgName string) ([]models.BuildTemplateResponse, error) {
	s.logger.Debug("Listing build templates", "org", orgName)

	// Get the build plane Kubernetes client
	buildPlaneClient, err := s.buildPlaneService.GetBuildPlaneClient(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	// List ClusterWorkflowTemplates using the build plane client
	var clusterWorkflowTemplates argo.ClusterWorkflowTemplateList
	err = buildPlaneClient.List(ctx, &clusterWorkflowTemplates)
	if err != nil {
		s.logger.Error("Failed to list ClusterWorkflowTemplates", "error", err)
		return nil, fmt.Errorf("failed to list ClusterWorkflowTemplates: %w", err)
	}

	s.logger.Debug("Found build templates", "count", len(clusterWorkflowTemplates.Items), "org", orgName)

	var templateResponses []models.BuildTemplateResponse
	for _, template := range clusterWorkflowTemplates.Items {
		var parameters []models.BuildTemplateParameter
		if template.Spec.Arguments.Parameters != nil {
			for _, param := range template.Spec.Arguments.Parameters {
				templateParam := models.BuildTemplateParameter{
					Name:     param.Name,
					Required: param.Default == nil, // If no default, it's required
				}

				if param.Default != nil {
					templateParam.Default = string(*param.Default)
				}

				parameters = append(parameters, templateParam)
			}
		}

		templateResponse := models.BuildTemplateResponse{
			Name:       template.Name,
			Parameters: parameters,
			CreatedAt:  template.CreationTimestamp.Time,
		}

		templateResponses = append(templateResponses, templateResponse)
	}

	return templateResponses, nil
}

// TriggerBuild creates a new build from a component
func (s *BuildService) TriggerBuild(ctx context.Context, orgName, projectName, componentName, commit string) (*models.BuildResponse, error) {
	s.logger.Debug("Triggering build", "org", orgName, "project", projectName, "component", componentName, "commit", commit)

	// Retrieve component and use that to create the build
	var component openchoreov1alpha1.ComponentV2
	err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}, &component)

	if err != nil {
		s.logger.Error("Failed to get component", "error", err, "org", orgName, "project", projectName, "component", componentName)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	buildUUID := uuid.New().String()
	buildID := strings.ReplaceAll(buildUUID[:8], "-", "")

	buildName := fmt.Sprintf("%s-build-%s", componentName, buildID)

	build := &openchoreov1alpha1.BuildV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildName,
			Namespace: orgName,
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: orgName,
				labels.LabelKeyProjectName:      projectName,
				labels.LabelKeyComponentName:    componentName,
			},
		},
		Spec: openchoreov1alpha1.BuildV2Spec{
			Owner: openchoreov1alpha1.BuildOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			Repository: openchoreov1alpha1.Repository{
				URL: component.Spec.Build.Repository.URL,
				Revision: openchoreov1alpha1.Revision{
					Branch: component.Spec.Build.Repository.Revision.Branch,
					Commit: commit,
				},
				AppPath: component.Spec.Build.Repository.AppPath,
			},
			TemplateRef: component.Spec.Build.TemplateRef,
		},
	}

	err = s.k8sClient.Create(ctx, build)
	if err != nil {
		s.logger.Error("Failed to create build", "error", err)
		return nil, fmt.Errorf("failed to create build: %w", err)
	}

	s.logger.Info("Build created successfully", "build", buildName)

	if commit == "" {
		commit = "latest"
	}

	return &models.BuildResponse{
		Name:          buildName,
		ComponentName: componentName,
		ProjectName:   projectName,
		OrgName:       orgName,
		Commit:        commit,
		Status:        "Created",
		CreatedAt:     build.CreationTimestamp.Time,
	}, nil
}

// ListBuilds retrieves builds for a component using spec.owner fields instead of labels
func (s *BuildService) ListBuilds(ctx context.Context, orgName, projectName, componentName string) ([]models.BuildResponse, error) {
	s.logger.Debug("Listing builds", "org", orgName, "project", projectName, "component", componentName)

	var builds openchoreov1alpha1.BuildV2List
	err := s.k8sClient.List(ctx, &builds, client.InNamespace(orgName))
	if err != nil {
		s.logger.Error("Failed to list builds", "error", err)
		return nil, fmt.Errorf("failed to list builds: %w", err)
	}

	var buildResponses []models.BuildResponse
	for _, build := range builds.Items {
		// Filter by spec.owner fields instead of labels
		if build.Spec.Owner.ProjectName != projectName || build.Spec.Owner.ComponentName != componentName {
			continue
		}

		// This commit hash should always be there since the build is triggered with a commit
		// If not provided, we can default to "latest" for now.
		commit := build.Spec.Repository.Revision.Commit
		if commit == "" {
			commit = "latest"
		}

		buildResponses = append(buildResponses, models.BuildResponse{
			Name:          build.Name,
			ComponentName: componentName,
			ProjectName:   projectName,
			OrgName:       orgName,
			Commit:        commit,
			Status:        GetLatestBuildStatus(build.Status.Conditions),
			CreatedAt:     build.CreationTimestamp.Time,
		})
	}

	return buildResponses, nil
}

func GetLatestBuildStatus(buildConditions []metav1.Condition) string {
	if len(buildConditions) == 0 {
		return "Unknown"
	}

	latestCondition := buildConditions[0]
	for _, condition := range buildConditions {
		if condition.LastTransitionTime.Time.After(latestCondition.LastTransitionTime.Time) {
			latestCondition = condition
		}
	}

	return string(latestCondition.Reason)
}
