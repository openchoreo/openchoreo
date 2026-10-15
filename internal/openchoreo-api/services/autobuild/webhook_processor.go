// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package autobuild

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
	"github.com/openchoreo/openchoreo/internal/releaseline"
)

// WorkflowRunTrigger is implemented by a workflow run service to trigger a workflow
// run for a component without user-level authorization (webhook requests are
// authenticated via HMAC signature validation instead).
type WorkflowRunTrigger interface {
	TriggerWorkflow(ctx context.Context, namespaceName, projectName, componentName, commit, releaseLine string) (*models.WorkflowRunTriggerResponse, error)
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

	// Find the components and release lines to build
	targets, err := s.findBuildTargets(ctx, event)
	if err != nil {
		s.logger.Error("Failed to find affected components", "error", err)
		return nil, fmt.Errorf("failed to find affected components: %w", err)
	}

	s.logger.Info("Found build targets", "count", len(targets))

	triggered := make([]string, 0, len(targets))
	for _, target := range targets {
		comp := target.component
		namespaceName := comp.Namespace
		projectName := comp.Spec.Owner.ProjectName
		componentName := comp.Name

		s.logger.Info("Triggering build for component",
			"namespace", namespaceName,
			"project", projectName,
			"component", componentName,
			"releaseLine", target.releaseLine,
			"commit", event.Commit)

		_, err := s.workflowTrigger.TriggerWorkflow(
			ctx,
			namespaceName,
			projectName,
			componentName,
			event.Commit,
			target.releaseLine,
		)
		if err != nil {
			// Log error but continue processing other targets
			s.logger.Error("Failed to trigger build for component",
				"error", err,
				"component", componentName,
				"releaseLine", target.releaseLine)
			continue
		}

		triggered = append(triggered, target.String())
	}

	s.logger.Info("Webhook processing completed",
		"buildTargets", len(targets),
		"triggeredBuilds", len(triggered))

	return triggered, nil
}

// buildTarget is a component, or one release line of it, that a webhook event builds.
type buildTarget struct {
	component   *v1alpha1.Component
	releaseLine string
}

// String returns "namespace/component", with "@releaseLine" appended for a release line.
func (t buildTarget) String() string {
	name := t.component.Namespace + "/" + t.component.Name
	if t.releaseLine != "" {
		name += "@" + t.releaseLine
	}
	return name
}

// findBuildTargets finds the components, and the release lines of components with release
// lines enabled, that should be built for the webhook event.
func (s *webhookProcessor) findBuildTargets(ctx context.Context, event *git.WebhookEvent) ([]buildTarget, error) {
	componentList := &v1alpha1.ComponentList{}
	if err := s.k8sClient.List(ctx, componentList); err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	targets := make([]buildTarget, 0)
	for i := range componentList.Items {
		comp := &componentList.Items[i]

		// Check if auto-build is enabled
		if comp.Spec.AutoBuild == nil || !*comp.Spec.AutoBuild {
			continue
		}
		if comp.Spec.Workflow == nil || comp.Spec.Workflow.Name == "" {
			s.logger.Info("Skipping component: component has no workflow configuration", "component", comp.Name)
			continue
		}

		if !comp.Spec.ReleaseLinesEnabled {
			if s.isAffected(ctx, comp, "", comp.Spec.Workflow.Parameters, event) {
				targets = append(targets, buildTarget{component: comp})
			}
			continue
		}

		lines, err := s.listReleaseLines(ctx, comp)
		if err != nil {
			s.logger.Info("Skipping component: failed to list release lines", "component", comp.Name, "error", err)
			continue
		}
		for j := range lines {
			line := &lines[j]
			parameters := comp.Spec.Workflow.Parameters
			if line.Spec.Workflow != nil {
				if parameters, err = releaseline.MergeParameters(parameters, line.Spec.Workflow.Parameters); err != nil {
					s.logger.Info("Skipping release line: failed to apply its workflow parameters",
						"component", comp.Name, "releaseLine", line.Name, "error", err)
					continue
				}
			}
			if s.isAffected(ctx, comp, line.Name, parameters, event) {
				targets = append(targets, buildTarget{component: comp, releaseLine: line.Name})
			}
		}
	}

	return targets, nil
}

