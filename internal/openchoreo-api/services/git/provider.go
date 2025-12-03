// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"fmt"
)

// Provider defines the interface for git provider operations
type Provider interface {
	// RegisterWebhook creates a webhook in the git repository
	RegisterWebhook(ctx context.Context, repoURL, webhookURL, secret string) (webhookID string, err error)

	// DeregisterWebhook removes a webhook from the git repository
	DeregisterWebhook(ctx context.Context, repoURL, webhookID string) error

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
func GetProvider(providerType ProviderType, config ProviderConfig) (Provider, error) {
	switch providerType {
	case ProviderGitHub:
		return NewGitHubProvider(config), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerType)
	}
}

// ProviderConfig holds configuration for git providers
type ProviderConfig struct {
	// Token for authenticating with the git provider
	Token string
	// BaseURL for self-hosted instances (optional)
	BaseURL string
}
