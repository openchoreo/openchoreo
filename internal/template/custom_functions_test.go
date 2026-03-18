// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"testing"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOmitCELValue(t *testing.T) {
	t.Parallel()

	val := omitCEL

	// ConvertToNative returns omitSentinel
	native, err := val.ConvertToNative(nil)
	require.NoError(t, err)
	assert.Equal(t, omitSentinel, native)

	// ConvertToType returns self
	assert.Equal(t, val, val.ConvertToType(types.StringType))

	// Equal with same type returns True
	assert.Equal(t, types.True, val.Equal(omitCEL))

	// Equal with different type returns False
	assert.Equal(t, types.False, val.Equal(types.String("other")))

	// Type returns custom omit type
	assert.Equal(t, omitTypeVal, val.Type())

	// Value returns omitSentinel
	assert.Equal(t, omitSentinel, val.Value())
}

func TestMergeMapFunction_RefValMaps(t *testing.T) {
	t.Parallel()

	// Create map[ref.Val]ref.Val CEL maps
	base := types.NewDynamicMap(types.DefaultTypeAdapter, map[ref.Val]ref.Val{
		types.String("a"): types.Int(1),
		types.String("b"): types.Int(2),
	})

	override := types.NewDynamicMap(types.DefaultTypeAdapter, map[ref.Val]ref.Val{
		types.String("b"): types.Int(20),
		types.String("c"): types.Int(30),
	})

	result := mergeMapFunction(base, override)

	// Convert result to map for assertion
	resultNative := convertCELValue(result)
	resultMap, ok := resultNative.(map[string]any)
	require.True(t, ok, "expected map[string]any, got %T", resultNative)

	assert.Equal(t, int64(1), resultMap["a"])
	assert.Equal(t, int64(20), resultMap["b"])
	assert.Equal(t, int64(30), resultMap["c"])
}

func TestMergeMapFunction_MixedMapTypes(t *testing.T) {
	t.Parallel()

	// Base is a Go map (map[string]any), override is CEL map (map[ref.Val]ref.Val)
	baseNative := types.DefaultTypeAdapter.NativeToValue(map[string]any{
		"x": int64(10),
		"y": int64(20),
	})

	overrideRefVal := types.NewDynamicMap(types.DefaultTypeAdapter, map[ref.Val]ref.Val{
		types.String("y"): types.Int(200),
		types.String("z"): types.Int(300),
	})

	result := mergeMapFunction(baseNative, overrideRefVal)
	resultNative := convertCELValue(result)
	resultMap, ok := resultNative.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, int64(10), resultMap["x"])
	assert.Equal(t, int64(200), resultMap["y"])
	assert.Equal(t, int64(300), resultMap["z"])
}

func TestGenerateK8sName_InputTypes(t *testing.T) {
	t.Parallel()

	t.Run("string input", func(t *testing.T) {
		t.Parallel()
		result := generateK8sName(types.String("hello-world"))
		assert.Contains(t, result.Value().(string), "hello-world")
	})

	t.Run("ref.Val list input", func(t *testing.T) {
		t.Parallel()
		// Construct a CEL list whose Value() returns []ref.Val
		refVals := []ref.Val{types.String("part1"), types.String("part2")}
		list := types.NewDynamicList(types.DefaultTypeAdapter, refVals)
		result := generateK8sName(list)
		val := result.Value().(string)
		assert.Contains(t, val, "part1")
		assert.Contains(t, val, "part2")
	})

	t.Run("any list input", func(t *testing.T) {
		t.Parallel()
		// Construct a CEL list whose Value() returns []any
		anyVals := []any{"seg1", "seg2"}
		list := types.NewDynamicList(types.DefaultTypeAdapter, anyVals)
		result := generateK8sName(list)
		val := result.Value().(string)
		assert.Contains(t, val, "seg1")
		assert.Contains(t, val, "seg2")
	})
}

func TestGenerateK8sDNSLabel_InputTypes(t *testing.T) {
	t.Parallel()

	t.Run("string input", func(t *testing.T) {
		t.Parallel()
		result := generateK8sDNSLabel(types.String("my-label"))
		val := result.Value().(string)
		assert.Contains(t, val, "my-label")
		assert.LessOrEqual(t, len(val), 63)
	})

	t.Run("ref.Val list input", func(t *testing.T) {
		t.Parallel()
		refVals := []ref.Val{types.String("label1"), types.String("label2")}
		list := types.NewDynamicList(types.DefaultTypeAdapter, refVals)
		result := generateK8sDNSLabel(list)
		val := result.Value().(string)
		assert.Contains(t, val, "label1")
		assert.LessOrEqual(t, len(val), 63)
	})

	t.Run("any list input", func(t *testing.T) {
		t.Parallel()
		anyVals := []any{"dns1", "dns2"}
		list := types.NewDynamicList(types.DefaultTypeAdapter, anyVals)
		result := generateK8sDNSLabel(list)
		val := result.Value().(string)
		assert.Contains(t, val, "dns1")
		assert.LessOrEqual(t, len(val), 63)
	})

	t.Run("long input truncated to 63 chars", func(t *testing.T) {
		t.Parallel()
		refVals := []ref.Val{
			types.String("very-long-endpoint-name"),
			types.String("very-long-component-name"),
			types.String("very-long-environment-name"),
		}
		list := types.NewDynamicList(types.DefaultTypeAdapter, refVals)
		result := generateK8sDNSLabel(list)
		val := result.Value().(string)
		assert.LessOrEqual(t, len(val), 63)
	})
}
