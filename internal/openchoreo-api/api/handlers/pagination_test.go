// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

func TestNormalizeListOptions(t *testing.T) {
	t.Run("defaults when all nil", func(t *testing.T) {
		opts := NormalizeListOptions(nil, nil, nil)
		assert.Equal(t, defaultPageLimit, opts.Limit)
		assert.Empty(t, opts.Cursor)
		assert.Empty(t, opts.LabelSelector)
	})

	t.Run("uses provided values", func(t *testing.T) {
		opts := NormalizeListOptions(ptr.To(gen.LimitParam(50)), ptr.To(gen.CursorParam("abc")), ptr.To(gen.LabelSelectorParam("app=test")))
		assert.Equal(t, 50, opts.Limit)
		assert.Equal(t, "abc", opts.Cursor)
		assert.Equal(t, "app=test", opts.LabelSelector)
	})

	t.Run("clamps limit below 1 to 1", func(t *testing.T) {
		opts := NormalizeListOptions(ptr.To(gen.LimitParam(0)), nil, nil)
		assert.Equal(t, 1, opts.Limit)

		opts = NormalizeListOptions(ptr.To(gen.LimitParam(-5)), nil, nil)
		assert.Equal(t, 1, opts.Limit)
	})

	t.Run("clamps limit above max to max", func(t *testing.T) {
		opts := NormalizeListOptions(ptr.To(gen.LimitParam(500)), nil, nil)
		assert.Equal(t, maxPageLimit, opts.Limit)
	})

	t.Run("limit at boundaries", func(t *testing.T) {
		opts := NormalizeListOptions(ptr.To(gen.LimitParam(1)), nil, nil)
		assert.Equal(t, 1, opts.Limit)

		opts = NormalizeListOptions(ptr.To(gen.LimitParam(maxPageLimit)), nil, nil)
		assert.Equal(t, maxPageLimit, opts.Limit)
	})
}

func TestToPagination(t *testing.T) {
	t.Run("sets next cursor when present", func(t *testing.T) {
		result := &services.ListResult[string]{
			Items:      []string{"a", "b"},
			NextCursor: "cursor-123",
		}
		p := ToPagination(result)
		require.NotNil(t, p.NextCursor)
		assert.Equal(t, "cursor-123", *p.NextCursor)
	})

	t.Run("nil next cursor when empty", func(t *testing.T) {
		result := &services.ListResult[string]{
			Items:      []string{"a"},
			NextCursor: "",
		}
		p := ToPagination(result)
		assert.Nil(t, p.NextCursor)
	})

	t.Run("empty result", func(t *testing.T) {
		result := &services.ListResult[string]{}
		p := ToPagination(result)
		assert.Nil(t, p.NextCursor)
	})
}
