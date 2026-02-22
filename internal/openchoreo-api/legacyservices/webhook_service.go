// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices/git"
)

// WebhookService handles webhook processing for all git providers
type WebhookService struct {
	k8sClient       client.Client
	workflowService *WorkflowRunService
}

// NewWebhookService creates a new WebhookService
func NewWebhookService(k8sClient client.Client, workflowService *WorkflowRunService) *WebhookService {
	return &WebhookService{
		k8sClient:       k8sClient,
		workflowService: workflowService,
	}
}

// ProcessWebhook processes an incoming webhook payload from any git provider
func (s *WebhookService) ProcessWebhook(ctx context.Context, provider git.Provider, payload []byte) ([]string, error) {
	logger := log.FromContext(ctx)

	// Parse payload using the provider
	event, err := provider.ParseWebhookPayload(payload)
	if err != nil {
		logger.Error(err, "Failed to parse webhook payload")
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	logger.Info("Processing webhook event",
		"provider", event.Provider,
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
		namespaceName := comp.Namespace
		projectName := comp.Spec.Owner.ProjectName
		componentName := comp.Name

		logger.Info("Triggering build for component",
			"namespace", namespaceName,
			"project", projectName,
			"component", componentName,
			"commit", event.Commit)

		_, err := s.workflowService.triggerWorkflowInternal(
			ctx,
			namespaceName,
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

		triggeredComponents = append(triggeredComponents, fmt.Sprintf("%s/%s", comp.Namespace, componentName))
	}

	logger.Info("Webhook processing completed",
		"affectedComponents", len(affectedComponents),
		"triggeredBuilds", len(triggeredComponents))

	return triggeredComponents, nil
}

// findAffectedComponents finds all components that should be built based on the webhook event
func (s *WebhookService) findAffectedComponents(ctx context.Context, event *git.WebhookEvent) ([]*v1alpha1.Component, error) {
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

		// Get repository URL from component workflow via Workflow CR annotation
		repoURL, appPath, err := s.extractRepoInfoFromComponent(ctx, comp)
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
		// If no modified paths (e.g., Bitbucket), trigger all components for the repo
		if len(event.ModifiedPaths) == 0 || s.isComponentAffected(appPath, event.ModifiedPaths) {
			logger.Info("Component is affected by webhook event",
				"component", comp.Name,
				"appPath", appPath,
				"modifiedPaths", len(event.ModifiedPaths))
			affected = append(affected, comp)
		}
	}

	return affected, nil
}

// extractRepoInfoFromComponent extracts repository URL and appPath from a component's workflow parameters
// by looking up the Workflow CR annotation to find the parameter paths.
func (s *WebhookService) extractRepoInfoFromComponent(ctx context.Context, comp *v1alpha1.Component) (repoURL string, appPath string, err error) {
	if comp.Spec.Workflow == nil || comp.Spec.Workflow.Name == "" {
		return "", "", fmt.Errorf("component has no workflow configuration")
	}

	// Fetch the Workflow CR to get the annotation mapping
	workflow := &v1alpha1.Workflow{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{
		Name:      comp.Spec.Workflow.Name,
		Namespace: comp.Namespace,
	}, workflow); err != nil {
		return "", "", fmt.Errorf("failed to get workflow %s: %w", comp.Spec.Workflow.Name, err)
	}

	// Parse the annotation that maps logical keys to parameter paths
	annotation := workflow.Annotations[controller.AnnotationKeyComponentWorkflowParameters]
	paramMap := parseComponentWorkflowAnnotation(annotation)

	// Get repoUrl path from the annotation
	repoURLPath, ok := paramMap["repoUrl"]
	if !ok {
		return "", "", fmt.Errorf("workflow %s annotation missing repoUrl mapping", comp.Spec.Workflow.Name)
	}

	repoURL, err = getNestedStringFromRawExtension(comp.Spec.Workflow.Parameters, repoURLPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to extract repoUrl from parameters: %w", err)
	}

	if repoURL == "" {
		return "", "", fmt.Errorf("repository URL is empty in component parameters")
	}

	// Get appPath (optional - not all workflows may have it)
	if appPathPath, ok := paramMap["appPath"]; ok {
		appPath, _ = getNestedStringFromRawExtension(comp.Spec.Workflow.Parameters, appPathPath)
	}

	return repoURL, appPath, nil
}

// parseComponentWorkflowAnnotation parses the component-workflow-parameters annotation string
// into a map of logical key to dotted parameter path.
// Format: "key1: path1, key2: path2, ..."
// Example: "repoUrl: parameters.repository.url, branch: parameters.repository.revision.branch"
func parseComponentWorkflowAnnotation(annotation string) map[string]string {
	result := make(map[string]string)
	if annotation == "" {
		return result
	}
	pairs := strings.Split(annotation, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			path := strings.TrimSpace(parts[1])
			if key != "" && path != "" {
				result[key] = path
			}
		}
	}
	return result
}

