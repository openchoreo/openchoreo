// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"fmt"
)

// Provider defines the interface for git provider operations
type Provider interface {
	// ValidateWebhookPayload validates the webhook payload signature
	ValidateWebhookPayload(payload []byte, signature, secret string) error

	// ParseWebhookPayload parses the webhook payload and extracts relevant information
	ParseWebhookPayload(payload []byte) (*WebhookEvent, error)
}

// WebhookEvent represents a normalized webhook event
type WebhookEvent struct {
	Provider      string
	RepositoryURL string
	Ref           string
	Commit        string
	Branch        string
	ModifiedPaths []string
}

// ProviderType represents the git provider type
type ProviderType string

const (
	ProviderGitHub    ProviderType = "github"
	ProviderGitLab    ProviderType = "gitlab"
	ProviderBitbucket ProviderType = "bitbucket"
)

// GetProvider returns a git provider instance based on the type
func GetProvider(providerType ProviderType) (Provider, error) {
	switch providerType {
	case ProviderGitHub:
		return NewGitHubProvider(), nil
	case ProviderGitLab:
		return NewGitLabProvider(), nil
	case ProviderBitbucket:
		return NewBitbucketProvider(), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerType)
	}
}
