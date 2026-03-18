// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"errors"
	"testing"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertCELValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  ref.Val
		want any
	}{
		{
			name: "string type",
			val:  types.String("hello"),
			want: "hello",
		},
		{
			name: "int type",
			val:  types.Int(42),
			want: int64(42),
		},
		{
			name: "uint type",
			val:  types.Uint(100),
			want: uint64(100),
		},
		{
			name: "double type",
			val:  types.Double(3.14),
			want: float64(3.14),
		},
		{
			name: "bool type",
			val:  types.Bool(true),
			want: true,
		},
		{
			name: "omitCELValue returns omitSentinel",
			val:  omitCEL,
			want: omitSentinel,
		},
		{
			name: "error-based omit returns omitSentinel",
			val:  types.NewErr(omitErrMsg),
			want: omitSentinel,
		},
		{
			name: "non-omit error returns original value",
			val:  types.NewErr("some other error"),
			want: errors.New("some other error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := convertCELValue(tt.val)

			// Special handling for error comparison
			if wantErr, ok := tt.want.(error); ok {
				gotErr, ok := got.(error)
				require.True(t, ok, "expected error type, got %T", got)
				assert.Equal(t, wantErr.Error(), gotErr.Error())
				return
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConvertAnyList(t *testing.T) {
	t.Parallel()

	t.Run("with ref.Val items", func(t *testing.T) {
		t.Parallel()

		list := []any{
			types.String("hello"),
			types.Int(42),
			"plain-string",
		}
		result := convertAnyList(list)
		assert.Equal(t, []any{"hello", int64(42), "plain-string"}, result)
	})

	t.Run("with map[ref.Val]ref.Val items", func(t *testing.T) {
		t.Parallel()

		refMap := map[ref.Val]ref.Val{
			types.String("key"): types.String("value"),
		}
		list := []any{refMap}
		result := convertAnyList(list)

		require.Len(t, result, 1)
		resultMap, ok := result[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "value", resultMap["key"])
	})

	t.Run("with omit sentinel ref.Val", func(t *testing.T) {
		t.Parallel()

		list := []any{
			types.String("keep"),
			omitCEL, // omitCELValue implements ref.Val
			types.String("also-keep"),
		}
		result := convertAnyList(list)
		assert.Equal(t, []any{"keep", "also-keep"}, result)
	})

	t.Run("with plain values", func(t *testing.T) {
		t.Parallel()

		list := []any{"a", int64(1), true}
		result := convertAnyList(list)
		assert.Equal(t, []any{"a", int64(1), true}, result)
	})
}

func TestConvertStringAnyMap(t *testing.T) {
	t.Parallel()

	t.Run("with ref.Val values", func(t *testing.T) {
		t.Parallel()

		m := map[string]any{
			"str": types.String("hello"),
			"num": types.Int(42),
		}
		result := convertStringAnyMap(m)
		assert.Equal(t, "hello", result["str"])
		assert.Equal(t, int64(42), result["num"])
	})

	t.Run("with omit ref.Val value excluded", func(t *testing.T) {
		t.Parallel()

		m := map[string]any{
			"keep":   types.String("value"),
			"remove": omitCEL,
		}
		result := convertStringAnyMap(m)
		assert.Equal(t, "value", result["keep"])
		_, hasRemove := result["remove"]
		assert.False(t, hasRemove, "omit value should be excluded")
	})

	t.Run("with plain values", func(t *testing.T) {
		t.Parallel()

		m := map[string]any{
			"str": "hello",
			"num": int64(42),
		}
		result := convertStringAnyMap(m)
		assert.Equal(t, "hello", result["str"])
		assert.Equal(t, int64(42), result["num"])
	})
}

func TestConvertRefValMap(t *testing.T) {
	t.Parallel()

	t.Run("basic conversion", func(t *testing.T) {
		t.Parallel()

		m := map[ref.Val]ref.Val{
			types.String("key1"): types.String("val1"),
			types.String("key2"): types.Int(42),
		}
		result := convertRefValMap(m)
		assert.Equal(t, "val1", result["key1"])
		assert.Equal(t, int64(42), result["key2"])
	})
}

func TestConvertCELList(t *testing.T) {
	t.Parallel()

	t.Run("ref.Val list", func(t *testing.T) {
		t.Parallel()

		list := []ref.Val{types.String("a"), types.Int(1)}
		result := convertCELList(list)

		resultSlice, ok := result.([]any)
		require.True(t, ok)
		assert.Equal(t, []any{"a", int64(1)}, resultSlice)
	})

	t.Run("any list delegates to convertAnyList", func(t *testing.T) {
		t.Parallel()

		list := []any{"a", "b"}
		result := convertCELList(list)

		resultSlice, ok := result.([]any)
		require.True(t, ok)
		assert.Equal(t, []any{"a", "b"}, resultSlice)
	})

	t.Run("unknown type passes through", func(t *testing.T) {
		t.Parallel()

		result := convertCELList("not-a-list")
		assert.Equal(t, "not-a-list", result)
	})
}

func TestNormalizeCELResult(t *testing.T) {
	t.Parallel()

	t.Run("error passthrough", func(t *testing.T) {
		t.Parallel()

		_, err := normalizeCELResult(nil, errors.New("test error"))
		require.Error(t, err)
		assert.Equal(t, "test error", err.Error())
	})

	t.Run("omit sentinel returns omit", func(t *testing.T) {
		t.Parallel()

		result, err := normalizeCELResult(omitSentinel, nil)
		require.NoError(t, err)
		assert.Equal(t, omitSentinel, result)
	})

	t.Run("normal value passthrough", func(t *testing.T) {
		t.Parallel()

		result, err := normalizeCELResult("hello", nil)
		require.NoError(t, err)
		assert.Equal(t, "hello", result)
	})
}

func TestRemoveOmittedFields(t *testing.T) {
	t.Parallel()

	t.Run("map with omit values removed", func(t *testing.T) {
		t.Parallel()

		data := map[string]any{
			"keep":   "value",
			"remove": omitSentinel,
			"nested": map[string]any{
				"inner":  "val",
				"remove": omitSentinel,
			},
		}
		result := RemoveOmittedFields(data)
		resultMap := result.(map[string]any)
		assert.Equal(t, "value", resultMap["keep"])
		_, hasRemove := resultMap["remove"]
		assert.False(t, hasRemove)

		nested := resultMap["nested"].(map[string]any)
		assert.Equal(t, "val", nested["inner"])
		_, hasNestedRemove := nested["remove"]
		assert.False(t, hasNestedRemove)
	})

	t.Run("array with omit values removed", func(t *testing.T) {
		t.Parallel()

		data := []any{"first", omitSentinel, "third"}
		result := RemoveOmittedFields(data)
		assert.Equal(t, []any{"first", "third"}, result)
	})

	t.Run("nested omit in array element", func(t *testing.T) {
		t.Parallel()

		data := []any{
			map[string]any{"key": omitSentinel},
			omitSentinel,
			"keep",
		}
		result := RemoveOmittedFields(data)
		resultSlice := result.([]any)
		require.Len(t, resultSlice, 2)
		assert.Equal(t, map[string]any{}, resultSlice[0])
		assert.Equal(t, "keep", resultSlice[1])
	})

	t.Run("non-collection passthrough", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "hello", RemoveOmittedFields("hello"))
		assert.Equal(t, int64(42), RemoveOmittedFields(int64(42)))
		assert.Nil(t, RemoveOmittedFields(nil))
	})
}
