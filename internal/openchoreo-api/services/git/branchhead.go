// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Default API base URLs for the hosted git providers.
const (
	githubAPIBaseURL    = "https://api.github.com"
	gitlabAPIBaseURL    = "https://gitlab.com"
	bitbucketAPIBaseURL = "https://api.bitbucket.org"
)

// branchHeadRequestTimeout bounds a single branch-head lookup against a git provider API.
const branchHeadRequestTimeout = 10 * time.Second

func newDefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: branchHeadRequestTimeout}
}

// DetectProviderTypeFromURL determines the git provider type from the repository URL host.
func DetectProviderTypeFromURL(repoURL string) (ProviderType, error) {
	host, _, err := parseRepoPath(repoURL)
	if err != nil {
		return "", err
	}
	switch strings.TrimPrefix(strings.ToLower(host), "www.") {
	case "github.com":
		return ProviderGitHub, nil
	case "gitlab.com":
		return ProviderGitLab, nil
	case "bitbucket.org":
		return ProviderBitbucket, nil
	default:
		return "", fmt.Errorf("cannot determine git provider from repository host %q", host)
	}
}

// ResolveBranchHead resolves the current head commit SHA of a branch by querying the
// API of the git provider determined from the repository URL.
func ResolveBranchHead(ctx context.Context, repoURL, branch string) (string, error) {
	providerType, err := DetectProviderTypeFromURL(repoURL)
	if err != nil {
		return "", err
	}
	provider, err := GetProvider(providerType)
	if err != nil {
		return "", err
	}
	return provider.GetBranchHead(ctx, repoURL, branch)
}

// SanitizeRepoURL strips any userinfo (credentials) from a repository URL so it is
// safe to include in logs and error messages.
func SanitizeRepoURL(repoURL string) string {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil || u.Host == "" {
		// Not a parseable absolute URL (e.g. SSH form); strip anything before '@' defensively.
		if i := strings.LastIndex(repoURL, "@"); i >= 0 {
			return repoURL[i+1:]
		}
		return repoURL
	}
	u.User = nil
	return u.String()
}

// parseRepoPath canonicalizes a repository URL (SSH form to HTTPS, trailing ".git"
// removed, case preserved) and splits it into its host and non-empty path segments.
func parseRepoPath(repoURL string) (string, []string, error) {
	canonical := strings.TrimSpace(repoURL)
	if strings.HasPrefix(canonical, "git@") {
		canonical = strings.Replace(canonical, ":", "/", 1)
		canonical = strings.Replace(canonical, "git@", "https://", 1)
	}
	canonical = strings.TrimSuffix(canonical, ".git")

	u, err := url.Parse(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("invalid repository URL %q: %w", repoURL, err)
	}
	if u.Host == "" {
		return "", nil, fmt.Errorf("repository URL %q has no host", repoURL)
	}

	segments := make([]string, 0, 2)
	for _, segment := range strings.Split(u.Path, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return u.Host, segments, nil
}

// getJSON performs a GET request against a git provider API and decodes the JSON response.
func getJSON(ctx context.Context, httpClient *http.Client, requestURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}
