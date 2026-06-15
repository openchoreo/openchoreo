// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// BitbucketProvider implements the Provider interface for Bitbucket
type BitbucketProvider struct {
	apiBaseURL string
	httpClient *http.Client
}

// NewBitbucketProvider creates a new Bitbucket provider
func NewBitbucketProvider() *BitbucketProvider {
	return &BitbucketProvider{
		apiBaseURL: bitbucketAPIBaseURL,
		httpClient: newDefaultHTTPClient(),
	}
}

// GetBranchHead returns the head commit SHA of the given branch via the Bitbucket API.
func (p *BitbucketProvider) GetBranchHead(ctx context.Context, repoURL, branch string) (string, error) {
	_, segments, err := parseRepoPath(repoURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse Bitbucket repository URL: %w", err)
	}
	if len(segments) < 2 {
		return "", fmt.Errorf("repository URL %q does not contain workspace and repository name", SanitizeRepoURL(repoURL))
	}

	requestURL := fmt.Sprintf("%s/2.0/repositories/%s/%s/refs/branches/%s",
		p.apiBaseURL, segments[0], segments[1], url.PathEscape(branch))

	var result struct {
		Target struct {
			Hash string `json:"hash"`
		} `json:"target"`
	}
	if err := getJSON(ctx, p.httpClient, requestURL, &result); err != nil {
		return "", fmt.Errorf("failed to get branch %q of %s from Bitbucket: %w", branch, SanitizeRepoURL(repoURL), err)
	}
	if result.Target.Hash == "" {
		return "", fmt.Errorf("bitbucket response for branch %q of %s has no commit SHA", branch, SanitizeRepoURL(repoURL))
	}
	return result.Target.Hash, nil
}

// ValidateWebhookPayload validates the Bitbucket webhook.
// When token is empty (Bitbucket does not send a signature header), validation is skipped.
// TODO: implement HMAC-based signature validation once Bitbucket webhook signing is supported.
func (p *BitbucketProvider) ValidateWebhookPayload(payload []byte, token, secret string) error {
	// Bitbucket does not include a signature header in webhook requests, so the
	// handler always forwards an empty token. Skip validation in that case.
	if token == "" {
		return nil
	}
	// Token-based validation: if a token was somehow provided, it must match the secret.
	if token != secret {
		return fmt.Errorf("invalid webhook token")
	}
	return nil
}

// ParseWebhookPayload parses Bitbucket webhook payload
func (p *BitbucketProvider) ParseWebhookPayload(payload []byte) (*WebhookEvent, error) {
	var bbPayload struct {
		Push struct {
			Changes []struct {
				New struct {
					Name string `json:"name"` // branch name
					Type string `json:"type"` // "branch"
				} `json:"new"`
				Commits []struct {
					Hash string `json:"hash"`
				} `json:"commits"`
			} `json:"changes"`
		} `json:"push"`
		Repository struct {
			Links struct {
				HTML struct {
					Href string `json:"href"`
				} `json:"html"`
			} `json:"links"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(payload, &bbPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Bitbucket payload: %w", err)
	}

	if len(bbPayload.Push.Changes) == 0 {
		return nil, fmt.Errorf("no changes in Bitbucket push event")
	}

	change := bbPayload.Push.Changes[0]
	branch := change.New.Name

	var commit string
	if len(change.Commits) > 0 {
		commit = change.Commits[len(change.Commits)-1].Hash
	}

	// NOTE: Bitbucket doesn't include modified file paths in push webhooks
	// We'll need to trigger all components for the repository
	// or implement a separate API call to fetch commit details
	modifiedPaths := []string{} // Empty means all components will be triggered

	return &WebhookEvent{
		Provider:      string(ProviderBitbucket),
		RepositoryURL: normalizeRepoURL(bbPayload.Repository.Links.HTML.Href),
		Ref:           "refs/heads/" + branch,
		Commit:        commit,
		Branch:        branch,
		ModifiedPaths: modifiedPaths, // Empty - will trigger all components
	}, nil
}
