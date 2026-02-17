// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"encoding/json"
	"fmt"
)

// BitbucketProvider implements the Provider interface for Bitbucket
type BitbucketProvider struct {
}

// NewBitbucketProvider creates a new Bitbucket provider
func NewBitbucketProvider() *BitbucketProvider {
	return &BitbucketProvider{}
}

// ValidateWebhookPayload validates the Bitbucket webhook
// Note: Bitbucket signature validation can be implemented later if needed
// For MVP, we use a simple token-based validation
func (p *BitbucketProvider) ValidateWebhookPayload(payload []byte, token, secret string) error {
	// For MVP: Simple token validation
	// Bitbucket doesn't use HMAC signatures like GitHub
	// Can configure a secret in Bitbucket webhook settings
	if secret != "" && token != secret {
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
