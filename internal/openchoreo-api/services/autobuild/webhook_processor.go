// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package autobuild

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
)

// WorkflowRunTrigger is implemented by a workflow run service to trigger a workflow
// run for a component without user-level authorization (webhook requests are
// authenticated via HMAC signature validation instead).
type WorkflowRunTrigger interface {
	TriggerWorkflow(ctx context.Context, namespaceName, projectName, componentName, commit string) (*models.WorkflowRunTriggerResponse, error)
}

// webhookProcessor handles webhook processing for all git providers.
// It finds affected components and triggers workflow runs for them.
// sshGitURLRegex matches SSH-style git URLs: git@host:org/repo or git@host:org/repo.git
var sshGitURLRegex = regexp.MustCompile(`^git@([^:]+):(.+)$`)

type webhookProcessor struct {
	k8sClient       client.Client
	workflowTrigger WorkflowRunTrigger
	logger          *slog.Logger
}

// NewWebhookProcessor creates a new webhookProcessor that implements WebhookProcessor.
func NewWebhookProcessor(k8sClient client.Client, workflowTrigger WorkflowRunTrigger, logger *slog.Logger) WebhookProcessor {
	return &webhookProcessor{
		k8sClient:       k8sClient,
		workflowTrigger: workflowTrigger,
		logger:          logger,
	}
}

// ProcessWebhook processes an incoming webhook payload from any git provider.
func (s *webhookProcessor) ProcessWebhook(ctx context.Context, provider git.Provider, payload []byte) ([]string, error) {
	// Parse payload using the provider
	event, err := provider.ParseWebhookPayload(payload)
	if err != nil {
		s.logger.Error("Failed to parse webhook payload", "error", err)
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	s.logger.Info("Processing webhook event",
		"provider", event.Provider,
		"repository", event.RepositoryURL,
		"branch", event.Branch,
		"commit", event.Commit,
		"modifiedPaths", len(event.ModifiedPaths))

	// Find affected components
	affectedComponents, err := s.findAffectedComponents(ctx, event)
	if err != nil {
		s.logger.Error("Failed to find affected components", "error", err)
		return nil, fmt.Errorf("failed to find affected components: %w", err)
	}

	s.logger.Info("Found affected components", "count", len(affectedComponents))

	// Trigger builds for affected components
	triggeredComponents := make([]string, 0)
	for _, comp := range affectedComponents {
		namespaceName := comp.Namespace
		projectName := comp.Spec.Owner.ProjectName
		componentName := comp.Name

		s.logger.Info("Triggering build for component",
			"namespace", namespaceName,
			"project", projectName,
			"component", componentName,
			"commit", event.Commit)

		_, err := s.workflowTrigger.TriggerWorkflow(
			ctx,
			namespaceName,
			projectName,
			componentName,
			event.Commit,
		)
		if err != nil {
			// Log error but continue processing other components
			s.logger.Error("Failed to trigger build for component",
				"error", err,
				"component", componentName)
			continue
		}

		triggeredComponents = append(triggeredComponents, fmt.Sprintf("%s/%s", comp.Namespace, componentName))
	}

	s.logger.Info("Webhook processing completed",
		"affectedComponents", len(affectedComponents),
		"triggeredBuilds", len(triggeredComponents))

	return triggeredComponents, nil
}

// findAffectedComponents finds all components that should be built based on the webhook event.
func (s *webhookProcessor) findAffectedComponents(ctx context.Context, event *git.WebhookEvent) ([]*v1alpha1.Component, error) {
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

		// Get repository URL, appPath, and branch from component workflow schema extensions
		repoURL, appPath, branch, err := s.extractRepoInfoFromComponent(ctx, comp)
		if err != nil {
			s.logger.Info("Skipping component: failed to extract repo info",
				"component", comp.Name,
				"error", err)
			continue
		}

		// Check if component's repository matches the webhook repository
		if !s.matchesRepository(repoURL, event.RepositoryURL) {
			s.logger.Info("Skipping component: repository mismatch",
				"component", comp.Name,
				"componentRepo", repoURL,
				"webhookRepo", event.RepositoryURL)
			continue
		}

		// Check if the webhook branch matches the component's configured branch.
		// If the component has no branch configured, all branches trigger builds.
		if branch != "" && branch != event.Branch {
			s.logger.Info("Skipping component: branch mismatch",
				"component", comp.Name,
				"componentBranch", branch,
				"webhookBranch", event.Branch)
			continue
		}

		// Check if modified paths affect this component
		// If no modified paths (e.g., Bitbucket), trigger all components for the repo
		if len(event.ModifiedPaths) == 0 || s.isComponentAffected(appPath, event.ModifiedPaths) {
			s.logger.Info("Component is affected by webhook event",
				"component", comp.Name,
				"appPath", appPath,
				"branch", branch,
				"modifiedPaths", len(event.ModifiedPaths))
			affected = append(affected, comp)
		} else {
			s.logger.Info("Skipping component: no modified paths match app path",
				"component", comp.Name,
				"appPath", appPath,
				"modifiedPaths", event.ModifiedPaths)
		}
	}

	return affected, nil
}

