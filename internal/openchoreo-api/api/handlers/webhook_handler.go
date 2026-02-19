// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// HandleWebhook processes incoming webhook events from any supported git provider.
// The provider is detected from the request headers (X-Hub-Signature-256, X-Gitlab-Token, X-Event-Key).
//
// NOTE: Webhook handling is currently served by the legacy HTTP handler at
// internal/openchoreo-api/handlers/webhook_handler.go, which is registered
// directly on the legacy mux. This stub exists to satisfy the StrictServerInterface
// contract during the incremental migration to the OpenAPI-generated server.
// It will be fully implemented once the migration is complete.
func (h *Handler) HandleWebhook(
	ctx context.Context,
	request gen.HandleWebhookRequestObject,
) (gen.HandleWebhookResponseObject, error) {
	return nil, errNotImplemented
}
