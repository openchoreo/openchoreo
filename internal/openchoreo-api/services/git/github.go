// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
	client *github.Client
	config ProviderConfig
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider(config ProviderConfig) *GitHubProvider {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.Token})
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)

	// Support for GitHub Enterprise
	if config.BaseURL != "" {
		var err error
		client, err = client.WithEnterpriseURLs(config.BaseURL, config.BaseURL)
		if err != nil {
			// Log error but continue with default client
			fmt.Printf("Failed to configure GitHub Enterprise URL: %v\n", err)
		}
	}

	return &GitHubProvider{
		client: client,
		config: config,
	}
}

// RegisterWebhook creates a webhook in the GitHub repository
func (p *GitHubProvider) RegisterWebhook(ctx context.Context, repoURL, webhookURL, secret string) (string, error) {
	owner, repo, err := parseGitHubRepoURL(repoURL)
	if err != nil {
		return "", err
	}

	hook := &github.Hook{
		Config: map[string]interface{}{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
			"insecure_ssl": "0",
		},
		Events: []string{"push"},
		Active: github.Bool(true),
	}

	createdHook, _, err := p.client.Repositories.CreateHook(ctx, owner, repo, hook)
	if err != nil {
		return "", fmt.Errorf("failed to create webhook: %w", err)
	}

	return fmt.Sprintf("%d", createdHook.GetID()), nil
}

// DeregisterWebhook removes a webhook from the GitHub repository
func (p *GitHubProvider) DeregisterWebhook(ctx context.Context, repoURL, webhookID string) error {
	owner, repo, err := parseGitHubRepoURL(repoURL)
	if err != nil {
		return err
	}

	id, err := strconv.ParseInt(webhookID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid webhook ID: %w", err)
	}

	_, err = p.client.Repositories.DeleteHook(ctx, owner, repo, id)
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	return nil
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

// parseGitHubRepoURL extracts owner and repo from GitHub URL
func parseGitHubRepoURL(repoURL string) (owner, repo string, err error) {
	// Parse URLs like:
	// - https://github.com/owner/repo
	// - https://github.com/owner/repo.git
	// - git@github.com:owner/repo.git

	// Normalize SSH URLs to HTTPS
	if strings.HasPrefix(repoURL, "git@") {
		// git@github.com:owner/repo.git -> https://github.com/owner/repo.git
		repoURL = strings.Replace(repoURL, ":", "/", 1)
		repoURL = strings.Replace(repoURL, "git@", "https://", 1)
	}

	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid repository URL: %w", err)
	}

	// Remove leading slash and .git suffix
	path := strings.TrimPrefix(parsedURL.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	// Split into owner/repo
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repository URL format: expected owner/repo")
	}

	return parts[0], parts[1], nil
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