// extractRepoInfoFromComponent extracts repository URL, appPath, and branch from a component's workflow parameters
// by scanning the Workflow or ClusterWorkflow CR's openAPIV3Schema for x-openchoreo-component-repository extensions.
func (s *webhookProcessor) extractRepoInfoFromComponent(ctx context.Context, comp *v1alpha1.Component) (repoURL string, appPath string, branch string, err error) {
	if comp.Spec.Workflow == nil || comp.Spec.Workflow.Name == "" {
		return "", "", "", fmt.Errorf("component has no workflow configuration")
	}

	// Fetch the Workflow or ClusterWorkflow CR to get the schema
	var parametersSchema *v1alpha1.SchemaSection
	if comp.Spec.Workflow.Kind == v1alpha1.WorkflowRefKindClusterWorkflow {
		cw := &v1alpha1.ClusterWorkflow{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name: comp.Spec.Workflow.Name,
		}, cw); err != nil {
			return "", "", "", fmt.Errorf("failed to get ClusterWorkflow %s: %w", comp.Spec.Workflow.Name, err)
		}
		parametersSchema = cw.Spec.Parameters
	} else {
		workflow := &v1alpha1.Workflow{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      comp.Spec.Workflow.Name,
			Namespace: comp.Namespace,
		}, workflow); err != nil {
			return "", "", "", fmt.Errorf("failed to get workflow %s: %w", comp.Spec.Workflow.Name, err)
		}
		parametersSchema = workflow.Spec.Parameters
	}

	// Extract parameter paths from x-openchoreo-component-parameter-repository-* schema extensions
	paramMap, err := controller.ExtractComponentRepositoryPaths(parametersSchema.GetRaw())
	if err != nil {
		return "", "", "", fmt.Errorf("failed to extract component repository paths from workflow %s schema: %w", comp.Spec.Workflow.Name, err)
	}

	// Get url path from schema extensions
	repoURLPath, ok := paramMap["url"]
	if !ok {
		return "", "", "", fmt.Errorf("workflow %s schema missing x-openchoreo-component-parameter-repository-url extension", comp.Spec.Workflow.Name)
	}

	repoURL, err = getNestedStringFromRawExtension(comp.Spec.Workflow.Parameters, repoURLPath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to extract repoUrl from parameters: %w", err)
	}

	if repoURL == "" {
		return "", "", "", fmt.Errorf("repository URL is empty in component parameters")
	}

	// Get appPath (optional - not all workflows may have it)
	if appPathPath, ok := paramMap["app-path"]; ok {
		appPath, _ = getNestedStringFromRawExtension(comp.Spec.Workflow.Parameters, appPathPath)
	}

	// Get branch. If the component parameters don't carry a value (missing key or empty string),
	// fall back to the schema default (e.g. "main"). An empty result means all branches trigger builds.
	if branchPath, ok := paramMap["branch"]; ok {
		branch, _ = getNestedStringFromRawExtension(comp.Spec.Workflow.Parameters, branchPath)
		if branch == "" {
			branch = getSchemaFieldDefault(parametersSchema.GetRaw(), branchPath)
		}
	}

	return repoURL, appPath, branch, nil
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

// getSchemaFieldDefault navigates an openAPIV3Schema RawExtension following "properties" at each
// segment of dottedPath (stripping a leading "parameters." prefix) and returns the "default"
// string value of the terminal field, or "" if not present or not a string.
func getSchemaFieldDefault(schema *runtime.RawExtension, dottedPath string) string {
	if schema == nil || schema.Raw == nil {
		return ""
	}
	path := strings.TrimPrefix(dottedPath, "parameters.")
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(schema.Raw, &schemaObj); err != nil {
		return ""
	}
	current := schemaObj
	for _, part := range strings.Split(path, ".") {
		props, ok := current["properties"].(map[string]interface{})
		if !ok {
			return ""
		}
		child, ok := props[part].(map[string]interface{})
		if !ok {
			return ""
		}
		current = child
	}
	def, _ := current["default"].(string)
	return def
}

// matchesRepository checks if component's repository matches the webhook repository.
func (s *webhookProcessor) matchesRepository(componentRepoURL, webhookRepoURL string) bool {
	// Normalize both URLs for comparison
	componentRepoURL = normalizeWebhookRepoURL(componentRepoURL)
	webhookRepoURL = normalizeWebhookRepoURL(webhookRepoURL)

	return componentRepoURL == webhookRepoURL
}

// isComponentAffected checks if any modified path affects the component.
func (s *webhookProcessor) isComponentAffected(appPath string, modifiedPaths []string) bool {
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

// normalizeWebhookRepoURL normalizes repository URLs for comparison.
// It converts any SSH-style git URL (including self-hosted hosts) to HTTPS form,
// strips the .git suffix, trailing slash, and lowercases the result.
func normalizeWebhookRepoURL(repoURL string) string {
	// Strip ssh:// or git+ssh:// scheme prefixes before applying SSH rewrite
	repoURL = strings.TrimPrefix(repoURL, "git+ssh://")
	repoURL = strings.TrimPrefix(repoURL, "ssh://")

	// Convert git@host:path → https://host/path (handles any host, including self-hosted)
	if m := sshGitURLRegex.FindStringSubmatch(repoURL); m != nil {
		repoURL = "https://" + m[1] + "/" + m[2]
	}

	// Remove .git suffix
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// Remove trailing slash
	repoURL = strings.TrimSuffix(repoURL, "/")

	// Convert to lowercase for case-insensitive comparison
	repoURL = strings.ToLower(repoURL)

	return repoURL
}