// getNestedStringFromRawExtension navigates a runtime.RawExtension JSON blob using a dotted path
// and returns the string value. The leading "parameters." prefix is stripped if present.
func getNestedStringFromRawExtension(raw *runtime.RawExtension, dottedPath string) (string, error) {
	if raw == nil || raw.Raw == nil {
		return "", fmt.Errorf("parameters is nil")
	}

	// Strip the "parameters." prefix since we're already inside the parameters object
	path := strings.TrimPrefix(dottedPath, "parameters.")

	var data map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return "", fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	parts := strings.Split(path, ".")
	current := interface{}(data)
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("path %s: expected object at %s", dottedPath, part)
		}
		current, ok = m[part]
		if !ok {
			return "", fmt.Errorf("path %s: key %s not found", dottedPath, part)
		}
	}

	str, ok := current.(string)
	if !ok {
		return "", fmt.Errorf("path %s: value is not a string", dottedPath)
	}
	return str, nil
}

// setNestedValueInParameters takes a runtime.RawExtension, sets a string value at the given
// dotted path (stripping leading "parameters."), and returns a new runtime.RawExtension.
func setNestedValueInParameters(raw *runtime.RawExtension, dottedPath, value string) (*runtime.RawExtension, error) {
	if raw == nil || raw.Raw == nil {
		return nil, fmt.Errorf("parameters is nil")
	}

	path := strings.TrimPrefix(dottedPath, "parameters.")

	var data map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	parts := strings.Split(path, ".")
	current := data
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			// Create intermediate objects if they don't exist
			newObj := make(map[string]interface{})
			current[part] = newObj
			current = newObj
			continue
		}
		m, ok := next.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("path %s: expected object at %s", dottedPath, part)
		}
		current = m
	}

	current[parts[len(parts)-1]] = value

	rawBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	return &runtime.RawExtension{Raw: rawBytes}, nil
}

// matchesRepository checks if component's repository matches the webhook repository
func (s *WebhookService) matchesRepository(componentRepoURL, webhookRepoURL string) bool {
	// Normalize both URLs for comparison
	componentRepoURL = normalizeRepoURL(componentRepoURL)
	webhookRepoURL = normalizeRepoURL(webhookRepoURL)

	return componentRepoURL == webhookRepoURL
}

// isComponentAffected checks if any modified path affects the component
func (s *WebhookService) isComponentAffected(appPath string, modifiedPaths []string) bool {
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

// normalizeRepoURL normalizes repository URLs for comparison
func normalizeRepoURL(repoURL string) string {
	// Convert SSH to HTTPS for different providers
	if strings.HasPrefix(repoURL, "git@github.com:") {
		repoURL = strings.Replace(repoURL, "git@github.com:", "https://github.com/", 1)
	}
	if strings.HasPrefix(repoURL, "git@gitlab.com:") {
		repoURL = strings.Replace(repoURL, "git@gitlab.com:", "https://gitlab.com/", 1)
	}
	if strings.HasPrefix(repoURL, "git@bitbucket.org:") {
		repoURL = strings.Replace(repoURL, "git@bitbucket.org:", "https://bitbucket.org/", 1)
	}

	// Remove .git suffix
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// Remove trailing slash
	repoURL = strings.TrimSuffix(repoURL, "/")

	// Convert to lowercase for case-insensitive comparison
	repoURL = strings.ToLower(repoURL)

	return repoURL
}
