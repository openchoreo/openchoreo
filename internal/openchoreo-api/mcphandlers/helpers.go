// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

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
