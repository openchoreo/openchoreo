// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
	apiBaseURL string
	httpClient *http.Client
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider() *GitHubProvider {
	return &GitHubProvider{
		apiBaseURL: githubAPIBaseURL,
		httpClient: newDefaultHTTPClient(),
	}
}

// GetBranchHead returns the head commit SHA of the given branch via the GitHub API.
func (p *GitHubProvider) GetBranchHead(ctx context.Context, repoURL, branch string) (string, error) {
	_, segments, err := parseRepoPath(repoURL)
	if err != nil {
		return "", err
	}
	if len(segments) < 2 {
		return "", fmt.Errorf("repository URL %q does not contain owner and repository name", SanitizeRepoURL(repoURL))
	}

	requestURL := fmt.Sprintf("%s/repos/%s/%s/branches/%s",
		p.apiBaseURL, segments[0], segments[1], url.PathEscape(branch))

	var result struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := getJSON(ctx, p.httpClient, requestURL, &result); err != nil {
		return "", fmt.Errorf("failed to get branch %q of %s from GitHub: %w", branch, SanitizeRepoURL(repoURL), err)
	}
	if result.Commit.SHA == "" {
		return "", fmt.Errorf("GitHub response for branch %q of %s has no commit SHA", branch, SanitizeRepoURL(repoURL))
	}
	return result.Commit.SHA, nil
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
