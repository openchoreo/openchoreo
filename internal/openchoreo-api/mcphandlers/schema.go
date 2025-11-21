// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
)

// ExplainSchema explains the schema of a Kubernetes resource kind.
// It accepts a kind (e.g., "Component") and an optional path (e.g., "spec" or "spec.build")
// to drill down into nested fields.
func (h *MCPHandler) ExplainSchema(ctx context.Context, kind, path string) (any, error) {
	return h.Services.SchemaService.ExplainSchema(ctx, kind, path)
}
