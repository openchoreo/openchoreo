// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GitLabProvider implements the Provider interface for GitLab
type GitLabProvider struct {
	apiBaseURL string
	httpClient *http.Client
}

// NewGitLabProvider creates a new GitLab provider
func NewGitLabProvider() *GitLabProvider {
	return &GitLabProvider{
		apiBaseURL: gitlabAPIBaseURL,
		httpClient: newDefaultHTTPClient(),
	}
}

// GetBranchHead returns the head commit SHA of the given branch via the GitLab API.
func (p *GitLabProvider) GetBranchHead(ctx context.Context, repoURL, branch string) (string, error) {
	_, segments, err := parseRepoPath(repoURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse GitLab repository URL: %w", err)
	}
	if len(segments) < 2 {
		return "", fmt.Errorf("repository URL %q does not contain a project path", SanitizeRepoURL(repoURL))
	}

	// GitLab identifies projects by their full URL-encoded path (supports nested groups).
	projectPath := url.PathEscape(strings.Join(segments, "/"))
	requestURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches/%s",
		p.apiBaseURL, projectPath, url.PathEscape(branch))

	var result struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := getJSON(ctx, p.httpClient, requestURL, &result); err != nil {
		return "", fmt.Errorf("failed to get branch %q of %s from GitLab: %w", branch, SanitizeRepoURL(repoURL), err)
	}
	if result.Commit.ID == "" {
		return "", fmt.Errorf("GitLab response for branch %q of %s has no commit SHA", branch, SanitizeRepoURL(repoURL))
	}
	return result.Commit.ID, nil
}

// ValidateWebhookPayload validates the GitLab webhook token
func (p *GitLabProvider) ValidateWebhookPayload(payload []byte, token, secret string) error {
	if token == "" {
		return fmt.Errorf("missing X-Gitlab-Token header")
	}

	if token != secret {
		return fmt.Errorf("invalid webhook token")
	}

	return nil
}

// ParseWebhookPayload parses GitLab webhook payload
func (p *GitLabProvider) ParseWebhookPayload(payload []byte) (*WebhookEvent, error) {
	var glPayload struct {
		Ref     string `json:"ref"`
		After   string `json:"after"`
		Project struct {
			GitHTTPURL string `json:"git_http_url"`
			WebURL     string `json:"web_url"`
		} `json:"project"`
		Commits []struct {
			Added    []string `json:"added"`
			Modified []string `json:"modified"`
			Removed  []string `json:"removed"`
		} `json:"commits"`
	}

	if err := json.Unmarshal(payload, &glPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GitLab payload: %w", err)
	}

	// Extract branch from ref (refs/heads/main -> main)
	branch := strings.TrimPrefix(glPayload.Ref, "refs/heads/")

	// Collect all modified paths
	modifiedPaths := make([]string, 0)
	for _, commit := range glPayload.Commits {
		modifiedPaths = append(modifiedPaths, commit.Added...)
		modifiedPaths = append(modifiedPaths, commit.Modified...)
		modifiedPaths = append(modifiedPaths, commit.Removed...)
	}

	return &WebhookEvent{
		Provider:      string(ProviderGitLab),
		RepositoryURL: normalizeRepoURL(glPayload.Project.GitHTTPURL),
		Ref:           glPayload.Ref,
		Commit:        glPayload.After,
		Branch:        branch,
		ModifiedPaths: modifiedPaths,
	}, nil
}
