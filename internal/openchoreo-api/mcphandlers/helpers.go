// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// setAuditResource records the persisted object's identity — namespace, UID and
// name — on the audit event for the tools/call in flight. The MCP audit
// middleware seeds name and namespace from the call's raw arguments, but the UID
// exists only once the write has happened, so every create/update handler adds
// it here (TestMCPHandlers_WritesCallSetResource enforces that).
//
// The object's own Namespace is authoritative, not the handler's namespaceName
// argument: it is empty for a cluster-scoped CRD, matching what the
// cluster-scoped REST handlers record.
func setAuditResource(ctx context.Context, obj metav1.Object) {
	audit.SetResource(ctx, &audit.Resource{
		Namespace: obj.GetNamespace(),
		ID:        string(obj.GetUID()),
		Name:      obj.GetName(),
	})
}

// wrapList wraps a slice in a map so that the MCP structured content response
// is a JSON object (record) instead of a bare array. The MCP specification
// requires structuredContent to be a record; returning an array directly
// causes validation errors. When nextCursor is non-empty it is included so
// that AI agents can paginate through results.
func wrapList(key string, items any, nextCursor string) map[string]any {
	result := map[string]any{key: items}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	return result
}

// toServiceListOptions converts MCP ListOpts to service ListOptions, applying
// the default page size when the caller did not specify a limit.
func toServiceListOptions(opts tools.ListOpts) services.ListOptions {
	return services.ListOptions{
		Limit:  opts.EffectiveLimit(),
		Cursor: opts.Cursor,
	}
}
