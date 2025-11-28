// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	v1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GitHubWebhookService handles GitHub webhook processing
type GitHubWebhookService struct {
	k8sClient    client.Client
	gitProvider  git.Provider
	buildService *BuildService
}

// NewGitHubWebhookService creates a new GitHubWebhookService
func NewGitHubWebhookService(k8sClient client.Client, gitProvider git.Provider, buildService *BuildService) *GitHubWebhookService {
	return &GitHubWebhookService{
		k8sClient:    k8sClient,
		gitProvider:  gitProvider,
		buildService: buildService,
	}
}

// ProcessWebhook processes an incoming webhook payload
func (s *GitHubWebhookService) ProcessWebhook(ctx context.Context, payload []byte, signature, webhookSecret string) ([]string, error) {
	logger := log.FromContext(ctx)

	// Validate signature
	if err := s.gitProvider.ValidateWebhookPayload(payload, signature, webhookSecret); err != nil {
		logger.Error(err, "Invalid webhook signature")
		return nil, fmt.Errorf("invalid webhook signature: %w", err)
	}

	// Parse payload
	event, err := s.gitProvider.ParseWebhookPayload(payload)
	if err != nil {
		logger.Error(err, "Failed to parse webhook payload")
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	logger.Info("Processing webhook event",
		"repository", event.RepositoryURL,
		"branch", event.Branch,
		"commit", event.Commit,
		"modifiedPaths", len(event.ModifiedPaths))

	// Find affected components
	affectedComponents, err := s.findAffectedComponents(ctx, event)
	if err != nil {
		logger.Error(err, "Failed to find affected components")
		return nil, fmt.Errorf("failed to find affected components: %w", err)
	}

	logger.Info("Found affected components", "count", len(affectedComponents))

	// Trigger builds for affected components
	triggeredComponents := make([]string, 0)
	for _, comp := range affectedComponents {
		orgName := comp.Namespace // Assuming namespace is the org name
		projectName := comp.Spec.Owner.ProjectName
		componentName := comp.Name

		logger.Info("Triggering build for component",
			"org", orgName,
			"project", projectName,
			"component", componentName,
			"commit", event.Commit)

		_, err := s.buildService.TriggerBuild(
			ctx,
			orgName,
			projectName,
			componentName,
			event.Commit,
		)
		if err != nil {
			// Log error but continue processing other components
			logger.Error(err, "Failed to trigger build for component",
				"component", componentName)
			continue
		}

		triggeredComponents = append(triggeredComponents, componentName)
	}

	logger.Info("Webhook processing completed",
		"affectedComponents", len(affectedComponents),
		"triggeredBuilds", len(triggeredComponents))

	return triggeredComponents, nil
}

// findAffectedComponents finds all components that should be built based on the webhook event
func (s *GitHubWebhookService) findAffectedComponents(ctx context.Context, event *git.WebhookEvent) ([]*v1alpha1.Component, error) {
	logger := log.FromContext(ctx)

	// List all components
	componentList := &v1alpha1.ComponentList{}
	if err := s.k8sClient.List(ctx, componentList); err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	affected := make([]*v1alpha1.Component, 0)
	for i := range componentList.Items {
		comp := &componentList.Items[i]

		// Check if auto-build is enabled
		if comp.Spec.AutoBuild == nil || !*comp.Spec.AutoBuild {
			continue
		}

		// Get repository URL from component workflow
		repoURL, appPath, err := s.extractRepoInfoFromComponent(comp)
		if err != nil {
			logger.V(1).Info("Failed to extract repo info from component",
				"component", comp.Name,
				"error", err)
			continue
		}

		// Check if component's repository matches the webhook repository
		if !s.matchesRepository(repoURL, event.RepositoryURL) {
			continue
		}

		// Check if modified paths affect this component
		if s.isComponentAffected(appPath, event.ModifiedPaths) {
			logger.Info("Component is affected by webhook event",
				"component", comp.Name,
				"appPath", appPath)
			affected = append(affected, comp)
		}
	}

	return affected, nil
}

// extractRepoInfoFromComponent extracts repository URL and appPath from component workflow schema
func (s *GitHubWebhookService) extractRepoInfoFromComponent(comp *v1alpha1.Component) (repoURL string, appPath string, err error) {
	if comp.Spec.Workflow == nil || comp.Spec.Workflow.Schema == nil {
		return "", "", fmt.Errorf("component has no workflow schema")
	}

	// Parse the workflow schema to extract repository information
	var schema map[string]interface{}
	if err := json.Unmarshal(comp.Spec.Workflow.Schema.Raw, &schema); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal workflow schema: %w", err)
	}

	// Extract repository URL
	if repo, ok := schema["repository"].(map[string]interface{}); ok {
		if url, ok := repo["url"].(string); ok {
			repoURL = url
		}
	}

	// Extract appPath
	if path, ok := schema["appPath"].(string); ok {
		appPath = path
	}

	if repoURL == "" {
		return "", "", fmt.Errorf("repository URL not found in workflow schema")
	}

	return repoURL, appPath, nil
}

// matchesRepository checks if component's repository matches the webhook repository
func (s *GitHubWebhookService) matchesRepository(componentRepoURL, webhookRepoURL string) bool {
	// Normalize both URLs for comparison
	componentRepoURL = normalizeRepoURL(componentRepoURL)
	webhookRepoURL = normalizeRepoURL(webhookRepoURL)

	return componentRepoURL == webhookRepoURL
}

// isComponentAffected checks if any modified path affects the component
func (s *GitHubWebhookService) isComponentAffected(appPath string, modifiedPaths []string) bool {
	// If no specific path filter, component is always affected
	if appPath == "" {
		return true
	}

	// Normalize appPath
	appPath = strings.TrimPrefix(appPath, "/")
	appPath = strings.TrimSuffix(appPath, "/")

	// Check if any modified path is under the component's appPath
	for _, path := range modifiedPaths {
		path = strings.TrimPrefix(path, "/")

		// Check if path is under appPath or is the appPath itself
		if path == appPath || strings.HasPrefix(path, appPath+"/") {
			return true
		}
	}

	return false
}