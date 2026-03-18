// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"fmt"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestLRUCache_Eviction(t *testing.T) {
	t.Parallel()

	cache := newLRUCache[string](2)

	cache.Set("a", "val-a")
	cache.Set("b", "val-b")
	cache.Set("c", "val-c") // should evict "a"

	_, ok := cache.Get("a")
	assert.False(t, ok, "evicted key 'a' should not be found")

	val, ok := cache.Get("b")
	assert.True(t, ok)
	assert.Equal(t, "val-b", val)

	val, ok = cache.Get("c")
	assert.True(t, ok)
	assert.Equal(t, "val-c", val)
}

func TestLRUCache_EvictionRespectsAccessOrder(t *testing.T) {
	t.Parallel()

	cache := newLRUCache[string](2)

	cache.Set("a", "val-a")
	cache.Set("b", "val-b")

	// Access "a" to make it most recently used
	cache.Get("a")

	cache.Set("c", "val-c") // should evict "b" (least recently used)

	_, ok := cache.Get("b")
	assert.False(t, ok, "evicted key 'b' should not be found")

	val, ok := cache.Get("a")
	assert.True(t, ok)
	assert.Equal(t, "val-a", val)

	val, ok = cache.Get("c")
	assert.True(t, ok)
	assert.Equal(t, "val-c", val)
}

func TestLRUCache_UpdateExisting(t *testing.T) {
	t.Parallel()

	cache := newLRUCache[string](2)

	cache.Set("a", "val-1")
	cache.Set("a", "val-2")

	val, ok := cache.Get("a")
	assert.True(t, ok)
	assert.Equal(t, "val-2", val, "value should be updated in place")
	assert.Equal(t, 1, cache.evictList.Len(), "duplicate Set should not grow the list")
}

func TestEngineCache_DisabledEnvCache(t *testing.T) {
	t.Parallel()

	cache := newEngineCache(true, false)

	env, ok := cache.GetEnv("key")
	assert.Nil(t, env)
	assert.False(t, ok)

	// SetEnv should be a no-op
	cache.SetEnv("key", nil)

	env, ok = cache.GetEnv("key")
	assert.Nil(t, env)
	assert.False(t, ok)

	// Program cache should also be nil when env cache is disabled
	prog, ok := cache.GetProgram("envKey", "expr")
	assert.Nil(t, prog)
	assert.False(t, ok)

	assert.Equal(t, 0, cache.ProgramCacheSize())
}

func TestEngineCache_DisabledProgramCacheOnly(t *testing.T) {
	t.Parallel()

	cache := newEngineCache(false, true)

	// Env cache should work
	assert.NotNil(t, cache.envCache)

	// Program cache should be disabled
	prog, ok := cache.GetProgram("envKey", "expr")
	assert.Nil(t, prog)
	assert.False(t, ok)

	cache.SetProgram("envKey", "expr", nil)

	prog, ok = cache.GetProgram("envKey", "expr")
	assert.Nil(t, prog)
	assert.False(t, ok)

	assert.Equal(t, 0, cache.ProgramCacheSize())
}

func TestNewEngineWithOptions_DisableCache(t *testing.T) {
	t.Parallel()

	engine := NewEngineWithOptions(DisableCache())

	tpl := map[string]any{"name": "${metadata.name}"}
	inputs := map[string]any{"metadata": map[string]any{"name": "test"}}

	result, err := engine.Render(tpl, inputs)
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test", resultMap["name"])

	assert.Equal(t, 0, engine.cache.ProgramCacheSize())

	// Render again — should still work without cache
	result2, err := engine.Render(tpl, inputs)
	require.NoError(t, err)

	resultMap2, ok := result2.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test", resultMap2["name"])

	assert.Equal(t, 0, engine.cache.ProgramCacheSize())
}

func TestNewEngineWithOptions_DisableProgramCacheOnly(t *testing.T) {
	t.Parallel()

	engine := NewEngineWithOptions(DisableProgramCacheOnly())

	tpl := map[string]any{"name": "${metadata.name}"}
	inputs := map[string]any{"metadata": map[string]any{"name": "test"}}

	result, err := engine.Render(tpl, inputs)
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test", resultMap["name"])

	assert.Equal(t, 0, engine.cache.ProgramCacheSize())
}

