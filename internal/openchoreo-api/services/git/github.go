// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider() *GitHubProvider {
	return &GitHubProvider{}
}

// ValidateWebhookPayload validates the GitHub webhook signature
func (p *GitHubProvider) ValidateWebhookPayload(payload []byte, signature, secret string) error {
	if signature == "" {
		return fmt.Errorf("missing signature header")
	}

	// GitHub sends signature as "sha256=<hash>"
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format")
	}

	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedMAC)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// ParseWebhookPayload parses GitHub webhook payload
func (p *GitHubProvider) ParseWebhookPayload(payload []byte) (*WebhookEvent, error) {
	var ghPayload struct {
		Ref        string `json:"ref"`
		After      string `json:"after"`
		Repository struct {
			CloneURL string `json:"clone_url"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
		Commits []struct {
			Added    []string `json:"added"`
			Modified []string `json:"modified"`
			Removed  []string `json:"removed"`
		} `json:"commits"`
	}

	if err := json.Unmarshal(payload, &ghPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GitHub payload: %w", err)
	}

	// Extract branch from ref (refs/heads/main -> main)
	branch := strings.TrimPrefix(ghPayload.Ref, "refs/heads/")

	// Collect all modified paths
	modifiedPaths := make([]string, 0)
	for _, commit := range ghPayload.Commits {
		modifiedPaths = append(modifiedPaths, commit.Added...)
		modifiedPaths = append(modifiedPaths, commit.Modified...)
		modifiedPaths = append(modifiedPaths, commit.Removed...)
	}

	return &WebhookEvent{
		Provider:      string(ProviderGitHub),
		RepositoryURL: normalizeRepoURL(ghPayload.Repository.CloneURL),
		Ref:           ghPayload.Ref,
		Commit:        ghPayload.After,
		Branch:        branch,
		ModifiedPaths: modifiedPaths,
	}, nil
}

// normalizeRepoURL normalizes repository URLs for comparison
func normalizeRepoURL(repoURL string) string {
	// Convert SSH to HTTPS
	if strings.HasPrefix(repoURL, "git@") {
		repoURL = strings.Replace(repoURL, ":", "/", 1)
		repoURL = strings.Replace(repoURL, "git@", "https://", 1)
	}

	// Remove .git suffix
	repoURL = strings.TrimSuffix(repoURL, ".git")

	// Convert to lowercase for case-insensitive comparison
	repoURL = strings.ToLower(repoURL)

	return repoURL
}
