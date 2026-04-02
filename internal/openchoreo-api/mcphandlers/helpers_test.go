// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func TestWrapList(t *testing.T) {
	items := []string{"a", "b", "c"}

	t.Run("no cursor: next_cursor absent", func(t *testing.T) {
		result := wrapList("items", items, "")
		assert.Equal(t, items, result["items"])
		_, hasCursor := result["next_cursor"]
		assert.False(t, hasCursor)
	})

	t.Run("with cursor: next_cursor included", func(t *testing.T) {
		result := wrapList("items", items, "abc123")
		assert.Equal(t, "abc123", result["next_cursor"])
	})

	t.Run("key used as-is in map", func(t *testing.T) {
		result := wrapList("my_resources", items, "")
		assert.Equal(t, items, result["my_resources"])
		assert.NotContains(t, result, "items")
	})
}

func TestToServiceListOptions(t *testing.T) {
	t.Run("zero limit uses effective default", func(t *testing.T) {
		opts := tools.ListOpts{Limit: 0, Cursor: ""}
		svcOpts := toServiceListOptions(opts)
		assert.Equal(t, opts.EffectiveLimit(), svcOpts.Limit)
		assert.Greater(t, svcOpts.Limit, 0, "default limit should be positive")
	})

	t.Run("custom limit and cursor passed through", func(t *testing.T) {
		opts := tools.ListOpts{Limit: 25, Cursor: "tok123"}
		svcOpts := toServiceListOptions(opts)
		assert.Equal(t, 25, svcOpts.Limit)
		assert.Equal(t, "tok123", svcOpts.Cursor)
	})

	t.Run("EffectiveLimit used when limit is zero", func(t *testing.T) {
		opts := tools.ListOpts{Limit: 0}
		svcOpts := toServiceListOptions(opts)
		require.Equal(t, opts.EffectiveLimit(), svcOpts.Limit)
	})
}
