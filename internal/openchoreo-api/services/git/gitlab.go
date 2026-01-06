// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GitLabProvider implements the Provider interface for GitLab
type GitLabProvider struct {
}

// NewGitLabProvider creates a new GitLab provider
func NewGitLabProvider() *GitLabProvider {
	return &GitLabProvider{}
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