// listReleaseLines returns the release lines owned by the component.
func (s *webhookProcessor) listReleaseLines(ctx context.Context, comp *v1alpha1.Component) ([]v1alpha1.ReleaseLine, error) {
	list := &v1alpha1.ReleaseLineList{}
	if err := s.k8sClient.List(ctx, list, client.InNamespace(comp.Namespace)); err != nil {
		return nil, err
	}
	lines := make([]v1alpha1.ReleaseLine, 0, len(list.Items))
	for _, line := range list.Items {
		if line.Spec.Owner.ProjectName == comp.Spec.Owner.ProjectName && line.Spec.Owner.ComponentName == comp.Name {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// isAffected reports whether the webhook event should build the component, or the given release
// line of it, with the given workflow parameters.
func (s *webhookProcessor) isAffected(
	ctx context.Context,
	comp *v1alpha1.Component,
	releaseLine string,
	parameters *runtime.RawExtension,
	event *git.WebhookEvent,
) bool {
	log := s.logger.With("component", comp.Name)
	if releaseLine != "" {
		log = log.With("releaseLine", releaseLine)
	}

	repo, err := s.extractRepoInfo(ctx, comp, parameters)
	if err != nil {
		log.Info("Skipping component: failed to extract repo info", "error", err)
		return false
	}

	// Check if component's repository matches the webhook repository
	if !s.matchesRepository(repo.url, event.RepositoryURL) {
		log.Info("Skipping component: repository mismatch",
			"componentRepo", repo.url,
			"webhookRepo", event.RepositoryURL)
		return false
	}

	// Ensure the authenticated webhook provider matches the provider that hosts the
	// component's repository. The webhook was validated against a specific provider's
	// secret; without this check a webhook validated for one provider could trigger
	// builds for components hosted on a different provider that share a repository URL.
	// For hosts that don't map to a known SaaS provider (e.g. self-hosted), the provider
	// cannot be inferred and no additional check is enforced.
	if expected := providerFromRepoURL(repo.url); expected != "" && string(expected) != event.Provider {
		log.Info("Skipping component: provider mismatch",
			"expectedProvider", expected,
			"webhookProvider", event.Provider)
		return false
	}

	// Check if the webhook branch matches the component's configured branch.
	// If the component has no branch configured, all branches trigger builds.
	if repo.branch != "" && repo.branch != event.Branch {
		log.Info("Skipping component: branch mismatch",
			"componentBranch", repo.branch,
			"webhookBranch", event.Branch)
		return false
	}

	// A release line pinned to a commit is not moved by pushes.
	if releaseLine != "" && repo.commit != "" {
		log.Info("Skipping release line: pinned to a commit", "commit", repo.commit)
		return false
	}

	// Check if modified paths affect this component
	// If no modified paths (e.g., Bitbucket), trigger all components for the repo
	if len(event.ModifiedPaths) > 0 && !s.isComponentAffected(repo.appPath, event.ModifiedPaths) {
		log.Info("Skipping component: no modified paths match app path",
			"appPath", repo.appPath,
			"modifiedPaths", event.ModifiedPaths)
		return false
	}

	log.Info("Component is affected by webhook event",
		"appPath", repo.appPath,
		"branch", repo.branch,
		"modifiedPaths", len(event.ModifiedPaths))
	return true
}

// repoInfo is the repository a component builds from, read from its workflow parameters.
type repoInfo struct {
	url     string
	appPath string
	branch  string
	commit  string
}

// extractRepoInfo extracts the repository URL, appPath, branch and commit from the given workflow parameters
// of a component by scanning the Workflow or ClusterWorkflow CR's openAPIV3Schema for
// x-openchoreo-component-repository extensions.
func (s *webhookProcessor) extractRepoInfo(ctx context.Context, comp *v1alpha1.Component, parameters *runtime.RawExtension) (repoInfo, error) {
	if comp.Spec.Workflow == nil || comp.Spec.Workflow.Name == "" {
		return repoInfo{}, fmt.Errorf("component has no workflow configuration")
	}

	// Fetch the Workflow or ClusterWorkflow CR to get the schema
	var parametersSchema *v1alpha1.SchemaSection
	if comp.Spec.Workflow.Kind == v1alpha1.WorkflowRefKindClusterWorkflow {
		cw := &v1alpha1.ClusterWorkflow{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name: comp.Spec.Workflow.Name,
		}, cw); err != nil {
			return repoInfo{}, fmt.Errorf("failed to get ClusterWorkflow %s: %w", comp.Spec.Workflow.Name, err)
		}
		parametersSchema = cw.Spec.Parameters
	} else {
		workflow := &v1alpha1.Workflow{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      comp.Spec.Workflow.Name,
			Namespace: comp.Namespace,
		}, workflow); err != nil {
			return repoInfo{}, fmt.Errorf("failed to get workflow %s: %w", comp.Spec.Workflow.Name, err)
		}
		parametersSchema = workflow.Spec.Parameters
	}

	// Extract parameter paths from x-openchoreo-component-parameter-repository-* schema extensions
	paramMap, err := controller.ExtractComponentRepositoryPaths(parametersSchema.GetRaw())
	if err != nil {
		return repoInfo{}, fmt.Errorf("failed to extract component repository paths from workflow %s schema: %w", comp.Spec.Workflow.Name, err)
	}

	// Get url path from schema extensions
	repoURLPath, ok := paramMap["url"]
	if !ok {
		return repoInfo{}, fmt.Errorf("workflow %s schema missing x-openchoreo-component-parameter-repository-url extension", comp.Spec.Workflow.Name)
	}

	var info repoInfo
	info.url, err = getNestedStringFromRawExtension(parameters, repoURLPath)
	if err != nil {
		return repoInfo{}, fmt.Errorf("failed to extract repoUrl from parameters: %w", err)
	}

	if info.url == "" {
		return repoInfo{}, fmt.Errorf("repository URL is empty in component parameters")
	}

	// Get appPath (optional - not all workflows may have it)
	if appPathPath, ok := paramMap["app-path"]; ok {
		info.appPath, _ = getNestedStringFromRawExtension(parameters, appPathPath)
	}

	// Get branch. If the component parameters don't carry a value (missing key or empty string),
	// fall back to the schema default (e.g. "main"). An empty result means all branches trigger builds.
	if branchPath, ok := paramMap["branch"]; ok {
		info.branch, _ = getNestedStringFromRawExtension(parameters, branchPath)
		if info.branch == "" {
			info.branch = getSchemaFieldDefault(parametersSchema.GetRaw(), branchPath)
		}
	}

	if commitPath, ok := paramMap["commit"]; ok {
		info.commit, _ = getNestedStringFromRawExtension(parameters, commitPath)
	}

	return info, nil
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

// providerFromRepoURL infers the git provider from a repository URL's host.
// It returns "" for hosts that don't map to a known SaaS provider (e.g. self-hosted
// installations), in which case callers should not enforce a provider-consistency check.
func providerFromRepoURL(repoURL string) git.ProviderType {
	// Inspect the URL host only. Matching against the full URL (e.g. via substring)
	// would misclassify a repository whose org/name path contains another provider's
	// domain, such as https://bitbucket.org/my-org/github.com-sync.
	u, err := url.Parse(normalizeWebhookRepoURL(repoURL))
	if err != nil {
		return ""
	}
	host := u.Hostname()
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return git.ProviderGitHub
	case host == "gitlab.com" || strings.HasSuffix(host, ".gitlab.com"):
		return git.ProviderGitLab
	case host == "bitbucket.org" || strings.HasSuffix(host, ".bitbucket.org"):
		return git.ProviderBitbucket
	default:
		return ""
	}
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

	// Normalize appPath, treating "." and "./" as the repository root
	appPath = strings.Trim(path.Clean("/"+appPath), "/")
	if appPath == "" {
		return true
	}

	// Check if any modified path is under the component's appPath
	for _, modified := range modifiedPaths {
		modified = strings.TrimPrefix(modified, "/")

		// Check if path is under appPath or is the appPath itself
		if modified == appPath || strings.HasPrefix(modified, appPath+"/") {
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
