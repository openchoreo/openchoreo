// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// HandleWebhook processes incoming webhook events from any supported git provider.
// The provider is detected from the request headers (X-Hub-Signature-256, X-Gitlab-Token, X-Event-Key).
func (h *Handler) HandleWebhook(
	ctx context.Context,
	request gen.HandleWebhookRequestObject,
) (gen.HandleWebhookResponseObject, error) {
	return nil, errNotImplemented
}
