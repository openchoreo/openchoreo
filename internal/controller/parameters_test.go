// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func makeRawParams(t *testing.T, data map[string]any) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(data)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

func TestGetNestedStringInParams(t *testing.T) {
	t.Run("simple key", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"url": "https://github.com/example"})
		val, err := GetNestedStringInParams(raw, "url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/example", val)
	})

	t.Run("nested key", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{
			"repo": map[string]any{
				"url": "https://github.com/example",
			},
		})
		val, err := GetNestedStringInParams(raw, "repo.url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/example", val)
	})

	t.Run("strips parameters prefix", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"url": "https://github.com/example"})
		val, err := GetNestedStringInParams(raw, "parameters.url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/example", val)
	})

	t.Run("nil raw extension", func(t *testing.T) {
		_, err := GetNestedStringInParams(nil, "url")
		require.Error(t, err)
	})

	t.Run("key not found", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"url": "https://github.com/example"})
		_, err := GetNestedStringInParams(raw, "missing")
		require.Error(t, err)
	})

	t.Run("value is not a string", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"count": 42})
		_, err := GetNestedStringInParams(raw, "count")
		require.Error(t, err)
	})

	t.Run("intermediate is not an object", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"repo": "not-an-object"})
		_, err := GetNestedStringInParams(raw, "repo.url")
		require.Error(t, err)
	})

	t.Run("deeply nested three levels", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{
			"a": map[string]any{
				"b": map[string]any{
					"c": "deep-value",
				},
			},
		})
		val, err := GetNestedStringInParams(raw, "a.b.c")
		require.NoError(t, err)
		assert.Equal(t, "deep-value", val)
	})

	t.Run("invalid JSON in raw extension", func(t *testing.T) {
		raw := &runtime.RawExtension{Raw: []byte("{invalid json")}
		_, err := GetNestedStringInParams(raw, "key")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal")
	})

	t.Run("nil raw bytes", func(t *testing.T) {
		raw := &runtime.RawExtension{Raw: nil}
		_, err := GetNestedStringInParams(raw, "key")
		require.Error(t, err)
	})

	t.Run("empty path after stripping parameters prefix", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"": "value"})
		// "parameters." stripped leaves empty string, which becomes [""]
		val, err := GetNestedStringInParams(raw, "parameters.")
		require.NoError(t, err)
		assert.Equal(t, "value", val)
	})

	t.Run("value is boolean not string", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"enabled": true})
		_, err := GetNestedStringInParams(raw, "enabled")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a string")
	})

	t.Run("value is array not string", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"items": []string{"a", "b"}})
		_, err := GetNestedStringInParams(raw, "items")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a string")
	})
}

func TestSetNestedStringInParams(t *testing.T) {
	t.Run("simple key", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"commit": "old"})
		result, err := SetNestedStringInParams(raw, "commit", "abc1234")
		require.NoError(t, err)

		val, err := GetNestedStringInParams(result, "commit")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", val)
	})

	t.Run("nested key", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{
			"repo": map[string]any{
				"commit": "old",
			},
		})
		result, err := SetNestedStringInParams(raw, "repo.commit", "abc1234")
		require.NoError(t, err)

		val, err := GetNestedStringInParams(result, "repo.commit")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", val)
	})

	t.Run("strips parameters prefix", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"commit": "old"})
		result, err := SetNestedStringInParams(raw, "parameters.commit", "abc1234")
		require.NoError(t, err)

		val, err := GetNestedStringInParams(result, "commit")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", val)
	})

	t.Run("creates intermediate objects", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{})
		result, err := SetNestedStringInParams(raw, "repo.commit", "abc1234")
		require.NoError(t, err)

		val, err := GetNestedStringInParams(result, "repo.commit")
		require.NoError(t, err)
		assert.Equal(t, "abc1234", val)
	})

	t.Run("nil raw extension", func(t *testing.T) {
		_, err := SetNestedStringInParams(nil, "commit", "abc1234")
		require.Error(t, err)
	})

	t.Run("intermediate is not an object", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"repo": "not-an-object"})
		_, err := SetNestedStringInParams(raw, "repo.commit", "abc1234")
		require.Error(t, err)
	})

	t.Run("invalid JSON in raw extension", func(t *testing.T) {
		raw := &runtime.RawExtension{Raw: []byte("{invalid json")}
		_, err := SetNestedStringInParams(raw, "key", "value")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal")
	})

	t.Run("nil raw bytes", func(t *testing.T) {
		raw := &runtime.RawExtension{Raw: nil}
		_, err := SetNestedStringInParams(raw, "key", "value")
		require.Error(t, err)
	})

	t.Run("deeply nested three levels creates all intermediates", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{})
		result, err := SetNestedStringInParams(raw, "a.b.c", "deep-value")
		require.NoError(t, err)

		val, err := GetNestedStringInParams(result, "a.b.c")
		require.NoError(t, err)
		assert.Equal(t, "deep-value", val)
	})

	t.Run("overwrites existing value", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{"key": "old"})
		result, err := SetNestedStringInParams(raw, "key", "new")
		require.NoError(t, err)

		val, err := GetNestedStringInParams(result, "key")
		require.NoError(t, err)
		assert.Equal(t, "new", val)
	})

	t.Run("preserves sibling keys", func(t *testing.T) {
		raw := makeRawParams(t, map[string]any{
			"repo": map[string]any{
				"url":    "https://github.com/example",
				"commit": "old-sha",
			},
		})
		result, err := SetNestedStringInParams(raw, "repo.commit", "new-sha")
		require.NoError(t, err)

		// Verify updated key
		val, err := GetNestedStringInParams(result, "repo.commit")
		require.NoError(t, err)
		assert.Equal(t, "new-sha", val)

		// Verify sibling key is preserved
		url, err := GetNestedStringInParams(result, "repo.url")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/example", url)
	})
}
