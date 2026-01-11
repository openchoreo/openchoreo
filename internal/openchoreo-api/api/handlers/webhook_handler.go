// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// HandleGitHubWebhook processes incoming GitHub webhook events
func (h *Handler) HandleGitHubWebhook(
	ctx context.Context,
	request gen.HandleGitHubWebhookRequestObject,
) (gen.HandleGitHubWebhookResponseObject, error) {
	return nil, errNotImplemented
}

// HandleGitLabWebhook processes incoming GitLab webhook events
func (h *Handler) HandleGitLabWebhook(
	ctx context.Context,
	request gen.HandleGitLabWebhookRequestObject,
) (gen.HandleGitLabWebhookResponseObject, error) {
	return nil, errNotImplemented
}

// HandleBitbucketWebhook processes incoming Bitbucket webhook events
func (h *Handler) HandleBitbucketWebhook(
	ctx context.Context,
	request gen.HandleBitbucketWebhookRequestObject,
) (gen.HandleBitbucketWebhookResponseObject, error) {
	return nil, errNotImplemented
}
