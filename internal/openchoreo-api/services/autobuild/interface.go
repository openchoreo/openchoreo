// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package autobuild

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
)

// ProcessWebhookParams holds all parameters needed to process a webhook event.
type ProcessWebhookParams struct {
	ProviderType    git.ProviderType
	SignatureHeader string
	Signature       string
	SecretKey       string
	Payload         []byte
}

// WebhookResult holds the result of a processed webhook event.
type WebhookResult struct {
	AffectedComponents []string
}

// Service defines the autobuild operations.
type Service interface {
	// ProcessWebhook validates and processes an incoming webhook event from a git provider.
	ProcessWebhook(ctx context.Context, params *ProcessWebhookParams) (*WebhookResult, error)
}