func TestNewEngineWithOptions_WithCELExtensions(t *testing.T) {
	t.Parallel()

	customFunc := cel.Function("oc_test_double",
		cel.Overload("oc_test_double_int", []*cel.Type{cel.IntType}, cel.IntType,
			cel.UnaryBinding(func(arg ref.Val) ref.Val {
				return types.Int(arg.Value().(int64) * 2)
			}),
		),
	)

	engine := NewEngineWithOptions(WithCELExtensions(customFunc))

	tpl := map[string]any{"result": "${oc_test_double(x)}"}
	inputs := map[string]any{"x": int64(21)}

	result, err := engine.Render(tpl, inputs)
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(42), resultMap["result"])
}

func TestNewEngineWithOptions_NoOptions(t *testing.T) {
	t.Parallel()

	engine := NewEngineWithOptions()
	require.NotNil(t, engine.cache)

	tpl := map[string]any{"val": "${x + y}"}
	inputs := map[string]any{"x": int64(1), "y": int64(2)}

	result, err := engine.Render(tpl, inputs)
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(3), resultMap["val"])
}

func TestNewEngineWithOptions_BadExtensionCausesRenderError(t *testing.T) {
	t.Parallel()

	// Register an extension that collides with the built-in oc_omit overload
	badFunc := cel.Function("oc_omit",
		cel.Overload("oc_omit", []*cel.Type{cel.IntType}, cel.DynType,
			cel.UnaryBinding(func(arg ref.Val) ref.Val { return arg }),
		),
	)
	engine := NewEngineWithOptions(WithCELExtensions(badFunc))

	tpl := map[string]any{"val": "${x}"}
	inputs := map[string]any{"x": int64(1)}

	_, err := engine.Render(tpl, inputs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build CEL environment")
}

func TestProgramCaching(t *testing.T) {
	t.Parallel()

	t.Run("repeated render reuses cached programs", func(t *testing.T) {
		t.Parallel()

		engine := NewEngine()

		templateYAML := `
name: ${metadata.name}
env: ${environment}
replicas: ${replicas}
`
		var tpl any
		require.NoError(t, yaml.Unmarshal([]byte(templateYAML), &tpl))

		inputs := map[string]any{
			"metadata":    map[string]any{"name": "test-app"},
			"environment": "production",
			"replicas":    int64(3),
		}

		result1, err := engine.Render(tpl, inputs)
		require.NoError(t, err)

		result2, err := engine.Render(tpl, inputs)
		require.NoError(t, err)

		yaml1, _ := yaml.Marshal(result1)
		yaml2, _ := yaml.Marshal(result2)
		assert.Equal(t, string(yaml1), string(yaml2))
	})

	t.Run("forEach iterations with shared cache", func(t *testing.T) {
		t.Parallel()

		engine := NewEngine()

		templateYAML := `
item1: ${metadata.name}-${item}
item2: ${metadata.name}-${item}
`
		var tpl any
		require.NoError(t, yaml.Unmarshal([]byte(templateYAML), &tpl))

		for i := 1; i <= 5; i++ {
			inputs := map[string]any{
				"metadata": map[string]any{"name": "test"},
				"item":     fmt.Sprintf("value-%d", i),
			}

			result, err := engine.Render(tpl, inputs)
			require.NoError(t, err, "iteration %d", i)

			resultMap := result.(map[string]any)
			expected := fmt.Sprintf("test-value-%d", i)
			assert.Equal(t, expected, resultMap["item1"], "iteration %d item1", i)
			assert.Equal(t, expected, resultMap["item2"], "iteration %d item2", i)
		}
	})

	t.Run("different variable sets create separate cache entries", func(t *testing.T) {
		t.Parallel()

		engine := NewEngine()

		templateYAML := `value: ${x + y}`
		var tpl any
		require.NoError(t, yaml.Unmarshal([]byte(templateYAML), &tpl))

		inputs2Vars := map[string]any{"x": int64(10), "y": int64(20)}
		inputs3Vars := map[string]any{"x": int64(5), "y": int64(15), "z": int64(100)}

		result2, err := engine.Render(tpl, inputs2Vars)
		require.NoError(t, err)
		assert.Equal(t, int64(30), result2.(map[string]any)["value"])

		result3, err := engine.Render(tpl, inputs3Vars)
		require.NoError(t, err)
		assert.Equal(t, int64(20), result3.(map[string]any)["value"])
	})

	t.Run("cache is populated after rendering", func(t *testing.T) {
		t.Parallel()

		engine := NewEngine()

		tpl := map[string]any{"val": "${x}"}
		_, err := engine.Render(tpl, map[string]any{"x": int64(1)})
		require.NoError(t, err)

		assert.Greater(t, engine.cache.ProgramCacheSize(), 0, "cache should have entries")
	})
}
